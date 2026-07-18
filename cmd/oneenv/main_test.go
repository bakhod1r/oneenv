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

func TestRunExample(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	if err := os.WriteFile(f, []byte("# foo token\nFOO=secret\nBAR=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, ".env.example")
	if err := run([]string{"-f", f, "-example", "-o", out}); err != nil {
		t.Fatalf("run example: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want := "# type: int\n# required: this field\nBAR=\n\n" +
		"# foo token\n# type: string\n# required: this field\nFOO=\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
