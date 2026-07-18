package oneenv

import (
	"fmt"
	"strings"
	"testing"
)

func TestRedactedTag(t *testing.T) {
	type Config struct {
		Host     string `env:"HOST"`
		Password string `env:"PASSWORD,secret"`
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{"HOST": "db", "PASSWORD": "hunter2"})); err != nil {
		t.Fatal(err)
	}

	red, err := Redacted(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := string(red)
	if strings.Contains(out, "hunter2") {
		t.Errorf("Redacted leaked secret:\n%s", out)
	}
	if !strings.Contains(out, "PASSWORD=****") || !strings.Contains(out, "HOST=db") {
		t.Errorf("unexpected redacted output:\n%s", out)
	}

	// Plain Marshal still emits the real value.
	plain, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plain), "hunter2") {
		t.Errorf("Marshal should keep the real value:\n%s", plain)
	}
}

func TestSecretType(t *testing.T) {
	type Config struct {
		Key  Secret[string] `env:"KEY"`
		Port Secret[int]    `env:"PORT"`
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{"KEY": "s3cr3t", "PORT": "8080"})); err != nil {
		t.Fatal(err)
	}
	if cfg.Key.Value() != "s3cr3t" {
		t.Errorf("value=%q", cfg.Key.Value())
	}
	if cfg.Port.Value() != 8080 {
		t.Errorf("port=%d", cfg.Port.Value())
	}
	if got := fmt.Sprintf("%v %s %#v", cfg.Key, cfg.Key, cfg.Key); strings.Contains(got, "s3cr3t") {
		t.Errorf("Secret leaked through fmt: %s", got)
	}
	if cfg.Key.String() != "****" {
		t.Errorf("String()=%q", cfg.Key.String())
	}
}

func TestSecretRoundTrip(t *testing.T) {
	type Config struct {
		Key Secret[string] `env:"KEY"`
	}
	src := Config{Key: NewSecret("abc")}
	data, err := Marshal(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "abc") {
		t.Errorf("Marshal should emit real secret value:\n%s", data)
	}
	var got Config
	if err := Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Key.Value() != "abc" {
		t.Errorf("round trip: %q", got.Key.Value())
	}
}
