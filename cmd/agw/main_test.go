package main

import (
	"reflect"
	"testing"
)

func TestNormalizeArgs(t *testing.T) {
	got := normalizeArgs([]string{"-config", "x.yaml", "-debug", "--listen=:1", "-h", "-timeout=2m", "-log-stderr"})
	want := []string{"--config", "x.yaml", "--debug", "--listen=:1", "-h", "--timeout=2m", "--log-stderr"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeArgs = %v, want %v", got, want)
	}
}
