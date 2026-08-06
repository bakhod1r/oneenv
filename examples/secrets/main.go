// Example: keep sensitive values out of logs.
//
// Secret[T] fields, ",secret" fields and ",noexpand" fields are never rewritten
// by expansion, so a password full of "$" decodes literally. Secret[T] also
// redacts itself in %v, %#v, JSON and text output — you have to ask for the
// value explicitly with .Value().
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/bakhod1r/oneenv"
)

type Config struct {
	// Redacted everywhere it is printed.
	Password oneenv.Secret[string] `env:"DB_PASSWORD,required"`

	// Read from a file path instead of the environment — Docker and
	// Kubernetes mount secrets at /run/secrets/<name>.
	APIKey oneenv.Secret[string] `env:"API_KEY,file"`

	// Plain string, but still never expanded.
	DSN string `env:"DATABASE_URL,noexpand"`

	// Ordinary field: "$HOME" here would expand.
	Host string `env:"HOST" default:"localhost"`
}

func main() {
	cfg, err := oneenv.Parse[Config](
		oneenv.WithFiles(".env"),
		oneenv.WithExpand(),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Printing the struct never leaks the secrets.
	fmt.Printf("%+v\n", *cfg)

	out, _ := json.Marshal(cfg)
	fmt.Println(string(out))

	// Redacted renders the struct back to .env with secrets masked — safe to
	// dump into a bug report.
	dump, err := oneenv.Redacted(cfg)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(dump))

	// The real value, only where you need it.
	connect(cfg.Password.Value())
}

func connect(password string) {
	_ = password // hand it to your database driver
}
