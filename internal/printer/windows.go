//go:build windows

package printer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"local-print-agent/internal/jobs"
)

const (
	windowsCommandTimeout = 30 * time.Second
	windowsPrinterQuery   = `[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Get-CimInstance Win32_Printer | Select-Object Name,Default | ConvertTo-Json -Compress`
)

var windowsJobID = regexp.MustCompile(`^[0-9a-f]{32}$`)

type commandOutput struct {
	stdout []byte
	stderr []byte
}

type commandRunner func(context.Context, string, ...string) (commandOutput, error)

type windowsAdapter struct {
	dataDir        string
	sumatraPDFPath string
	powerShellPath string
	run            commandRunner
}

var _ Adapter = (*windowsAdapter)(nil)

// NewPlatformAdapter constructs the Windows SumatraPDF printer boundary.
func NewPlatformAdapter(cfg PlatformConfig) (Adapter, error) {
	dataDir, err := filepath.Abs(strings.TrimSpace(cfg.DataDir))
	if err != nil || strings.TrimSpace(cfg.DataDir) == "" {
		return nil, printCommandError("printer data directory is unavailable", err)
	}
	sumatraPDFPath, err := validateWindowsExecutable(cfg.SumatraPDFPath)
	if err != nil {
		return nil, printCommandError("SumatraPDF is unavailable", err)
	}
	return &windowsAdapter{
		dataDir:        dataDir,
		sumatraPDFPath: sumatraPDFPath,
		powerShellPath: "powershell.exe",
		run:            runWindowsCommand,
	}, nil
}

func (a *windowsAdapter) List(ctx context.Context) ([]Info, error) {
	if a == nil || a.run == nil {
		return nil, printCommandError("printer enumeration is unavailable", errors.New("Windows adapter is not initialized"))
	}
	output, err := a.runWithTimeout(ctx, a.powerShellPath,
		"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", windowsPrinterQuery)
	if err != nil {
		return nil, a.commandError(ctx, "printer enumeration failed", output, err)
	}
	printers, err := parseWindowsPrinters(output.stdout)
	if err != nil {
		return nil, err
	}
	return printers, nil
}

func (a *windowsAdapter) Print(ctx context.Context, printerName, pdfPath string) error {
	if a == nil || a.run == nil {
		return printCommandError("printing is unavailable", errors.New("Windows adapter is not initialized"))
	}
	validatedPDF, err := validateWindowsPrintPDF(a.dataDir, pdfPath)
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
		return jobs.NewJobError(jobs.ErrorCodePrinterNotFound, "selected printer is unavailable", errors.New("printer was not returned by Win32_Printer"))
	}
	validatedPDF, err = validateWindowsPrintPDF(a.dataDir, validatedPDF)
	if err != nil {
		return printCommandError("print file is invalid", err)
	}
	output, err := a.runWithTimeout(ctx, a.sumatraPDFPath,
		"-print-to", printerName, "-silent", validatedPDF)
	if err != nil {
		return a.commandError(ctx, "printing command failed", output, err)
	}
	return nil
}

func (a *windowsAdapter) runWithTimeout(ctx context.Context, name string, args ...string) (commandOutput, error) {
	if ctx == nil {
		return commandOutput{}, context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return commandOutput{}, err
	}
	commandContext, cancel := context.WithTimeout(ctx, windowsCommandTimeout)
	defer cancel()
	return a.run(commandContext, name, args...)
}

func (a *windowsAdapter) commandError(ctx context.Context, publicMessage string, output commandOutput, commandErr error) error {
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

func runWindowsCommand(ctx context.Context, name string, args ...string) (commandOutput, error) {
	command := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return commandOutput{stdout: stdout.Bytes(), stderr: stderr.Bytes()}, err
}

func parseWindowsPrinters(output []byte) ([]Info, error) {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, printerNotFoundError()
	}
	type windowsPrinter struct {
		Name    string `json:"Name"`
		Default bool   `json:"Default"`
	}
	var decoded []windowsPrinter
	switch trimmed[0] {
	case '[':
		if err := json.Unmarshal(trimmed, &decoded); err != nil {
			return nil, printCommandError("printer enumeration failed", err)
		}
	case '{':
		var single windowsPrinter
		if err := json.Unmarshal(trimmed, &single); err != nil {
			return nil, printCommandError("printer enumeration failed", err)
		}
		decoded = []windowsPrinter{single}
	default:
		return nil, printCommandError("printer enumeration failed", errors.New("PowerShell returned non-JSON output"))
	}
	printers := make([]Info, 0, len(decoded))
	for _, item := range decoded {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		printers = append(printers, Info{Name: item.Name, IsDefault: item.Default})
	}
	if len(printers) == 0 {
		return nil, printerNotFoundError()
	}
	return printers, nil
}

func validateWindowsExecutable(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("SumatraPDF path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", err
	}
	unsafe, err := windowsPathUnsafe(absolute, info)
	if err != nil {
		return "", err
	}
	if unsafe || !info.Mode().IsRegular() {
		return "", errors.New("SumatraPDF path is not a regular file")
	}
	return absolute, nil
}

func validateWindowsPrintPDF(dataDir, pdfPath string) (string, error) {
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
	if len(parts) != 2 || !windowsJobID.MatchString(parts[0]) || parts[1] != "preview.pdf" {
		return "", errors.New("PDF is not a generated job preview")
	}
	jobDir := filepath.Join(jobsRoot, parts[0])
	if err := validateNoWindowsReparseComponents(absolutePDF); err != nil {
		return "", err
	}
	for index, path := range []string{dataRoot, jobsRoot, jobDir, absolutePDF} {
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		unsafe, err := windowsPathUnsafe(path, info)
		if err != nil {
			return "", err
		}
		if unsafe {
			return "", errors.New("print path contains a link or reparse point")
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

func validateNoWindowsReparseComponents(path string) error {
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
		component := components[index]
		info, err := os.Lstat(component)
		if err != nil {
			return err
		}
		unsafe, err := windowsPathUnsafe(component, info)
		if err != nil {
			return err
		}
		if unsafe {
			return errors.New("print path contains a link or reparse point")
		}
	}
	return nil
}

func windowsPathUnsafe(_ string, info os.FileInfo) (bool, error) {
	if info.Mode()&os.ModeSymlink != 0 {
		return true, nil
	}
	attributes, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false, errors.New("Windows file attributes are unavailable")
	}
	return attributes.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}

func printerNotFoundError() error {
	return jobs.NewJobError(jobs.ErrorCodePrinterNotFound, "no printers are available", errors.New("Win32_Printer returned no usable printer"))
}

func printCommandError(message string, cause error) error {
	return jobs.NewJobError(jobs.ErrorCodePrintFailed, message, cause)
}
