package config

import (
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
