package secrets

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type userDPAPI struct{}

// NewUserDPAPI encrypts with DPAPI for the current user (upstream proxy
// passwords). Empty input is allowed, as it always was for passwords.
func NewUserDPAPI() Protector { return userDPAPI{} }

func (userDPAPI) Protect(in []byte) ([]byte, error) {
	var inBlob windows.DataBlob
	if len(in) > 0 {
		inBlob = windows.DataBlob{Size: uint32(len(in)), Data: &in[0]}
	}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&inBlob, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("dpapi: protect: %w", err)
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) }()
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

// Unprotect fails for data protected by another user or machine.
func (userDPAPI) Unprotect(in []byte) ([]byte, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("dpapi: empty blob")
	}
	inBlob := windows.DataBlob{Size: uint32(len(in)), Data: &in[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&inBlob, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("dpapi: unprotect: %w", err)
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) }()
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

type machineDPAPI struct{}

// NewMachineDPAPI encrypts with DPAPI for this computer (any account on it
// can decrypt, so files holding the result must be ACL-protected).
// Ghostline runs elevated, possibly as another admin than the one logged
// on, so per-user DPAPI would not survive a change of elevating account.
func NewMachineDPAPI() Protector { return machineDPAPI{} }

func (machineDPAPI) Protect(b []byte) ([]byte, error) {
	return dpapi(b, true, windows.CRYPTPROTECT_UI_FORBIDDEN|windows.CRYPTPROTECT_LOCAL_MACHINE)
}

func (machineDPAPI) Unprotect(b []byte) ([]byte, error) {
	return dpapi(b, false, windows.CRYPTPROTECT_UI_FORBIDDEN)
}

func dpapi(in []byte, protect bool, flags uint32) ([]byte, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("dpapi: empty input")
	}
	inBlob := windows.DataBlob{Size: uint32(len(in)), Data: &in[0]}
	var out windows.DataBlob
	var err error
	if protect {
		err = windows.CryptProtectData(&inBlob, nil, nil, 0, nil, flags, &out)
	} else {
		err = windows.CryptUnprotectData(&inBlob, nil, nil, 0, nil, flags, &out)
	}
	if err != nil {
		return nil, fmt.Errorf("dpapi: %w", err)
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) }()
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
