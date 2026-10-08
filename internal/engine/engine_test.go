package engine_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// fakeUp answers every A query with ip and counts calls.
type fakeUp struct {
	ip     net.IP
	calls  atomic.Int32
	closed atomic.Bool
	name   string
}

func (f *fakeUp) Exchange(_ context.Context, req *dns.Msg) (*dns.Msg, error) {
	f.calls.Add(1)
	m := new(dns.Msg).SetReply(req)
	m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: req.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 1}, A: f.ip}}
	return m, nil
}
func (f *fakeUp) Address() string { return f.name }
func (f *fakeUp) Close() error    { f.closed.Store(true); return nil }

var _ upstream.Upstream = (*fakeUp)(nil)

func start(t *testing.T, onQuery func(engine.QueryEvent), ups ...upstream.Upstream) *engine.Engine {
	t.Helper()
	// One port for both protocols, so Swap (which listens again on the same
	// address for UDP and TCP) cannot land on a Windows-reserved TCP port.
	var err error
	for range 10 {
		e := engine.New(onQuery)
		if err = e.Start(context.Background(), engine.Config{
			ListenV4:  netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), freeDualPort(t)),
			Upstreams: ups,
		}); err == nil {
			t.Cleanup(func() { _ = e.Stop(context.Background()) })
			return e
		}
	}
	require.NoError(t, err)
	return nil
}

func query(t *testing.T, e *engine.Engine, name string) *dns.Msg {
	t.Helper()
	c := &dns.Client{Timeout: 2 * time.Second}
	r, _, err := c.Exchange(new(dns.Msg).SetQuestion(name, dns.TypeA), e.ListenAddr().String())
	require.NoError(t, err)
	return r
}

func TestEngine_ResolvesThroughUpstream(t *testing.T) {
	up := &fakeUp{ip: net.IPv4(192, 0, 2, 80), name: "fake1"}
	e := start(t, nil, up)
	r := query(t, e, "example.org.")
	require.Equal(t, "192.0.2.80", r.Answer[0].(*dns.A).A.String())
}

func TestEngine_AnswersVerifyLocally(t *testing.T) {
	up := &fakeUp{ip: net.IPv4(192, 0, 2, 80), name: "fake1"}
	e := start(t, nil, up)
	e.ExpectVerify("n1")
	require.False(t, e.SawVerify("n1"))
	r := query(t, e, "n1.verify.ghostline.test.")
	require.Equal(t, "192.0.2.1", r.Answer[0].(*dns.A).A.String())
	require.True(t, e.SawVerify("n1"))
	require.Zero(t, up.calls.Load())
}

func TestEngine_SelfTest(t *testing.T) {
	e := engine.New(nil)
	require.NoError(t, e.Start(context.Background(), engine.Config{
		ListenV4: netip.MustParseAddrPort("127.0.0.1:0"), Upstreams: []upstream.Upstream{&fakeUp{ip: net.IPv4(192, 0, 2, 80)}},
	}))
	require.NoError(t, e.SelfTest(context.Background()))
	require.NoError(t, e.Stop(context.Background()))
	require.Error(t, e.SelfTest(context.Background()))
}

func TestEngine_SwapReplacesUpstreams(t *testing.T) {
	up1 := &fakeUp{ip: net.IPv4(192, 0, 2, 81), name: "up1"}
	up2 := &fakeUp{ip: net.IPv4(192, 0, 2, 82), name: "up2"}
	e := start(t, nil, up1)
	addr := e.ListenAddr()
	require.NoError(t, e.Swap(context.Background(), []upstream.Upstream{up2}))
	require.Equal(t, addr, e.ListenAddr(), "swap keeps the listen address")
	r := query(t, e, "swap.example.")
	require.Equal(t, "192.0.2.82", r.Answer[0].(*dns.A).A.String())
	require.True(t, up1.closed.Load())
}

func TestEngine_StatsAndOnQuery(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	e := start(t, func(q engine.QueryEvent) { mu.Lock(); seen = append(seen, q.Domain); mu.Unlock() },
		&fakeUp{ip: net.IPv4(192, 0, 2, 80), name: "fake1"})
	for _, n := range []string{"a.example.org.", "b.example.org.", "c.example.org."} {
		query(t, e, n)
	}
	st := e.Stats()
	require.Equal(t, uint64(3), st.Queries)
	require.Equal(t, uint64(3), st.PerUpstream["fake1"].Queries)
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, seen, "a.example.org.")
}

type errUp struct{}

func (errUp) Exchange(context.Context, *dns.Msg) (*dns.Msg, error) {
	return nil, errors.New(`Get "https://dns.example/dns-query?dns=c2VjcmV0": connection refused`)
}
func (errUp) Address() string { return "https://dns.example/dns-query" }
func (errUp) Close() error    { return nil }

func TestEngine_NeverLogsToDefaultLogger(t *testing.T) { // review I11: domains must not reach the file log
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)
	// Port 0: picking a free TCP port and binding UDP on it too was flaky
	// on Windows, where some UDP ports are reserved.
	e := start(t, nil, errUp{})
	c := &dns.Client{Timeout: 2 * time.Second}
	_, _, err := c.Exchange(new(dns.Msg).SetQuestion("secret-domain.example.", dns.TypeA), e.ListenAddr().String())
	require.NoError(t, err, "the query must reach the engine, or the test proves nothing")
	time.Sleep(50 * time.Millisecond)
	require.NotContains(t, buf.String(), "secret")
	require.NotContains(t, buf.String(), "c2VjcmV0")
}

func TestEngine_StatsResetOnStartAndSwap(t *testing.T) { // review minor: stale stats
	up := &fakeUp{ip: net.IPv4(192, 0, 2, 80), name: "fake1"}
	e := start(t, nil, up)
	query(t, e, "a.example.")
	require.NoError(t, e.Swap(context.Background(), []upstream.Upstream{&fakeUp{ip: net.IPv4(192, 0, 2, 81), name: "fake2"}}))
	require.Empty(t, e.Stats().PerUpstream, "old upstreams must not count after a swap")
	query(t, e, "b.example.")
	require.NoError(t, e.Stop(context.Background()))
	require.NoError(t, e.Start(context.Background(), engine.Config{ListenV4: netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), freeDualPort(t)), Upstreams: []upstream.Upstream{up}}))
	require.Zero(t, e.Stats().Queries, "a new session starts from zero")
}
