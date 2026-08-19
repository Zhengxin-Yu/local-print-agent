//go:build windows

package printer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"local-print-agent/internal/jobs"
)

type recordedCommand struct {
	name     string
	args     []string
	deadline time.Duration
}

type runnerResponse struct {
	stdout string
	stderr string
	err    error
}

type recordingCommandRunner struct {
	commands  []recordedCommand
	responses []runnerResponse
	afterRun  func(int)
}

func (r *recordingCommandRunner) run(ctx context.Context, name string, args ...string) (commandOutput, error) {
	remaining := time.Duration(-1)
	if deadline, ok := ctx.Deadline(); ok {
		remaining = time.Until(deadline)
	}
	r.commands = append(r.commands, recordedCommand{name: name, args: append([]string(nil), args...), deadline: remaining})
	commandIndex := len(r.commands) - 1
	response := runnerResponse{}
	if len(r.responses) > 0 {
		response = r.responses[0]
		r.responses = r.responses[1:]
	}
	if r.afterRun != nil {
		r.afterRun(commandIndex)
	}
	return commandOutput{stdout: []byte(response.stdout), stderr: []byte(response.stderr)}, response.err
}

func TestWindowsAdapterPrintUsesEnumeratedNameAndStrictSumatraArguments(t *testing.T) {
	adapter, pdfPath, sumatraPath := testWindowsAdapter(t)
	runner := &recordingCommandRunner{responses: []runnerResponse{
		{stdout: `[{"Name":"Microsoft Print to PDF","Default":true}]`},
		{},
	}}
	adapter.run = runner.run

	if err := adapter.Print(context.Background(), "Microsoft Print to PDF", pdfPath); err != nil {
		t.Fatalf("Print() error = %v (cause: %v)", err, errors.Unwrap(err))
	}
	if len(runner.commands) != 2 {
		t.Fatalf("commands = %#v, want enumeration then one print command", runner.commands)
	}
	if runner.commands[0].name != "powershell.exe" || !strings.Contains(strings.Join(runner.commands[0].args, " "), "Get-CimInstance Win32_Printer | Select-Object Name,Default | ConvertTo-Json") {
		t.Fatalf("enumeration command = %#v, want fixed Win32_Printer JSON pipeline", runner.commands[0])
	}
	wantArgs := []string{"-print-to", "Microsoft Print to PDF", "-silent", pdfPath}
	if runner.commands[1].name != sumatraPath || !reflect.DeepEqual(runner.commands[1].args, wantArgs) {
		t.Fatalf("print command = %#v, want %q %#v", runner.commands[1], sumatraPath, wantArgs)
	}
	for _, command := range runner.commands {
		if command.deadline < 29*time.Second || command.deadline > 30*time.Second {
			t.Fatalf("command deadline remaining = %s, want 30 second timeout", command.deadline)
		}
	}
}

func TestRunWindowsCommandCapturesStdoutAndStderrSeparately(t *testing.T) {
	if os.Getenv("LOCAL_PRINT_AGENT_TEST_COMMAND_HELPER") == "1" {
		_, _ = fmt.Fprint(os.Stdout, "controlled stdout")
		_, _ = fmt.Fprint(os.Stderr, "controlled stderr")
		os.Exit(23)
	}
	t.Setenv("LOCAL_PRINT_AGENT_TEST_COMMAND_HELPER", "1")
	output, err := runWindowsCommand(context.Background(), os.Args[0], "-test.run=TestRunWindowsCommandCapturesStdoutAndStderrSeparately")
	if err == nil {
		t.Fatal("controlled helper exit error = nil, want exit status")
	}
	if string(output.stdout) != "controlled stdout" || string(output.stderr) != "controlled stderr" {
		t.Fatalf("command output = stdout %q stderr %q", output.stdout, output.stderr)
	}
}

func TestWindowsAdapterListsPowerShellJSONShapes(t *testing.T) {
	adapter, _, _ := testWindowsAdapter(t)
	tests := []struct {
		name   string
		output string
		want   []Info
	}{
		{
			name:   "array",
			output: `[{"Name":"Office","Default":false},{"Name":"PDF 中文","Default":true}]`,
			want:   []Info{{Name: "Office"}, {Name: "PDF 中文", IsDefault: true}},
		},
		{
			name:   "single object",
			output: `{"Name":"Only Printer","Default":true}`,
			want:   []Info{{Name: "Only Printer", IsDefault: true}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &recordingCommandRunner{responses: []runnerResponse{{stdout: test.output}}}
			adapter.run = runner.run
			got, err := adapter.List(context.Background())
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("List() = %#v, %v; want %#v", got, err, test.want)
			}
		})
	}
}

func TestWindowsAdapterReportsNoEnumeratedPrinters(t *testing.T) {
	adapter, _, _ := testWindowsAdapter(t)
	for _, output := range []string{"[]", "null", "  "} {
		runner := &recordingCommandRunner{responses: []runnerResponse{{stdout: output}}}
		adapter.run = runner.run
		_, err := adapter.List(context.Background())
		assertJobErrorCode(t, err, jobs.ErrorCodePrinterNotFound)
	}
}

func TestWindowsAdapterRejectsUnknownPrinterBeforePrintCommand(t *testing.T) {
	adapter, pdfPath, _ := testWindowsAdapter(t)
	runner := &recordingCommandRunner{responses: []runnerResponse{{stdout: `{"Name":"Office","Default":true}`}}}
	adapter.run = runner.run

	err := adapter.Print(context.Background(), "Attacker; Remove-Item C:\\", pdfPath)
	assertJobErrorCode(t, err, jobs.ErrorCodePrinterNotFound)
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %#v, want enumeration only", runner.commands)
	}
}

func TestWindowsAdapterRejectsAnythingExceptGeneratedJobPreview(t *testing.T) {
	adapter, _, _ := testWindowsAdapter(t)
	inJobWrongName := filepath.Join(adapter.dataDir, "jobs", "0123456789abcdef0123456789abcdef", "render.pdf")
	if err := os.WriteFile(inJobWrongName, []byte("%PDF-1.4\n%%EOF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{filepath.Join(t.TempDir(), "private", "secret.pdf"), inJobWrongName} {
		runner := &recordingCommandRunner{}
		adapter.run = runner.run
		err := adapter.Print(context.Background(), "Office", sensitive)
		assertJobErrorCode(t, err, jobs.ErrorCodePrintFailed)
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("public error leaked PDF path: %q", err)
		}
		if len(runner.commands) != 0 {
			t.Fatalf("unsafe PDF started external commands: %#v", runner.commands)
		}
	}
}

func TestWindowsAdapterDoesNotLeakCommandDiagnostics(t *testing.T) {
	adapter, pdfPath, sumatraPath := testWindowsAdapter(t)
	diagnostic := "cannot open " + pdfPath + " with " + sumatraPath
	runner := &recordingCommandRunner{responses: []runnerResponse{
		{stdout: `{"Name":"Office","Default":true}`},
		{stderr: diagnostic, err: errors.New(diagnostic)},
	}}
	adapter.run = runner.run

	err := adapter.Print(context.Background(), "Office", pdfPath)
	assertJobErrorCode(t, err, jobs.ErrorCodePrintFailed)
	if strings.Contains(err.Error(), pdfPath) || strings.Contains(err.Error(), sumatraPath) || strings.Contains(err.Error(), diagnostic) {
		t.Fatalf("public error leaked command diagnostics: %q", err)
	}
}

func TestWindowsAdapterRevalidatesPreviewAfterEnumeration(t *testing.T) {
	adapter, pdfPath, _ := testWindowsAdapter(t)
	var removeErr error
	runner := &recordingCommandRunner{
		responses: []runnerResponse{{stdout: `{"Name":"Office","Default":true}`}},
		afterRun: func(index int) {
			if index == 0 {
				removeErr = os.Remove(pdfPath)
			}
		},
	}
	adapter.run = runner.run

	err := adapter.Print(context.Background(), "Office", pdfPath)
	if removeErr != nil {
		t.Fatal(removeErr)
	}
	assertJobErrorCode(t, err, jobs.ErrorCodePrintFailed)
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %#v, want enumeration only after preview changed", runner.commands)
	}
}

func TestNewPlatformAdapterRejectsMissingSumatraWithoutLeakingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "private", "SumatraPDF.exe")
	_, err := NewPlatformAdapter(PlatformConfig{DataDir: t.TempDir(), SumatraPDFPath: missing})
	assertJobErrorCode(t, err, jobs.ErrorCodePrintFailed)
	if strings.Contains(err.Error(), missing) {
		t.Fatalf("public error leaked SumatraPDF path: %q", err)
	}
}

func testWindowsAdapter(t *testing.T) (*windowsAdapter, string, string) {
	t.Helper()
	root := t.TempDir()
	sumatraPath := filepath.Join(t.TempDir(), "SumatraPDF.exe")
	if err := os.WriteFile(sumatraPath, []byte("controlled test executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(root, "jobs", "0123456789abcdef0123456789abcdef")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join(jobDir, "preview.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n%%EOF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := NewPlatformAdapter(PlatformConfig{DataDir: root, SumatraPDFPath: sumatraPath})
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := created.(*windowsAdapter)
	if !ok {
		t.Fatalf("NewPlatformAdapter() = %T, want *windowsAdapter", created)
	}
	return adapter, pdfPath, sumatraPath
}

func assertJobErrorCode(t *testing.T, err error, want jobs.ErrorCode) {
	t.Helper()
	var jobError *jobs.JobError
	if !errors.As(err, &jobError) || jobError.Code != want {
		t.Fatalf("error = %T %v, want code %s", err, err, want)
	}
}
