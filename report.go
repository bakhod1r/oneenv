package oneenv

import (
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

// Entry is the full resolution detail of one configuration key: what it
// resolved to, where that came from, and what the struct asked for. A secret's
// Value is already masked, so an Entry is safe to print or log as it stands.
type Entry struct {
	Key      string // env key, including any prefix
	Field    string // struct field path, e.g. "DB.Host"
	Value    string // resolved value, masked when Secret is true
	Source   Source // env, file, default or unset
	File     string // .env file that supplied the value, when Source is SourceFile
	Default  string // the `default` tag, empty when there is none
	Type     string // Go type of the field
	Required bool
	Secret   bool
}

// Origin describes where a value came from in one word: the .env file name when
// it came from a file, otherwise the layer ("env", "default", "unset").
func (e Entry) Origin() string {
	if e.Source == SourceFile && e.File != "" {
		return e.File
	}
	return string(e.Source)
}

// Report is the record of one Load: every key it resolved, where each value
// came from, and what the struct declared. Install it with WithReport.
//
// A Report is filled once and then read; it is not safe for concurrent writes,
// but reading a completed Report from several goroutines is fine.
type Report struct {
	entries []Entry
	index   map[string]int
}

// recorder collects entries during a decode. It is the internal half of Report,
// used even when the caller asked only for a table.
type recorder struct{ entries []Entry }

// Entries returns every resolved key, sorted by key name.
func (r *Report) Entries() []Entry {
	if r == nil {
		return nil
	}
	out := make([]Entry, len(r.entries))
	copy(out, r.entries)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Lookup returns the entry for an env key.
func (r *Report) Lookup(key string) (Entry, bool) {
	if r == nil {
		return Entry{}, false
	}
	if i, ok := r.index[key]; ok {
		return r.entries[i], true
	}
	return Entry{}, false
}

// Source returns where the value for key came from: the .env file name, or
// "env", "default" or "unset". The second result is false when the key is not
// part of the loaded struct at all.
//
//	src, ok := rep.Source("PORT") // ".env.production"
func (r *Report) Source(key string) (string, bool) {
	e, ok := r.Lookup(key)
	if !ok {
		return "", false
	}
	return e.Origin(), true
}

// Explain renders everything known about one key, for debugging a value that is
// not what you expected:
//
//	Key      : PORT
//	Value    : 8080
//	Source   : .env.local
//	Default  : 8000
//	Required : true
//	Type     : int
//
// An unknown key yields a single line saying so.
func (r *Report) Explain(key string) string {
	e, ok := r.Lookup(key)
	if !ok {
		return "unknown key: " + key
	}
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 4, 1, ' ', 0)
	row := func(name, value string) { _, _ = io.WriteString(tw, name+"\t: "+value+"\n") }
	row("Key", e.Key)
	row("Value", displayValue(e.Value))
	row("Source", e.Origin())
	if e.Field != "" {
		row("Field", e.Field)
	}
	if e.Default != "" {
		row("Default", e.Default)
	}
	row("Required", boolWord(e.Required))
	row("Secret", boolWord(e.Secret))
	row("Type", e.Type)
	_ = tw.Flush()
	return b.String()
}

// String renders the whole report as the same KEY / VALUE / SOURCE table that
// WithTable prints.
func (r *Report) String() string {
	var b strings.Builder
	_ = writeTable(&b, r.Entries())
	return b.String()
}

func boolWord(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// writeTable renders entries as an aligned KEY / VALUE / SOURCE table.
func writeTable(w io.Writer, entries []Entry) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	// tabwriter buffers, so intermediate writes never surface an error; Flush
	// reports it.
	_, _ = io.WriteString(tw, "KEY\tVALUE\tSOURCE\n")
	for _, e := range entries {
		_, _ = io.WriteString(tw, e.Key+"\t"+displayValue(e.Value)+"\t"+e.Origin()+"\n")
	}
	return tw.Flush()
}

// finish transfers the recorded entries into the caller's Report and prints the
// table, when either was asked for. It runs only after a successful decode.
func (c config) finish() error {
	if c.rec == nil {
		return nil
	}
	entries := c.rec.entries
	if c.report != nil {
		c.report.entries = entries
		c.report.index = make(map[string]int, len(entries))
		for i, e := range entries {
			c.report.index[e.Key] = i
		}
	}
	if !c.table {
		return nil
	}
	sorted := make([]Entry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })
	return writeTable(c.writer(), sorted)
}

// Print writes the decoded configuration in v as an aligned KEY / VALUE table.
// Secret fields — those marked ",secret" and every Secret[T] — are masked in
// full unless WithSecretReveal opens a window on them.
//
//	oneenv.Print(cfg)
//
//	KEY       VALUE
//	API_KEY   ****
//	HOST      localhost
//	PORT      8080
//
// Output goes to os.Stdout unless WithOutput redirects it. Pass the same
// WithPrefix or WithTagKey used for the Load so the keys match.
func Print(v any, opts ...Option) error {
	cfg := newConfig(opts)
	m, err := marshalMasked(v, func(s string) string { return maskSecret(s, cfg.reveal()) }, opts...)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	tw := tabwriter.NewWriter(cfg.writer(), 0, 4, 2, ' ', 0)
	_, _ = io.WriteString(tw, "KEY\tVALUE\n")
	for _, k := range keys {
		_, _ = io.WriteString(tw, k+"\t"+displayValue(m[k])+"\n")
	}
	return tw.Flush()
}

// displayValue renders an empty value as a visible placeholder, so a blank cell
// is not mistaken for a missing key, and keeps a multiline value on one row.
func displayValue(s string) string {
	if s == "" {
		return `""`
	}
	return strings.NewReplacer("\n", "\\n", "\r", "\\r", "\t", "\\t").Replace(s)
}
