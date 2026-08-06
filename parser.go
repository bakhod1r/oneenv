package oneenv

import (
	"strings"
)

// parser turns .env source bytes into key/value pairs. It scans the source
// once, index-based, without regexp or bufio, allocating only when a value
// must actually be unquoted or expanded.
type parser struct {
	file   string
	expand bool
	strict bool
	src    []byte
	pos    int
	line   int
	// raw collects the pre-expansion form of every value, so fields that opt out
	// of expansion (secrets, ",noexpand") can be decoded from the literal text.
	// It is nil when expansion is off, since raw and expanded then coincide.
	raw map[string]string
	// origin records which file each key was last seen in, so a report can name
	// the file a value came from. It is nil when nobody asked.
	origin map[string]string
}

// parse scans src and writes each key/value into out. On a syntax error it
// returns a *ParseError with the offending line.
func parse(file string, src []byte, expand bool, out map[string]string) error {
	return parseInto(file, src, expandOptions{enabled: expand}, out, nil)
}

// expandOptions carries the expansion behavior for one parse.
type expandOptions struct {
	enabled bool
	strict  bool
}

// parseInto is parse with the literal, pre-expansion values also collected into
// raw when raw is non-nil.
func parseInto(file string, src []byte, exp expandOptions, out, raw map[string]string) error {
	return parseOrigin(file, src, exp, out, raw, nil)
}

// parseOrigin is parseInto that also records, in origin, which file supplied
// each key.
func parseOrigin(file string, src []byte, exp expandOptions, out, raw, origin map[string]string) error {
	p := &parser{file: file, expand: exp.enabled, strict: exp.strict, src: src, line: 1, origin: origin}
	if exp.enabled {
		p.raw = raw
	}
	for {
		p.skipInsignificant()
		if p.pos >= len(p.src) {
			return nil
		}
		if err := p.statement(out); err != nil {
			return err
		}
	}
}

// skipInsignificant advances past whitespace, blank lines and comment lines.
func (p *parser) skipInsignificant() {
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case '\n':
			p.line++
			p.pos++
		case '\r', ' ', '\t', '\v', '\f':
			p.pos++
		case '#':
			p.skipToEOL()
		default:
			return
		}
	}
}

func (p *parser) skipToEOL() {
	for p.pos < len(p.src) && p.src[p.pos] != '\n' {
		p.pos++
	}
}

// statement parses a single KEY=VALUE (or KEY: VALUE) entry.
func (p *parser) statement(out map[string]string) error {
	p.skipOptionalExport()

	key, err := p.key()
	if err != nil {
		return err
	}

	value, literal, err := p.value(out)
	if err != nil {
		return err
	}

	out[key] = value
	if p.raw != nil {
		p.raw[key] = literal
	}
	if p.origin != nil {
		p.origin[key] = p.file
	}
	return nil
}

// skipOptionalExport consumes a leading "export " if present.
func (p *parser) skipOptionalExport() {
	const kw = "export"
	if p.pos+len(kw) < len(p.src) &&
		string(p.src[p.pos:p.pos+len(kw)]) == kw &&
		isSpaceByte(p.src[p.pos+len(kw)]) {
		p.pos += len(kw)
		p.skipSpaces()
	}
}

func (p *parser) skipSpaces() {
	for p.pos < len(p.src) && isSpaceByte(p.src[p.pos]) {
		p.pos++
	}
}

// key parses a variable name terminated by '=' or ':'.
func (p *parser) key() (string, error) {
	start := p.pos
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case c == '=' || c == ':':
			key := strings.TrimRight(string(p.src[start:p.pos]), " \t")
			p.pos++ // consume separator
			if key == "" {
				return "", p.errf("empty variable name")
			}
			return key, nil
		case isKeyByte(c):
			p.pos++
		case isSpaceByte(c):
			p.pos++
		case c == '\n':
			return "", p.errf("missing '=' in assignment")
		default:
			return "", p.errf("unexpected character %q in variable name", string(c))
		}
	}
	return "", p.errf("missing '=' in assignment")
}

// value parses the right-hand side, handling quotes, escapes, inline comments
// and optional expansion. It returns the final value and its literal,
// pre-expansion form.
func (p *parser) value(vars map[string]string) (value, literal string, err error) {
	p.skipSpaces()
	if p.pos >= len(p.src) {
		return "", "", nil
	}

	switch p.src[p.pos] {
	case '\'':
		v, err := p.singleQuoted()
		return v, v, err
	case '"':
		return p.doubleQuoted(vars)
	default:
		return p.unquoted(vars)
	}
}

// expandValue applies expansion to s when it is enabled, reporting an undefined
// reference as a *ParseError under strict mode.
func (p *parser) expandValue(s string, vars map[string]string) (string, error) {
	if !p.expand {
		return s, nil
	}
	v, err := expand(s, vars, p.strict)
	if err != nil {
		return "", &ParseError{File: p.file, Line: p.line, Msg: err.Error(), Err: err}
	}
	return v, nil
}

// unquoted reads until end of line, stripping a trailing " # comment".
func (p *parser) unquoted(vars map[string]string) (value, literal string, err error) {
	start := p.pos
	for p.pos < len(p.src) && p.src[p.pos] != '\n' {
		p.pos++
	}
	raw := p.src[start:p.pos]

	// Strip inline comment: a '#' preceded by whitespace.
	if i := inlineCommentIndex(raw); i >= 0 {
		raw = raw[:i]
	}
	literal = strings.TrimRight(strings.TrimLeft(string(raw), " \t"), " \t\r")
	value, err = p.expandValue(literal, vars)
	return value, literal, err
}

// singleQuoted reads a raw single-quoted value; no escapes, no expansion.
func (p *parser) singleQuoted() (string, error) {
	p.pos++ // opening quote
	start := p.pos
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == '\'' {
			v := string(p.src[start:p.pos])
			p.pos++ // closing quote
			return v, nil
		}
		if c == '\n' {
			p.line++
		}
		p.pos++
	}
	return "", p.errf("unterminated single-quoted value")
}

// doubleQuoted reads a double-quoted value, honoring backslash escapes,
// multiline content and (optionally) variable expansion.
func (p *parser) doubleQuoted(vars map[string]string) (value, literal string, err error) {
	p.pos++ // opening quote
	var b strings.Builder
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch c {
		case '"':
			p.pos++ // closing quote
			s := b.String()
			v, err := p.expandValue(s, vars)
			return v, s, err
		case '\\':
			if p.pos+1 < len(p.src) {
				p.pos++
				b.WriteByte(unescape(p.src[p.pos]))
			} else {
				b.WriteByte('\\')
			}
		case '\n':
			p.line++
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
		p.pos++
	}
	return "", "", p.errf("unterminated double-quoted value")
}

func (p *parser) errf(format string, args ...any) error {
	return &ParseError{File: p.file, Line: p.line, Msg: sprintf(format, args...)}
}
