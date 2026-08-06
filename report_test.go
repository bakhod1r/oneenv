package oneenv_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/bakhod1r/oneenv"
)

type appConfig struct {
	Host   string                `env:"HOST" default:"localhost" example:"api.example.com"`
	Port   int                   `env:"PORT" default:"8000"`
	Mode   string                `env:"MODE" default:"dev" enum:"dev,test,prod"`
	APIKey oneenv.Secret[string] `env:"API_KEY"`
	Server string                `env:"SERVER_HOST" alias:"HOST_NAME"`
}

// writeEnv creates a .env file in a fresh directory and returns the directory.
func writeEnv(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReportSourceNamesTheFile(t *testing.T) {
	dir := writeEnv(t, ".env", "PORT=9090\nAPI_KEY=sk-live-abcdef9f2a\n")

	var (
		cfg appConfig
		rep oneenv.Report
	)
	err := oneenv.Load(&cfg,
		oneenv.WithBaseDir(dir),
		oneenv.WithLookuper(oneenv.MapLookuper{"MODE": "prod"}),
		oneenv.WithReport(&rep),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := map[string]string{
		"PORT": filepath.Join(dir, ".env"),
		"MODE": "env",
		"HOST": "default",
	}
	for key, src := range want {
		got, ok := rep.Source(key)
		if !ok {
			t.Fatalf("no report entry for %s", key)
		}
		if got != src {
			t.Fatalf("%s source = %q, want %q", key, got, src)
		}
	}

	explain := rep.Explain("PORT")
	for _, want := range []string{"Key", "PORT", "Value", "9090", "Default", "8000", "Type", "int"} {
		if !strings.Contains(explain, want) {
			t.Fatalf("Explain missing %q:\n%s", want, explain)
		}
	}
	if e, _ := rep.Lookup("API_KEY"); e.Value != "****" {
		t.Fatalf("report stored an unmasked secret: %q", e.Value)
	}
	if got := rep.Explain("NOPE"); !strings.Contains(got, "unknown key") {
		t.Fatalf("Explain of an unknown key = %q", got)
	}
}

func TestWithBaseDirResolvesRelativeFiles(t *testing.T) {
	dir := writeEnv(t, ".env", "PORT=7777\n")
	var cfg appConfig
	if err := oneenv.Load(&cfg, oneenv.WithBaseDir(dir), oneenv.WithLookuper(oneenv.MapLookuper{})); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 7777 {
		t.Fatalf("Port = %d, want 7777", cfg.Port)
	}
}

func TestWithStrictKeysCatchesTypos(t *testing.T) {
	dir := writeEnv(t, ".env", "PORT=8080\nPORRT=9999\n")
	var cfg appConfig
	err := oneenv.Load(&cfg,
		oneenv.WithBaseDir(dir),
		oneenv.WithLookuper(oneenv.MapLookuper{}),
		oneenv.WithStrictKeys(),
	)
	if err == nil {
		t.Fatal("expected an error for the unknown key")
	}
	if !strings.Contains(err.Error(), "PORRT") {
		t.Fatalf("error does not name the typo: %v", err)
	}
	if strings.Contains(err.Error(), "PORT (") {
		t.Fatalf("a known key was reported as unknown: %v", err)
	}
}

func TestAliasAndEnumAndPattern(t *testing.T) {
	var cfg appConfig
	// The alias supplies the value when the current key is absent.
	err := oneenv.Load(&cfg,
		oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{"HOST_NAME": "old-style"}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server != "old-style" {
		t.Fatalf("alias not honored: %q", cfg.Server)
	}

	// An enum rejects a value outside the list.
	err = oneenv.Load(&cfg, oneenv.WithFiles(), oneenv.WithLookuper(oneenv.MapLookuper{"MODE": "staging"}))
	if err == nil || !strings.Contains(err.Error(), "dev, test, prod") {
		t.Fatalf("enum did not reject the value: %v", err)
	}

	type patterned struct {
		Key string `env:"API_KEY" pattern:"^[A-Za-z0-9]{6}$"`
	}
	var p patterned
	err = oneenv.Load(&p, oneenv.WithFiles(), oneenv.WithLookuper(oneenv.MapLookuper{"API_KEY": "no!"}))
	if err == nil || !strings.Contains(err.Error(), "pattern") {
		t.Fatalf("pattern did not reject the value: %v", err)
	}
}

func TestWithWriteExampleDefaultsNextToTheEnvFile(t *testing.T) {
	dir := writeEnv(t, ".env", "PORT=9090\n")
	var cfg appConfig
	err := oneenv.Load(&cfg,
		oneenv.WithBaseDir(dir),
		oneenv.WithLookuper(oneenv.MapLookuper{}),
		oneenv.WithWriteExample(),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.example")); err != nil {
		t.Fatalf("example not written next to .env: %v", err)
	}
}

func TestWithWriteExample(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.example")

	var cfg appConfig
	err := oneenv.Load(&cfg,
		oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{"API_KEY": "sk-live-abcdef9f2a"}),
		oneenv.WithWriteExample(path),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("example not written: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "example: api.example.com") {
		t.Fatalf("the example tag was not used:\n%s", out)
	}
	if strings.Contains(out, "HOST=api.example.com") {
		t.Fatalf("a key was written with a value:\n%s", out)
	}
	if !strings.Contains(out, "allowed: dev|test|prod") {
		t.Fatalf("enum values missing from the example:\n%s", out)
	}
	if strings.Contains(out, "sk-live") {
		t.Fatalf("a secret leaked into the example:\n%s", out)
	}
}

func TestDiffAndHash(t *testing.T) {
	a := appConfig{Host: "localhost", Port: 8080, APIKey: oneenv.NewSecret("sk-live-abcdef9f2a")}
	b := a
	b.Port = 9090
	b.APIKey = oneenv.NewSecret("sk-live-000000000")

	changes := oneenv.Diff(a, b)
	var got []string
	for _, c := range changes {
		got = append(got, c.String())
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "PORT: 8080 -> 9090") {
		t.Fatalf("port change missing:\n%s", joined)
	}
	if strings.Contains(joined, "sk-live") {
		t.Fatalf("a secret leaked into the diff:\n%s", joined)
	}
	if !strings.Contains(joined, "API_KEY: **** -> ****") {
		t.Fatalf("changed secret not reported:\n%s", joined)
	}
	if oneenv.Diff(a, a) != nil {
		t.Fatal("identical configs reported a change")
	}
	if !strings.Contains(oneenv.DiffString(a, a), "no changes") {
		t.Fatal("DiffString did not report an identical pair")
	}

	if h := oneenv.Hash(a); len(h) != 8 || h != oneenv.Hash(a) {
		t.Fatalf("hash is not stable or not 8 chars: %q", h)
	}
	if oneenv.Hash(a) == oneenv.Hash(b) {
		t.Fatal("different configs share a hash")
	}
}

func TestEmptySecretIsNotMasked(t *testing.T) {
	var buf bytes.Buffer
	type conf struct {
		Pass string `env:"REDIS_PASSWORD,secret"`
	}
	var cfg conf
	err := oneenv.Load(&cfg,
		oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{"REDIS_PASSWORD": ""}),
		oneenv.WithTable(),
		oneenv.WithOutput(&buf),
		oneenv.WithSecretReveal(4),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "****") {
		t.Fatalf("an empty secret was masked, claiming a value it does not have:\n%s", out)
	}
	for _, want := range []string{"TYPE", "NULL", "string", "yes"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table missing %q:\n%s", want, out)
		}
	}
}

func TestSecretsMaskedByDefault(t *testing.T) {
	var buf bytes.Buffer
	cfg := appConfig{APIKey: oneenv.NewSecret("sk-live-abcdef9f2a")}
	if err := oneenv.Print(cfg, oneenv.WithOutput(&buf)); err != nil {
		t.Fatalf("Print: %v", err)
	}
	if strings.Contains(buf.String(), "sk-l") {
		t.Fatalf("a secret was revealed without WithSecretReveal:\n%s", buf.String())
	}

	// WithRedacted wins over a reveal window whatever the order.
	buf.Reset()
	err := oneenv.Print(cfg, oneenv.WithOutput(&buf), oneenv.WithSecretReveal(4), oneenv.WithRedacted())
	if err != nil {
		t.Fatalf("Print: %v", err)
	}
	if strings.Contains(buf.String(), "sk-l") {
		t.Fatalf("WithRedacted did not override the reveal:\n%s", buf.String())
	}
}

func TestReportMissingKeys(t *testing.T) {
	var (
		buf bytes.Buffer
		rep oneenv.Report
	)
	type conf struct {
		Name  string `env:"APP_NAME"`
		Port  int    `env:"APP_PORT" default:"8080"`
		SMTP  string `env:"SMTP_HOST"`
		Debug bool   `env:"DEBUG"`
	}
	var cfg conf
	err := oneenv.Load(&cfg,
		oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{"APP_NAME": "superapp"}),
		oneenv.WithReport(&rep),
		oneenv.WithTable(),
		oneenv.WithOutput(&buf),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := rep.MissingKeys()
	want := []string{"DEBUG", "SMTP_HOST"} // APP_NAME came from env, APP_PORT from its default
	if len(got) != len(want) {
		t.Fatalf("MissingKeys() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("MissingKeys() = %v, want %v", got, want)
		}
	}
	if !strings.Contains(buf.String(), "2 not set: DEBUG, SMTP_HOST") {
		t.Fatalf("table did not summarize the unset keys:\n%s", buf.String())
	}
}

func TestTableIsRuledAndPlainWhenCaptured(t *testing.T) {
	var buf bytes.Buffer
	type conf struct {
		Host string `env:"HOST" default:"localhost"`
		Port int    `env:"PORT" default:"8080"`
	}
	var cfg conf
	err := oneenv.Load(&cfg,
		oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{}),
		oneenv.WithTable(),
		oneenv.WithOutput(&buf),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	out := buf.String()

	// A rule between every row: header rule plus one between the two rows.
	if n := strings.Count(out, "├"); n != 2 {
		t.Fatalf("expected a rule between every row, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "┌") || !strings.Contains(out, "┘") {
		t.Fatalf("table is not bordered:\n%s", out)
	}
	// Bold is for terminals; a captured table stays plain so it can be compared.
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("escape sequences leaked into captured output:\n%q", out)
	}
	// Every line of a bordered table is the same width.
	var width int
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		n := len([]rune(line))
		if width == 0 {
			width = n
		} else if n != width {
			t.Fatalf("ragged table line %q (%d, want %d):\n%s", line, n, width, out)
		}
	}
}

func TestTableStaysEvenWithWideCharacters(t *testing.T) {
	var buf bytes.Buffer
	type conf struct {
		Name   string `env:"NAME"`
		Emoji  string `env:"EMOJI"`
		Accent string `env:"ACCENT"`
	}
	var cfg conf
	err := oneenv.Load(&cfg,
		oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{
			"NAME":   "東京都",    // three double-width ideographs
			"EMOJI":  "🚀 ship", // an emoji is two columns wide
			"ACCENT": "café",   // may arrive with a combining accent
		}),
		oneenv.WithTable(),
		oneenv.WithOutput(&buf),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var width int
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if !strings.HasPrefix(line, "│") && !strings.HasPrefix(line, "┌") &&
			!strings.HasPrefix(line, "├") && !strings.HasPrefix(line, "└") {
			continue
		}
		n := terminalWidth(line)
		if width == 0 {
			width = n
		} else if n != width {
			t.Fatalf("ragged line %q (%d columns, want %d):\n%s", line, n, width, buf.String())
		}
	}
}

// terminalWidth mirrors what a terminal does with the characters used above, so
// the test measures the same thing the table does.
func terminalWidth(s string) int {
	n := 0
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r) || r == 0xFE0F:
		case r >= 0x1100 && r <= 0x115F, r >= 0x2E80 && r <= 0xA4CF,
			r >= 0xAC00 && r <= 0xD7A3, r >= 0xF900 && r <= 0xFAFF,
			r >= 0xFE10 && r <= 0xFE19, r >= 0xFE30 && r <= 0xFE6F,
			r >= 0xFF00 && r <= 0xFF60, r >= 0xFFE0 && r <= 0xFFE6,
			r >= 0x1F300 && r <= 0x1F64F, r >= 0x1F900 && r <= 0x1F9FF,
			r >= 0x20000 && r <= 0x3FFFD:
			n += 2
		default:
			n++
		}
	}
	return n
}
