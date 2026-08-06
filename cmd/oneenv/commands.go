package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/bakhod1r/oneenv"
)

// keyShape is the conventional spelling of an env key: upper snake case,
// optionally with digits. A key outside it still works, but is worth a warning.
var keyShape = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// doctor checks the health of a set of .env files and prints a report. It
// returns an error when something is actually wrong, so the exit code is usable
// in CI.
func doctor(w io.Writer, files []string) error {
	var problems int

	for _, f := range files {
		if exists(f) {
			fmt.Fprintf(w, "✓ %s found\n", f)
			continue
		}
		// A missing file is only a warning: the cascade makes every file but the
		// first optional, and even .env is allowed to be absent.
		fmt.Fprintf(w, "✗ %s not found\n", f)
	}
	fmt.Fprintln(w)

	seen := make(map[string]string) // key -> file it was first defined in
	for _, f := range files {
		lines, err := scanFile(f)
		if err != nil {
			continue
		}
		for _, l := range lines {
			if l.Invalid {
				fmt.Fprintf(w, "✗ %s:%d invalid line: %s\n", l.File, l.No, strings.TrimSpace(l.Raw))
				problems++
				continue
			}
			if l.Key == "" {
				continue
			}
			if prev, dup := seen[l.Key]; dup {
				fmt.Fprintf(w, "⚠ %s:%d duplicate key %s (already set in %s)\n", l.File, l.No, l.Key, prev)
			} else {
				seen[l.Key] = l.File
			}
			if !keyShape.MatchString(l.Key) {
				fmt.Fprintf(w, "⚠ %s:%d unconventional key name: %s\n", l.File, l.No, l.Key)
			}
			if l.Value == "" {
				fmt.Fprintf(w, "⚠ %s:%d %s is empty\n", l.File, l.No, l.Key)
			}
			if _, overridden := os.LookupEnv(l.Key); overridden {
				fmt.Fprintf(w, "⚠ %s is also set in the environment, which wins\n", l.Key)
			}
		}
	}

	keys := sortedKeys(seen)
	fmt.Fprintln(w)
	for _, k := range keys {
		fmt.Fprintf(w, "✓ %s\n", k)
	}
	if problems > 0 {
		return fmt.Errorf("doctor found %d problem(s)", problems)
	}
	return nil
}

// lintFiles reports syntax and hygiene problems in .env files: invalid lines,
// duplicate keys within one file, unconventional key names, and values whose
// quoting is likely a mistake. It returns an error when any problem was found.
func lintFiles(w io.Writer, files []string) error {
	var found int
	for _, f := range files {
		lines, err := scanFile(f)
		if err != nil {
			fmt.Fprintf(w, "%s: %v\n", f, err)
			found++
			continue
		}
		seen := make(map[string]int)
		for _, l := range lines {
			switch {
			case l.Invalid:
				fmt.Fprintf(w, "%s:%d: invalid line\n", l.File, l.No)
				found++
			case l.Key == "":
				// blank or comment
			default:
				if prev, dup := seen[l.Key]; dup {
					fmt.Fprintf(w, "%s:%d: duplicate key %s (first at line %d)\n", l.File, l.No, l.Key, prev)
					found++
				}
				seen[l.Key] = l.No
				if !keyShape.MatchString(l.Key) {
					fmt.Fprintf(w, "%s:%d: key %s is not UPPER_SNAKE_CASE\n", l.File, l.No, l.Key)
					found++
				}
				if unbalancedQuote(l.Value) {
					fmt.Fprintf(w, "%s:%d: unbalanced quote in value of %s\n", l.File, l.No, l.Key)
					found++
				}
				if strings.HasPrefix(l.Raw, " ") || strings.HasPrefix(l.Raw, "\t") {
					fmt.Fprintf(w, "%s:%d: leading whitespace before %s\n", l.File, l.No, l.Key)
					found++
				}
			}
		}
	}
	if found > 0 {
		return fmt.Errorf("%d problem(s) found", found)
	}
	fmt.Fprintln(w, "no problems found")
	return nil
}

// unbalancedQuote reports whether a value opens a quote it never closes.
func unbalancedQuote(v string) bool {
	for _, q := range []string{`"`, `'`} {
		if strings.HasPrefix(v, q) && !strings.HasSuffix(strings.TrimSuffix(v, " "), q) {
			return true
		}
		if strings.HasPrefix(v, q) && len(v) == 1 {
			return true
		}
	}
	return false
}

// formatFiles rewrites .env files with their assignments sorted by key, each
// keeping the comment block that sat above it. When write is false the result
// goes to w and the files are left alone.
func formatFiles(w io.Writer, files []string, write bool) error {
	for _, f := range files {
		lines, err := scanFile(f)
		if err != nil {
			return err
		}
		out := formatLines(lines)
		if !write {
			if len(files) > 1 {
				fmt.Fprintf(w, "# %s\n", f)
			}
			if _, err := io.WriteString(w, out); err != nil {
				return err
			}
			continue
		}
		if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(w, "formatted %s\n", f)
	}
	return nil
}

// formatLines renders assignments sorted by key. Leading comments that belong
// to no assignment are kept at the top, so a file header survives.
func formatLines(lines []line) string {
	var header []string
	for _, l := range lines {
		if l.Key != "" {
			break
		}
		if l.Comment {
			header = append(header, l.Raw)
		}
	}
	// A duplicate key is reported by lint; here the last assignment simply wins,
	// which is how the parser resolves it too.
	entries := dedupe(assignments(lines))
	// The header comments were also attached to the first assignment; dropping
	// them there keeps them from being printed twice.
	if len(entries) > 0 && len(header) > 0 && sameStrings(entries[0].Comments, header) {
		entries[0].Comments = nil
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })

	var b strings.Builder
	for _, h := range header {
		b.WriteString(h)
		b.WriteByte('\n')
	}
	if len(header) > 0 && len(entries) > 0 {
		b.WriteByte('\n')
	}
	for _, e := range entries {
		for _, c := range e.Comments {
			b.WriteString(c)
			b.WriteByte('\n')
		}
		b.WriteString(e.Key)
		b.WriteByte('=')
		b.WriteString(e.Value)
		b.WriteByte('\n')
	}
	return b.String()
}

// dedupe keeps the last assignment for each key, preserving input order for the
// keys that remain.
func dedupe(entries []line) []line {
	last := make(map[string]int, len(entries))
	for i, e := range entries {
		last[e.Key] = i
	}
	out := make([]line, 0, len(last))
	for i, e := range entries {
		if last[e.Key] == i {
			out = append(out, e)
		}
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// explain prints everything the CLI can determine about one key: its effective
// value, which file supplied it, whether the process environment overrides it,
// and every file that defines it.
func explain(w io.Writer, files []string, key string) error {
	type hit struct {
		file  string
		no    int
		value string
	}
	var hits []hit
	for _, f := range files {
		lines, err := scanFile(f)
		if err != nil {
			continue
		}
		for _, l := range lines {
			if l.Key == key {
				hits = append(hits, hit{l.File, l.No, l.Value})
			}
		}
	}

	envVal, inEnv := os.LookupEnv(key)
	if len(hits) == 0 && !inEnv {
		return fmt.Errorf("unknown key: %s", key)
	}

	value, source := "", ""
	if len(hits) > 0 {
		last := hits[len(hits)-1]
		value, source = last.value, fmt.Sprintf("%s:%d", last.file, last.no)
	}
	// The process environment wins over every file, exactly as Load resolves it.
	if inEnv {
		value, source = envVal, "environment"
	}

	tw := tabwriter.NewWriter(w, 0, 4, 1, ' ', 0)
	row := func(k, v string) { fmt.Fprintf(tw, "%s\t: %s\n", k, v) }
	row("Key", key)
	row("Value", value)
	row("Source", source)
	if len(hits) > 1 {
		var others []string
		for _, h := range hits[:len(hits)-1] {
			others = append(others, fmt.Sprintf("%s:%d", h.file, h.no))
		}
		row("Shadowed", strings.Join(others, ", "))
	}
	return tw.Flush()
}

// initFiles creates a starter .env and a matching .env.example. An existing
// file is never overwritten — the command is safe to run in a live project.
func initFiles(w io.Writer, files []string) error {
	target := ".env"
	if len(files) > 0 {
		target = files[0]
	}
	example := target + ".example"

	if !exists(target) {
		const starter = "# Environment configuration for this project.\n" +
			"# Copy to .env.local for machine-specific overrides.\n\n"
		if err := os.WriteFile(target, []byte(starter), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(w, "created %s\n", target)
	} else {
		fmt.Fprintf(w, "%s already exists, left alone\n", target)
	}

	if exists(example) {
		fmt.Fprintf(w, "%s already exists, left alone\n", example)
		return nil
	}
	// The example mirrors the real file with every value stripped.
	vals, err := oneenv.Read(target)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Copy this file to " + target + " and fill in the values.\n\n")
	for _, k := range sortedKeys(vals) {
		b.WriteString(k)
		b.WriteString("=\n")
	}
	if err := os.WriteFile(example, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "created %s\n", example)
	return nil
}

// migrate renames keys across .env files according to OLD=NEW pairs. Without
// -w it only reports what it would change.
func migrate(w io.Writer, files []string, renames map[string]string, write bool) error {
	if len(renames) == 0 {
		return fmt.Errorf("migrate needs at least one OLD=NEW pair")
	}
	for _, f := range files {
		lines, err := scanFile(f)
		if err != nil {
			continue
		}
		changed := false
		var b strings.Builder
		for _, l := range lines {
			raw := l.Raw
			if to, ok := renames[l.Key]; ok && l.Key != "" {
				raw = strings.Replace(raw, l.Key, to, 1)
				changed = true
				fmt.Fprintf(w, "%s:%d: %s is deprecated, use %s\n", l.File, l.No, l.Key, to)
			}
			b.WriteString(raw)
			b.WriteByte('\n')
		}
		if !changed || !write {
			continue
		}
		if err := os.WriteFile(f, []byte(b.String()), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(w, "rewrote %s\n", f)
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
