package docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var absoluteFilePathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)(^|[\s'"` + "`" + `(])([A-Za-z]:[\\/])`),
	regexp.MustCompile(`(^|[\s'"` + "`" + `(])/(home|Users|tmp|usr/bin|opt)/`),
}

func TestSubmissionDocumentsUseRelativeFilePaths(t *testing.T) {
	root := filepath.Clean("..")
	var files []string
	files = append(files, filepath.Join(root, "README.md"))
	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range files {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Error(err)
			continue
		}
		for _, pattern := range absoluteFilePathPatterns {
			if match := pattern.Find(contents); match != nil {
				t.Errorf("%s contains absolute file path %q; submission documents must use paths relative to the repository root", filepath.ToSlash(path), match)
			}
		}
	}
}
