package oneenv

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWithTypeParser(t *testing.T) {
	type Config struct {
		IP net.IP `env:"IP"`
	}
	var cfg Config
	err := Load(&cfg,
		WithLookuper(MapLookuper{"IP": "10.0.0.1"}),
		WithTypeParser(func(s string) (net.IP, error) { return net.ParseIP(s), nil }),
	)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IP.String() != "10.0.0.1" {
		t.Errorf("got %v", cfg.IP)
	}
}

func TestWithTypeParserSlice(t *testing.T) {
	type Config struct {
		IPs []net.IP `env:"IPS" separator:","`
	}
	var cfg Config
	err := Load(&cfg,
		WithLookuper(MapLookuper{"IPS": "10.0.0.1,10.0.0.2"}),
		WithTypeParser(func(s string) (net.IP, error) { return net.ParseIP(s), nil }),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.IPs) != 2 || cfg.IPs[1].String() != "10.0.0.2" {
		t.Errorf("got %v", cfg.IPs)
	}
}

func TestFileSecret(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "secret")
	if err := os.WriteFile(p, []byte("s3cr3t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	type Config struct {
		Password string `env:"PASSWORD,file"`
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{"PASSWORD": p})); err != nil {
		t.Fatal(err)
	}
	if cfg.Password != "s3cr3t" {
		t.Errorf("got %q", cfg.Password)
	}
}

func TestFileSecretMissing(t *testing.T) {
	type Config struct {
		Password string `env:"PASSWORD,file"`
	}
	var cfg Config
	err := Load(&cfg, WithLookuper(MapLookuper{"PASSWORD": "/no/such/file"}))
	if !errors.Is(err, ErrSecretFile) {
		t.Fatalf("want ErrSecretFile, got %v", err)
	}
}

func TestNotEmpty(t *testing.T) {
	type Config struct {
		Name string `env:"NAME,notEmpty"`
	}
	var cfg Config
	err := Load(&cfg, WithLookuper(MapLookuper{"NAME": ""}))
	if !errors.Is(err, ErrEmpty) {
		t.Fatalf("want ErrEmpty, got %v", err)
	}
}

func TestEnvSeparatorAlias(t *testing.T) {
	type Config struct {
		Tags []string `env:"TAGS" envSeparator:";"`
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{"TAGS": "a;b;c"})); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Tags) != 3 || cfg.Tags[2] != "c" {
		t.Errorf("got %v", cfg.Tags)
	}
}

func TestMutator(t *testing.T) {
	type Config struct {
		Name string `env:"NAME"`
	}
	var cfg Config
	err := LoadContext(context.Background(), &cfg,
		WithLookuper(MapLookuper{"NAME": "abc"}),
		WithMutator(func(_ context.Context, _, v string) (string, error) {
			return strings.ToUpper(v), nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "ABC" {
		t.Errorf("got %q", cfg.Name)
	}
}

func TestMutatorError(t *testing.T) {
	type Config struct {
		Name string `env:"NAME"`
	}
	sentinel := errors.New("boom")
	var cfg Config
	err := Load(&cfg,
		WithLookuper(MapLookuper{"NAME": "abc"}),
		WithMutator(func(_ context.Context, _, _ string) (string, error) { return "", sentinel }),
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel, got %v", err)
	}
}

func TestValidator(t *testing.T) {
	type Config struct {
		Port int `env:"PORT"`
	}
	sentinel := errors.New("invalid")
	var cfg Config
	err := Load(&cfg,
		WithLookuper(MapLookuper{"PORT": "70000"}),
		WithValidator(func(v any) error {
			if v.(*Config).Port > 65535 {
				return sentinel
			}
			return nil
		}),
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel, got %v", err)
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	type DB struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	}
	type Config struct {
		Name string   `env:"NAME"`
		Tags []string `env:"TAGS" separator:","`
		DB   DB       `envPrefix:"DB_"`
	}
	src := Config{Name: "app one", Tags: []string{"a", "b"}, DB: DB{Host: "localhost", Port: 5432}}

	data, err := Marshal(src)
	if err != nil {
		t.Fatal(err)
	}
	var got Config
	if err := Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal %q: %v", data, err)
	}
	if got.Name != src.Name || got.DB.Port != 5432 || len(got.Tags) != 2 {
		t.Errorf("round trip mismatch: %+v (from %s)", got, data)
	}
}

func TestEnvTagAliases(t *testing.T) {
	type DB struct {
		Port int `env:"PORT" env-default:"5432"`
	}
	type Config struct {
		Host    string    `env:"HOST" env-default:"localhost" env-description:"bind host"`
		Tags    []string  `env:"TAGS" env-separator:";"`
		Started time.Time `env:"STARTED" env-layout:"2006-01-02"`
		Token   string    `env:"TOKEN" env-required:"true"`
		DB      DB        `env-prefix:"DB_"`
	}

	// Missing env-required field fails.
	var c0 Config
	if err := Load(&c0, WithLookuper(MapLookuper{})); !errors.Is(err, ErrRequired) {
		t.Fatalf("env-required not enforced: %v", err)
	}

	var cfg Config
	err := Load(&cfg, WithLookuper(MapLookuper{
		"TAGS": "a;b", "STARTED": "2020-01-02", "TOKEN": "t", "DB_PORT": "6000",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "localhost" { // env-default
		t.Errorf("host=%q", cfg.Host)
	}
	if len(cfg.Tags) != 2 || cfg.Tags[1] != "b" { // env-separator
		t.Errorf("tags=%v", cfg.Tags)
	}
	if cfg.Started.Year() != 2020 { // env-layout
		t.Errorf("started=%v", cfg.Started)
	}
	if cfg.DB.Port != 6000 { // env-prefix
		t.Errorf("db.port=%d", cfg.DB.Port)
	}
}

func TestEnvTagPriorityOverNative(t *testing.T) {
	// When both the env-* alias and the native tag are present, env-* wins.
	type Config struct {
		Port int      `env:"PORT" env-default:"9000" default:"8080"`
		Tags []string `env:"TAGS" env-separator:";" separator:","`
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{"TAGS": "a;b,c"})); err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9000 { // env-default beats default
		t.Errorf("port=%d, want 9000", cfg.Port)
	}
	if len(cfg.Tags) != 2 { // split on ';' (env-separator), not ','
		t.Errorf("tags=%v, want 2 elems", cfg.Tags)
	}
}

func TestEnvBooleanOptionAliases(t *testing.T) {
	// env-notempty
	type NE struct {
		V string `env:"V" env-notempty:"true"`
	}
	if err := Load(&NE{}, WithLookuper(MapLookuper{"V": ""})); !errors.Is(err, ErrEmpty) {
		t.Fatalf("env-notempty: want ErrEmpty, got %v", err)
	}

	// env-file
	dir := t.TempDir()
	p := filepath.Join(dir, "s")
	if err := os.WriteFile(p, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	type F struct {
		S string `env:"S" env-file:"true"`
	}
	var f F
	if err := Load(&f, WithLookuper(MapLookuper{"S": p})); err != nil || f.S != "secret" {
		t.Fatalf("env-file: %+v %v", f, err)
	}

	// env-init
	type I struct {
		P *int `env:"P" env-init:"true"`
	}
	var iv I
	if err := Load(&iv, WithLookuper(MapLookuper{})); err != nil || iv.P == nil {
		t.Fatalf("env-init: %+v %v", iv, err)
	}

	// env-unset
	t.Setenv("EU_KEY", "v")
	type U struct {
		K string `env:"EU_KEY" env-unset:"true"`
	}
	if err := Load(&U{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := os.LookupEnv("EU_KEY"); ok {
		t.Fatal("env-unset: expected EU_KEY removed")
	}
}

func TestEnvRequiredFalse(t *testing.T) {
	// env-required:"false" explicitly disables requiredness.
	type Config struct {
		Opt string `env:"OPT" env-required:"false"`
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{})); err != nil {
		t.Fatalf("env-required:false should not require: %v", err)
	}
}

func TestUsage(t *testing.T) {
	type Config struct {
		Port int    `env:"PORT" default:"8080" desc:"listen port"`
		Host string `env:"HOST,required" desc:"bind host"`
	}
	var b strings.Builder
	if err := Usage[Config](&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "PORT") || !strings.Contains(out, "listen port") || !strings.Contains(out, "yes") {
		t.Errorf("usage output missing content:\n%s", out)
	}
}
