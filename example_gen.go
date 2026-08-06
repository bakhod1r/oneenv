package oneenv

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// examplePathFor resolves where the generated example file goes. Without an
// explicit path it sits next to the first .env file this call reads, so ".env"
// documents itself as ".env.example" in the same directory.
func (c config) examplePathFor() string {
	path := c.examplePath
	if path == "" {
		base := ".env"
		if len(c.files) > 0 && c.files[0] != "" {
			base = c.files[0]
		}
		path = base + ".example"
	}
	if !filepath.IsAbs(path) && c.baseDir != "" {
		path = filepath.Join(c.baseDir, path)
	}
	return path
}

// writeExample writes the example file named by WithWriteExample, generated
// from the struct that was just decoded. It is a no-op when the option is not
// set, and it leaves the file alone when its contents would not change, so a
// directory watcher is not woken on every start.
func (c config) writeExample(v any, opts []Option) error {
	if !c.writeExampleOn {
		return nil
	}
	t := reflect.TypeOf(v)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return ErrNotAStruct
	}

	var buf bytes.Buffer
	if err := writeExampleTo(&buf, t, c.prefix, c); err != nil {
		return err
	}
	path := c.examplePathFor()
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, buf.Bytes()) {
		return nil
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// Example writes a ready-to-fill .env.example file for the environment
// variables that a struct of type T consumes. Each key is preceded by a
// comment carrying its description, Go type, and whether it is required;
// the value is the default when one is declared, and empty otherwise.
// Secret fields never leak their default value.
//
//	oneenv.Example[Config](os.Stdout)
func Example[T any](w io.Writer, opts ...Option) error {
	cfg := newConfig(opts)
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil || t.Kind() != reflect.Struct {
		return ErrNotAStruct
	}
	return writeExampleTo(w, t, cfg.prefix, cfg)
}

func writeExampleTo(w io.Writer, t reflect.Type, prefix string, cfg config) error {
	schema, err := schemaFor(t, cfg)
	if err != nil {
		return err
	}
	for i := range schema.fields {
		fp := &schema.fields[i]
		ft := t.FieldByIndex(fp.index).Type
		if fp.nested {
			if err := writeExampleTo(w, ft, prefix+fp.envPrefix, cfg); err != nil {
				return err
			}
			continue
		}
		if fp.desc != "" {
			if _, err := fmt.Fprintf(w, "# %s\n", fp.desc); err != nil {
				return err
			}
		}
		req := ""
		if fp.required || cfg.requiredAll {
			req = ", required"
		}
		if _, err := fmt.Fprintf(w, "# type: %s%s\n", ft.String(), req); err != nil {
			return err
		}
		if len(fp.enum) > 0 {
			if _, err := fmt.Fprintf(w, "# allowed: %s\n", strings.Join(fp.enum, ", ")); err != nil {
				return err
			}
		}
		if fp.patternSrc != "" {
			if _, err := fmt.Fprintf(w, "# pattern: %s\n", fp.patternSrc); err != nil {
				return err
			}
		}
		// The `example` tag wins over `default`: it exists precisely to show a
		// realistic value here. Neither is written for a secret.
		val := ""
		if !fp.secret {
			if fp.example != "" {
				val = fp.example
			} else if fp.hasDefant {
				val = fp.defval
			}
		}
		if _, err := fmt.Fprintf(w, "%s%s=%s\n\n", prefix, fp.key, val); err != nil {
			return err
		}
	}
	return nil
}
