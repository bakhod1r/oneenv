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
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", cfg)
}
