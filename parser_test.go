package oneenv

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		expand bool
		want   map[string]string
	}{
		{
			name: "simple",
			src:  "FOO=bar\nBAZ=qux",
			want: map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name: "comments and blanks",
			src:  "# a comment\n\nFOO=bar\n  # indented\nBAZ=qux\n",
			want: map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name: "export prefix",
			src:  "export FOO=bar",
			want: map[string]string{"FOO": "bar"},
		},
		{
			name: "inline comment",
			src:  "FOO=bar # trailing",
			want: map[string]string{"FOO": "bar"},
		},
		{
			name: "hash without space is literal",
			src:  "URL=http://x#frag",
			want: map[string]string{"URL": "http://x#frag"},
		},
		{
			name: "single quotes are raw",
			src:  `FOO='bar $BAZ #x'`,
			want: map[string]string{"FOO": "bar $BAZ #x"},
		},
		{
			name: "double quotes with escapes",
			src:  `FOO="line1\nline2\t!"`,
			want: map[string]string{"FOO": "line1\nline2\t!"},
		},
		{
			name: "multiline double quoted",
			src:  "FOO=\"a\nb\"",
			want: map[string]string{"FOO": "a\nb"},
		},
		{
			name: "crlf line endings",
			src:  "FOO=bar\r\nBAZ=qux\r\n",
			want: map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name: "yaml style colon",
			src:  "FOO: bar",
			want: map[string]string{"FOO": "bar"},
		},
		{
			name:   "expansion",
			src:    "A=1\nB=${A}2",
			expand: true,
			want:   map[string]string{"A": "1", "B": "12"},
		},
		{
			name:   "no expansion by default",
			src:    "A=1\nB=${A}2",
			expand: false,
			want:   map[string]string{"A": "1", "B": "${A}2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := make(map[string]string)
			if err := parse("test", []byte(tc.src), tc.expand, got); err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"unterminated double quote", `FOO="bar`},
		{"unterminated single quote", `FOO='bar`},
		{"missing equals", "FOOBAR\n"},
		{"empty key", "=bar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := parse("test", []byte(tc.src), false, map[string]string{})
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			var pe *ParseError
			if !asParseError(err, &pe) {
				t.Fatalf("expected *ParseError, got %T", err)
			}
		})
	}
}

func asParseError(err error, target **ParseError) bool {
	pe, ok := err.(*ParseError)
	if ok {
		*target = pe
	}
	return ok
}

func FuzzParse(f *testing.F) {
	f.Add([]byte("FOO=bar\n# c\nBAZ=\"q\\nx\""))
	f.Add([]byte("export A: ${B}\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		out := make(map[string]string)
		_ = parse("fuzz", data, true, out) // must not panic
	})
}
