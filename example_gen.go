package oneenv

import (
	"fmt"
	"io"
	"reflect"
)

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
	return writeExample(w, t, cfg.prefix, cfg)
}

func writeExample(w io.Writer, t reflect.Type, prefix string, cfg config) error {
	schema, err := schemaFor(t, cfg)
	if err != nil {
		return err
	}
	for i := range schema.fields {
		fp := &schema.fields[i]
		ft := t.FieldByIndex(fp.index).Type
		if fp.nested {
			if err := writeExample(w, ft, prefix+fp.envPrefix, cfg); err != nil {
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
		val := ""
		if fp.hasDefant && !fp.secret {
			val = fp.defval
		}
		if _, err := fmt.Fprintf(w, "%s%s=%s\n\n", prefix, fp.key, val); err != nil {
			return err
		}
	}
	return nil
}
