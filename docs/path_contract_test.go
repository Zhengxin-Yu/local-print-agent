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
	regexp.MustCompile(`(?m)(^|[\s'"` + "`" + `(])(?:\\\\|//)[^\\/\s]+[\\/]`),
	regexp.MustCompile(`(?m)(^|[\s'"` + "`" + `(])~[\\/]`),
	regexp.MustCompile(`(?m)(^|[\s'"` + "`" + `(])/(bin|boot|dev|etc|home|lib|lib64|media|mnt|opt|proc|root|run|sbin|srv|sys|tmp|usr|var|Users|Applications|Library|System)/`),
}

func TestAbsoluteFilePathPatterns(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "windows drive backslash", value: `open C:\tools\chrome.exe`, want: true},
		{name: "windows drive slash", value: `open C:/tools/chrome.exe`, want: true},
		{name: "windows UNC", value: `open \\server\share\file.pdf`, want: true},
		{name: "slash UNC", value: `open //server/share/file.pdf`, want: true},
		{name: "home shorthand", value: `open ~/tools/chrome`, want: true},
		{name: "home shorthand backslash", value: `open ~\tools\chrome.exe`, want: true},
		{name: "POSIX bin", value: `open /bin/tool`, want: true},
		{name: "POSIX boot", value: `open /boot/file`, want: true},
		{name: "POSIX dev", value: `open /dev/null`, want: true},
		{name: "POSIX etc", value: `open /etc/app`, want: true},
		{name: "POSIX home", value: `open /home/user`, want: true},
		{name: "POSIX lib", value: `open /lib/a`, want: true},
		{name: "POSIX lib64", value: `open /lib64/a`, want: true},
		{name: "POSIX media", value: `open /media/a`, want: true},
		{name: "POSIX mnt", value: `open /mnt/a`, want: true},
		{name: "POSIX opt", value: `open /opt/a`, want: true},
		{name: "POSIX proc", value: `open /proc/a`, want: true},
		{name: "POSIX root", value: `open /root/a`, want: true},
		{name: "POSIX run", value: `open /run/a`, want: true},
		{name: "POSIX sbin", value: `open /sbin/a`, want: true},
		{name: "POSIX srv", value: `open /srv/a`, want: true},
		{name: "POSIX sys", value: `open /sys/a`, want: true},
		{name: "POSIX tmp", value: `open /tmp/a`, want: true},
		{name: "POSIX usr", value: `open /usr/a`, want: true},
		{name: "POSIX var", value: `open /var/a`, want: true},
		{name: "macOS Users", value: `open /Users/a`, want: true},
		{name: "macOS Applications", value: `open /Applications/a`, want: true},
		{name: "macOS Library", value: `open /Library/a`, want: true},
		{name: "macOS System", value: `open /System/a`, want: true},
		{name: "windows relative", value: `.\scripts\run-windows.ps1 .cache\go-test tools\chrome\chrome.exe`, want: false},
		{name: "POSIX relative", value: `./scripts/run-linux.sh .cache/go-test tools/chrome/chrome data/jobs/id/preview.pdf`, want: false},
		{name: "HTTP URLs", value: `http://127.0.0.1:17653/health https://example.com/usr/guide`, want: false},
		{name: "reference URL", value: `https://github.com/example/project`, want: false},
		{name: "API routes", value: `/health /api/v1/print-jobs/{jobID}/preview`, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := false
			for _, pattern := range absoluteFilePathPatterns {
				if pattern.MatchString(test.value) {
					got = true
					break
				}
			}
			if got != test.want {
				t.Fatalf("absolute path match for %q = %v, want %v", test.value, got, test.want)
			}
		})
	}
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
