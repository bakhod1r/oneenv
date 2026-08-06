package oneenv

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/tabwriter"
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

// exampleGroup is the block of keys contributed by one struct: the top-level
// struct, or a nested one. Keeping a group together in the output means the
// variables that belong to the same concern sit next to each other.
type exampleGroup struct {
	name string // struct field path of the nested struct, empty for the root
	rows []exampleRow
}

// exampleRow is one KEY= line and the inline comment describing it.
type exampleRow struct {
	key     string
	comment string
}

// writeExampleTo renders the example file: one blank-line-separated block per
// struct, each key written without a value and annotated in place.
//
//	# App
//	APP_ENV=   # type: string, required
//	APP_NAME=  # type: string, default: superapp
func writeExampleTo(w io.Writer, t reflect.Type, prefix string, cfg config) error {
	groups, err := exampleGroups(t, prefix, "", cfg)
	if err != nil {
		return err
	}
	// One tabwriter for the whole file keeps every comment in the same column,
	// including across groups, so the result reads as a single table.
	tw := tabwriter.NewWriter(w, 0, 4, 1, ' ', 0)
	for i, g := range groups {
		if len(g.rows) == 0 {
			continue
		}
		if i > 0 {
			// Two blank lines between structs: the gap should read as a section
			// break, not as the spacing inside one.
			_, _ = io.WriteString(tw, "\n\n")
		}
		if g.name != "" {
			_, _ = io.WriteString(tw, "# "+g.name+"\n")
		}
		for _, r := range g.rows {
			_, _ = io.WriteString(tw, r.key+"=\t"+r.comment+"\n")
		}
	}
	return tw.Flush()
}

// exampleGroups walks the struct, collecting one group per struct so nested
// configuration stays together instead of being interleaved.
func exampleGroups(t reflect.Type, prefix, path string, cfg config) ([]exampleGroup, error) {
	schema, err := schemaFor(t, cfg)
	if err != nil {
		return nil, err
	}
	group := exampleGroup{name: path}
	var nested []exampleGroup

	for i := range schema.fields {
		fp := &schema.fields[i]
		field := t.FieldByIndex(fp.index)
		if fp.nested {
			sub, err := exampleGroups(field.Type, prefix+fp.envPrefix, joinPath(path, field.Name), cfg)
			if err != nil {
				return nil, err
			}
			nested = append(nested, sub...)
			continue
		}
		group.rows = append(group.rows, exampleRow{
			key:     prefix + fp.key,
			comment: exampleComment(fp, field.Type, cfg),
		})
	}
	return append([]exampleGroup{group}, nested...), nil
}

// exampleComment describes one variable in a single inline comment: its type
// first, then whether it is required, then whatever else is worth knowing.
func exampleComment(fp *fieldPlan, ft reflect.Type, cfg config) string {
	parts := []string{"type: " + ft.String()}
	if fp.required || cfg.requiredAll {
		parts = append(parts, "required")
	}
	if fp.secret {
		parts = append(parts, "secret")
	}
	// A secret never carries its example or default into the file.
	if !fp.secret {
		if fp.example != "" {
			parts = append(parts, "example: "+fp.example)
		} else if fp.hasDefant && fp.defval != "" {
			parts = append(parts, "default: "+fp.defval)
		}
	}
	if len(fp.enum) > 0 {
		parts = append(parts, "allowed: "+strings.Join(fp.enum, "|"))
	}
	if fp.patternSrc != "" {
		parts = append(parts, "pattern: "+fp.patternSrc)
	}
	if fp.desc != "" {
		parts = append(parts, fp.desc)
	}
	return "# " + strings.Join(parts, ", ")
}
