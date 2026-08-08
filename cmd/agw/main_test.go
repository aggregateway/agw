package main

import (
	"reflect"
	"testing"
)

func TestNormalizeArgs(t *testing.T) {
	got := normalizeArgs([]string{"-config", "x.yaml", "-allow-debug", "-dev", "-data-dir", "s", "-admin-user", "u", "-admin-password", "p", "--listen=:1", "-h", "-timeout=2m", "-log-stderr"})
	want := []string{"--config", "x.yaml", "--allow-debug", "--dev", "--data-dir", "s", "--admin-user", "u", "--admin-password", "p", "--listen=:1", "-h", "--timeout=2m", "--log-stderr"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeArgs = %v, want %v", got, want)
	}
}
