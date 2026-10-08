// Package zapret2 is the zapret2 engine (winws2 on Windows, nfqws2 on
// Linux): it turns a strategy into a command line and checks every
// strategy argument against an allow-list, since the engine runs strategy
// Lua with elevated rights.
package zapret2

import (
	"errors"
	"fmt"

	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/dpi/zapret2/strategies"
)

type engine struct{ list func() strategies.List }

// New returns the engine. list is read on every Args/Strategies call so a
// newly downloaded list applies on the next start.
func New(list func() strategies.List) dpi.Engine { return engine{list: list} }

func (engine) ID() string                    { return "zapret2" }
func (engine) Exe() string                   { return exeName }
func (engine) HotReloadsLists() bool         { return true }
func (engine) Files() map[string]string      { return Pinned }
func (engine) ValidateCustom(s string) error { _, err := ValidateCustom(s); return err }

func (e engine) Strategies() []dpi.Strategy {
	var out []dpi.Strategy
	for _, s := range e.list().Zapret2 {
		out = append(out, dpi.Strategy{ID: s.ID, Name: s.Name})
	}
	return out
}

// luaLibs are loaded in this order; strategies call functions they define.
var luaLibs = []string{"lua/zapret-lib.lua", "lua/zapret-antidpi.lua", "lua/zapret-auto.lua"}

// Args builds: interception filter, init, a TCP profile and, when the
// strategy has one, a QUIC profile. Every value uses the --name=value form
// (winws2's getopt silently drops stray space-separated values).
func (e engine) Args(p dpi.Plan) ([]string, error) {
	tcp, quic, err := e.profiles(p)
	if err != nil {
		return nil, err
	}
	args := interceptArgs(len(quic) > 0)
	for _, l := range luaLibs {
		args = append(args, "--lua-init=@"+l)
	}
	args = append(args, "--filter-tcp=80,443")
	args = append(append(args, scope(p)...), tcp...)
	if len(quic) > 0 {
		args = append(args, "--new", "--filter-udp=443", "--filter-l7=quic")
		args = append(append(args, scope(p)...), quic...)
	}
	return args, nil
}

func (e engine) profiles(p dpi.Plan) (tcp, quic []string, err error) {
	if p.Strategy == "custom" {
		tcp, err = ValidateCustom(p.Custom)
		if err == nil && len(tcp) == 0 {
			err = errors.New("zapret2: custom args are empty")
		}
		return tcp, nil, err
	}
	for _, s := range e.list().Zapret2 {
		if s.ID == p.Strategy {
			// The list was validated when loaded; check again so a bad
			// list can never reach winws2.
			if err := ValidateArgs(append(append([]string{}, s.TCP...), s.QUIC...)); err != nil {
				return nil, nil, err
			}
			return s.TCP, s.QUIC, nil
		}
	}
	return nil, nil, fmt.Errorf("zapret2: unknown strategy %q", p.Strategy)
}

// scope limits a profile to the blacklist (and what auto-detection added).
// With scope "all" the strategy already applies to every host, so
// auto-detection would add nothing.
func scope(p dpi.Plan) []string {
	if p.Scope != dpi.ScopeBlacklist {
		return nil
	}
	out := []string{"--hostlist=" + p.Blacklist}
	if p.AutoHostlist != "" {
		out = append(out, "--hostlist-auto="+p.AutoHostlist)
	}
	return out
}
