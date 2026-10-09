package shell

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                        = windows.NewLazySystemDLL("user32.dll")
	procRegisterWindowMessage     = user32.NewProc("RegisterWindowMessageW")
	procChangeWindowMessageFilter = user32.NewProc("ChangeWindowMessageFilter")
)

const msgfltAdd = 1

// allowTaskbarCreated lets Explorer's TaskbarCreated broadcast reach
// Ghostline. Ghostline runs elevated and Explorer does not, so UIPI drops
// the message unless the process allows it, and the tray icon never comes
// back: not after Explorer restarts, nor at logon when Ghostline starts
// before the taskbar exists.
func allowTaskbarCreated() error {
	name, err := windows.UTF16PtrFromString("TaskbarCreated")
	if err != nil {
		return err
	}
	msg, _, err := procRegisterWindowMessage.Call(uintptr(unsafe.Pointer(name)))
	if msg == 0 {
		return fmt.Errorf("shell: RegisterWindowMessage(TaskbarCreated): %w", err)
	}
	if ok, _, err := procChangeWindowMessageFilter.Call(msg, msgfltAdd); ok == 0 {
		return fmt.Errorf("shell: ChangeWindowMessageFilter(TaskbarCreated): %w", err)
	}
	return nil
}
