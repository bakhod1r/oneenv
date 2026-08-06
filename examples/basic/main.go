// Example: load a .env file straight into a struct.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/bakhod1r/oneenv"
)

type Config struct {
	Host    string        `env:"HOST" default:"localhost"`
	Port    int           `env:"PORT" default:"8080"`
	Debug   bool          `env:"DEBUG"`
	Timeout time.Duration `env:"TIMEOUT" default:"5s"`
	Tags    []string      `env:"TAGS" separator:","`
}

func main() {
	var cfg Config
	if err := oneenv.Load(&cfg, oneenv.WithFiles(".env", ".env.local")); err != nil {
		// AsParseError / AsFieldError return the typed error and a bool, so it
		// can be used in the same expression that finds it.
		if pe, ok := oneenv.AsParseError(err); ok {
			log.Fatalf("config %s:%d: %s", pe.File, pe.Line, pe.Msg)
		}
		if fe, ok := oneenv.AsFieldError(err); ok {
			log.Fatalf("config field %s (env %s): %v", fe.Field, fe.Key, fe.Err)
		}
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", cfg)
}
