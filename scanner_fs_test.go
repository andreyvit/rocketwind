package rocketwind

import (
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestScanEmbeddedFrontendAssets(t *testing.T) {
	for _, compat := range []Compatibility{Tailwind3, Tailwind4} {
		scanner := NewScanner(nil, ScannerOptions{Compatibility: compat})
		views := fstest.MapFS{
			"views/page.html":               {Data: []byte(`<div class="flex w-[179px]"></div>`)},
			"views/ignored.css":             {Data: []byte("z-[712]")},
			"node_modules/vendor/card.html": {Data: []byte("z-[713]")},
			"image.png":                     {Data: []byte("z-[714]")},
		}
		scripts := fstest.MapFS{"components/card.js": {Data: []byte(`element.classList.add('grid-cols-3')`)}}
		names, err := scanner.ScanFS(views, scripts)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"flex", "w-[179px]", "grid-cols-3"} {
			if !slices.Contains(names, name) {
				t.Fatal(name, names)
			}
		}
		for _, name := range names {
			if strings.HasPrefix(name, "z-[71") {
				t.Fatal("scanned excluded file", name)
			}
		}
		views["views/new.html"] = &fstest.MapFile{Data: []byte(`<aside class="p-9"></aside>`)}
		again, err := scanner.ScanFS(views)
		if err != nil || !slices.Contains(again, "p-9") || !slices.Contains(again, "grid-cols-3") {
			t.Fatal(again, err)
		}
	}
}

type unreadableScanFS struct{ fs.FS }

func (f unreadableScanFS) ReadFile(string) ([]byte, error) { return nil, fs.ErrPermission }

func TestScanFSReportsInputErrors(t *testing.T) {
	scanner := NewScanner(nil)
	for _, files := range []fs.FS{
		unreadableScanFS{fstest.MapFS{"page.html": {Data: []byte("flex")}}},
		failingScanRoot{},
	} {
		if _, err := scanner.ScanFS(files); !errors.Is(err, fs.ErrPermission) {
			t.Fatal(err)
		}
	}
}

type failingScanRoot struct{}

func (failingScanRoot) Open(string) (fs.File, error) { return nil, fs.ErrPermission }
