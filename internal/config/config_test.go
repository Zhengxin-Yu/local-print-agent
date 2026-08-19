package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultCandidatePorts(t *testing.T) {
	got := Default().CandidatePorts()
	want := []int{17653, 17654, 17655, 17656, 17657, 17658, 17659, 17660}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CandidatePorts() = %v, want %v", got, want)
	}
}

func TestDefaultReadsConfiguredSumatraPDFPath(t *testing.T) {
	want := filepath.Join(t.TempDir(), "SumatraPDF.exe")
	t.Setenv("LOCAL_PRINT_AGENT_SUMATRA_PATH", "  "+want+"  ")
	if got := Default().SumatraPDFPath; got != want {
		t.Fatalf("Default().SumatraPDFPath = %q, want %q", got, want)
	}
}
