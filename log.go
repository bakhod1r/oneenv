package oneenv

import (
	"io"
	"log/slog"
	"sort"
	"strings"
	"text/tabwriter"
)

// Source names where a resolved value came from. It is reported by the logger
// installed with WithLogger, so a startup log answers "which layer won?" for
// every key.
type Source string

const (
	// SourceEnv means the value came from the process environment (or whatever
	// Lookuper was installed with WithLookuper).
	SourceEnv Source = "env"
	// SourceFile means the value came from a parsed .env file.
	SourceFile Source = "file"
	// SourceDefault means no value was found and the `default` tag was used.
	SourceDefault Source = "default"
	// SourceUnset means no value was found at all; the field keeps its zero value.
	SourceUnset Source = "unset"
)

// sourceLookuper is implemented by sources that can report which layer a value
// came from. It is consulted only when a logger is installed, so the ordinary
// decode path pays nothing for it.
type sourceLookuper interface {
	LookupSource(key string) (value string, src Source, ok bool)
}

// lookupSource reports the origin of key when src can tell, treating any other
// Lookuper as the environment layer.
func lookupSource(src Lookuper, key string) (string, Source, bool) {
	if s, ok := src.(sourceLookuper); ok {
		return s.LookupSource(key)
	}
	v, ok := src.Lookup(key)
	if !ok {
		return "", SourceUnset, false
	}
	return v, SourceEnv, true
}

// LookupSource implements sourceLookuper for the decoder's layered source.
func (l layeredSource) LookupSource(key string) (string, Source, bool) {
	envKey := l.prefix + key
	if l.override {
		if v, ok := l.file[key]; ok {
			return v, SourceFile, true
		}
	}
	if v, ok := l.env.Lookup(envKey); ok {
		return v, SourceEnv, true
	}
	if v, ok := l.file[key]; ok {
		return v, SourceFile, true
	}
	return "", SourceUnset, false
}

// LookupSource implements sourceLookuper, propagating the request down the
// chain so prefixed and nested fields report the same origins.
func (p PrefixLookuper) LookupSource(key string) (string, Source, bool) {
	return lookupSource(p.Next, p.Prefix+key)
}

// defaultSecretReveal is how many leading and trailing characters of a secret
// stay visible when WithSecretReveal is not used.
const defaultSecretReveal = 4

// reveal returns the configured number of visible characters per end.
func (c config) reveal() int {
	if c.secretRevealSet {
		return c.secretReveal
	}
	return defaultSecretReveal
}

// logValue renders a value for logging: a non-secret value passes through
// untouched, a secret is partially masked.
func (c config) logValue(secret bool, value string) string {
	if !secret {
		return value
	}
	return maskSecret(value, c.reveal())
}

// maskSecret keeps the first and last n characters of s visible and replaces
// the middle with the mask. When s is too short for both ends to show without
// touching (or n is 0) the whole value is masked, so a short secret is never
// effectively printed in full. Counting is in runes, so a multi-byte value is
// never cut mid-character.
func maskSecret(s string, n int) string {
	if n == 0 {
		return redactedMask
	}
	r := []rune(s)
	// Require at least one hidden rune between the two visible ends.
	if len(r) < 2*n+1 {
		return redactedMask
	}
	return string(r[:n]) + redactedMask + string(r[len(r)-n:])
}

// fieldRecord is one resolved field, as reported to the logger and rendered by
// the WithTable output. Value is already masked for secrets.
type fieldRecord struct {
	Key    string
	Field  string
	Source Source
	Value  string
	Secret bool
}

// recorder collects fieldRecords during a decode so they can be rendered as a
// table once decoding succeeds.
type recorder struct{ entries []fieldRecord }

// tracing reports whether this call needs per-field reporting at all. When it
// is false the decoder skips every source lookup the reporting would need.
func (c config) tracing() bool { return c.logger != nil || c.table != nil }

// logField emits one record per decoded field: to the slog logger when one is
// installed, and to the table recorder when WithTable is in use.
func (c config) logField(key, field string, src Source, value string, secret bool) {
	value = c.logValue(secret, value)
	if c.rec != nil {
		c.rec.entries = append(c.rec.entries, fieldRecord{
			Key: key, Field: field, Source: src, Value: value, Secret: secret,
		})
	}
	if c.logger == nil {
		return
	}
	c.logger.Debug("oneenv: field resolved",
		slog.String("key", key),
		slog.String("field", field),
		slog.String("source", string(src)),
		slog.String("value", value),
		slog.Bool("secret", secret),
	)
}

// logFiles records which .env files were considered for this Load.
func (c config) logFiles(files []string) {
	if c.logger == nil {
		return
	}
	c.logger.Debug("oneenv: loading", slog.String("files", strings.Join(files, ", ")))
}

// Print writes the decoded configuration in v to w as an aligned KEY / VALUE
// table, with every field marked ",secret" (or of type Secret[T]) partially
// masked — the first and last characters stay visible, the middle becomes
// "****". It is the human-readable counterpart of Redacted, meant for a startup
// banner:
//
//	oneenv.Print(os.Stdout, cfg)
//
//	KEY       VALUE
//	API_KEY   sk-l****9f2a
//	HOST      localhost
//	PORT      8080
//
// Options are accepted so the same WithPrefix or WithTagKey used for Load
// produces matching keys, and WithSecretReveal controls how much of a secret is
// shown.
func Print(w io.Writer, v any, opts ...Option) error {
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

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	// tabwriter buffers, so intermediate writes never surface an error; Flush
	// reports it.
	_, _ = io.WriteString(tw, "KEY\tVALUE\n")
	for _, k := range keys {
		_, _ = io.WriteString(tw, k+"\t"+displayValue(m[k])+"\n")
	}
	return tw.Flush()
}

// printTable renders the recorded fields as a KEY / VALUE / SOURCE table. It is
// called by Load once decoding succeeds, when WithTable installed a writer.
func (c config) printTable() error {
	if c.table == nil || c.rec == nil {
		return nil
	}
	entries := make([]fieldRecord, len(c.rec.entries))
	copy(entries, c.rec.entries)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })

	tw := tabwriter.NewWriter(c.table, 0, 4, 2, ' ', 0)
	_, _ = io.WriteString(tw, "KEY\tVALUE\tSOURCE\n")
	for _, e := range entries {
		_, _ = io.WriteString(tw, e.Key+"\t"+displayValue(e.Value)+"\t"+string(e.Source)+"\n")
	}
	return tw.Flush()
}

// displayValue renders an empty value as a visible placeholder, so a blank line
// is not mistaken for a missing key.
func displayValue(s string) string {
	if s == "" {
		return `""`
	}
	return strings.NewReplacer("\n", "\\n", "\r", "\\r", "\t", "\\t").Replace(s)
}
