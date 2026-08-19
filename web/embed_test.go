package web

import (
	"io/fs"
	"testing"
)

func TestAssetsContainTheOnlyPublishedWebFiles(t *testing.T) {
	for _, name := range []string{"index.html", "app.js", "styles.css"} {
		contents, err := fs.ReadFile(Assets, name)
		if err != nil || len(contents) == 0 {
			t.Fatalf("%s is not an embedded non-empty asset: %v", name, err)
		}
	}
}
