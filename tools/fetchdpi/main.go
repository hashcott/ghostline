// Command fetchdpi downloads the official GoodbyeDPI or zapret2 release and
// writes the files Ghostline bundles into an assets directory, refusing any
// file whose SHA-256 differs from the pins in internal/dpi.
//
//	go run ./tools/fetchdpi -what zapret2 -out assets/zapret2
//	go run ./tools/fetchdpi -what goodbyedpi -out assets/goodbyedpi
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/hashcott/ghostline/internal/dpi/goodbyedpi"
	"github.com/hashcott/ghostline/internal/dpi/zapret2"
)

const goodbyedpiZip = "https://github.com/ValdikSS/GoodbyeDPI/releases/download/0.2.3rc3/goodbyedpi-0.2.3rc3-2.zip"

func main() {
	what := flag.String("what", "", "zapret2 | goodbyedpi")
	out := flag.String("out", "", "assets directory to write into")
	flag.Parse()
	if *out == "" {
		log.Fatal("-out is required")
	}
	var files map[string][]byte
	var err error
	switch *what {
	case "zapret2":
		files, err = fetchZapret2()
	case "goodbyedpi":
		files, err = fetchGoodbyeDPI()
	default:
		log.Fatal("-what must be zapret2 or goodbyedpi")
	}
	if err != nil {
		log.Fatal(err)
	}
	for name, b := range files {
		dst := filepath.Join(*out, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println("wrote", dst)
	}
}

func fetchZapret2() (map[string][]byte, error) {
	v := zapret2.Version
	base := "https://github.com/bol-van/zapret2/releases/download/" + v + "/"
	sums, err := get(base + "sha256sum.txt")
	if err != nil {
		return nil, err
	}
	listed := parseSHA256Sums(string(sums))
	tgz, err := get(base + "zapret2-" + v + ".tar.gz")
	if err != nil {
		return nil, err
	}
	root := "zapret2-" + v + "/"
	winDir, linDir := root+"binaries/windows-x86_64/", root+"binaries/linux-x86_64/"
	all, err := untar(tgz, func(name string) (string, bool) {
		switch {
		case strings.HasPrefix(name, winDir):
			return strings.TrimPrefix(name, winDir), true
		case name == linDir+"nfqws2":
			return "nfqws2", true
		case strings.HasPrefix(name, root+"lua/"):
			return strings.TrimPrefix(name, root), true
		case name == root+"docs/LICENSE.txt":
			return "LICENSE-zapret2.txt", true
		}
		return "", false
	})
	if err != nil {
		return nil, err
	}
	// The release's own list must agree with the pins for every binary it covers.
	if err := checkListed(listed, winDir, zapret2.WindowsPins); err != nil {
		return nil, err
	}
	if err := checkListed(listed, linDir, zapret2.LinuxPins); err != nil {
		return nil, err
	}
	files, err := pick(all, zapret2.PinsFor(zapret2.WindowsPins))
	if err != nil {
		return nil, err
	}
	linux, err := pick(all, zapret2.LinuxPins)
	if err != nil {
		return nil, err
	}
	for name, b := range linux {
		files[name] = b
	}
	files["LICENSE-zapret2.txt"] = all["LICENSE-zapret2.txt"]
	return files, nil
}

func fetchGoodbyeDPI() (map[string][]byte, error) {
	b, err := get(goodbyedpiZip)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	all := map[string][]byte{}
	for _, f := range zr.File {
		if path.Base(path.Dir(f.Name)) != "x86_64" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		all[path.Base(f.Name)] = data
	}
	return pick(all, goodbyedpi.Pinned)
}

func get(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// untar returns the regular files of a .tar.gz that keep maps to a name.
func untar(tgz []byte, keep func(string) (string, bool)) (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	out := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		name, ok := keep(h.Name)
		if !ok || h.Typeflag != tar.TypeReg {
			continue
		}
		if out[name], err = io.ReadAll(tr); err != nil {
			return nil, err
		}
	}
}

// parseSHA256Sums reads "<hex>  <path>" or "<hex> *<path>" lines.
func parseSHA256Sums(body string) map[string]string {
	out := map[string]string{}
	for _, l := range strings.Split(body, "\n") {
		f := strings.Fields(strings.TrimSpace(l))
		if len(f) != 2 {
			continue
		}
		if _, err := hex.DecodeString(f[0]); err != nil {
			continue
		}
		out[strings.TrimPrefix(f[1], "*")] = f[0]
	}
	return out
}

// pick returns exactly the wanted files, each matching its pinned hash.
// checkListed fails when sha256sum.txt lists a pinned file under dir with
// another hash.
func checkListed(listed map[string]string, dir string, pins map[string]string) error {
	for name, want := range pins {
		if got, ok := listed[dir+name]; ok && got != want {
			return fmt.Errorf("%s: sha256sum.txt says %s, pinned %s", name, got, want)
		}
	}
	return nil
}

func pick(files map[string][]byte, want map[string]string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for name, sum := range want {
		b, ok := files[name]
		if !ok {
			return nil, fmt.Errorf("%s: missing from the release", name)
		}
		h := sha256.Sum256(b)
		if got := hex.EncodeToString(h[:]); got != sum {
			return nil, fmt.Errorf("%s: sha256 %s, pinned %s", name, got, sum)
		}
		out[name] = b
	}
	return out, nil
}
