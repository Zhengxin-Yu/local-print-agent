//go:build linux

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

type linuxRecordedCommand struct {
	name     string
	args     []string
	deadline time.Duration
}

type linuxRunnerResponse struct {
	stdout string
	stderr string
	err    error
}

type linuxRecordingRunner struct {
	commands  []linuxRecordedCommand
	responses []linuxRunnerResponse
	afterRun  func(int)
}

func (r *linuxRecordingRunner) run(ctx context.Context, name string, args ...string) (linuxCommandOutput, error) {
	remaining := time.Duration(-1)
	if deadline, ok := ctx.Deadline(); ok {
		remaining = time.Until(deadline)
	}
	r.commands = append(r.commands, linuxRecordedCommand{name: name, args: append([]string(nil), args...), deadline: remaining})
	index := len(r.commands) - 1
	response := linuxRunnerResponse{}
	if len(r.responses) > 0 {
		response = r.responses[0]
		r.responses = r.responses[1:]
	}
	if r.afterRun != nil {
		r.afterRun(index)
	}
	return linuxCommandOutput{stdout: []byte(response.stdout), stderr: []byte(response.stderr)}, response.err
}

func TestParseLinuxPrintersMarksSystemDefault(t *testing.T) {
	output := []byte("printer PDF is idle. enabled since Wed 19 Aug 2026 10:00:00 AM CST\n" +
		"printer Office disabled since Wed 19 Aug 2026 09:00:00 AM CST\n" +
		"system default destination: PDF\n")
	want := []Info{{Name: "PDF", IsDefault: true}, {Name: "Office"}}
	got, err := parseLinuxPrinters(output)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("parseLinuxPrinters() = %#v, %v; want %#v", got, err, want)
	}
}

func TestParseLinuxPrintersMapsDefaultInstanceToItsBaseQueue(t *testing.T) {
	output := []byte("printer Office is idle. enabled since today\n" +
		"system default destination: Office/draft\n")
	want := []Info{{Name: "Office", IsDefault: true}}
	got, err := parseLinuxPrinters(output)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("parseLinuxPrinters() = %#v, %v; want %#v", got, err, want)
	}
}

func TestLinuxAdapterPrintUsesEnumeratedNameAndStrictLPArguments(t *testing.T) {
	adapter, pdfPath := testLinuxAdapter(t)
	runner := &linuxRecordingRunner{responses: []linuxRunnerResponse{
		{stdout: "printer PDF is idle. enabled since today\n"},
		{stdout: "system default destination: PDF\n"},
		{stdout: "request id is PDF-42 (1 file(s))\n"},
	}}
	adapter.run = runner.run

	if err := adapter.Print(context.Background(), "PDF", pdfPath); err != nil {
		t.Fatalf("Print() error = %v (cause: %v)", err, errors.Unwrap(err))
	}
	if len(runner.commands) != 3 {
		t.Fatalf("commands = %#v, want printer list, default query, then one print command", runner.commands)
	}
	if runner.commands[0].name != "/controlled/lpstat" || !reflect.DeepEqual(runner.commands[0].args, []string{"-p"}) {
		t.Fatalf("enumeration command = %#v", runner.commands[0])
	}
	if runner.commands[1].name != "/controlled/lpstat" || !reflect.DeepEqual(runner.commands[1].args, []string{"-d"}) {
		t.Fatalf("default query = %#v", runner.commands[1])
	}
	if runner.commands[2].name != "/controlled/lp" || !reflect.DeepEqual(runner.commands[2].args, []string{"-d", "PDF", pdfPath}) {
		t.Fatalf("print command = %#v", runner.commands[2])
	}
	for _, command := range runner.commands {
		if command.deadline < 29*time.Second || command.deadline > 30*time.Second {
			t.Fatalf("command deadline remaining = %s, want 30 second timeout", command.deadline)
		}
	}
}

func TestRunLinuxCommandCapturesStdoutAndStderrSeparately(t *testing.T) {
	if os.Getenv("LOCAL_PRINT_AGENT_LINUX_COMMAND_HELPER") == "1" {
		_, _ = fmt.Fprint(os.Stdout, "controlled stdout")
		_, _ = fmt.Fprint(os.Stderr, "controlled stderr")
		os.Exit(23)
	}
	t.Setenv("LOCAL_PRINT_AGENT_LINUX_COMMAND_HELPER", "1")
	output, err := runLinuxCommand(context.Background(), os.Args[0], "-test.run=TestRunLinuxCommandCapturesStdoutAndStderrSeparately")
	if err == nil {
		t.Fatal("controlled helper exit error = nil, want exit status")
	}
	if string(output.stdout) != "controlled stdout" || string(output.stderr) != "controlled stderr" {
		t.Fatalf("command output = stdout %q stderr %q", output.stdout, output.stderr)
	}
}

func TestNewLinuxAdapterFindsBothCUPSCommands(t *testing.T) {
	var lookedUp []string
	adapter, err := newLinuxAdapter(PlatformConfig{DataDir: t.TempDir()}, func(name string) (string, error) {
		lookedUp = append(lookedUp, name)
		return "/resolved/" + name, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lookedUp, []string{"lp", "lpstat"}) {
		t.Fatalf("LookPath calls = %#v", lookedUp)
	}
	if adapter.lpPath != "/resolved/lp" || adapter.lpstatPath != "/resolved/lpstat" {
		t.Fatalf("resolved commands = lp %q lpstat %q", adapter.lpPath, adapter.lpstatPath)
	}
}

func TestNewLinuxAdapterReportsMissingCUPSClientWithoutDiagnostics(t *testing.T) {
	for _, missing := range []string{"lp", "lpstat"} {
		t.Run(missing, func(t *testing.T) {
			secret := "/private/tools/" + missing
			_, err := newLinuxAdapter(PlatformConfig{DataDir: t.TempDir()}, func(name string) (string, error) {
				if name == missing {
					return "", errors.New("missing " + secret)
				}
				return "/resolved/" + name, nil
			})
			assertLinuxJobErrorCode(t, err, jobs.ErrorCodePrintFailed)
			if !strings.Contains(err.Error(), "install the CUPS client") || strings.Contains(err.Error(), secret) {
				t.Fatalf("public error = %q", err)
			}
		})
	}
}

func TestLinuxAdapterRejectsUnknownPrinterBeforeLP(t *testing.T) {
	adapter, pdfPath := testLinuxAdapter(t)
	runner := &linuxRecordingRunner{responses: []linuxRunnerResponse{
		{stdout: "printer PDF is idle. enabled since today\n"},
		{stdout: "system default destination: PDF\n"},
	}}
	adapter.run = runner.run

	err := adapter.Print(context.Background(), "-o raw; rm -rf private", pdfPath)
	assertLinuxJobErrorCode(t, err, jobs.ErrorCodePrinterNotFound)
	if len(runner.commands) != 2 || runner.commands[0].name != adapter.lpstatPath || runner.commands[1].name != adapter.lpstatPath {
		t.Fatalf("commands = %#v, want lpstat queries only", runner.commands)
	}
}

func TestLinuxAdapterReportsNoEnumeratedPrinters(t *testing.T) {
	adapter, _ := testLinuxAdapter(t)
	for _, output := range []string{"", "system default destination: PDF\n", "no system default destination\n"} {
		runner := &linuxRecordingRunner{responses: []linuxRunnerResponse{{stdout: output}}}
		adapter.run = runner.run
		_, err := adapter.List(context.Background())
		assertLinuxJobErrorCode(t, err, jobs.ErrorCodePrinterNotFound)
	}
}

func TestLinuxAdapterListsPrintersWhenSystemDefaultIsUnset(t *testing.T) {
	adapter, _ := testLinuxAdapter(t)
	runner := &linuxRecordingRunner{responses: []linuxRunnerResponse{
		{stdout: "printer PDF is idle. enabled since today\n"},
		{stderr: "lpstat: no system default destination\n", err: errors.New("exit status 1")},
	}}
	adapter.run = runner.run

	got, err := adapter.List(context.Background())
	want := []Info{{Name: "PDF"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %#v, %v; want %#v without a default", got, err, want)
	}
}

func TestLinuxAdapterMapsCUPSNoDestinationsDiagnosticToPrinterNotFound(t *testing.T) {
	adapter, _ := testLinuxAdapter(t)
	runner := &linuxRecordingRunner{responses: []linuxRunnerResponse{{
		stderr: "lpstat: No destinations added.\n",
		err:    errors.New("exit status 1"),
	}}}
	adapter.run = runner.run

	_, err := adapter.List(context.Background())
	assertLinuxJobErrorCode(t, err, jobs.ErrorCodePrinterNotFound)
}

func TestLinuxAdapterContextCancellationTakesPriorityOverCUPSDiagnostics(t *testing.T) {
	tests := []struct {
		name      string
		responses []linuxRunnerResponse
		cancelAt  int
	}{
		{
			name: "printer list",
			responses: []linuxRunnerResponse{{
				stderr: "lpstat: No destinations added.\n",
				err:    context.Canceled,
			}},
			cancelAt: 0,
		},
		{
			name: "default query",
			responses: []linuxRunnerResponse{
				{stdout: "printer PDF is idle. enabled since today\n"},
				{stderr: "lpstat: no system default destination\n", err: context.Canceled},
			},
			cancelAt: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter, _ := testLinuxAdapter(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runner := &linuxRecordingRunner{responses: test.responses, afterRun: func(index int) {
				if index == test.cancelAt {
					cancel()
				}
			}}
			adapter.run = runner.run

			_, err := adapter.List(ctx)
			assertLinuxJobErrorCode(t, err, jobs.ErrorCodeContextCanceled)
		})
	}
}

func TestLinuxAdapterRejectsAnythingExceptGeneratedJobPreview(t *testing.T) {
	adapter, _ := testLinuxAdapter(t)
	wrongName := filepath.Join(adapter.dataDir, "jobs", "0123456789abcdef0123456789abcdef", "render.pdf")
	if err := os.WriteFile(wrongName, []byte("%PDF-1.4\n%%EOF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{filepath.Join(t.TempDir(), "private", "secret.pdf"), wrongName} {
		runner := &linuxRecordingRunner{}
		adapter.run = runner.run
		err := adapter.Print(context.Background(), "PDF", sensitive)
		assertLinuxJobErrorCode(t, err, jobs.ErrorCodePrintFailed)
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("public error leaked PDF path: %q", err)
		}
		if len(runner.commands) != 0 {
			t.Fatalf("unsafe PDF started external commands: %#v", runner.commands)
		}
	}
}

func TestLinuxAdapterRejectsSymlinkedPreviewBeforeCommands(t *testing.T) {
	adapter, pdfPath := testLinuxAdapter(t)
	realPDF := filepath.Join(t.TempDir(), "private.pdf")
	if err := os.WriteFile(realPDF, []byte("%PDF-1.4\n%%EOF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(pdfPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realPDF, pdfPath); err != nil {
		t.Fatal(err)
	}
	runner := &linuxRecordingRunner{}
	adapter.run = runner.run

	err := adapter.Print(context.Background(), "PDF", pdfPath)
	assertLinuxJobErrorCode(t, err, jobs.ErrorCodePrintFailed)
	if strings.Contains(err.Error(), pdfPath) || strings.Contains(err.Error(), realPDF) {
		t.Fatalf("public error leaked symlink paths: %q", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("symlinked PDF started commands: %#v", runner.commands)
	}
}

func TestLinuxAdapterRevalidatesPreviewAfterEnumeration(t *testing.T) {
	adapter, pdfPath := testLinuxAdapter(t)
	var removeErr error
	runner := &linuxRecordingRunner{
		responses: []linuxRunnerResponse{
			{stdout: "printer PDF is idle. enabled since today\n"},
			{stdout: "system default destination: PDF\n"},
		},
		afterRun: func(index int) {
			if index == 0 {
				removeErr = os.Remove(pdfPath)
			}
		},
	}
	adapter.run = runner.run

	err := adapter.Print(context.Background(), "PDF", pdfPath)
	if removeErr != nil {
		t.Fatal(removeErr)
	}
	assertLinuxJobErrorCode(t, err, jobs.ErrorCodePrintFailed)
	if len(runner.commands) != 2 {
		t.Fatalf("commands = %#v, want lpstat queries only after preview changed", runner.commands)
	}
}

func TestLinuxAdapterDoesNotLeakCommandDiagnostics(t *testing.T) {
	adapter, pdfPath := testLinuxAdapter(t)
	diagnostic := "cannot open " + pdfPath + " with " + adapter.lpPath
	runner := &linuxRecordingRunner{responses: []linuxRunnerResponse{
		{stdout: "printer PDF is idle. enabled since today\n"},
		{stdout: "system default destination: PDF\n"},
		{stdout: diagnostic, stderr: diagnostic, err: errors.New(diagnostic)},
	}}
	adapter.run = runner.run

	err := adapter.Print(context.Background(), "PDF", pdfPath)
	assertLinuxJobErrorCode(t, err, jobs.ErrorCodePrintFailed)
	if strings.Contains(err.Error(), pdfPath) || strings.Contains(err.Error(), adapter.lpPath) || strings.Contains(err.Error(), diagnostic) {
		t.Fatalf("public error leaked command diagnostics: %q", err)
	}
}

func testLinuxAdapter(t *testing.T) (*linuxAdapter, string) {
	t.Helper()
	root := t.TempDir()
	jobDir := filepath.Join(root, "jobs", "0123456789abcdef0123456789abcdef")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join(jobDir, "preview.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n%%EOF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter, err := newLinuxAdapter(PlatformConfig{DataDir: root}, func(name string) (string, error) {
		return "/controlled/" + name, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter, pdfPath
}

func assertLinuxJobErrorCode(t *testing.T, err error, want jobs.ErrorCode) {
	t.Helper()
	var jobError *jobs.JobError
	if !errors.As(err, &jobError) || jobError.Code != want {
		t.Fatalf("error = %T %v, want code %s", err, err, want)
	}
}
