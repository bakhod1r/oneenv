package main

import (
	"os"
	"strings"
)

// line is one significant line of a .env file, kept with its position and the
// comment block that sits directly above it, so a rewrite can preserve both.
type line struct {
	File     string
	No       int // 1-based
	Key      string
	Value    string
	Comments []string
	Raw      string
	Blank    bool
	Comment  bool
	Invalid  bool // has content but is not a comment and not KEY=VALUE
}

// scanFile splits a .env file into lines, attaching each pending comment block
// to the assignment that follows it. It is deliberately more tolerant than the
// package parser: the linter needs to see the broken lines, not stop at the
// first one.
func scanFile(name string) ([]line, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var (
		out     []line
		pending []string
	)
	for i, raw := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		l := line{File: name, No: i + 1, Raw: raw}
		trimmed := strings.TrimSpace(raw)
		switch {
		case trimmed == "":
			l.Blank = true
			pending = nil
		case strings.HasPrefix(trimmed, "#"):
			l.Comment = true
			pending = append(pending, trimmed)
		default:
			kv := strings.TrimPrefix(trimmed, "export ")
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				k, v, ok = strings.Cut(kv, ":")
			}
			if !ok {
				l.Invalid = true
			} else {
				l.Key = strings.TrimSpace(k)
				l.Value = strings.TrimSpace(v)
				l.Comments = pending
				if l.Key == "" {
					l.Invalid = true
				}
			}
			pending = nil
		}
		out = append(out, l)
	}
	// A trailing newline produces a final empty element; drop it so a rewrite
	// does not grow a blank line on every pass.
	if n := len(out); n > 0 && out[n-1].Blank && out[n-1].Raw == "" {
		out = out[:n-1]
	}
	return out, nil
}

// assignments returns only the KEY=VALUE lines.
func assignments(lines []line) []line {
	out := make([]line, 0, len(lines))
	for _, l := range lines {
		if l.Key != "" {
			out = append(out, l)
		}
	}
	return out
}

// exists reports whether a path is readable.
func exists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}
