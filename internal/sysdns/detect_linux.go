package sysdns

import (
	"errors"
	"os/exec"

	"github.com/hashcott/ghostline/internal/netwatch"
)

const resolvConfPath = "/etc/resolv.conf"

// DetectLinux returns the backend for this machine's DNS stack:
// NetworkManager, systemd-resolved, or /etc/resolv.conf (also when the
// system bus is unavailable).
func DetectLinux(dataDir string) Backend {
	apis, busErr := newDBus()
	flush := func() error {
		var errs []error
		if busErr == nil && apis.Resolved.Running() {
			errs = append(errs, apis.Resolved.FlushCaches())
		}
		if nscd, err := exec.LookPath("nscd"); err == nil {
			errs = append(errs, exec.Command(nscd, "-i", "hosts").Run())
		}
		return errors.Join(errs...)
	}
	fileWatch := func(onChange func()) (func(), error) { return watchFile(resolvConfPath, onChange) }
	fallback := newResolvConf(resolvConfPath, dataDir, flush, netwatch.Combine(fileWatch, netwatch.Watch))
	if busErr != nil {
		return fallback
	}
	return detect(apis.NM, apis.Resolved, apis.Units, fallback, flush, netwatch.Watch)
}
