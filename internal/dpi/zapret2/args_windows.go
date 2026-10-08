package zapret2

const exeName = "winws2.exe"

// Pinned is what the Windows build extracts and verifies.
var Pinned = PinsFor(WindowsPins)

// interceptArgs are winws2's WinDivert filters, built from Filter.
func interceptArgs(quic bool) []string {
	var out []string
	for _, r := range Filter {
		if r.QUIC && !quic {
			continue
		}
		out = append(out, "--wf-"+r.Proto+"-out="+portsCSV(r))
	}
	return append(out, "--wf-dup-check=1")
}
