package oneenv

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode"
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
	Null     bool // the value is empty or was never set, secret or not
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
	row("Null", boolWord(e.Null))
	row("Required", boolWord(e.Required))
	row("Secret", boolWord(e.Secret))
	row("Type", e.Type)
	_ = tw.Flush()
	return b.String()
}

// Missing returns the keys the struct declares that no source supplied: not the
// process environment, not a .env file, not a `default` tag. These are the
// variables a deployment still has to provide — the mirror image of
// WithStrictKeys, which reports keys in a file that the struct does not know.
func (r *Report) Missing() []Entry {
	var out []Entry
	for _, e := range r.Entries() {
		if e.Source == SourceUnset {
			out = append(out, e)
		}
	}
	return out
}

// MissingKeys is Missing reduced to the key names, sorted.
func (r *Report) MissingKeys() []string {
	entries := r.Missing()
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Key)
	}
	return out
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

// writeTable renders entries as a ruled KEY / VALUE / TYPE / NULL / SOURCE
// table. The rules are drawn with box characters, which every terminal oneenv
// is likely to print to renders, and which keep a long value from running into
// the next column at a glance.
func writeTable(w io.Writer, entries []Entry) error {
	rows := make([][]string, 0, len(entries))
	for i, e := range entries {
		null := ""
		if e.Null {
			null = "yes"
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), e.Key, displayValue(e.Value), e.Type, null, e.Origin()})
	}
	return writeRuledTableEntries(w, []string{"#", "KEY", "VALUE", "TYPE", "NULL", "SOURCE"}, rows, entries)
}

// writeRuledTable draws a table with a border, a rule between every row, and
// formatting applied when outputting to a terminal.
func writeRuledTable(w io.Writer, header []string, rows [][]string) error {
	return writeRuledTableEntries(w, header, rows, nil)
}

func writeRuledTableEntries(w io.Writer, header []string, rows [][]string, entries []Entry) error {
	term := isTerminal(w)

	width := make([]int, len(header))
	for i, h := range header {
		width[i] = runeLen(h)
	}
	for _, r := range rows {
		for i, cell := range r {
			if i < len(width) && runeLen(cell) > width[i] {
				width[i] = runeLen(cell)
			}
		}
	}

	var b strings.Builder
	rule := func(left, mid, right string) {
		b.WriteString(left)
		for i, n := range width {
			if i > 0 {
				b.WriteString(mid)
			}
			b.WriteString(strings.Repeat("─", n+2))
		}
		b.WriteString(right + "\n")
	}

	line := func(rowIdx int, cells []string) {
		b.WriteString("│")
		for i, n := range width {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			text := formatCell(cell, rowIdx, i, term, entries)
			b.WriteString(" " + text + strings.Repeat(" ", n-runeLen(cell)) + " │")
		}
		b.WriteByte('\n')
	}

	rule("┌", "┬", "┐")
	line(-1, header)
	rule("├", "┼", "┤")
	for i, r := range rows {
		if i > 0 {
			rule("├", "┼", "┤")
		}
		line(i, r)
	}
	rule("└", "┴", "┘")

	_, err := io.WriteString(w, b.String())
	return err
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	return true
}

func formatCell(cell string, rowIdx, colIdx int, term bool, entries []Entry) string {
	if cell == "" || !term {
		return cell
	}
	// Header row
	if rowIdx < 0 {
		return "\x1b[1;36m" + cell + "\x1b[0m" // Bold Cyan header
	}

	if entries != nil && rowIdx < len(entries) {
		e := entries[rowIdx]
		// Required but empty or unset -> BOLD RED!
		if e.Required && (e.Null || e.Value == "" || e.Source == SourceUnset) {
			return "\x1b[1;31m" + cell + "\x1b[0m" // Bold Red for entire row
		}

		// Normal entry styling
		if colIdx == 0 { // #
			return "\x1b[1;30m" + cell + "\x1b[0m" // Bold Gray row index
		}
		if colIdx == 1 { // KEY
			return "\x1b[1m" + cell + "\x1b[0m" // Bold Key
		}
		if colIdx == 2 { // VALUE
			if e.Null {
				return "\x1b[2m" + cell + "\x1b[0m" // Dim for null
			}
			return "\x1b[32m" + cell + "\x1b[0m" // Green for set value
		}
		if colIdx == 4 && e.Null { // NULL
			return "\x1b[33m" + cell + "\x1b[0m" // Yellow null
		}
		if colIdx == 5 { // SOURCE
			switch e.Source {
			case SourceEnv:
				return "\x1b[36m" + cell + "\x1b[0m" // Cyan
			case SourceFile:
				return "\x1b[32m" + cell + "\x1b[0m" // Green
			case SourceDefault:
				return "\x1b[33m" + cell + "\x1b[0m" // Yellow
			case SourceUnset:
				return "\x1b[31m" + cell + "\x1b[0m" // Red
			}
		}
	} else if colIdx == 0 || colIdx == 1 {
		return "\x1b[1m" + cell + "\x1b[0m" // Bold
	}

	return cell
}

// runeLen is the width a string occupies in a terminal, which is what the
// column arithmetic needs: bytes are wrong for anything non-ASCII, and even
// counting runes is wrong for the two cases below. Getting this wrong shows up
// immediately as a ragged right border.
//
//   - A combining mark (an accent, a variation selector) draws on top of the
//     previous character and takes no column of its own.
//   - A CJK ideograph, a fullwidth form or an emoji takes two.
func runeLen(s string) int {
	n := 0
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || r == 0xFE0F:
			// zero columns
		case isWide(r):
			n += 2
		default:
			n++
		}
	}
	return n
}

// wideRanges are the code points a terminal draws two columns wide: the East
// Asian Wide and Fullwidth blocks, plus the emoji ranges that behave the same.
var wideRanges = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x1100, Hi: 0x115F, Stride: 1}, // Hangul Jamo
		{Lo: 0x2E80, Hi: 0x303E, Stride: 1}, // CJK radicals, Kangxi
		{Lo: 0x3041, Hi: 0x33FF, Stride: 1}, // kana, CJK compatibility
		{Lo: 0x3400, Hi: 0x4DBF, Stride: 1}, // CJK extension A
		{Lo: 0x4E00, Hi: 0x9FFF, Stride: 1}, // CJK unified ideographs
		{Lo: 0xA000, Hi: 0xA4CF, Stride: 1}, // Yi
		{Lo: 0xAC00, Hi: 0xD7A3, Stride: 1}, // Hangul syllables
		{Lo: 0xF900, Hi: 0xFAFF, Stride: 1}, // CJK compatibility ideographs
		{Lo: 0xFE10, Hi: 0xFE19, Stride: 1}, // vertical forms
		{Lo: 0xFE30, Hi: 0xFE6F, Stride: 1}, // CJK compatibility forms
		{Lo: 0xFF00, Hi: 0xFF60, Stride: 1}, // fullwidth forms
		{Lo: 0xFFE0, Hi: 0xFFE6, Stride: 1}, // fullwidth signs
	},
	R32: []unicode.Range32{
		{Lo: 0x1F300, Hi: 0x1F64F, Stride: 1}, // symbols, pictographs, emoticons
		{Lo: 0x1F900, Hi: 0x1F9FF, Stride: 1}, // supplemental symbols
		{Lo: 0x20000, Hi: 0x3FFFD, Stride: 1}, // CJK extensions B and beyond
	},
}

func isWide(r rune) bool { return unicode.Is(wideRanges, r) }

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
	sorted := make([]Entry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })

	// Keys the struct declares that nothing supplied are the ones a deployment
	// still has to fill in, so they are called out rather than left to be
	// spotted among the rows.
	var missing []string
	for _, e := range sorted {
		if e.Source == SourceUnset {
			missing = append(missing, e.Key)
		}
	}
	c.logMissing(missing)

	if !c.table {
		return nil
	}
	if err := writeTable(c.writer(), sorted); err != nil {
		return err
	}
	if len(missing) > 0 {
		_, err := fmt.Fprintf(c.writer(), "\n%d not set: %s\n", len(missing), strings.Join(missing, ", "))
		return err
	}
	return nil
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
	mask := func(s string) string {
		// An empty secret has nothing to hide; masking it would claim otherwise.
		if s == "" {
			return ""
		}
		return maskSecret(s, cfg.reveal())
	}
	m, err := marshalMasked(v, mask, opts...)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([][]string, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, []string{k, displayValue(m[k])})
	}
	return writeRuledTable(cfg.writer(), []string{"KEY", "VALUE"}, rows)
}

// displayValue renders an empty value as a visible placeholder, so a blank cell
// is not mistaken for a missing key, and keeps a multiline value on one row.
func displayValue(s string) string {
	if s == "" {
		return `""`
	}
	return strings.NewReplacer("\n", "\\n", "\r", "\\r", "\t", "\\t").Replace(s)
}
