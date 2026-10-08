package app

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

type rulesHarness struct {
	*svcHarness
	holder *rules.Holder
	srv    *httptest.Server
	body   map[string]string
}

func newRulesSvc(t *testing.T) *rulesHarness {
	sh := newSvc(t)
	rh := &rulesHarness{svcHarness: sh, holder: &rules.Holder{}, body: map[string]string{}}
	rh.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(rh.body[r.URL.Path]))
	}))
	t.Cleanup(rh.srv.Close)
	addr := rh.srv.Listener.Addr().String()
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}}
	sh.svc.x.Rules = rh.holder
	sh.svc.x.RulesPath = sh.paths.Rules
	sh.svc.x.Fetcher = &lists.Fetcher{Client: client, Dir: sh.paths.ListsDir, Now: time.Now, WriteFile: store.WriteFileAtomic}
	t.Cleanup(sh.svc.waitBackground)
	return rh
}

func (rh *rulesHarness) waitList(t *testing.T, id string, cond func(lists.List) bool) lists.List {
	t.Helper()
	var got lists.List
	require.Eventually(t, func() bool {
		for _, l := range rh.svc.GetRules().Lists {
			if l.ID == id && cond(l) {
				got = l
				return true
			}
		}
		return false
	}, 5*time.Second, 20*time.Millisecond)
	return got
}

func TestSaveRulesText_ErrorsDoNotSave(t *testing.T) {
	rh := newRulesSvc(t)
	require.Empty(t, rh.svc.SaveRulesText("ads.com block\n"))
	errs := rh.svc.SaveRulesText("x.com block\nbad.com fragment=maybe\n")
	require.Len(t, errs, 1)
	require.Equal(t, 2, errs[0].Line)
	f, _, err := store.LoadRules(rh.paths.Rules)
	require.NoError(t, err)
	require.Len(t, f.Rules, 1)
	require.Equal(t, "ads.com", f.Rules[0].Pattern)
	require.False(t, rh.holder.Load().Explain("x.com").Block)
	require.True(t, rh.holder.Load().Explain("ads.com").Block)
}

func TestSaveRulesTable_RecompilesAndEmits(t *testing.T) {
	rh := newRulesSvc(t)
	errs := rh.svc.SaveRulesTable([]rules.Rule{
		{Pattern: "ads.com", Action: rules.Action{Block: true}, Enabled: true},
		{Pattern: "=bad pattern", Action: rules.Action{Block: true}, Enabled: true},
	})
	require.Len(t, errs, 1)
	require.Equal(t, 2, errs[0].Line)
	require.Empty(t, rh.svc.SaveRulesTable([]rules.Rule{{Pattern: "ads.com", Action: rules.Action{Block: true}, Enabled: true}}))
	require.True(t, rh.svc.Explain("x.ads.com").Block)
	require.Positive(t, rh.em.count(EventRulesCompiled))
	v := rh.svc.GetRules()
	require.Equal(t, "ads.com block\n", v.Text)
}

func TestAddList_FetchesInBackground(t *testing.T) {
	rh := newRulesSvc(t)
	rh.body["/u/r/main/hosts"] = "0.0.0.0 ads.example\n0.0.0.0 track.example\n"
	l, err := rh.svc.AddList(lists.List{Name: "My hosts", Source: "url", URL: "https://github.com/u/r/blob/main/hosts", Action: "block"})
	require.NoError(t, err)
	require.NotEmpty(t, l.ID)
	require.Equal(t, "auto", l.Format)
	require.Equal(t, 24, l.UpdateHours)
	require.True(t, l.Enabled)
	got := rh.waitList(t, l.ID, func(x lists.List) bool { return x.Detected != "" })
	require.Equal(t, "hosts", got.Detected)
	require.Equal(t, 2, got.Counts["exact"])
	require.Positive(t, rh.em.count(EventListsProgress))
	require.Eventually(t, func() bool { return rh.svc.Explain("ads.example").Block }, 2*time.Second, 20*time.Millisecond)

	// Delete removes the cache and the entries.
	require.NoError(t, rh.svc.DeleteList(l.ID))
	require.False(t, rh.svc.Explain("ads.example").Block)
	_, err = os.Stat(filepath.Join(rh.paths.ListsDir, l.ID+".txt"))
	require.True(t, os.IsNotExist(err))
}

func TestAddList_Validation(t *testing.T) {
	rh := newRulesSvc(t)
	_, err := rh.svc.AddList(lists.List{Name: "x", Source: "url", URL: "http://insecure/x", Action: "block"})
	require.Error(t, err)
	_, err = rh.svc.AddList(lists.List{Name: "x", Source: "url", URL: "https://ok/x", Action: "explode"})
	require.Error(t, err)
	_, err = rh.svc.AddList(lists.List{Name: "x", Source: "url", URL: "https://ok/x", Action: "upstream=nope"})
	require.Error(t, err)
}

func TestTotalLimitDisablesNewest(t *testing.T) {
	rh := newRulesSvc(t)
	rh.svc.x.MaxEntries = 10
	rh.body["/a.txt"] = "a1.ex\na2.ex\na3.ex\na4.ex\na5.ex\na6.ex\n"
	rh.body["/b.txt"] = "b1.ex\nb2.ex\nb3.ex\nb4.ex\nb5.ex\nb6.ex\n"
	a, err := rh.svc.AddList(lists.List{Name: "A", Source: "url", URL: "https://x/a.txt", Action: "block"})
	require.NoError(t, err)
	rh.waitList(t, a.ID, func(x lists.List) bool { return x.Detected != "" })
	b, err := rh.svc.AddList(lists.List{Name: "B", Source: "url", URL: "https://x/b.txt", Action: "block"})
	require.NoError(t, err)
	got := rh.waitList(t, b.ID, func(x lists.List) bool { return !x.Enabled })
	require.Equal(t, CodeListTooLarge, got.LastError)
	require.True(t, rh.svc.Explain("a1.ex").Block)
	require.False(t, rh.svc.Explain("b1.ex").Block)
}

func TestMoveList(t *testing.T) {
	rh := newRulesSvc(t)
	var ids []string
	for _, n := range []string{"A", "B", "C"} {
		l, err := rh.svc.AddList(lists.List{Name: n, Source: "url", URL: "https://x/" + n, Action: "block", UpdateHours: 0})
		require.NoError(t, err)
		ids = append(ids, l.ID)
	}
	require.NoError(t, rh.svc.MoveList(ids[2], 0))
	var got []string
	for _, l := range rh.svc.GetRules().Lists {
		got = append(got, l.ID)
	}
	require.Equal(t, []string{ids[2], ids[0], ids[1]}, got)
}

func TestCatalogBinding(t *testing.T) {
	rh := newRulesSvc(t)
	require.Len(t, rh.svc.Catalog(), 31)
}

func TestRulesLoadedAtStart(t *testing.T) {
	rh := newRulesSvc(t)
	require.NoError(t, store.SaveRules(rh.paths.Rules, store.RulesFile{Rules: []rules.Rule{{Pattern: "boot.com", Action: rules.Action{Block: true}, Enabled: true}}}))
	recovered, err := LoadRules(rh.svc)
	require.NoError(t, err)
	require.False(t, recovered)
	require.True(t, rh.svc.Explain("boot.com").Block)
	require.True(t, strings.Contains(rh.svc.GetRules().Text, "boot.com"))
}

// C2: when the UI runs in another process (the Linux daemon), a list may
// not name a file on the daemon's machine.
func TestAddList_FileRefusedWhenNoFileLists(t *testing.T) {
	rh := newRulesSvc(t)
	rh.svc.x.NoFileLists = true
	_, err := rh.svc.AddList(lists.List{Name: "x", Source: "file", Path: "/etc/shadow", Format: "hosts", Action: "block"})
	require.ErrorIs(t, err, lists.ErrFileListsOff)
	require.Empty(t, rh.svc.GetRules().Lists)
}
