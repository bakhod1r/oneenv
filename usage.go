package oneenv

import (
	"fmt"
	"io"
	"reflect"
	"text/tabwriter"
)

// Usage writes a human-readable table of the environment variables that a
// struct of type T consumes: the key, Go type, whether it is required, its
// default, and the "desc" tag. It is handy for --help output.
//
//	oneenv.Usage[Config](os.Stdout)
func Usage[T any](w io.Writer, opts ...Option) error {
	cfg := newConfig(opts)
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil || t.Kind() != reflect.Struct {
		return ErrNotAStruct
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	// tabwriter buffers, so intermediate writes never surface an error; the
	// underlying writer's error is reported by Flush.
	_, _ = fmt.Fprintln(tw, "KEY\tTYPE\tREQUIRED\tDEFAULT\tDESCRIPTION")
	if err := writeUsage(tw, t, cfg.prefix, cfg); err != nil {
		return err
	}
	return tw.Flush()
}

func writeUsage(w io.Writer, t reflect.Type, prefix string, cfg config) error {
	schema, err := schemaFor(t, cfg)
	if err != nil {
		return err
	}
	for i := range schema.fields {
		fp := &schema.fields[i]
		ft := t.FieldByIndex(fp.index).Type
		if fp.nested {
			if err := writeUsage(w, ft, prefix+fp.envPrefix, cfg); err != nil {
				return err
			}
			continue
		}
		req := "no"
		if fp.required || cfg.requiredAll {
			req = "yes"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", prefix+fp.key, ft.String(), req, fp.defval, fp.desc)
	}
	return nil
}
