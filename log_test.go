package oneenv_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/bakhod1r/oneenv"
)

type logConfig struct {
	Host   string                `env:"HOST" default:"localhost"`
	Port   int                   `env:"PORT"`
	APIKey oneenv.Secret[string] `env:"API_KEY"`
	Token  string                `env:"TOKEN,secret"`
	Absent string                `env:"ABSENT"`
}

func loadLogConfig(t *testing.T, extra ...oneenv.Option) (*logConfig, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	opts := append([]oneenv.Option{
		oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{
			"PORT":    "8080",
			"API_KEY": "sk-live-abcdef9f2a",
			"TOKEN":   "short",
		}),
		oneenv.WithTable(),
		oneenv.WithOutput(&buf),
		oneenv.WithSecretReveal(4),
	}, extra...)

	var cfg logConfig
	if err := oneenv.Load(&cfg, opts...); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return &cfg, &buf
}

func TestWithTableMasksSecrets(t *testing.T) {
	cfg, buf := loadLogConfig(t)
	out := buf.String()

	if cfg.APIKey.Value() != "sk-live-abcdef9f2a" {
		t.Fatalf("secret decoded wrong: %q", cfg.APIKey.Value())
	}
	if strings.Contains(out, "sk-live-abcdef9f2a") {
		t.Fatalf("plaintext secret leaked into table:\n%s", out)
	}
	// A long secret shows its first and last 4 characters.
	if !strings.Contains(out, "sk-l****9f2a") {
		t.Fatalf("partial mask missing:\n%s", out)
	}
	// A short secret gets a narrower window, never more than half of it: "short"
	// is five characters, so only one at each end.
	if !strings.Contains(out, "s****t") {
		t.Fatalf("short secret not shown with a narrowed window:\n%s", out)
	}
	for _, want := range []string{"KEY", "VALUE", "SOURCE", "PORT", "8080", "env", "localhost", "default", "unset"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table missing %q:\n%s", want, out)
		}
	}
}

func TestWithSecretRevealZeroMasksEverything(t *testing.T) {
	_, buf := loadLogConfig(t, oneenv.WithSecretReveal(0))
	if out := buf.String(); strings.Contains(out, "sk-l") {
		t.Fatalf("reveal 0 still shows secret prefix:\n%s", out)
	}
}

func TestNoTableWithoutOption(t *testing.T) {
	var cfg logConfig
	if err := oneenv.Load(&cfg, oneenv.WithFiles(), oneenv.WithLookuper(oneenv.MapLookuper{})); err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Nothing to assert beyond "it did not panic and wrote nowhere"; the absence
	// of a writer is the contract.
}

func TestWithLoggerMasksSecrets(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	var cfg logConfig
	err := oneenv.Load(&cfg,
		oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{"API_KEY": "sk-live-abcdef9f2a"}),
		oneenv.WithLogger(logger),
		oneenv.WithSecretReveal(4),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "sk-live-abcdef9f2a") {
		t.Fatalf("plaintext secret leaked into log:\n%s", out)
	}
	if !strings.Contains(out, "sk-l****9f2a") {
		t.Fatalf("masked secret missing from log:\n%s", out)
	}
	if !strings.Contains(out, "key=API_KEY") || !strings.Contains(out, "source=env") {
		t.Fatalf("log record missing key/source:\n%s", out)
	}
}

func TestPrintMasksSecrets(t *testing.T) {
	cfg := logConfig{
		Host:   "localhost",
		Port:   8080,
		APIKey: oneenv.NewSecret("sk-live-abcdef9f2a"),
		Token:  "short",
	}
	var buf bytes.Buffer
	if err := oneenv.Print(cfg, oneenv.WithOutput(&buf), oneenv.WithSecretReveal(4)); err != nil {
		t.Fatalf("Print: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "sk-live-abcdef9f2a") {
		t.Fatalf("plaintext secret leaked into Print:\n%s", out)
	}
	if !strings.Contains(out, "sk-l****9f2a") {
		t.Fatalf("partial mask missing:\n%s", out)
	}
	if !strings.Contains(out, `""`) {
		t.Fatalf("empty value placeholder missing:\n%s", out)
	}
}

func TestRedactedStillFullyMasks(t *testing.T) {
	cfg := logConfig{APIKey: oneenv.NewSecret("sk-live-abcdef9f2a")}
	out, err := oneenv.Redacted(cfg)
	if err != nil {
		t.Fatalf("Redacted: %v", err)
	}
	if !strings.Contains(string(out), "API_KEY=****") {
		t.Fatalf("Redacted no longer fully masks:\n%s", out)
	}
	if strings.Contains(string(out), "sk-l") {
		t.Fatalf("Redacted leaked secret prefix:\n%s", out)
	}
}
