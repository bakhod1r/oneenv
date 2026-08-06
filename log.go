package oneenv

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Source names the layer a resolved value came from. When the value came from a
// file, Entry.File carries the file name; Source is then SourceFile.
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
// came from. It is consulted only when a report, logger or table asked for it,
// so the ordinary decode path pays nothing for it.
type sourceLookuper interface {
	LookupSource(key string) (value string, src Source, file string, ok bool)
}

// lookupSource reports the origin of key when src can tell, treating any other
// Lookuper as the environment layer.
func lookupSource(src Lookuper, key string) (string, Source, string, bool) {
	if s, ok := src.(sourceLookuper); ok {
		return s.LookupSource(key)
	}
	v, ok := src.Lookup(key)
	if !ok {
		return "", SourceUnset, "", false
	}
	return v, SourceEnv, "", true
}

// LookupSource implements sourceLookuper for the decoder's layered source,
// naming the .env file a value came from.
func (l layeredSource) LookupSource(key string) (string, Source, string, bool) {
	envKey := l.prefix + key
	if l.override {
		if v, ok := l.file[key]; ok {
			return v, SourceFile, l.origin[key], true
		}
	}
	if v, ok := l.env.Lookup(envKey); ok {
		return v, SourceEnv, "", true
	}
	if v, ok := l.file[key]; ok {
		return v, SourceFile, l.origin[key], true
	}
	return "", SourceUnset, "", false
}

// LookupSource implements sourceLookuper, propagating the request down the
// chain so prefixed and nested fields report the same origins.
func (p PrefixLookuper) LookupSource(key string) (string, Source, string, bool) {
	return lookupSource(p.Next, p.Prefix+key)
}

// redactedMask is defined in marshal.go and is the placeholder written in place
// of a secret value.

// reveal returns how many characters of a secret stay visible at each end.
// Secrets are masked in full unless WithSecretReveal opened a window, and
// WithRedacted closes it again whatever the order of the options.
func (c config) reveal() int {
	if c.redacted || !c.secretRevealSet {
		return 0
	}
	return c.secretReveal
}

// logValue renders a value for output: a non-secret value passes through
// untouched, a secret is masked according to the reveal policy.
func (c config) logValue(secret bool, value string) string {
	// An empty secret has nothing to hide, and printing "****" for it would
	// claim a value is set when none is.
	if !secret || value == "" {
		return value
	}
	return maskSecret(value, c.reveal())
}

// maskSecret keeps the first and last n characters of s visible and replaces
// the middle with the mask.
//
// A value too short to give both ends n characters gets a narrower window
// rather than nothing, so a short secret is still recognizable in a table:
// at most half of it is ever shown, split evenly between the two ends. An
// eight-character secret with n=4 therefore shows two characters at each end,
// not four. A value with no room even for that is masked in full. Counting is
// in runes, so a multi-byte value is never cut mid-character.
func maskSecret(s string, n int) string {
	if n == 0 {
		return redactedMask
	}
	r := []rune(s)
	// n characters at each end must not add up to more than half the value.
	if half := len(r) / 4; n > half {
		n = half
	}
	if n <= 0 {
		return redactedMask
	}
	return string(r[:n]) + redactedMask + string(r[len(r)-n:])
}

// tracing reports whether this call needs per-field reporting at all. When it
// is false the decoder skips the source lookup the reporting would need.
func (c config) tracing() bool {
	return c.logger != nil || c.table || c.report != nil
}

// writer returns where printed output goes: the WithOutput writer, or stdout.
func (c config) writer() io.Writer {
	if c.out != nil {
		return c.out
	}
	return os.Stdout
}

// logField records one resolved field: into the report/table recorder, and into
// the slog logger when one is installed.
func (c config) logField(e Entry) {
	e.Null = e.Value == ""
	e.Value = c.logValue(e.Secret, e.Value)
	if c.rec != nil {
		c.rec.entries = append(c.rec.entries, e)
	}
	if c.logger == nil {
		return
	}
	attrs := []any{
		slog.String("key", e.Key),
		slog.String("field", e.Field),
		slog.String("source", string(e.Source)),
		slog.String("value", e.Value),
		slog.String("type", e.Type),
		slog.Bool("null", e.Null),
		slog.Bool("secret", e.Secret),
	}
	if e.File != "" {
		attrs = append(attrs, slog.String("file", e.File))
	}
	c.logger.Debug("oneenv: field resolved", attrs...)
}

// logFiles records which .env files were considered for this Load.
func (c config) logFiles(files []string) {
	if c.logger == nil {
		return
	}
	c.logger.Debug("oneenv: loading", slog.String("files", strings.Join(files, ", ")))
}

// logDeprecated warns that a value was taken from a deprecated key. Unlike the
// per-field debug records this is a warning: it asks the operator to act.
func (c config) logDeprecated(key, msg string) {
	if c.logger == nil {
		return
	}
	c.logger.Warn("oneenv: deprecated key", slog.String("key", key), slog.String("hint", msg))
}
