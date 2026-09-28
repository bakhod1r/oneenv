package oneenv

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestSecretDoesNotLeakThroughSlogText(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logger.Info("cfg", "key", NewSecret("hunter2"))
	if strings.Contains(buf.String(), "hunter2") {
		t.Fatalf("slog TextHandler leaked secret: %s", buf.String())
	}
}

func TestSecretDoesNotLeakThroughFmtVerbs(t *testing.T) {
	s := NewSecret(424242)
	for _, verb := range []string{"%d", "%v", "%+v", "%#v", "%s", "%x", "%q", "%08d"} {
		if out := fmt.Sprintf(verb, s); strings.Contains(out, "424242") || strings.Contains(out, "67932") {
			t.Errorf("fmt %s leaked secret: %q", verb, out)
		}
	}
}

func TestSecretDoesNotLeakThroughXML(t *testing.T) {
	type C struct{ Key Secret[string] }
	b, err := xml.Marshal(C{Key: NewSecret("hunter2")})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "hunter2") {
		t.Fatalf("xml leaked secret: %s", b)
	}
}

func TestSecretMarshalYAMLMasks(t *testing.T) {
	v, err := NewSecret("hunter2").MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	if v != redactedMask {
		t.Fatalf("MarshalYAML = %v, want mask", v)
	}
}

func TestParseEmptyValueWithInlineComment(t *testing.T) {
	for _, src := range []string{
		"KEY= # note\n",
		"KEY=\t# type: string\n",
		"KEY=   #note\n",
	} {
		m := map[string]string{}
		err := parse("t", []byte(src), false, m)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if got := m["KEY"]; got != "" {
			t.Errorf("%q: KEY = %q, want empty", src, got)
		}
	}
	// A '#' glued to the '=' is still part of the value, as before.
	m := map[string]string{}
	if err := parse("t", []byte("COLOR=#fff\n"), false, m); err != nil {
		t.Fatal(err)
	}
	if m["COLOR"] != "#fff" {
		t.Errorf("COLOR = %q, want #fff", m["COLOR"])
	}
}

func TestFileFieldIsTreatedAsSecret(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/pw"
	if err := os.WriteFile(p, []byte("s3cr3t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	type Config struct {
		Password string `env:"PASSWORD,file"`
	}
	var cfg Config
	var rep Report
	var logBuf bytes.Buffer
	err := Load(&cfg,
		WithLookuper(MapLookuper{"PASSWORD": p}),
		WithReport(&rep),
		WithLogger(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))),
	)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Password != "s3cr3t" {
		t.Fatalf("Password = %q", cfg.Password)
	}
	if s := rep.String(); strings.Contains(s, "s3cr3t") {
		t.Errorf("report leaked file contents:\n%s", s)
	}
	if strings.Contains(logBuf.String(), "s3cr3t") {
		t.Errorf("logger leaked file contents:\n%s", logBuf.String())
	}
	red, err := Redacted(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(red), "s3cr3t") {
		t.Errorf("Redacted leaked file contents:\n%s", red)
	}
}

// A file saved by Windows Notepad starts with a UTF-8 BOM. It is not part of
// the first key.
func TestParseStripsUTF8BOM(t *testing.T) {
	m := map[string]string{}
	if err := parse("t", []byte("\xef\xbb\xbfHOST=db\nPORT=5432\n"), false, m); err != nil {
		t.Fatalf("parse with BOM: %v", err)
	}
	if m["HOST"] != "db" {
		t.Fatalf("HOST = %q, want db (keys: %v)", m["HOST"], m)
	}
}

// The schema depends on the tag key. A struct loaded once with the default
// "env" tags must not keep answering from that schema when a later load asks
// for a different tag key.
func TestSchemaCacheRespectsTagKey(t *testing.T) {
	type Config struct {
		Port int `env:"ENV_PORT" cfg:"CFG_PORT"`
	}
	env := MapLookuper{"ENV_PORT": "1", "CFG_PORT": "2"}
	var a Config
	if err := Load(&a, WithLookuper(env)); err != nil {
		t.Fatal(err)
	}
	var b Config
	if err := Load(&b, WithLookuper(env), WithTagKey("cfg")); err != nil {
		t.Fatal(err)
	}
	if a.Port != 1 || b.Port != 2 {
		t.Fatalf("env tag gave %d (want 1), cfg tag gave %d (want 2)", a.Port, b.Port)
	}
}
