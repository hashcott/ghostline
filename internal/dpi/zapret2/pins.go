package zapret2

// Version is the bundled zapret2 release.
const Version = "v1.0.5.2"

// The SHA-256 of the zapret2 files Ghostline bundles, as listed in the
// release's sha256sum.txt (binaries) or computed from the release (Lua).
// All three maps exist in every build: tools/fetchdpi fetches both
// platforms' files from any OS.
var (
	WindowsPins = map[string]string{
		"winws2.exe":      "eb9807972c15f05a00416b0549d766a71c9186f8101d406e3cc6c16278164d9e",
		"cygwin1.dll":     "103104a52e5293ce418944725df19e2bf81ad9269b9a120d71d39028e821499b",
		"WinDivert.dll":   "16abd6a029e65557c6a309bea7b13bf81fff4e193567582e1cddbf6719f323e0",
		"WinDivert64.sys": "8da085332782708d8767bcace5327a6ec7283c17cfb85e40b03cd2323a90ddc2",
	}
	LinuxPins = map[string]string{
		"nfqws2": "de1414b1e0f9a5659d438cddcb0c7e9099b4533bf5c8a3cf841c7bc7e5aa1473",
	}
	LuaPins = map[string]string{
		"lua/zapret-lib.lua":     "b67a470f23b00a8d6e732c4e5135a39b224511e0b71809d5f4616adf62674980",
		"lua/zapret-antidpi.lua": "31c9dd75b0bd55e98e5306293f2be81e9d2ecadcbbf9157394ff37dcff7dc85a",
		"lua/zapret-auto.lua":    "c2028f544134b0dec33f054ac54fe4f2315d969ba034601456f72017fd5f8528",
	}
)

// PinsFor is one platform's binaries plus the Lua libraries.
func PinsFor(bins map[string]string) map[string]string {
	out := make(map[string]string, len(bins)+len(LuaPins))
	for k, v := range bins {
		out[k] = v
	}
	for k, v := range LuaPins {
		out[k] = v
	}
	return out
}
