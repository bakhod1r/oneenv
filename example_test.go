package oneenv_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bakhod1r/oneenv"
)

// Example shows the common case: parse configuration straight into a struct.
func Example() {
	type Config struct {
		Port int      `env:"PORT" default:"8080"`
		Host string   `env:"HOST,required"`
		Tags []string `env:"TAGS" separator:","`
	}

	cfg, err := oneenv.Parse[Config](
		oneenv.WithLookuper(oneenv.MapLookuper{"HOST": "localhost", "TAGS": "a,b,c"}),
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s:%d %v\n", cfg.Host, cfg.Port, cfg.Tags)
	// Output: localhost:8080 [a b c]
}

func ExampleUnmarshal() {
	type Config struct {
		Host    string        `env:"HOST" default:"localhost"`
		Port    int           `env:"PORT" default:"8080"`
		Timeout time.Duration `env:"TIMEOUT" default:"5s"`
	}

	src := []byte("HOST=example.com\nPORT=9090")

	var cfg Config
	if err := oneenv.Unmarshal(src, &cfg); err != nil {
		panic(err)
	}
	fmt.Printf("%s:%d timeout=%s\n", cfg.Host, cfg.Port, cfg.Timeout)
	// Output: example.com:9090 timeout=5s
}

func ExampleParse() {
	type Config struct {
		Name string `env:"NAME"`
	}

	cfg, err := oneenv.Parse[Config](
		oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{"NAME": "myapp"}),
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(cfg.Name)
	// Output: myapp
}

// ExampleParse_errorHandling shows how a single Parse reports every problem,
// each retrievable as a typed *FieldError.
func ExampleParse_errorHandling() {
	type Config struct {
		Host string `env:"HOST,required"`
	}

	_, err := oneenv.Parse[Config](oneenv.WithLookuper(oneenv.MapLookuper{}))

	var fe *oneenv.FieldError
	if errors.As(err, &fe) {
		fmt.Printf("%s is required\n", fe.Key)
	}
	// Output: HOST is required
}

// ExampleParse_envTags shows the env-* tag aliases, which work alongside the
// native tags and take priority when both are present.
func ExampleParse_envTags() {
	type Config struct {
		Port int      `env:"PORT" env-default:"8080"`
		Tags []string `env:"TAGS" env-separator:";"`
	}

	cfg, err := oneenv.Parse[Config](
		oneenv.WithLookuper(oneenv.MapLookuper{"TAGS": "a;b;c"}),
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%d %v\n", cfg.Port, cfg.Tags)
	// Output: 8080 [a b c]
}

func ExampleWithExpand() {
	type Config struct {
		URL string `env:"URL"`
	}

	src := []byte("HOST=db\nURL=postgres://${HOST}:5432")

	var cfg Config
	if err := oneenv.Unmarshal(src, &cfg, oneenv.WithExpand()); err != nil {
		panic(err)
	}
	fmt.Println(cfg.URL)
	// Output: postgres://db:5432
}

// ExampleWithTypeParser registers a parser for a type that has no built-in
// support (here net.IP). It also applies inside slices, maps and pointers.
func ExampleWithTypeParser() {
	type Config struct {
		Addr net.IP `env:"ADDR"`
	}

	cfg, err := oneenv.Parse[Config](
		oneenv.WithLookuper(oneenv.MapLookuper{"ADDR": "10.0.0.1"}),
		oneenv.WithTypeParser(func(s string) (net.IP, error) { return net.ParseIP(s), nil }),
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(cfg.Addr)
	// Output: 10.0.0.1
}

// ExampleWithMutator transforms every value before it is decoded.
func ExampleWithMutator() {
	type Config struct {
		Name string `env:"NAME"`
	}

	cfg, err := oneenv.ParseContext[Config](context.Background(),
		oneenv.WithLookuper(oneenv.MapLookuper{"NAME": "app"}),
		oneenv.WithMutator(func(_ context.Context, _, v string) (string, error) {
			return strings.ToUpper(v), nil
		}),
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(cfg.Name)
	// Output: APP
}

// ExampleWithValidator runs a validation callback on the decoded struct,
// letting you plug in any validator without adding a dependency.
func ExampleWithValidator() {
	type Config struct {
		Port int `env:"PORT"`
	}

	_, err := oneenv.Parse[Config](
		oneenv.WithLookuper(oneenv.MapLookuper{"PORT": "70000"}),
		oneenv.WithValidator(func(v any) error {
			if v.(*Config).Port > 65535 {
				return errors.New("port out of range")
			}
			return nil
		}),
	)
	fmt.Println(err)
	// Output: port out of range
}

// ExampleMarshal renders a struct back into .env bytes.
func ExampleMarshal() {
	type Config struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	}

	data, err := oneenv.Marshal(Config{Host: "localhost", Port: 5432})
	if err != nil {
		panic(err)
	}
	fmt.Print(string(data))
	// Output:
	// HOST=localhost
	// PORT=5432
}

// ExampleRedacted renders a struct to .env bytes with ",secret" fields masked,
// so a configuration can be logged without leaking sensitive values.
func ExampleRedacted() {
	type Config struct {
		Host     string `env:"HOST"`
		Password string `env:"PASSWORD,secret"`
	}

	cfg := Config{Host: "db", Password: "hunter2"}
	data, err := oneenv.Redacted(cfg)
	if err != nil {
		panic(err)
	}
	fmt.Print(string(data))
	// Output:
	// HOST=db
	// PASSWORD=****
}

// ExampleSecret wraps a sensitive value so it never prints in the clear, while
// the real value stays available through Value.
func ExampleSecret() {
	type Config struct {
		APIKey oneenv.Secret[string] `env:"API_KEY"`
	}

	cfg, err := oneenv.Parse[Config](
		oneenv.WithLookuper(oneenv.MapLookuper{"API_KEY": "s3cr3t"}),
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("masked=%v real=%s\n", cfg.APIKey, cfg.APIKey.Value())
	// Output: masked=**** real=s3cr3t
}

// ExampleParse_sliceOfStructs decodes a repeated struct from indexed keys.
func ExampleParse_sliceOfStructs() {
	type Server struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	}
	type Config struct {
		Servers []Server `env:"SERVER"`
	}

	cfg, err := oneenv.Parse[Config](oneenv.WithLookuper(oneenv.MapLookuper{
		"SERVER_0_HOST": "a", "SERVER_0_PORT": "1",
		"SERVER_1_HOST": "b", "SERVER_1_PORT": "2",
	}))
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s:%d %s:%d\n", cfg.Servers[0].Host, cfg.Servers[0].Port, cfg.Servers[1].Host, cfg.Servers[1].Port)
	// Output: a:1 b:2
}

// ExampleUsage prints a table of the variables a struct consumes, handy for a
// --help flag.
func ExampleUsage() {
	type Config struct {
		Port int    `env:"PORT" default:"8080" desc:"listen port"`
		Host string `env:"HOST,required" desc:"bind address"`
	}

	_ = oneenv.Usage[Config](os.Stdout)
	// Output:
	// KEY   TYPE    REQUIRED  DEFAULT  DESCRIPTION
	// PORT  int     no        8080     listen port
	// HOST  string  yes                bind address
}

// ExampleParse_defaults shows how the `default` tag fills a field when no
// source provides a value. Anything present in the environment or file wins
// over the default.
func ExampleParse_defaults() {
	type Config struct {
		Host string `env:"HOST" default:"localhost"`
		Port int    `env:"PORT" default:"8080"`
	}

	cfg, err := oneenv.Parse[Config](
		oneenv.WithLookuper(oneenv.MapLookuper{"PORT": "9090"}), // HOST falls back
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s:%d\n", cfg.Host, cfg.Port)
	// Output: localhost:9090
}

// ExampleParse_nested decodes a nested struct. The envPrefix tag prefixes every
// key inside it, so DB.Host reads from DB_HOST.
func ExampleParse_nested() {
	type DB struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	}
	type Config struct {
		Name string `env:"NAME"`
		DB   DB     `envPrefix:"DB_"`
	}

	cfg, err := oneenv.Parse[Config](oneenv.WithLookuper(oneenv.MapLookuper{
		"NAME":    "api",
		"DB_HOST": "db.internal",
		"DB_PORT": "5432",
	}))
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s -> %s:%d\n", cfg.Name, cfg.DB.Host, cfg.DB.Port)
	// Output: api -> db.internal:5432
}

// ExampleParse_types shows the built-in support for durations, booleans, slices
// and maps. Slices split on the separator; maps take key:value pairs.
func ExampleParse_types() {
	type Config struct {
		Debug   bool           `env:"DEBUG"`
		Timeout time.Duration  `env:"TIMEOUT"`
		Tags    []string       `env:"TAGS" separator:","`
		Limits  map[string]int `env:"LIMITS" separator:","` // key:value,key:value
	}

	cfg, err := oneenv.Parse[Config](oneenv.WithLookuper(oneenv.MapLookuper{
		"DEBUG":   "true",
		"TIMEOUT": "1m30s",
		"TAGS":    "web,api,worker",
		"LIMITS":  "cpu:8,mem:16",
	}))
	if err != nil {
		panic(err)
	}
	fmt.Printf("debug=%t timeout=%s tags=%v cpu=%d\n",
		cfg.Debug, cfg.Timeout, cfg.Tags, cfg.Limits["cpu"])
	// Output: debug=true timeout=1m30s tags=[web api worker] cpu=8
}

// ExampleParse_time parses a time.Time with a custom layout.
func ExampleParse_time() {
	type Config struct {
		Release time.Time `env:"RELEASE" layout:"2006-01-02"`
	}

	cfg, err := oneenv.Parse[Config](
		oneenv.WithLookuper(oneenv.MapLookuper{"RELEASE": "2026-07-18"}),
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(cfg.Release.Year())
	// Output: 2026
}

// ExampleWithPrefix restricts every lookup to a prefix, so env:"PORT" reads
// from APP_PORT. Handy when one process hosts several components.
func ExampleWithPrefix() {
	type Config struct {
		Port int `env:"PORT"`
	}

	cfg, err := oneenv.Parse[Config](
		oneenv.WithPrefix("APP_"),
		oneenv.WithLookuper(oneenv.MapLookuper{"APP_PORT": "9090"}),
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(cfg.Port)
	// Output: 9090
}

// ExampleWithEnvFiles shows the environment-aware cascade: on top of the base
// file it also reads <base>.<env>, so with APP_ENV=production the
// production override wins.
func ExampleWithEnvFiles() {
	dir, _ := os.MkdirTemp("", "oneenv")
	defer os.RemoveAll(dir)
	base := filepath.Join(dir, ".env")
	_ = os.WriteFile(base, []byte("HOST=localhost\nPORT=8080\n"), 0o600)
	_ = os.WriteFile(base+".production", []byte("HOST=prod.internal\n"), 0o600)

	type Config struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	}

	cfg, err := oneenv.Parse[Config](
		oneenv.WithFiles(base),
		oneenv.WithEnvFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{"APP_ENV": "production"}),
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s:%d\n", cfg.Host, cfg.Port)
	// Output: prod.internal:8080
}

// ExampleRedactedMap masks secret fields while returning a map, handy for
// structured logging.
func ExampleRedactedMap() {
	type Config struct {
		User     string `env:"USER"`
		Password string `env:"PASSWORD,secret"`
	}

	m, err := oneenv.RedactedMap(Config{User: "admin", Password: "hunter2"})
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s / %s\n", m["USER"], m["PASSWORD"])
	// Output: admin / ****
}

// ExampleRead parses a .env file into a plain map without touching the process
// environment or a struct.
func ExampleRead() {
	dir, _ := os.MkdirTemp("", "oneenv")
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, ".env")
	_ = os.WriteFile(path, []byte("HOST=localhost\nPORT=5432\n"), 0o600)

	m, err := oneenv.Read(path)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s:%s\n", m["HOST"], m["PORT"])
	// Output: localhost:5432
}
