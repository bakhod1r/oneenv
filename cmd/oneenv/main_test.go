package main

import (
	"os"
	"path/filepath"
	"strings"
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
	if err := run([]string{"-f", f, "-override", "--", "true"}); err != nil {
		t.Fatalf("run exec override: %v", err)
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

	// Test example to stdout
	if err := run([]string{"-f", f, "-example", "-o", "-"}); err != nil {
		t.Fatalf("run example stdout: %v", err)
	}
}

func TestSubcommandDoctor(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, ".env")
	f2 := filepath.Join(dir, ".env.local")
	if err := os.WriteFile(f1, []byte("PORT=8080\nBAD_key=1\nPORT=9090\nEMPTY_KEY=\nINVALID LINE WITHOUT EQUALS\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// doctor should report problems and return an error for invalid lines
	err := run([]string{"doctor", "-f", f1, "-f", f2})
	if err == nil {
		t.Fatal("expected doctor error due to invalid line")
	}

	// valid file doctor
	fValid := filepath.Join(dir, ".env.valid")
	if err := os.WriteFile(fValid, []byte("HOST=localhost\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"doctor", "-f", fValid}); err != nil {
		t.Fatalf("doctor valid: %v", err)
	}
}

func TestSubcommandLint(t *testing.T) {
	dir := t.TempDir()
	fClean := filepath.Join(dir, ".env.clean")
	if err := os.WriteFile(fClean, []byte("PORT=8080\nHOST=localhost\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"lint", "-f", fClean}); err != nil {
		t.Fatalf("lint clean: %v", err)
	}

	fDirty := filepath.Join(dir, ".env.dirty")
	if err := os.WriteFile(fDirty, []byte("  LEADING=space\nbad_name=1\nDUP=1\nDUP=2\nUNCLOSED=\"quote\nINVALID\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"lint", "-f", fDirty}); err == nil {
		t.Fatal("expected lint error for dirty file")
	}
}

func TestSubcommandFormat(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	if err := os.WriteFile(f, []byte("# Header\n\n# B comment\nB=2\n# A comment\nA=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"format", "-f", f}); err != nil {
		t.Fatalf("format preview: %v", err)
	}
	if err := run([]string{"format", "-w", "-f", f}); err != nil {
		t.Fatalf("format write: %v", err)
	}
	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "A=1") || !strings.Contains(string(got), "B=2") {
		t.Fatalf("unexpected format output: %s", string(got))
	}
}

func TestSubcommandInit(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	if err := run([]string{"init", "-f", f}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := os.Stat(f); err != nil {
		t.Fatalf(".env not created: %v", err)
	}
	ex := f + ".example"
	if _, err := os.Stat(ex); err != nil {
		t.Fatalf(".env.example not created: %v", err)
	}

	// Re-running init should leave existing files alone
	if err := run([]string{"init", "-f", f}); err != nil {
		t.Fatalf("init re-run: %v", err)
	}
}

func TestSubcommandExplain(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	if err := os.WriteFile(f, []byte("PORT=8080\nPORT=9090\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"explain", "PORT", "-f", f}); err != nil {
		t.Fatalf("explain: %v", err)
	}
	if err := run([]string{"explain", "UNKNOWN", "-f", f}); err == nil {
		t.Fatal("expected error for unknown key")
	}
	if err := run([]string{"explain"}); err == nil {
		t.Fatal("expected error for missing key arg")
	}

	t.Setenv("ENV_ONLY_KEY", "value")
	if err := run([]string{"explain", "ENV_ONLY_KEY", "-f", f}); err != nil {
		t.Fatalf("explain env key: %v", err)
	}
}

func TestSubcommandMigrate(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	if err := os.WriteFile(f, []byte("OLD_KEY=val\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"migrate"}); err == nil {
		t.Fatal("expected error for missing migrate pairs")
	}
	if err := run([]string{"migrate", "INVALID_PAIR", "-f", f}); err == nil {
		t.Fatal("expected error for invalid migrate pair format")
	}

	if err := run([]string{"migrate", "OLD_KEY=NEW_KEY", "-f", f}); err != nil {
		t.Fatalf("migrate preview: %v", err)
	}
	if err := run([]string{"migrate", "-w", "OLD_KEY=NEW_KEY", "-f", f}); err != nil {
		t.Fatalf("migrate write: %v", err)
	}

	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "NEW_KEY=val") {
		t.Fatalf("got %q, want NEW_KEY=val", string(got))
	}
}

func TestInferType(t *testing.T) {
	tests := map[string]string{
		"":                      "string",
		"true":                  "bool",
		"false":                 "bool",
		"123":                   "int",
		"3.14":                  "float",
		"10s":                   "duration",
		"http://localhost:8080": "url",
		"hello world":           "string",
	}
	for val, want := range tests {
		if got := inferType(val); got != want {
			t.Errorf("inferType(%q) = %q, want %q", val, got, want)
		}
	}
}
