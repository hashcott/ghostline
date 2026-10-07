package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/backup"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

// exportFrom fills service a with data and exports every section.
func exportFrom(t *testing.T, a *toolsHarness) []byte {
	t.Helper()
	st := a.box.Get()
	st.Pinned = []string{"cf"}
	st.DPI.Preset = "medium"
	st.Proxy.Port = 8181
	st.Proxy.Upstreams = []store.UpstreamProxy{{ID: "corp", Type: "socks5", Addr: "10.0.0.1:1080", PassEnc: "secret"}}
	st.FakeSNI = store.FakeSNISettings{Enabled: true, AckVersion: FakeSNIWarningVersion}
	st.DNSServer.ShareLAN = true
	require.NoError(t, a.box.Save(st))
	require.Empty(t, a.svc.SaveRulesTable([]rules.Rule{{Pattern: "a.com", Action: rules.Action{Block: true}, Enabled: true}}))
	n, _ := a.svc.AddServers("https://mine.example/dns-query")
	require.Equal(t, 1, n)
	require.NoError(t, os.WriteFile(a.paths.DPIBlacklist, []byte("youtube.com\n"), 0o644))
	var out []byte
	a.svc.x.SaveFile = func(_ context.Context, name string, data []byte) error {
		require.Equal(t, backup.FileName(time.Now()), name)
		out = data
		return nil
	}
	require.NoError(t, a.svc.ExportSettings(context.Background(), backup.AllSections))
	require.NotEmpty(t, out)
	return out
}

// importInto offers data through the file dialog of service b.
func importInto(t *testing.T, b *toolsHarness, data []byte) ImportPreview {
	t.Helper()
	b.svc.x.OpenFile = func(context.Context, string) (string, []byte, error) { return "in.ghostline.json", data, nil }
	p, err := b.svc.PreviewImport(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, p.Token)
	return p
}

func TestExport_RoundTrip(t *testing.T) {
	a, b := newTools(t), newTools(t)
	data := exportFrom(t, a)
	require.NotContains(t, string(data), "secret")
	p := importInto(t, b, data)
	require.NotEmpty(t, p.Preview.Warnings)
	require.NoError(t, b.svc.ApplyImport(p.Token, backup.Choices{Sections: backup.AllSections}))

	got := b.box.Get()
	require.Equal(t, []string{"cf"}, got.Pinned)
	require.Equal(t, "medium", got.DPI.Preset)
	require.Equal(t, 8181, got.Proxy.Port)
	require.Equal(t, "", got.Proxy.Upstreams[0].PassEnc)
	require.False(t, got.FakeSNI.Enabled)
	require.False(t, got.DNSServer.ShareLAN || got.DNSServer.Enabled || got.Proxy.ShareLAN)
	require.Equal(t, 0, got.FakeSNI.AckVersion, "the warning must be read on this machine")
	onDisk, _, err := store.LoadSettings(b.paths.Settings)
	require.NoError(t, err)
	require.Equal(t, got.Pinned, onDisk.Pinned)
	require.Equal(t, []string{"a.com"}, patternsOf(b.svc.GetRules().Rules))
	require.Equal(t, "https://mine.example/dns-query", b.custom[len(b.custom)-1].Address)
	bl, _ := b.svc.GetDPIBlacklist()
	require.Equal(t, "youtube.com\n", bl)
}

func patternsOf(rs []rules.Rule) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Pattern)
	}
	return out
}

func TestApplyImport_WhileConnected(t *testing.T) {
	a, b := newTools(t), newTools(t)
	p := importInto(t, b, exportFrom(t, a))
	for _, st := range []Status{StatusConnecting, StatusProtected, StatusDegraded, StatusDisconnecting} {
		b.o.update(func(s *Snapshot) { s.Status = st })
		require.Equal(t, CodeImportWhileConnected, code(t, b.svc.ApplyImport(p.Token, backup.Choices{Sections: []string{"rules"}})), st)
	}
	b.o.update(func(s *Snapshot) { s.Status = StatusError })
	require.NoError(t, b.svc.ApplyImport(p.Token, backup.Choices{Sections: []string{"rules"}}))
}

func TestApplyImport_TokenExpires(t *testing.T) {
	a, b := newTools(t), newTools(t)
	now := time.Now()
	b.svc.now = func() time.Time { return now }
	p := importInto(t, b, exportFrom(t, a))
	now = now.Add(10*time.Minute + time.Second)
	require.Equal(t, CodeImportExpired, code(t, b.svc.ApplyImport(p.Token, backup.Choices{Sections: []string{"rules"}})))
	require.Equal(t, CodeImportExpired, code(t, b.svc.ApplyImport("bogus", backup.Choices{})))
}

func TestApplyImport_ReloadsEverything(t *testing.T) {
	a, b := newTools(t), newTools(t)
	var changed bool
	b.svc.x.OnSettingsChanged = func(_, n store.Settings) { changed = n.Proxy.Port == 8181 }
	p := importInto(t, b, exportFrom(t, a))
	require.NoError(t, b.svc.ApplyImport(p.Token, backup.Choices{Sections: backup.AllSections}))
	require.True(t, changed)
	require.Equal(t, 8181, b.svc.GetSettings().Proxy.Port)
	require.Equal(t, 1, len(b.svc.GetRules().Rules))
}

func TestApplyImport_WriteFailRollsBack(t *testing.T) {
	a, b := newTools(t), newTools(t)
	p := importInto(t, b, exportFrom(t, a))
	require.NoError(t, b.box.Save(b.box.Get()))
	before, err := os.ReadFile(b.paths.Settings)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(b.paths.Rules, "blocker"), 0o755)) // rules.json is now a directory
	err = b.svc.ApplyImport(p.Token, backup.Choices{Sections: []string{"settings", "rules"}})
	var ae *AppError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, CodeImportWriteFailed, ae.Code)
	require.Equal(t, "rules.json", ae.Params["file"])
	after, _ := os.ReadFile(b.paths.Settings)
	require.Equal(t, before, after)
	require.NotEqual(t, 8181, b.svc.GetSettings().Proxy.Port, "in-memory settings unchanged too")
}

func TestApplyImport_SNIRulesNeedConfirmation(t *testing.T) {
	a, b := newTools(t), newTools(t)
	require.Empty(t, a.svc.SaveRulesTable([]rules.Rule{{Pattern: "f.com", Action: rules.Action{SNI: "cdn.example"}, Enabled: true}}))
	var out []byte
	a.svc.x.SaveFile = func(_ context.Context, _ string, d []byte) error { out = d; return nil }
	require.NoError(t, a.svc.ExportSettings(context.Background(), []string{"rules"}))
	p := importInto(t, b, out)
	require.Len(t, p.Preview.SNIRules, 1)
	require.Equal(t, CodeImportInvalid, code(t, b.svc.ApplyImport(p.Token, backup.Choices{Sections: []string{"rules"}})))
	require.NoError(t, b.svc.ApplyImport(p.Token, backup.Choices{Sections: []string{"rules"}, SNIRules: "drop"}))
	require.Empty(t, b.svc.GetRules().Rules)
}

func TestPreviewImport_Invalid(t *testing.T) {
	b := newTools(t)
	b.svc.x.OpenFile = func(context.Context, string) (string, []byte, error) {
		return "x.json", []byte(`{"format":"ghostline-backup","formatVersion":9}`), nil
	}
	_, err := b.svc.PreviewImport(context.Background())
	var ae *AppError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, CodeImportInvalid, ae.Code)
	require.Equal(t, "newer", ae.Params["detail"])

	b.svc.x.OpenFile = func(context.Context, string) (string, []byte, error) { return "", nil, nil } // cancelled
	p, err := b.svc.PreviewImport(context.Background())
	require.NoError(t, err)
	require.Empty(t, p.Token)
}

func TestLoadBackupData_FromDisk(t *testing.T) {
	a := newTools(t)
	exportFrom(t, a)
	require.NoError(t, store.WriteJSONAtomic(a.paths.ServersCustom, a.custom))
	d, err := LoadBackupData(a.paths)
	require.NoError(t, err)
	require.Equal(t, 8181, d.Settings.Proxy.Port)
	require.Equal(t, []string{"a.com"}, patternsOf(d.Rules.Rules))
	require.Equal(t, "youtube.com\n", d.Blacklist)
	require.NotEmpty(t, d.Custom)
	_ = []model.Server(d.Custom) // Custom keeps the full server type
}

func TestApplyImport_RefusedWhileAConnectIsRunning(t *testing.T) {
	a, b := newTools(t), newTools(t)
	p := importInto(t, b, exportFrom(t, a))
	b.o.opMu.Lock() // a connect/disconnect/autotune is in progress
	err := b.svc.ApplyImport(p.Token, backup.Choices{Sections: []string{"rules"}})
	b.o.opMu.Unlock()
	require.Equal(t, CodeImportWhileConnected, code(t, err))
	require.NoError(t, b.svc.ApplyImport(p.Token, backup.Choices{Sections: []string{"rules"}}))
}

func TestApplyImport_HoldsRulesLockAcrossWrite(t *testing.T) {
	a, b := newTools(t), newTools(t)
	p := importInto(t, b, exportFrom(t, a))
	b.svc.rmu.Lock() // a rules save is in progress
	done := make(chan error, 1)
	go func() { done <- b.svc.ApplyImport(p.Token, backup.Choices{Sections: []string{"rules"}}) }()
	select {
	case <-done:
		b.svc.rmu.Unlock()
		t.Fatal("import wrote rules while a rules save held the lock")
	case <-time.After(150 * time.Millisecond):
	}
	b.svc.rmu.Unlock()
	require.NoError(t, <-done)
	require.Equal(t, []string{"a.com"}, patternsOf(b.svc.GetRules().Rules))
}

// The GUI reads the file and hands over its contents: the daemon never opens
// a path the user picked.
func TestPreviewImport_ReadsContentFromOpenFile(t *testing.T) {
	a, b := newTools(t), newTools(t)
	data := exportFrom(t, a)
	var title string
	b.svc.x.OpenFile = func(_ context.Context, ttl string) (string, []byte, error) {
		title = ttl
		return "x.ghostline.json", data, nil
	}
	p, err := b.svc.PreviewImport(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, p.Token)
	require.Equal(t, "x.ghostline.json", p.Path)
	require.NotEmpty(t, title)
}

func TestPreviewImport_CancelledIsNoError(t *testing.T) {
	b := newTools(t)
	b.svc.x.OpenFile = func(context.Context, string) (string, []byte, error) { return "", nil, nil }
	p, err := b.svc.PreviewImport(context.Background())
	require.NoError(t, err)
	require.Equal(t, ImportPreview{}, p)
}

func TestPreviewImport_TooLargeIsInvalid(t *testing.T) {
	b := newTools(t)
	b.svc.x.OpenFile = func(context.Context, string) (string, []byte, error) {
		return "big.json", make([]byte, backup.MaxSize+1), nil
	}
	_, err := b.svc.PreviewImport(context.Background())
	var ae *AppError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, CodeImportInvalid, ae.Code)
	require.Equal(t, "too_large", ae.Params["detail"])
}

func TestLANDNSClients_NilDepIsZero(t *testing.T) {
	b := newTools(t)
	b.svc.x.LANDNSClients = nil
	require.Equal(t, 0, b.svc.LANDNSClients())
	b.svc.x.LANDNSClients = func() int { return 4 }
	require.Equal(t, 4, b.svc.LANDNSClients())
}
