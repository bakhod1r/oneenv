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
		"# listen port\n# type: int\nPORT=8080\n",
		"TOKEN=\n", // secret default must not leak
		"# database connection string\n# type: string, required\nDB_URL=\n",
		"NO_DESC=\n",
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("output missing %q:\n%s", w, got)
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
