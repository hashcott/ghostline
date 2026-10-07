package main

import (
	"hash/fnv"
	"os"
	"reflect"
	"testing"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/stretchr/testify/require"
)

func TestBindingID_IsFNV32aOfAppFQN(t *testing.T) {
	h := fnv.New32a()
	_, _ = h.Write([]byte("github.com/hashcott/ghostline/internal/app.Service.Connect"))
	require.Equal(t, h.Sum32(), bindingID("Connect"))
}

// The committed file is what `go generate` writes today.
func TestRender_IsUpToDate(t *testing.T) {
	want, err := os.ReadFile("../../internal/rpc/client/service_gen.go")
	require.NoError(t, err)
	got, err := render(appMethods(), defaultSkip())
	require.NoError(t, err)
	require.Equal(t, string(want), string(got))
}

func TestAppMethods_AreTheServiceMethods(t *testing.T) {
	require.Equal(t, reflect.TypeOf(&app.Service{}).NumMethod(), len(appMethods()))
}
