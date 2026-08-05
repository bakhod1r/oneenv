package oneenv

import (
	"errors"
	"strings"
	"testing"
)

// The reported bug: with WithExpand, a password containing '$' was silently
// mangled — "pa$$word" collapsed to "pa$word", "pa$word" truncated to "pa", and
// "$ecret123" vanished entirely. Secret fields must decode from the literal.
func TestSecretFieldNeverExpanded(t *testing.T) {
	type Config struct {
		Password Secret[string] `env:"DB_PASSWORD"`
	}

	cases := []struct{ name, literal string }{
		{"double dollar", `pa$$word`},
		{"bare dollar", `pa$word`},
		{"leading dollar", `$ecret123`},
		{"braced reference", `${PATH}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg Config
			src := "DB_PASSWORD=" + tc.literal + "\n"
			if err := Unmarshal([]byte(src), &cfg, WithExpand()); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got := cfg.Password.Value(); got != tc.literal {
				t.Fatalf("got %q, want %q", got, tc.literal)
			}
		})
	}
}

func TestSecretTagNeverExpanded(t *testing.T) {
	type Config struct {
		Tagged   string `env:"A,secret"`
		Boolean  string `env:"B" env-secret:"true"`
		OptOut   string `env:"C,noexpand"`
		Explicit string `env:"D" env-noexpand:"true"`
	}
	var cfg Config
	src := "A=pa$$word\nB=pa$word\nC=$ecret123\nD=${PATH}\n"
	if err := Unmarshal([]byte(src), &cfg, WithExpand()); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg.Tagged != `pa$$word` || cfg.Boolean != `pa$word` ||
		cfg.OptOut != `$ecret123` || cfg.Explicit != `${PATH}` {
		t.Fatalf("%+v", cfg)
	}
}

// Ordinary fields must keep expanding, otherwise the fix would break the
// documented feature.
func TestNonSecretStillExpands(t *testing.T) {
	type Config struct {
		Host string `env:"HOST"`
		DSN  string `env:"DSN"`
	}
	var cfg Config
	src := "HOST=db.internal\nDSN=postgres://${HOST}/app\n"
	if err := Unmarshal([]byte(src), &cfg, WithExpand()); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg.DSN != "postgres://db.internal/app" {
		t.Fatalf("got %q", cfg.DSN)
	}
}

// A secret nested behind a prefix must still reach the raw values.
func TestSecretInNestedStructNeverExpanded(t *testing.T) {
	type DB struct {
		Password Secret[string] `env:"PASSWORD"`
	}
	type Config struct {
		DB DB `envPrefix:"DB_"`
	}
	var cfg Config
	if err := Unmarshal([]byte("DB_PASSWORD=pa$$word\n"), &cfg, WithExpand()); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got := cfg.DB.Password.Value(); got != `pa$$word` {
		t.Fatalf("got %q", got)
	}
}

// Without expansion enabled nothing changes: the raw map is unused and secrets
// still decode.
func TestSecretWithoutExpand(t *testing.T) {
	type Config struct {
		Password Secret[string] `env:"DB_PASSWORD"`
	}
	var cfg Config
	if err := Unmarshal([]byte("DB_PASSWORD=pa$$word\n"), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got := cfg.Password.Value(); got != `pa$$word` {
		t.Fatalf("got %q", got)
	}
}

// A process-environment value is never expanded, and must survive a noexpand
// field lookup unchanged.
func TestNoExpandFallsBackToEnvValue(t *testing.T) {
	type Config struct {
		Password string `env:"DB_PASSWORD,secret"`
	}
	cfg, err := Parse[Config](
		WithFiles(),
		WithExpand(),
		WithLookuper(MapLookuper{"DB_PASSWORD": `pa$$word`}),
	)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Password != `pa$$word` {
		t.Fatalf("got %q", cfg.Password)
	}
}

// Strict mode reports an undefined reference wherever it appears, including on
// a line that only a secret field reads: the parser expands before any field is
// known. The value fails loudly instead of vanishing, which is the point.
func TestExpandStrictReportsUnquotedSecretLine(t *testing.T) {
	type Config struct {
		Password Secret[string] `env:"DB_PASSWORD"`
	}
	var cfg Config
	err := Unmarshal([]byte("DB_PASSWORD=$ecret123\n"), &cfg, WithExpandStrict())
	if !errors.Is(err, ErrUnknownVariable) {
		t.Fatalf("got %v, want ErrUnknownVariable", err)
	}
}

func TestExpandStrictUnknownVariable(t *testing.T) {
	type Config struct {
		Password string `env:"DB_PASSWORD"`
	}
	var cfg Config
	err := Unmarshal([]byte("DB_PASSWORD=$ecret123\n"), &cfg, WithExpandStrict())
	if !errors.Is(err, ErrUnknownVariable) {
		t.Fatalf("got %v, want ErrUnknownVariable", err)
	}
	var perr *ParseError
	if !errors.As(err, &perr) || perr.Line != 1 {
		t.Fatalf("want *ParseError on line 1, got %#v", err)
	}
	if !strings.Contains(err.Error(), "ecret123") {
		t.Fatalf("error should name the variable: %v", err)
	}
}

func TestExpandStrictResolvesKnownVariable(t *testing.T) {
	type Config struct {
		DSN string `env:"DSN"`
	}
	var cfg Config
	src := "HOST=db.internal\nDSN=postgres://${HOST}/app\n"
	if err := Unmarshal([]byte(src), &cfg, WithExpandStrict()); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg.DSN != "postgres://db.internal/app" {
		t.Fatalf("got %q", cfg.DSN)
	}
}

// Strict mode must not fire on a secret, whose value is never expanded at all.
func TestExpandStrictIgnoresSecretValue(t *testing.T) {
	type Config struct {
		Password Secret[string] `env:"DB_PASSWORD"`
	}
	var cfg Config
	err := Unmarshal([]byte("DB_PASSWORD='$ecret123'\n"), &cfg, WithExpandStrict())
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got := cfg.Password.Value(); got != `$ecret123` {
		t.Fatalf("got %q", got)
	}
}
