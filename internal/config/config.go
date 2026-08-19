package config

import (
	"os"
	"strings"
)

type PrinterMode string

const (
	PrinterModeDemo     PrinterMode = "demo"
	PrinterModePlatform PrinterMode = "platform"
)

type Config struct {
	Host           string
	FirstPort      int
	LastPort       int
	DataDir        string
	SumatraPDFPath string
	PrinterMode    PrinterMode
}

func Default() Config {
	cfg := Config{
		Host:           "127.0.0.1",
		FirstPort:      17653,
		LastPort:       17660,
		DataDir:        "data",
		SumatraPDFPath: strings.TrimSpace(os.Getenv("LOCAL_PRINT_AGENT_SUMATRA_PATH")),
		PrinterMode:    PrinterMode(strings.ToLower(strings.TrimSpace(os.Getenv("LOCAL_PRINT_AGENT_PRINTER_MODE")))),
	}
	if cfg.PrinterMode == "" {
		cfg.PrinterMode = PrinterModeDemo
	}
	return cfg
}

func (c Config) CandidatePorts() []int {
	ports := make([]int, 0, c.LastPort-c.FirstPort+1)
	for port := c.FirstPort; port <= c.LastPort; port++ {
		ports = append(ports, port)
	}
	return ports
}
