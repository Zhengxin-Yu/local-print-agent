//go:build linux

package printer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"local-print-agent/internal/jobs"
)

const linuxCommandTimeout = 30 * time.Second

var linuxJobID = regexp.MustCompile(`^[0-9a-f]{32}$`)

type linuxCommandOutput struct {
	stdout []byte
	stderr []byte
}

type linuxCommandRunner func(context.Context, string, ...string) (linuxCommandOutput, error)
type linuxLookPath func(string) (string, error)

type linuxAdapter struct {
	dataDir    string
	lpPath     string
	lpstatPath string
	run        linuxCommandRunner
}

var _ Adapter = (*linuxAdapter)(nil)

// NewPlatformAdapter constructs the Linux CUPS printer boundary.
func NewPlatformAdapter(cfg PlatformConfig) (Adapter, error) {
	return newLinuxAdapter(cfg, exec.LookPath)
}

func newLinuxAdapter(cfg PlatformConfig, lookPath linuxLookPath) (*linuxAdapter, error) {
	if strings.TrimSpace(cfg.DataDir) == "" {
		return nil, printCommandError("printer data directory is unavailable", errors.New("printer data directory is required"))
	}
	dataDir, err := filepath.Abs(strings.TrimSpace(cfg.DataDir))
	if err != nil {
		return nil, printCommandError("printer data directory is unavailable", err)
	}
	if lookPath == nil {
		return nil, cupsClientUnavailable(errors.New("command lookup is unavailable"))
	}
	lpPath, err := lookPath("lp")
	if err != nil {
		return nil, cupsClientUnavailable(err)
	}
	lpstatPath, err := lookPath("lpstat")
	if err != nil {
		return nil, cupsClientUnavailable(err)
	}
	return &linuxAdapter{
		dataDir:    dataDir,
		lpPath:     lpPath,
		lpstatPath: lpstatPath,
		run:        runLinuxCommand,
	}, nil
}

func (a *linuxAdapter) List(ctx context.Context) ([]Info, error) {
	if a == nil || a.run == nil || a.lpstatPath == "" {
		return nil, printCommandError("printer enumeration is unavailable", errors.New("Linux adapter is not initialized"))
	}
	output, err := a.runWithTimeout(ctx, a.lpstatPath, "-p")
	if err != nil {
		if ctx == nil || ctx.Err() != nil {
			return nil, a.commandError(ctx, "printer enumeration failed", output, err)
		}
		if linuxCommandReports(output, "no destinations added") {
			return nil, printerNotFoundError()
		}
		return nil, a.commandError(ctx, "printer enumeration failed", output, err)
	}
	printers, err := parseLinuxPrinters(output.stdout)
	if err != nil {
		return nil, err
	}
	defaultOutput, err := a.runWithTimeout(ctx, a.lpstatPath, "-d")
	if err != nil {
		if ctx == nil || ctx.Err() != nil {
			return nil, a.commandError(ctx, "printer enumeration failed", defaultOutput, err)
		}
		if linuxCommandReports(defaultOutput, "no system default destination") {
			return printers, nil
		}
		return nil, a.commandError(ctx, "printer enumeration failed", defaultOutput, err)
	}
	defaultName := linuxBaseDestination(parseLinuxDefaultDestination(defaultOutput.stdout))
	for index := range printers {
		printers[index].IsDefault = printers[index].Name == defaultName
	}
	return printers, nil
}

func (a *linuxAdapter) Print(ctx context.Context, printerName, pdfPath string) error {
	if a == nil || a.run == nil || a.lpPath == "" {
		return printCommandError("printing is unavailable", errors.New("Linux adapter is not initialized"))
	}
	validatedPDF, err := validateLinuxPrintPDF(a.dataDir, pdfPath)
	if err != nil {
		return printCommandError("print file is invalid", err)
	}
	printers, err := a.List(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, item := range printers {
		if item.Name == printerName {
			found = true
			break
		}
	}
	if !found {
		return jobs.NewJobError(jobs.ErrorCodePrinterNotFound, "selected printer is unavailable", errors.New("printer was not returned by lpstat"))
	}
	validatedPDF, err = validateLinuxPrintPDF(a.dataDir, validatedPDF)
	if err != nil {
		return printCommandError("print file is invalid", err)
	}
	output, err := a.runWithTimeout(ctx, a.lpPath, "-d", printerName, validatedPDF)
	if err != nil {
		return a.commandError(ctx, "printing command failed", output, err)
	}
	return nil
}

func (a *linuxAdapter) runWithTimeout(ctx context.Context, name string, args ...string) (linuxCommandOutput, error) {
	if ctx == nil {
		return linuxCommandOutput{}, context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return linuxCommandOutput{}, err
	}
	commandContext, cancel := context.WithTimeout(ctx, linuxCommandTimeout)
	defer cancel()
	return a.run(commandContext, name, args...)
}

func (a *linuxAdapter) commandError(ctx context.Context, publicMessage string, output linuxCommandOutput, commandErr error) error {
	if ctx == nil {
		return jobs.NewJobError(jobs.ErrorCodeContextCanceled, "printing was canceled", context.Canceled)
	}
	if err := ctx.Err(); err != nil {
		return jobs.NewJobError(jobs.ErrorCodeContextCanceled, "printing was canceled", err)
	}
	cause := fmt.Errorf("external command failed: %w", commandErr)
	if len(output.stdout) > 0 || len(output.stderr) > 0 {
		cause = fmt.Errorf("external command failed (stdout %d bytes, stderr %d bytes): %w", len(output.stdout), len(output.stderr), commandErr)
	}
	return printCommandError(publicMessage, cause)
}

func runLinuxCommand(ctx context.Context, name string, args ...string) (linuxCommandOutput, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = append(os.Environ(), "LC_ALL=C")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return linuxCommandOutput{stdout: stdout.Bytes(), stderr: stderr.Bytes()}, err
}

func parseLinuxPrinters(output []byte) ([]Info, error) {
	var names []string
	seen := make(map[string]bool)
	defaultName := linuxBaseDestination(parseLinuxDefaultDestination(output))
	for _, rawLine := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "printer ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] == "" || seen[fields[1]] {
			continue
		}
		seen[fields[1]] = true
		names = append(names, fields[1])
	}
	if len(names) == 0 {
		return nil, printerNotFoundError()
	}
	printers := make([]Info, 0, len(names))
	for _, name := range names {
		printers = append(printers, Info{Name: name, IsDefault: name == defaultName})
	}
	return printers, nil
}

func parseLinuxDefaultDestination(output []byte) string {
	for _, rawLine := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "system default destination:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "system default destination:"))
		}
	}
	return ""
}

func linuxBaseDestination(destination string) string {
	base, _, _ := strings.Cut(destination, "/")
	return base
}

func linuxCommandReports(output linuxCommandOutput, phrase string) bool {
	diagnostic := strings.ToLower(string(output.stdout) + "\n" + string(output.stderr))
	return strings.Contains(diagnostic, strings.ToLower(phrase))
}

func validateLinuxPrintPDF(dataDir, pdfPath string) (string, error) {
	if strings.TrimSpace(dataDir) == "" || strings.TrimSpace(pdfPath) == "" {
		return "", errors.New("print data directory and PDF path are required")
	}
	dataRoot, err := filepath.Abs(dataDir)
	if err != nil {
		return "", err
	}
	absolutePDF, err := filepath.Abs(pdfPath)
	if err != nil {
		return "", err
	}
	jobsRoot := filepath.Join(dataRoot, "jobs")
	relative, err := filepath.Rel(jobsRoot, absolutePDF)
	if err != nil {
		return "", err
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) != 2 || !linuxJobID.MatchString(parts[0]) || parts[1] != "preview.pdf" {
		return "", errors.New("PDF is not a generated job preview")
	}
	if err := validateNoLinuxSymlinkComponents(absolutePDF); err != nil {
		return "", err
	}
	jobDir := filepath.Join(jobsRoot, parts[0])
	for index, path := range []string{dataRoot, jobsRoot, jobDir, absolutePDF} {
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("print path contains a symbolic link")
		}
		if index < 3 && !info.IsDir() {
			return "", errors.New("print path component is not a directory")
		}
		if index == 3 && !info.Mode().IsRegular() {
			return "", errors.New("print PDF is not a regular file")
		}
	}
	return absolutePDF, nil
}

func validateNoLinuxSymlinkComponents(path string) error {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	var components []string
	for current := absolute; ; current = filepath.Dir(current) {
		components = append(components, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	for index := len(components) - 1; index >= 0; index-- {
		info, err := os.Lstat(components[index])
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("print path contains a symbolic link")
		}
	}
	return nil
}

func cupsClientUnavailable(cause error) error {
	return printCommandError("CUPS client commands are unavailable; install the CUPS client", cause)
}

func printerNotFoundError() error {
	return jobs.NewJobError(jobs.ErrorCodePrinterNotFound, "no printers are available", errors.New("lpstat returned no usable printer"))
}

func printCommandError(message string, cause error) error {
	return jobs.NewJobError(jobs.ErrorCodePrintFailed, message, cause)
}
