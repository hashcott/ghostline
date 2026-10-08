package dpi

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/unix"
)

// NewNftables captures through Ghostline's nftables table and an NFQUEUE
// that nfqws2 reads; filter is the engine's port table (zapret2.Filter).
func NewNftables(filter []CaptureRule) Interceptor {
	return &nftInterceptor{
		filter:  filter,
		install: installTable,
		remove:  deleteTable,
		missing: func() []string {
			var uts unix.Utsname
			_ = unix.Uname(&uts)
			list, _ := os.ReadFile(filepath.Join("/lib/modules", unix.ByteSliceToString(uts.Release[:]), "modules.builtin"))
			builtin := builtinModules(list)
			return missingModules(func(m string) bool {
				_, err := os.Stat("/sys/module/" + m)
				return err == nil || builtin[m]
			})
		},
		modprobe: func(m string) error { return exec.Command("modprobe", m).Run() },
		queue:    func() ([]byte, error) { return os.ReadFile("/proc/net/netfilter/nfnetlink_queue") },
		prepDir:  prepareEngineDir,
	}
}

// prepareEngineDir lets the drop user read the engine directory through its
// group and write only auto/ (the autohostlist).
func prepareEngineDir(dir string) error {
	u, err := user.Lookup(DropUser)
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return prepareEngineDirAs(dir, "nfqws2", uid, gid)
}

// prepareEngineDirAs: dir and its subdirectories 0750 root:gid, files 0644,
// exe 0755, auto/ 0700 owned by uid:gid; the bin directory 0755 and the
// directory above it traversable (o+x) but not listable.
func prepareEngineDirAs(dir, exe string, uid, gid int) error {
	owner := os.Geteuid()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return nil // a link or pipe left by the engine: never chmod through it
		}
		mode := os.FileMode(0o644)
		switch {
		case d.IsDir():
			mode = 0o750
		case p == filepath.Join(dir, exe):
			mode = 0o755
		}
		if err := os.Lchown(p, owner, gid); err != nil {
			return err
		}
		return os.Chmod(p, mode)
	})
	if err != nil {
		return err
	}
	auto := filepath.Join(dir, "auto")
	if err := os.MkdirAll(auto, 0o700); err != nil {
		return err
	}
	err = filepath.WalkDir(auto, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return nil
		}
		return errors.Join(os.Lchown(p, uid, gid), os.Chmod(p, map[bool]os.FileMode{true: 0o700, false: 0o644}[d.IsDir()]))
	})
	if err != nil {
		return err
	}
	bin := filepath.Dir(dir)
	if err := os.Chmod(bin, 0o755); err != nil {
		return err
	}
	root := filepath.Dir(bin)
	fi, err := os.Stat(root)
	if err != nil {
		return err
	}
	return os.Chmod(root, fi.Mode().Perm()|0o001)
}
