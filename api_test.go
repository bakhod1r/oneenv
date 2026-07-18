package oneenv

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadMergesFiles(t *testing.T) {
	a := writeTemp(t, "a.env", "FOO=1\nBAR=2\n")
	b := writeTemp(t, "b.env", "BAR=3\nBAZ=4\n")

	vals, err := Read(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if vals["FOO"] != "1" || vals["BAR"] != "3" || vals["BAZ"] != "4" {
		t.Errorf("merge wrong: %v", vals)
	}
}

func TestReadMissingFileErrors(t *testing.T) {
	if _, err := Read("does-not-exist.env"); err == nil {
		t.Fatal("expected error for missing explicit file")
	}
}

func TestReadDefaultMissingIsOK(t *testing.T) {
	// Default ".env" is optional; run in an empty dir.
	dir := t.TempDir()
	old, _ := os.Getwd()
	defer func() { _ = os.Chdir(old) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	vals, err := Read()
	if err != nil {
		t.Fatalf("default missing should not error: %v", err)
	}
	if len(vals) != 0 {
		t.Errorf("want empty, got %v", vals)
	}
}

func TestLoadEnvAndOverload(t *testing.T) {
	f := writeTemp(t, ".env", "ONEENV_TEST_KEY=fromfile\n")
	_ = os.Unsetenv("ONEENV_TEST_KEY")

	if err := LoadEnv(f); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("ONEENV_TEST_KEY"); got != "fromfile" {
		t.Fatalf("LoadEnv = %q", got)
	}

	// Existing var is preserved by LoadEnv...
	_ = os.Setenv("ONEENV_TEST_KEY", "preset")
	f2 := writeTemp(t, ".env", "ONEENV_TEST_KEY=changed\n")
	if err := LoadEnv(f2); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("ONEENV_TEST_KEY"); got != "preset" {
		t.Errorf("LoadEnv should preserve, got %q", got)
	}
	// ...but Overload replaces it.
	if err := Overload(f2); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("ONEENV_TEST_KEY"); got != "changed" {
		t.Errorf("Overload = %q", got)
	}
	_ = os.Unsetenv("ONEENV_TEST_KEY")
}

func TestWithPrefix(t *testing.T) {
	type C struct {
		Port int `env:"PORT"`
	}
	var c C
	err := Load(&c,
		WithFiles(),
		WithPrefix("APP_"),
		WithLookuper(MapLookuper{"APP_PORT": "1234"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 1234 {
		t.Errorf("Port = %d", c.Port)
	}
}

func TestWithOverride(t *testing.T) {
	f := writeTemp(t, ".env", "K=fromfile\n")
	type C struct {
		K string `env:"K"`
	}
	var c C
	err := Load(&c,
		WithFiles(f),
		WithOverride(),
		WithLookuper(MapLookuper{"K": "fromenv"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c.K != "fromfile" {
		t.Errorf("override: K = %q", c.K)
	}
}

func TestWithTagKeyAndRequired(t *testing.T) {
	type C struct {
		Name string `cfg:"NAME"`
	}
	var c C
	err := Load(&c, WithFiles(), WithTagKey("cfg"), WithRequired(),
		WithLookuper(MapLookuper{}))
	if !errors.Is(err, ErrRequired) {
		t.Errorf("want ErrRequired, got %v", err)
	}
}

func TestPointerAndTextUnmarshaler(t *testing.T) {
	type C struct {
		Count *int     `env:"COUNT"`
		Addr  net.IP   `env:"ADDR"` // net.IP implements TextUnmarshaler
		List  []string `env:"LIST" separator:";"`
	}
	var c C
	err := Unmarshal([]byte("COUNT=7\nADDR=10.0.0.1\nLIST=x;y;z"), &c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Count == nil || *c.Count != 7 {
		t.Errorf("Count = %v", c.Count)
	}
	if c.Addr.String() != "10.0.0.1" {
		t.Errorf("Addr = %v", c.Addr)
	}
	if len(c.List) != 3 || c.List[1] != "y" {
		t.Errorf("List = %v", c.List)
	}
}

func TestTimeLayout(t *testing.T) {
	type C struct {
		Day time.Time `env:"DAY" layout:"2006-01-02"`
	}
	var c C
	if err := Unmarshal([]byte("DAY=2026-07-18"), &c); err != nil {
		t.Fatal(err)
	}
	if c.Day.Year() != 2026 || c.Day.Month() != 7 || c.Day.Day() != 18 {
		t.Errorf("Day = %v", c.Day)
	}
}

func TestParseErrorMessage(t *testing.T) {
	err := parse("cfg.env", []byte("A=1\nBADLINE\n"), false, map[string]string{})
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T", err)
	}
	if pe.Line != 2 || pe.File != "cfg.env" {
		t.Errorf("position wrong: %s:%d", pe.File, pe.Line)
	}
	_ = pe.Error()
	_ = (&FieldError{Field: "X", Key: "X", Err: ErrRequired}).Error()
}

func TestBadValueYieldsFieldError(t *testing.T) {
	type C struct {
		Port int `env:"PORT"`
	}
	var c C
	err := Unmarshal([]byte("PORT=notanumber"), &c)
	var fe *FieldError
	if !errors.As(err, &fe) {
		t.Fatalf("want *FieldError, got %v", err)
	}
	if fe.Key != "PORT" {
		t.Errorf("Key = %q", fe.Key)
	}
}
