// Package bench compares oneenv against the popular .env / env-to-struct
// libraries. It lives in its own module so the root oneenv module stays
// dependency-free.
//
// Each benchmark measures the full realistic pipeline: turn .env text into a
// populated config struct. Libraries that only decode from the process
// environment are paired with godotenv (the usual real-world combo) and the
// parsed values are loaded into os.Environ inside the timed loop.
package bench

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bakhod1r/oneenv"
	env "github.com/caarlos0/env/v11"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joeshaw/envdecode"
	"github.com/joho/godotenv"
	kelsey "github.com/kelseyhightower/envconfig"
	sethvargo "github.com/sethvargo/go-envconfig"
	"github.com/spf13/viper"
)

const sample = "NAME=myapp\nPORT=9090\nDEBUG=true\nTIMEOUT=30s\nTAGS=a,b,c\n"

// Config uses tags understood by every library benchmarked here.
type Config struct {
	Name    string        `env:"NAME" mapstructure:"NAME"`
	Port    int           `env:"PORT" mapstructure:"PORT"`
	Debug   bool          `env:"DEBUG" mapstructure:"DEBUG"`
	Timeout time.Duration `env:"TIMEOUT" mapstructure:"TIMEOUT"`
	Tags    []string      `env:"TAGS" envSeparator:"," separator:"," mapstructure:"TAGS"`
}

// loadEnvInLoop parses the sample and pushes it into os.Environ, so libraries
// that read only from the process environment are timed on the same full
// pipeline (parse + populate + decode) as oneenv. Caller must unset after.
func loadEnvInLoop() map[string]string {
	m, _ := godotenv.Parse(strings.NewReader(sample))
	for k, v := range m {
		_ = os.Setenv(k, v)
	}
	return m
}

// loadSample parses an arbitrary .env sample into os.Environ. Caller unsets.
func loadSample(s string) map[string]string {
	m, _ := godotenv.Parse(strings.NewReader(s))
	for k, v := range m {
		_ = os.Setenv(k, v)
	}
	return m
}

func unset(m map[string]string) {
	for k := range m {
		_ = os.Unsetenv(k)
	}
}

// BenchmarkOneenv — single call parses AND decodes into the struct.
func BenchmarkOneenv(b *testing.B) {
	data := []byte(sample)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var cfg Config
		if err := oneenv.Unmarshal(data, &cfg); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGodotenvCaarlos0 — godotenv parses to a map, caarlos0/env decodes.
func BenchmarkGodotenvCaarlos0(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m, err := godotenv.Parse(strings.NewReader(sample))
		if err != nil {
			b.Fatal(err)
		}
		var cfg Config
		if err := env.ParseWithOptions(&cfg, env.Options{Environment: m}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGodotenvSethvargo — godotenv parses, sethvargo/go-envconfig decodes
// straight from the map via its Lookuper (no os.Environ round-trip).
func BenchmarkGodotenvSethvargo(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m, err := godotenv.Parse(strings.NewReader(sample))
		if err != nil {
			b.Fatal(err)
		}
		var cfg Config
		if err := sethvargo.ProcessWith(ctx, &sethvargo.Config{
			Target:   &cfg,
			Lookuper: sethvargo.MapLookuper(m),
		}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkViper — viper reads the .env text and unmarshals into the struct.
func BenchmarkViper(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v := viper.New()
		v.SetConfigType("env")
		if err := v.ReadConfig(strings.NewReader(sample)); err != nil {
			b.Fatal(err)
		}
		var cfg Config
		if err := v.Unmarshal(&cfg); err != nil {
			b.Fatal(err)
		}
	}
}

// The three below read from the process environment, so each loop iteration
// parses the text with godotenv, populates os.Environ, then decodes — the same
// end-to-end work oneenv does in one call.

func BenchmarkGodotenvKelsey(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := loadEnvInLoop()
		var cfg Config
		if err := kelsey.Process("", &cfg); err != nil {
			b.Fatal(err)
		}
		unset(m)
	}
}

func BenchmarkGodotenvEnvdecode(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := loadEnvInLoop()
		var cfg Config
		if err := envdecode.Decode(&cfg); err != nil {
			b.Fatal(err)
		}
		unset(m)
	}
}

func BenchmarkGodotenvCleanenv(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := loadEnvInLoop()
		var cfg Config
		if err := cleanenv.ReadEnv(&cfg); err != nil {
			b.Fatal(err)
		}
		unset(m)
	}
}
