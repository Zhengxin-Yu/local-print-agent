package web

import (
	"io/fs"
	"strings"
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

func TestEmbeddedConsoleWarningDistinguishesDemoAndPlatformPrinting(t *testing.T) {
	contents, err := fs.ReadFile(Assets, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(contents)
	for _, required := range []string{"演示模式", "Mock Printer", "platform 模式", "操作系统队列", "确认安全"} {
		if !strings.Contains(page, required) {
			t.Fatalf("embedded console warning lacks %q", required)
		}
	}
	for _, misleading := range []string{"不连接 OJ 或真实打印机", "仅使用演示用 Fake Printer，不会产生物理打印"} {
		if strings.Contains(page, misleading) {
			t.Fatalf("embedded console contains unconditional no-print claim %q", misleading)
		}
	}
}
