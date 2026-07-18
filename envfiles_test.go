package oneenv

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEnvFilesCascade(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, ".env")
	writeFile(t, base, "HOST=base\nPORT=1\n")
	writeFile(t, base+".local", "PORT=2\n")            // overrides base
	writeFile(t, base+".production", "HOST=prod\n")    // env-specific
	writeFile(t, base+".production.local", "PORT=3\n") // highest priority

	type Config struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	}
	var cfg Config
	err := Load(&cfg,
		WithFiles(base),
		WithEnvFiles(),
		WithLookuper(MapLookuper{"APP_ENV": "production"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "prod" {
		t.Errorf("host=%q, want prod", cfg.Host)
	}
	if cfg.Port != 3 {
		t.Errorf("port=%d, want 3", cfg.Port)
	}
}

func TestEnvFilesCascadeMissingOK(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, ".env")
	writeFile(t, base, "HOST=base\n")
	// No .local, no env-specific files exist; those are optional.

	type Config struct {
		Host string `env:"HOST"`
	}
	var cfg Config
	if err := Load(&cfg, WithFiles(base), WithEnvFiles(), WithLookuper(MapLookuper{})); err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "base" {
		t.Errorf("host=%q, want base", cfg.Host)
	}
}

func TestFilesFor(t *testing.T) {
	got := FilesFor(WithFiles(".env"), WithEnvFiles(), WithLookuper(MapLookuper{"GO_ENV": "dev"}))
	want := []string{".env", ".env.local", ".env.dev", ".env.dev.local"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("files[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}
