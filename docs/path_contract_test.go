package docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var absoluteFilePathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)(^|[\s'"` + "`" + `(])([A-Za-z]:[\\/])`),
	regexp.MustCompile(`(?m)(^|[\s'"` + "`" + `(])(?:\\\\|//)[^\\/\s]+[\\/]`),
	regexp.MustCompile(`(?m)(^|[\s'"` + "`" + `(])~[\\/]`),
	regexp.MustCompile(`(?m)(^|[\s'"` + "`" + `(])/(bin|boot|dev|etc|home|lib|lib64|media|mnt|opt|proc|root|run|sbin|srv|sys|tmp|usr|var|Users|Applications|Library|System)/`),
}

var repositoryEscapingPathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)(^|[\s'"` + "`" + `(])(?:[A-Za-z0-9._-]+[\\/])*(?:\.\.[\\/])+[^\s'"` + "`" + `)<>\]}]+`),
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

func TestRepositoryEscapingPathPatterns(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "parent slash path", value: "open ../outside/file.pdf", want: true},
		{name: "nested parent slash path", value: "fixture `../../testdata/source.json`", want: true},
		{name: "parent backslash path", value: `open ..\outside\file.pdf`, want: true},
		{name: "nested parent backslash path", value: `fixture ..\..\testdata\source.json`, want: true},
		{name: "prefixed slash escape", value: "open docs/../../outside/file.pdf", want: true},
		{name: "prefixed backslash escape", value: `open docs\..\..\outside\file.pdf`, want: true},
		{name: "HTTP URL traversal segment", value: "https://example.test/a/../reference", want: false},
		{name: "API route", value: "/api/v1/print-jobs/../reference", want: false},
		{name: "ordinary prose punctuation", value: "版本 1.2.3... next sentence", want: false},
		{name: "repository root relative", value: "testdata/source.json docs/reports/day-05.md", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := false
			for _, pattern := range repositoryEscapingPathPatterns {
				if pattern.MatchString(test.value) {
					got = true
					break
				}
			}
			if got != test.want {
				t.Fatalf("repository escape match for %q = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

func TestSubmissionDocumentsUseRelativeFilePaths(t *testing.T) {
	root := filepath.Clean("..")
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && path != root && excludedMarkdownDirectory(entry.Name()) {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)

	for _, path := range files {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Error(err)
			continue
		}
		patterns := append(append([]*regexp.Regexp(nil), absoluteFilePathPatterns...), repositoryEscapingPathPatterns...)
		for _, pattern := range patterns {
			if match := pattern.Find(contents); match != nil {
				relative, relErr := filepath.Rel(root, path)
				if relErr != nil {
					relative = path
				}
				t.Errorf("%s contains prohibited file path %q; submission documents must use paths relative to the repository root without parent escapes", filepath.ToSlash(relative), match)
			}
		}
	}
}

func excludedMarkdownDirectory(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case ".git", ".cache", ".worktrees", "data", "node_modules", "vendor":
		return true
	default:
		return strings.HasPrefix(lower, ".tmp-")
	}
}

func TestExcludedMarkdownDirectorySkipsGitWorktrees(t *testing.T) {
	if !excludedMarkdownDirectory(".worktrees") {
		t.Fatal(".worktrees must be excluded from submission document scans")
	}
}

func TestFinalReportHasNoLocalMarkdownDestinations(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "relative link", value: `[Day 1](day-01.md)`, want: true},
		{name: "relative image", value: `![evidence](assets/evidence.png)`, want: true},
		{name: "relative reference link", value: "[Day 1][day-1]\n\n[day-1]: docs/reports/day-01.md", want: true},
		{name: "HTTP link", value: `[reference](http://example.test/reference)`, want: false},
		{name: "HTTPS link", value: `[reference](https://example.test/reference)`, want: false},
		{name: "HTTPS reference link", value: "[reference][docs]\n\n[docs]: https://example.test/reference", want: false},
		{name: "anchor link", value: `[section](#section)`, want: false},
		{name: "mail link", value: `[contact](mailto:owner@example.test)`, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := len(localMarkdownDestinations([]byte(test.value))) > 0
			if got != test.want {
				t.Fatalf("local Markdown destination for %q = %v, want %v", test.value, got, test.want)
			}
		})
	}

	contents, err := os.ReadFile(filepath.Join("reports", "day-09-final.md"))
	if err != nil {
		t.Fatal(err)
	}
	if destinations := localMarkdownDestinations(contents); len(destinations) > 0 {
		t.Fatalf("final report contains local Markdown destinations %q; write repository-root-relative evidence paths as inline code", destinations)
	}
}

func localMarkdownDestinations(contents []byte) []string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`!?\[[^\]\r\n]*\]\(\s*([^)]+?)\s*\)`),
		regexp.MustCompile(`(?m)^[ \t]{0,3}\[[^\]\r\n]+\]:[ \t]*(.+?)[ \t]*$`),
	}
	var destinations []string
	for _, pattern := range patterns {
		for _, match := range pattern.FindAllSubmatch(contents, -1) {
			destination := strings.TrimSpace(string(match[1]))
			if strings.HasPrefix(destination, "<") {
				if end := strings.Index(destination, ">"); end >= 0 {
					destination = destination[1:end]
				}
			} else if fields := strings.Fields(destination); len(fields) > 0 {
				destination = fields[0]
			}
			lower := strings.ToLower(destination)
			if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(destination, "#") {
				continue
			}
			destinations = append(destinations, destination)
		}
	}
	return destinations
}
