package dpi

// Interceptor sets up, checks and removes the packet capture an engine
// runs behind: the WinDivert driver on Windows, an nftables queue on Linux.
type Interceptor interface {
	// Prepare runs before the engine starts.
	Prepare() error
	// Ready reports whether the engine (pid) is capturing packets.
	Ready(pid int) bool
	// Cleanup runs after the engine stops and during recovery; nothing to
	// remove is not an error.
	Cleanup() error
	Info() InterceptorInfo
}

// InterceptorInfo describes the capture for the UI.
type InterceptorInfo struct {
	Mechanism    string `json:"mechanism"`
	AVExclusions bool   `json:"avExclusions"` // antivirus exclusions matter (Windows)
}

// NoInterceptor is the Interceptor for an OS without packet capture yet.
type NoInterceptor struct{}

func (NoInterceptor) Prepare() error        { return nil }
func (NoInterceptor) Ready(int) bool        { return true }
func (NoInterceptor) Cleanup() error        { return nil }
func (NoInterceptor) Info() InterceptorInfo { return InterceptorInfo{} }
