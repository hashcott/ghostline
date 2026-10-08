package session

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
	"time"
)

// runTimeout bounds a one-shot task (gsettings, kwriteconfig, certutil).
const runTimeout = 10 * time.Second

type linuxSessions struct {
	exe      string   // the daemon's own binary (root-owned)
	extraEnv []string // tests only
	lg       logindAPI
	lookup   lookupFunc
	log      *slog.Logger
}

// NewLinux finds sessions through systemd-logind and runs the daemon's own
// binary as their agent.
func NewLinux(log *slog.Logger) (Sessions, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	lg, err := newLogind()
	if err != nil {
		return nil, err
	}
	return &linuxSessions{exe: exe, lg: lg, lookup: lookupUser, log: log}, nil
}

func lookupUser(uid int) (string, string, int, []int, error) {
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return "", "", 0, nil, err
	}
	gid, _ := strconv.Atoi(u.Gid)
	var groups []int
	if ids, err := u.GroupIds(); err == nil {
		for _, s := range ids {
			if g, err := strconv.Atoi(s); err == nil {
				groups = append(groups, g)
			}
		}
	}
	return u.Username, u.HomeDir, gid, groups, nil
}

func hasBus(uid int) bool {
	_, err := os.Stat(fmt.Sprintf("/run/user/%d/bus", uid))
	return err == nil
}

func (s *linuxSessions) Active() (User, bool) { return activeUser(s.lg, s.lookup) }
func (s *linuxSessions) ByUID(uid int) (User, bool) {
	return userByUID(s.lg, s.lookup, hasBus, uid)
}

func (s *linuxSessions) WatchNew(onNew func(User)) (func(), error) {
	return s.lg.WatchNew(func(si sessionInfo) {
		if u, ok := userOf(si, s.lookup); ok {
			onNew(u)
		}
	})
}

// command starts the agent as u with only u's session environment.
func (s *linuxSessions) command(ctx context.Context, u User) *exec.Cmd {
	cmd := exec.CommandContext(ctx, s.exe, "--session-agent")
	cmd.Dir = "/"
	cmd.Env = append([]string{
		"HOME=" + u.Home,
		"USER=" + u.Name,
		"LOGNAME=" + u.Name,
		"PATH=/usr/local/bin:/usr/bin:/bin",
		fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", u.UID),
		fmt.Sprintf("DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/%d/bus", u.UID),
		"XDG_CURRENT_DESKTOP=" + u.Desktop,
	}, s.extraEnv...)
	if u.UID != os.Geteuid() {
		groups := make([]uint32, len(u.Groups))
		for i, g := range u.Groups {
			groups[i] = uint32(g)
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(u.UID), Gid: uint32(u.GID), Groups: groups}}
	}
	return cmd
}

type wireReply struct {
	Result json.RawMessage `json:"result"`
	Error  *Error          `json:"error"`
}

func requestLine(task string, in any) ([]byte, error) {
	args, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(request{Task: task, Args: args})
	return append(b, '\n'), err
}

func (s *linuxSessions) Run(u User, task string, in, out any) error {
	line, err := requestLine(task, in)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	cmd := s.command(ctx, u)
	cmd.Stdin = bytes.NewReader(line)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	runErr := cmd.Run()
	var r wireReply
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &r); err != nil {
		return fmt.Errorf("session: agent %s for uid %d: %v (%v)", task, u.UID, runErr, err)
	}
	if r.Error != nil {
		return r.Error
	}
	if out == nil || len(r.Result) == 0 {
		return nil
	}
	return json.Unmarshal(r.Result, out)
}

func (s *linuxSessions) Stream(u User, task string, in any, onLine func(json.RawMessage)) (func(), error) {
	line, err := requestLine(task, in)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := s.command(ctx, u)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	if _, err := stdin.Write(line); err != nil {
		cancel()
		_ = cmd.Wait()
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			var r wireReply
			if json.Unmarshal(sc.Bytes(), &r) != nil || ctx.Err() != nil {
				continue
			}
			if r.Error != nil {
				if s.log != nil {
					s.log.Warn("session agent stream", "task", task, "uid", u.UID, "err", r.Error)
				}
				continue
			}
			onLine(r.Result)
		}
		_, _ = io.Copy(io.Discard, stdout)
	}()
	return func() {
		_ = stdin.Close() // the agent's stream task ends
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		cancel() // kills it if it did not
		_ = cmd.Wait()
	}, nil
}
