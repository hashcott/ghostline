package shell

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/backup"
)

// saveFileHandler answers the daemon's "saveFile" (args: name, data): the
// GUI asks where, then writes as the user, never as root.
func saveFileHandler(pickPath func(name string) (string, error)) func(args []json.RawMessage) (any, error) {
	return func(args []json.RawMessage) (any, error) {
		if len(args) != 2 {
			return nil, errors.New("saveFile: want name and data")
		}
		var name string
		var data []byte
		if err := json.Unmarshal(args[0], &name); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(args[1], &data); err != nil {
			return nil, err
		}
		path, err := pickPath(name)
		if err != nil || path == "" {
			return nil, err // cancelled: nothing to save
		}
		return nil, os.WriteFile(path, data, 0o644)
	}
}

// openFileHandler answers the daemon's "openFile" (args: title): the GUI
// reads the chosen file as the user and sends its contents, at most one
// backup's worth.
func openFileHandler(pickPath func(title string) (string, error)) func(args []json.RawMessage) (any, error) {
	type picked struct {
		Name string `json:"name"`
		Data []byte `json:"data"`
	}
	return func(args []json.RawMessage) (any, error) {
		var title string
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &title)
		}
		path, err := pickPath(title)
		if err != nil || path == "" {
			return picked{}, err // cancelled
		}
		data, err := readLimited(path, backup.MaxSize+1)
		if err != nil {
			return nil, err
		}
		if len(data) > backup.MaxSize {
			return nil, fmt.Errorf("%s: file too large", app.CodeImportInvalid)
		}
		return picked{Name: filepath.Base(path), Data: data}, nil
	}
}
