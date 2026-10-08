package platform

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// Every Deps field must be set by every OS, so adding a field forces each
// platform file to decide what it does there.
func TestNew_FillsEveryField(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "ghostline"))
	require.NoError(t, err)
	v := reflect.ValueOf(d)
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		if name == "SecureDir" || name == "OwnedByAdmins" || name == "UsesDaemon" || name == "WatchResume" || name == "WatchSessions" {
			continue // documented as optional
		}
		f := v.Field(i)
		switch f.Kind() {
		case reflect.Func, reflect.Interface:
			require.False(t, f.IsNil(), name)
		}
	}
	require.NotEmpty(t, d.Paths.DataDir)
}
