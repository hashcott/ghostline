package shell

import "errors"

// runClient: Windows runs Ghostline in the GUI process; there is no daemon.
func runClient(Options) error { return errors.New("shell: no daemon client on Windows") }
