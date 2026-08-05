package oneenv

import (
	"fmt"
	"os"
	"strings"
)

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

// isSpaceByte reports whether b is intra-line whitespace (not a newline).
func isSpaceByte(b byte) bool {
	switch b {
	case ' ', '\t', '\v', '\f', '\r':
		return true
	}
	return false
}

// isKeyByte reports whether b is allowed inside a variable name.
func isKeyByte(b byte) bool {
	switch {
	case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b >= '0' && b <= '9':
		return true
	case b == '_' || b == '.' || b == '-':
		return true
	}
	return false
}

// inlineCommentIndex returns the index of an inline comment start (a '#'
// preceded by whitespace) within an unquoted value, or -1.
func inlineCommentIndex(raw []byte) int {
	for i := 1; i < len(raw); i++ {
		if raw[i] == '#' && isSpaceByte(raw[i-1]) {
			return i
		}
	}
	return -1
}

// unescape maps the character after a backslash to its literal value.
func unescape(c byte) byte {
	switch c {
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	default:
		return c
	}
}

// expand replaces ${VAR} and $VAR references, resolving first against vars
// parsed so far and then against the process environment. When strict is set,
// a reference to a variable that resolves nowhere is an error instead of an
// empty string, so a typo or an unescaped '$' in a literal value cannot
// silently blank the value out.
func expand(s string, vars map[string]string, strict bool) (string, error) {
	if !strings.ContainsRune(s, '$') {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' {
			b.WriteByte(s[i])
			i++
			continue
		}
		// Escaped: "\$" already unescaped to "$"? We treat "$$" as literal "$".
		if i+1 < len(s) && s[i+1] == '$' {
			b.WriteByte('$')
			i += 2
			continue
		}
		name, next := readVarName(s, i+1)
		if name == "" {
			b.WriteByte('$')
			i++
			continue
		}
		v, ok := resolve(name, vars)
		if !ok && strict {
			return "", fmt.Errorf("%w: %q", ErrUnknownVariable, name)
		}
		b.WriteString(v)
		i = next
	}
	return b.String(), nil
}

// readVarName parses a variable reference starting just after '$'. It supports
// both ${NAME} and bare NAME forms and returns the name and the index past it.
func readVarName(s string, i int) (name string, next int) {
	if i < len(s) && s[i] == '{' {
		end := strings.IndexByte(s[i:], '}')
		if end < 0 {
			return "", i
		}
		return s[i+1 : i+end], i + end + 1
	}
	start := i
	for i < len(s) && isKeyByte(s[i]) {
		i++
	}
	return s[start:i], i
}

func resolve(name string, vars map[string]string) (string, bool) {
	if v, ok := vars[name]; ok {
		return v, true
	}
	if v, ok := os.LookupEnv(name); ok {
		return v, true
	}
	return "", false
}
