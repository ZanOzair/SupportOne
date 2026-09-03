package localui

import (
	"io/fs"
	"testing/fstest"
)

// testAssets stands in for the built interface, so the server's behaviour can
// be tested without a Node build.
func testAssets() fs.FS {
	return fstest.MapFS{
		"dist/index.html":            {Data: []byte("<!doctype html><title>SupportOne</title>")},
		"dist/assets/index-test.js":  {Data: []byte("// built interface")},
		"dist/assets/index-test.css": {Data: []byte("body{}")},
	}
}
