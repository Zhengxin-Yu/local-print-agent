package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"local-print-agent/internal/jobs"
)

func TestRenderBalloonHTMLShowsRequiredTicketDetailsAndEscapesMetadata(t *testing.T) {
	job := loadRenderJob(t, "balloon.json")
	job.ID = "balloon-job-001"

	html, err := RenderBalloonHTML(job)
	if err != nil {
		t.Fatalf("RenderBalloonHTML() error = %v", err)
	}
	page := string(html)
	for _, want := range []string{"team001", "C", "A101", "red", "balloon-job-001", "2026-08-19T09:30:00&#43;08:00"} {
		if !strings.Contains(page, want) {
			t.Errorf("balloon HTML does not contain %q", want)
		}
	}
	if !strings.Contains(page, "@page { size: 80mm 120mm; margin: 4mm; }") || !strings.Contains(page, `<meta charset="utf-8">`) {
		t.Error("balloon HTML is missing the narrow-page print specification")
	}

	job.Payload = json.RawMessage(`{"team_name":"<script>alert(1)</script>","problem_id":"C","solved_at":"2026-08-19T09:30:00+08:00","contest_name":"<img src=x onerror=1>","team_id":"team001","room":"A101","balloon_color":"red"}`)
	html, err = RenderBalloonHTML(job)
	if err != nil {
		t.Fatalf("RenderBalloonHTML() with HTML input error = %v", err)
	}
	page = string(html)
	for _, forbidden := range []string{"<script>", "<img src=x"} {
		if strings.Contains(page, forbidden) {
			t.Errorf("balloon HTML contains unescaped user content %q", forbidden)
		}
	}
	if !strings.Contains(page, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Error("balloon HTML did not display escaped team name")
	}
}

func TestRenderSourceHTMLHighlightsSupportedLanguagesAndPreservesSourceAsText(t *testing.T) {
	for _, language := range []string{"cpp", "go", "python", "java"} {
		t.Run(language, func(t *testing.T) {
			job := loadRenderJob(t, "source_cpp.json")
			job.ID = "source-job-001"
			var payload jobs.SourceCodePayload
			if err := json.Unmarshal(job.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			payload.Language = language
			payload.SourceCode = "// 中文注释\n#include <iostream>\nint main() { return 0; }"
			job.Payload, _ = json.Marshal(payload)

			html, err := RenderSourceHTML(job)
			if err != nil {
				t.Fatalf("RenderSourceHTML() error = %v", err)
			}
			page := string(html)
			for _, want := range []string{"source-job-001", "中文注释", "chroma", "line"} {
				if !strings.Contains(page, want) {
					t.Errorf("source HTML does not contain %q", want)
				}
			}
			if strings.Contains(page, "<iostream>") || !strings.Contains(page, "&lt;") || !strings.Contains(page, "iostream") || !strings.Contains(page, "&gt;") {
				t.Error("source code was interpreted as HTML instead of escaped text")
			}
			assertSourcePrintContract(t, page)
		})
	}
}

func TestRenderSourceHTMLUsesLineStructureForWrappedCode(t *testing.T) {
	job := &jobs.Job{Type: jobs.JobTypeSource, Payload: json.RawMessage(`{"language":"go","source_code":"// 第一行\n// 第二行"}`)}
	html, err := RenderSourceHTML(job)
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	for _, want := range []string{
		`<span class="line"><span class="ln">1</span><span class="cl">`,
		`<span class="line"><span class="ln">2</span><span class="cl">`,
		`.chroma .line { display: flex; align-items: flex-start; }`,
		`.chroma .ln { flex: none; color: #6a737d; padding-right: 1em; user-select: none; }`,
		`.chroma .cl { flex: 1; min-width: 0; white-space: pre-wrap; overflow-wrap: anywhere; word-break: break-word; }`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("source HTML missing exact Chroma line contract %q", want)
		}
	}
}

func TestSourceRenderDeclaresMinimumChromeVersion(t *testing.T) {
	if MinimumChromeMajor != 131 {
		t.Fatalf("MinimumChromeMajor = %d, want 131", MinimumChromeMajor)
	}
	page, err := RenderSourceHTML(&jobs.Job{Type: jobs.JobTypeSource, Payload: json.RawMessage(`{"language":"go","source_code":"func main() {}"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if want := `<meta name="local-print-agent-minimum-chrome-major" content="131">`; !strings.Contains(string(page), want) {
		t.Fatalf("source HTML does not declare Chrome margin-box requirement %q", want)
	}
}

func TestSourceFixtureExercisesMultipageChineseLineNumberContract(t *testing.T) {
	job := loadRenderJob(t, "source_cpp.json")
	var payload jobs.SourceCodePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(payload.SourceCode, "\n") + 1; lines < 120 {
		t.Fatalf("source_cpp.json lines = %d, want at least 120 for pagination", lines)
	}
	if !strings.Contains(payload.SourceCode, "中文注释") {
		t.Fatal("source_cpp.json lacks the Chinese comment used by the PDF demo")
	}
}

func TestRenderSourceHTMLEscapesLongMetadataWithoutChangingHeaderHeightContract(t *testing.T) {
	longMetadata := "BEGIN-<script>alert(1)</script>-END-" + strings.Repeat("超长元数据", 1000)
	payload, err := json.Marshal(jobs.SourceCodePayload{
		Language:    "go",
		SourceCode:  "func main() {}",
		ContestName: longMetadata,
		TeamID:      longMetadata,
		TeamName:    longMetadata,
		Room:        longMetadata,
		ProblemID:   longMetadata,
	})
	if err != nil {
		t.Fatal(err)
	}

	page, err := RenderSourceHTML(&jobs.Job{ID: longMetadata, Type: jobs.JobTypeSource, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(page)
	if strings.Contains(rendered, "<script>") {
		t.Fatal("source HTML contains raw script markup from long metadata")
	}
	if got := strings.Count(rendered, "BEGIN-&lt;script&gt;alert(1)&lt;/script&gt;-END-"); got != 1 {
		t.Fatalf("escaped job ID occurrences = %d, want 1", got)
	}
	assertSourcePrintContract(t, rendered)
}

func assertSourcePrintContract(t *testing.T, page string) {
	t.Helper()
	page = strings.ReplaceAll(page, "\r\n", "\n")
	for _, want := range []string{"@page {\n  size: A4;\n}"} {
		if !strings.Contains(page, want) {
			t.Errorf("source HTML missing exact print contract %q", want)
		}
	}
	if strings.Contains(page, "page-header") || strings.Contains(page, `<header`) || strings.Contains(page, "@bottom-center") {
		t.Error("source HTML embeds pagination decorations instead of using CDP header/footer templates")
	}
	if strings.Contains(page, "size: A4;\n  margin") {
		t.Error("source @page CSS overrides the explicit CDP header/footer margins")
	}
	if strings.Contains(page, "main { padding-top:") {
		t.Error("source HTML relies on a first-page main padding instead of the repeated @page top margin")
	}
}

func TestRenderedVisibleValuesAreEscapedAndMissingMetadataUsesPlaceholder(t *testing.T) {
	balloon := &jobs.Job{ID: `<script>job</script>`, Type: jobs.JobTypeBalloon, Payload: json.RawMessage(`{"team_name":"<script>team</script>","problem_id":"<script>problem</script>","solved_at":"<script>time</script>","contest_name":"<script>contest</script>","team_id":"<script>id</script>","room":"<script>room</script>","balloon_color":"<script>color</script>"}`)}
	page, err := RenderBalloonHTML(balloon)
	if err != nil {
		t.Fatal(err)
	}
	assertNoRawScript(t, string(page))

	source := &jobs.Job{ID: `<script>job</script>`, Type: jobs.JobTypeSource, Payload: json.RawMessage(`{"language":"go","source_code":"// <script>source</script>\nfunc main() {}","contest_name":"<script>contest</script>","team_id":"<script>id</script>","team_name":"<script>team</script>","room":"<script>room</script>","problem_id":"<script>problem</script>"}`)}
	page, err = RenderSourceHTML(source)
	if err != nil {
		t.Fatal(err)
	}
	assertNoRawScript(t, string(page))

	emptySource := &jobs.Job{Type: jobs.JobTypeSource, Payload: json.RawMessage(`{"language":"go","source_code":"func main() {}"}`)}
	page, err = RenderSourceHTML(emptySource)
	if err != nil {
		t.Fatal(err)
	}
	options, err := printOptionsForJob(emptySource)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(options.headerTemplate, "未提供") != 6 {
		t.Fatalf("source metadata placeholders = %d, want 6", strings.Count(options.headerTemplate, "未提供"))
	}
	page, err = RenderBalloonHTML(&jobs.Job{Type: jobs.JobTypeBalloon, Payload: json.RawMessage(`{"team_name":"Team Atlas","problem_id":"C","solved_at":"2026-08-19T09:30:00Z"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(page), "未提供") != 4 {
		t.Fatalf("balloon metadata placeholders = %d, want 4", strings.Count(string(page), "未提供"))
	}
}

func assertNoRawScript(t *testing.T, page string) {
	t.Helper()
	if strings.Contains(page, "<script>") || !strings.Contains(page, "&lt;script&gt;") {
		t.Fatalf("rendered page did not safely escape script-like visible input: %s", page)
	}
}

func TestRenderFunctionsRejectInvalidJobsAndStrictPayloads(t *testing.T) {
	validBalloon := loadRenderJob(t, "balloon.json")
	validSource := loadRenderJob(t, "source_cpp.json")
	cases := []struct {
		name string
		run  func(*jobs.Job) ([]byte, error)
		job  *jobs.Job
	}{
		{"nil balloon", RenderBalloonHTML, nil},
		{"wrong balloon type", RenderBalloonHTML, validSource},
		{"balloon unknown field", RenderBalloonHTML, &jobs.Job{Type: jobs.JobTypeBalloon, Payload: json.RawMessage(`{"team_name":"T","problem_id":"A","solved_at":"2026-08-19T09:30:00Z","extra":true}`)}},
		{"balloon trailing value", RenderBalloonHTML, &jobs.Job{Type: jobs.JobTypeBalloon, Payload: json.RawMessage(`{"team_name":"T","problem_id":"A","solved_at":"2026-08-19T09:30:00Z"} {}`)}},
		{"nil source", RenderSourceHTML, nil},
		{"wrong source type", RenderSourceHTML, validBalloon},
		{"source unknown field", RenderSourceHTML, &jobs.Job{Type: jobs.JobTypeSource, Payload: json.RawMessage(`{"language":"go","source_code":"func main() {}","extra":true}`)}},
		{"source trailing value", RenderSourceHTML, &jobs.Job{Type: jobs.JobTypeSource, Payload: json.RawMessage(`{"language":"go","source_code":"func main() {}"} {}`)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.run(tc.job); err == nil {
				t.Fatal("render function accepted invalid job")
			}
		})
	}
}

func loadRenderJob(t *testing.T, name string) *jobs.Job {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var job jobs.Job
	if err := json.Unmarshal(contents, &job); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return &job
}
