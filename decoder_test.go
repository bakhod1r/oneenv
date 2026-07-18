package oneenv

import (
	"errors"
	"testing"
	"time"
)

type DBConfig struct {
	Host string `env:"HOST" default:"localhost"`
	Port int    `env:"PORT" default:"5432"`
}

type Config struct {
	Name    string         `env:"NAME,required"`
	Port    int            `env:"PORT" default:"8080"`
	Debug   bool           `env:"DEBUG"`
	Timeout time.Duration  `env:"TIMEOUT" default:"5s"`
	Rate    float64        `env:"RATE" default:"1.5"`
	Tags    []string       `env:"TAGS" separator:","`
	Limits  map[string]int `env:"LIMITS"`
	DB      DBConfig       `envPrefix:"DB_"`
	Skip    string         `env:"-"`
}

func TestUnmarshalFull(t *testing.T) {
	src := []byte(`
NAME=myapp
DEBUG=true
TIMEOUT=30s
TAGS=a,b,c
LIMITS=cpu:2,mem:8
DB_HOST=db.example.com
DB_PORT=6543
`)
	var cfg Config
	if err := Unmarshal(src, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if cfg.Name != "myapp" {
		t.Errorf("Name = %q", cfg.Name)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want default 8080", cfg.Port)
	}
	if !cfg.Debug {
		t.Errorf("Debug = false")
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v", cfg.Timeout)
	}
	if cfg.Rate != 1.5 {
		t.Errorf("Rate = %v, want default", cfg.Rate)
	}
	if len(cfg.Tags) != 3 || cfg.Tags[2] != "c" {
		t.Errorf("Tags = %v", cfg.Tags)
	}
	if cfg.Limits["cpu"] != 2 || cfg.Limits["mem"] != 8 {
		t.Errorf("Limits = %v", cfg.Limits)
	}
	if cfg.DB.Host != "db.example.com" || cfg.DB.Port != 6543 {
		t.Errorf("DB = %+v", cfg.DB)
	}
}

func TestRequiredMissing(t *testing.T) {
	var cfg Config
	err := Unmarshal([]byte(`PORT=9090`), &cfg)
	if err == nil {
		t.Fatal("expected required error")
	}
	if !errors.Is(err, ErrRequired) {
		t.Errorf("want ErrRequired, got %v", err)
	}
	var fe *FieldError
	if !errors.As(err, &fe) {
		t.Fatalf("want *FieldError, got %T", err)
	}
	if fe.Key != "NAME" {
		t.Errorf("FieldError.Key = %q", fe.Key)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	var cfg Config
	err := Load(&cfg,
		WithFiles(), // no files
		WithLookuper(MapLookuper{"NAME": "fromenv", "PORT": "7000"}),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Name != "fromenv" || cfg.Port != 7000 {
		t.Errorf("got %+v", cfg)
	}
}

func TestParseGeneric(t *testing.T) {
	cfg, err := Parse[Config](
		WithFiles(),
		WithLookuper(MapLookuper{"NAME": "gen"}),
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Name != "gen" {
		t.Errorf("Name = %q", cfg.Name)
	}
}

func TestNotAStruct(t *testing.T) {
	x := 5
	if err := Unmarshal(nil, &x); !errors.Is(err, ErrNotAStruct) {
		t.Errorf("want ErrNotAStruct, got %v", err)
	}
}

func BenchmarkLoad(b *testing.B) {
	src := []byte("NAME=myapp\nPORT=9090\nDEBUG=true\nTAGS=a,b,c\n")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var cfg Config
		_ = Unmarshal(src, &cfg)
	}
}
