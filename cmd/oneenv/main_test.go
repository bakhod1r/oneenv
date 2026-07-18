package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunPrintsVars(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	if err := os.WriteFile(f, []byte("FOO=bar\nBAZ=qux\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-f", f}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunJSON(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	if err := os.WriteFile(f, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-f", f, "-json"}); err != nil {
		t.Fatalf("run json: %v", err)
	}
}

func TestRunExec(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	if err := os.WriteFile(f, []byte("ONEENV_CLI_TEST=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-f", f, "--", "true"}); err != nil {
		t.Fatalf("run exec: %v", err)
	}
}

func TestRunMissingFile(t *testing.T) {
	if err := run([]string{"-f", "nope.env"}); err == nil {
		t.Fatal("expected error for missing file")
	}
}
