package sysproxy

import (
	"fmt"
	"log/slog"
	"runtime"
	"unsafe"

	"github.com/hashcott/ghostline/internal/model"
	"golang.org/x/sys/windows"
)

var (
	wininet         = windows.NewLazySystemDLL("wininet.dll")
	procQueryOption = wininet.NewProc("InternetQueryOptionW")
	procSetOption   = wininet.NewProc("InternetSetOptionW")
	kernel32        = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalFree  = kernel32.NewProc("GlobalFree")
)

const (
	optPerConnection   = 75 // INTERNET_OPTION_PER_CONNECTION_OPTION
	optSettingsChanged = 39 // INTERNET_OPTION_SETTINGS_CHANGED
	optRefresh         = 37 // INTERNET_OPTION_REFRESH

	perConnFlags         = 1
	perConnProxyServer   = 2
	perConnProxyBypass   = 3
	perConnAutoconfigURL = 4
)

// perConnOption is INTERNET_PER_CONN_OPTIONW (x64: 4 + pad + 8-byte union).
type perConnOption struct {
	option uint32
	_      uint32
	value  uintptr
}

// perConnOptionList is INTERNET_PER_CONN_OPTION_LISTW.
type perConnOptionList struct {
	size        uint32
	connection  *uint16 // nil: the default LAN connection
	optionCount uint32
	optionError uint32
	options     *perConnOption
}

type winAPI struct{}

// NewWindowsAPI returns the WinINET implementation.
func NewWindowsAPI() API { return winAPI{} }

func (winAPI) Query() (model.WinINETProxy, error) {
	opts := []perConnOption{{option: perConnFlags}, {option: perConnProxyServer}, {option: perConnProxyBypass}, {option: perConnAutoconfigURL}}
	list := perConnOptionList{optionCount: uint32(len(opts)), options: &opts[0]}
	list.size = uint32(unsafe.Sizeof(list))
	size := list.size
	r, _, err := procQueryOption.Call(0, optPerConnection, uintptr(unsafe.Pointer(&list)), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return model.WinINETProxy{}, fmt.Errorf("sysproxy: InternetQueryOption: %w", err)
	}
	// The OS wrote LocalAlloc'd string pointers into the union; read them as
	// pointers (not via a uintptr conversion) and free them.
	str := func(o *perConnOption) string {
		if o.value == 0 {
			return ""
		}
		p := *(**uint16)(unsafe.Pointer(&o.value))
		s := windows.UTF16PtrToString(p)
		_, _, _ = procGlobalFree.Call(o.value)
		return s
	}
	return model.WinINETProxy{
		Flags:         uint32(opts[0].value),
		Server:        str(&opts[1]),
		Bypass:        str(&opts[2]),
		AutoconfigURL: str(&opts[3]),
	}, nil
}

func (winAPI) Set(s model.WinINETProxy) error {
	ptr := func(v string) (*uint16, uintptr) {
		if v == "" {
			return nil, 0
		}
		p, _ := windows.UTF16PtrFromString(v)
		return p, uintptr(unsafe.Pointer(p))
	}
	server, serverP := ptr(s.Server)
	bypass, bypassP := ptr(s.Bypass)
	pac, pacP := ptr(s.AutoconfigURL)
	opts := []perConnOption{
		{option: perConnFlags, value: uintptr(s.Flags)},
		{option: perConnProxyServer, value: serverP},
		{option: perConnProxyBypass, value: bypassP},
		{option: perConnAutoconfigURL, value: pacP},
	}
	list := perConnOptionList{optionCount: uint32(len(opts)), options: &opts[0]}
	list.size = uint32(unsafe.Sizeof(list))
	r, _, err := procSetOption.Call(0, optPerConnection, uintptr(unsafe.Pointer(&list)), uintptr(list.size))
	runtime.KeepAlive(server)
	runtime.KeepAlive(bypass)
	runtime.KeepAlive(pac)
	if r == 0 {
		return fmt.Errorf("sysproxy: InternetSetOption: %w", err)
	}
	// Without these broadcasts running browsers keep the old proxy until
	// restarted; the setting itself is already written.
	if r, _, err := procSetOption.Call(0, optSettingsChanged, 0, 0); r == 0 {
		slog.Warn("sysproxy: broadcast INTERNET_OPTION_SETTINGS_CHANGED failed", "err", err)
	}
	if r, _, err := procSetOption.Call(0, optRefresh, 0, 0); r == 0 {
		slog.Warn("sysproxy: broadcast INTERNET_OPTION_REFRESH failed", "err", err)
	}
	return nil
}
