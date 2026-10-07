package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// blockUp never answers until ctx ends.
type blockUp struct{}

func (blockUp) Exchange(ctx context.Context, _ *dns.Msg) (*dns.Msg, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (blockUp) Address() string { return "block" }
func (blockUp) Close() error    { return nil }

func waitAdvDone(t *testing.T, h *toolsHarness) {
	t.Helper()
	require.Eventually(t, func() bool {
		h.svc.mu.Lock()
		defer h.svc.mu.Unlock()
		return h.svc.advCancel == nil
	}, 10*time.Second, 10*time.Millisecond)
}

func lastAdvEvent(h *toolsHarness) AdvScanProgress {
	h.em.mu.Lock()
	defer h.em.mu.Unlock()
	evs := h.em.events[EventToolsScan]
	return evs[len(evs)-1].(AdvScanProgress)
}

func TestAdvScan_FilterAndPasted(t *testing.T) {
	h := newTools(t)
	h.custom = []model.Server{
		{ID: "a", Name: "A", Protocol: model.ProtoDoH, Tags: []string{"no-log"}, Address: "https://a.example/dns-query", Source: model.SourceCustom},
		{ID: "b", Name: "B", Protocol: model.ProtoDoT, Tags: []string{"no-log"}, Address: "tls://b.example", Source: model.SourceCustom},
		{ID: "c", Name: "C", Protocol: model.ProtoDoH, Address: "https://c.example/dns-query", Source: model.SourceCustom},
	}
	st, err := h.svc.StartAdvancedScan(AdvScanRequest{Filter: &ServerFilter{Protocols: []string{"doh"}, Tags: []string{"no-log"}}})
	require.NoError(t, err)
	require.Equal(t, 1, st.Total)
	waitAdvDone(t, h)
	rows := h.svc.AdvancedResults()
	require.Len(t, rows, 1)
	require.Equal(t, "a", rows[0].Server.ID)
	require.False(t, lastAdvEvent(h).Running)

	st, err = h.svc.StartAdvancedScan(AdvScanRequest{Pasted: "https://p.example/dns-query\nudp://1.1.1.1\n"})
	require.NoError(t, err)
	require.Equal(t, 1, st.Total)
	require.Len(t, st.Bad, 1)
	waitAdvDone(t, h)
	rows = h.svc.AdvancedResults()
	require.Len(t, rows, 1)
	require.True(t, rows[0].Pasted)
	n, err := h.svc.AddScannedServers([]string{rows[0].Server.ID})
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, "https://p.example/dns-query", h.custom[len(h.custom)-1].Address)
}

func TestAdvScan_Busy(t *testing.T) {
	h := newTools(t)
	h.svc.x.BuildUpstream = func(model.Server) (upstream.Upstream, error) { return blockUp{}, nil }
	_, err := h.svc.StartAdvancedScan(AdvScanRequest{Filter: &ServerFilter{}})
	require.NoError(t, err)
	_, err = h.svc.StartAdvancedScan(AdvScanRequest{Filter: &ServerFilter{}})
	var ae *AppError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, CodeToolBusy, ae.Code)
	require.Equal(t, "advanced", ae.Params["tool"])
	h.svc.CancelAdvancedScan()
	waitAdvDone(t, h)
}

func TestAdvScan_TooMany(t *testing.T) {
	h := newTools(t)
	var lines []string
	for i := range 501 {
		lines = append(lines, fmt.Sprintf("https://s%d.example/dns-query", i))
	}
	_, err := h.svc.StartAdvancedScan(AdvScanRequest{Pasted: strings.Join(lines, "\n")})
	var ae *AppError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, CodeScanTooMany, ae.Code)
	require.Equal(t, 501, ae.Params["count"])
}

func TestTools_DoNotBlockConnect(t *testing.T) {
	h := newTools(t)
	h.svc.x.BuildUpstream = func(model.Server) (upstream.Upstream, error) { return blockUp{}, nil }
	_, err := h.svc.StartAdvancedScan(AdvScanRequest{Filter: &ServerFilter{}})
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		_ = h.svc.Connect()
		_ = h.svc.Disconnect()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Connect/Disconnect blocked by a running tool")
	}
	h.svc.mu.Lock()
	running := h.svc.advCancel != nil
	h.svc.mu.Unlock()
	require.True(t, running, "the scan keeps running across Connect/Disconnect")
	h.svc.CancelAdvancedScan()
	waitAdvDone(t, h)
}

func TestAdvScan_DoesNotTouchScanCache(t *testing.T) {
	h := newTools(t)
	before, _ := os.ReadFile(h.paths.ScanCache)
	_, err := h.svc.StartAdvancedScan(AdvScanRequest{Filter: &ServerFilter{}})
	require.NoError(t, err)
	waitAdvDone(t, h)
	after, _ := os.ReadFile(h.paths.ScanCache)
	require.Equal(t, before, after)
}

func TestExportAdvancedCSV_BOM(t *testing.T) {
	h := newTools(t)
	var name string
	var data []byte
	h.svc.x.SaveFile = func(_ context.Context, n string, d []byte) error { name, data = n, d; return nil }
	_, err := h.svc.StartAdvancedScan(AdvScanRequest{Filter: &ServerFilter{}})
	require.NoError(t, err)
	waitAdvDone(t, h)
	require.NoError(t, h.svc.ExportAdvancedCSV(context.Background()))
	require.True(t, strings.HasSuffix(name, ".csv"))
	require.True(t, bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
	recs, err := csv.NewReader(bytes.NewReader(data[3:])).ReadAll()
	require.NoError(t, err)
	require.Equal(t, []string{"server", "protocol", "ok", "median_ms", "p90_ms", "jitter_ms", "loss", "dnssec", "ad_filter", "poisoned"}, recs[0])
	require.Len(t, recs, 1+len(h.svc.AdvancedResults()))
}

func TestExportAdvancedCSV_NeutralisesFormulas(t *testing.T) {
	h := newTools(t)
	h.custom = []model.Server{
		{ID: "evil", Name: "=HYPERLINK(\"http://x\")", Protocol: model.ProtoDoH, Address: "https://e.example/dns-query", Source: model.SourceCustom},
		{ID: "ok", Name: "Plain name", Protocol: model.ProtoDoH, Address: "https://o.example/dns-query", Source: model.SourceCustom},
	}
	var data []byte
	h.svc.x.SaveFile = func(_ context.Context, _ string, d []byte) error { data = d; return nil }
	_, err := h.svc.StartAdvancedScan(AdvScanRequest{Filter: &ServerFilter{Sources: []string{"custom"}}})
	require.NoError(t, err)
	waitAdvDone(t, h)
	require.NoError(t, h.svc.ExportAdvancedCSV(context.Background()))
	recs, err := csv.NewReader(bytes.NewReader(data[3:])).ReadAll()
	require.NoError(t, err)
	names := map[string]bool{}
	for _, r := range recs[1:] {
		names[r[0]] = true
	}
	require.True(t, names[`'=HYPERLINK("http://x")`], "%v", names)
	require.True(t, names["Plain name"])
}

func TestCSVSafe(t *testing.T) {
	for in, want := range map[string]string{"=1+1": "'=1+1", "+x": "'+x", "-x": "'-x", "@x": "'@x", "\tx": "'\tx", "\rx": "'\rx", "x=1": "x=1", "": ""} {
		require.Equal(t, want, csvSafe(in), in)
	}
}

func TestAdvScan_LargeFilterScansBest500(t *testing.T) {
	h := newTools(t)
	h.custom = nil
	for i := range 600 {
		h.custom = append(h.custom, model.Server{ID: fmt.Sprintf("d%03d", i), Name: fmt.Sprint("D", i), Protocol: model.ProtoDNSCrypt,
			Address: fmt.Sprintf("https://d%d.example/dns-query", i), Source: model.SourceDNSCrypt})
	}
	st := h.box.Get()
	st.Pinned = []string{"d599"}
	require.NoError(t, h.box.Save(st))
	h.o.d.Scans = &fScans{results: []scanner.Result{{ServerID: "d598", OK: true, Latency: 5 * time.Millisecond}}}
	h.svc.x.BuildUpstream = func(model.Server) (upstream.Upstream, error) { return blockUp{}, nil }

	require.Equal(t, 601, h.svc.CountScanServers(ServerFilter{}))
	start, err := h.svc.StartAdvancedScan(AdvScanRequest{Filter: &ServerFilter{}})
	require.NoError(t, err, "a large filter is cut down, not refused")
	require.Equal(t, 500, start.Total)
	require.Equal(t, 101, start.Skipped)
	h.svc.mu.Lock()
	_, pinned := h.svc.adv.servers["d599"]
	_, wasOK := h.svc.adv.servers["d598"]
	_, builtin := h.svc.adv.servers["cf"]
	_, last := h.svc.adv.servers["d597"]
	h.svc.mu.Unlock()
	require.True(t, pinned && wasOK && builtin, "pinned, previously good and built-in servers come first")
	require.False(t, last)
	h.svc.CancelAdvancedScan()
	waitAdvDone(t, h)
}

func TestAdvScan_LimitComesFromSettings(t *testing.T) {
	h := newTools(t)
	h.custom = nil
	for i := range 120 {
		h.custom = append(h.custom, model.Server{ID: fmt.Sprintf("x%03d", i), Name: fmt.Sprint("X", i), Protocol: model.ProtoDoH,
			Address: fmt.Sprintf("https://x%d.example/dns-query", i), Source: model.SourceCustom})
	}
	st := h.box.Get()
	st.Tools.Scanner.MaxServers = 50
	require.NoError(t, h.box.Save(st))
	h.svc.x.BuildUpstream = func(model.Server) (upstream.Upstream, error) { return blockUp{}, nil }

	start, err := h.svc.StartAdvancedScan(AdvScanRequest{Filter: &ServerFilter{}})
	require.NoError(t, err)
	require.Equal(t, 50, start.Total)
	require.Equal(t, 71, start.Skipped) // 120 custom + 1 built-in
	h.svc.CancelAdvancedScan()
	waitAdvDone(t, h)

	var lines []string
	for i := range 51 {
		lines = append(lines, fmt.Sprintf("https://p%d.example/dns-query", i))
	}
	_, err = h.svc.StartAdvancedScan(AdvScanRequest{Pasted: strings.Join(lines, "\n")})
	var ae *AppError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, CodeScanTooMany, ae.Code)
	require.Equal(t, 50, ae.Params["max"])
}
