// Command sizecheck fails a release whose file inside a zip grew more than
// a set percentage over the previous release's (spec Linux §17: Linux work
// must not bloat the Windows exe).
//
//	go run ./tools/sizecheck -old prev.zip -new cur.zip -entry ghostline.exe -max 2
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"os"
)

func main() {
	oldZip := flag.String("old", "", "previous release zip")
	newZip := flag.String("new", "", "this release zip")
	entry := flag.String("entry", "ghostline.exe", "file inside both zips")
	maxPct := flag.Float64("max", 2, "largest allowed growth, in percent")
	flag.Parse()

	oldSize, err := EntrySize(*oldZip, *entry)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sizecheck:", err)
		os.Exit(2)
	}
	newSize, err := EntrySize(*newZip, *entry)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sizecheck:", err)
		os.Exit(2)
	}
	pct, ok := Grew(oldSize, newSize, *maxPct)
	fmt.Printf("%s: %d → %d bytes (%+.2f%%)\n", *entry, oldSize, newSize, pct)
	if !ok {
		fmt.Printf("sizecheck: grew more than %.2f%%\n", *maxPct)
		os.Exit(1)
	}
}

// Grew reports the growth from oldSize to newSize in percent, and whether
// it stays within maxPct. Shrinking is always fine.
func Grew(oldSize, newSize int64, maxPct float64) (pct float64, ok bool) {
	pct = float64(newSize-oldSize) / float64(oldSize) * 100
	return pct, pct <= maxPct
}

// EntrySize is the uncompressed size of name inside the zip at path.
func EntrySize(path, name string) (int64, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return 0, err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name == name {
			return int64(f.UncompressedSize64), nil
		}
	}
	return 0, fmt.Errorf("%s: no %s inside", path, name)
}
