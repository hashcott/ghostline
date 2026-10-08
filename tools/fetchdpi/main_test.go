package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSHA256Sums(t *testing.T) {
	got := parseSHA256Sums("aa11  zapret2-v1/binaries/windows-x86_64/winws2.exe\n" +
		"bb22 *zapret2-v1/lua/x.lua\r\n\n  \nnot a line\n")
	require.Equal(t, map[string]string{
		"zapret2-v1/binaries/windows-x86_64/winws2.exe": "aa11",
		"zapret2-v1/lua/x.lua":                          "bb22",
	}, got)
}

func TestPick(t *testing.T) {
	files := map[string][]byte{"a.exe": []byte("a"), "lua/b.lua": []byte("b"), "extra": []byte("x")}
	want := map[string]string{
		"a.exe":     "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb",
		"lua/b.lua": "3e23e8160039594a33894f6564e1b1348bbd7a0088d42c4acb73eeaed59c009d",
	}
	got, err := pick(files, want)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, []byte("b"), got["lua/b.lua"])
}

func TestPick_RejectsHashMismatch(t *testing.T) {
	_, err := pick(map[string][]byte{"a.exe": []byte("evil")},
		map[string]string{"a.exe": "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"})
	require.Error(t, err)
}

func TestPick_RejectsMissingFile(t *testing.T) {
	_, err := pick(map[string][]byte{}, map[string]string{"a.exe": "00"})
	require.Error(t, err)
}

// The release's sha256sum.txt must agree with the pins for both platforms.
func TestCheckListed_LinuxBinary(t *testing.T) {
	root := "zapret2-v1/"
	pins := map[string]string{"nfqws2": "aa"}
	require.NoError(t, checkListed(map[string]string{root + "binaries/linux-x86_64/nfqws2": "aa"}, root+"binaries/linux-x86_64/", pins))
	require.Error(t, checkListed(map[string]string{root + "binaries/linux-x86_64/nfqws2": "bb"}, root+"binaries/linux-x86_64/", pins))
}
