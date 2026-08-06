package oneenv

import (
	"strings"
	"testing"
)

func TestExampleGen(t *testing.T) {
	type DB struct {
		URL string `env:"URL,required" desc:"database connection string"`
	}
	type Config struct {
		Port   int    `env:"PORT" default:"8080" desc:"listen port"`
		Token  string `env:"TOKEN,secret" default:"hidden"`
		DB     DB     `env-prefix:"DB_"`
		NoDesc string `env:"NO_DESC"`
	}

	var sb strings.Builder
	if err := Example[Config](&sb); err != nil {
		t.Fatal(err)
	}
	got := sb.String()

	want := []string{
		"PORT=",               // never a value, whatever the default
		"type: int",           //
		"default: 8080",       // the default is described, not assigned
		"listen port",         // the desc tag rides along
		"TOKEN=",              //
		"secret",              //
		"DB_URL=",             // nested struct, prefixed
		"required",            //
		"database connection", //
		"NO_DESC=",            //
		"\n\n# DB\n",          // its own section, two blank lines above
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("output missing %q:\n%s", w, got)
		}
	}
	// No key may carry a value: the file is a form to fill in.
	for _, l := range strings.Split(got, "\n") {
		if k, v, ok := strings.Cut(l, "="); ok && strings.TrimSpace(strings.SplitN(v, "#", 2)[0]) != "" {
			t.Errorf("key %s was written with a value: %q", k, l)
		}
	}
	if strings.Contains(got, "hidden") {
		t.Errorf("secret default leaked:\n%s", got)
	}
}

func TestExampleGenNotStruct(t *testing.T) {
	var sb strings.Builder
	if err := Example[int](&sb); err != ErrNotAStruct {
		t.Fatalf("got %v, want ErrNotAStruct", err)
	}
}
