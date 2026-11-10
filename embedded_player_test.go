package main

import (
	"io/fs"
	"strings"
	"testing"
)

// The shipping binary must contain the player and HLS runtime without a CDN.
func TestEmbeddedPlayerAssets(t *testing.T) {
	web := mustWebFS()
	for _, name := range []string{
		"assets/js/vendor/xgplayer.min.js",
		"assets/css/vendor/xgplayer.min.css",
		"assets/js/vendor/xgplayer.LICENSE.txt",
		"assets/js/vendor/hls.min.js",
	} {
		data, err := fs.ReadFile(web, name)
		if err != nil || len(data) == 0 {
			t.Errorf("embedded asset %s missing: %v", name, err)
		}
	}
	index, err := fs.ReadFile(web, "index.html")
	if err != nil || !strings.Contains(string(index), "/assets/css/vendor/xgplayer.min.css") {
		t.Errorf("embedded player stylesheet is not referenced: %v", err)
	}
}
