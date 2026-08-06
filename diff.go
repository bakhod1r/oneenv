package oneenv

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

// Change is one key whose value differs between two configurations. An added
// key has an empty Old, a removed key an empty New; Added and Removed say which
// it is, so an empty value is not mistaken for absence.
type Change struct {
	Key     string
	Old     string
	New     string
	Added   bool
	Removed bool
}

// String renders the change on one line, e.g. `PORT: 8080 -> 9090`.
func (c Change) String() string {
	switch {
	case c.Added:
		return c.Key + ": (unset) -> " + displayValue(c.New)
	case c.Removed:
		return c.Key + ": " + displayValue(c.Old) + " -> (unset)"
	default:
		return c.Key + ": " + displayValue(c.Old) + " -> " + displayValue(c.New)
	}
}

// Diff reports every key whose value differs between two configurations,
// sorted by key. Keys that are equal are omitted, so an empty result means the
// two configurations are identical as far as oneenv is concerned.
//
// Secret values are masked in both columns, following the same
// WithSecretReveal and WithRedacted policy as the printed table — so a diff is
// safe to log even when it touches a password. A secret whose value changed is
// still reported: both sides simply read "****".
//
//	for _, c := range oneenv.Diff(oldCfg, newCfg) {
//	    fmt.Println(c) // PORT: 8080 -> 9090
//	}
func Diff(oldCfg, newCfg any, opts ...Option) []Change {
	cfg := newConfig(opts)
	mask := func(s string) string { return maskSecret(s, cfg.reveal()) }

	oldVals, err := marshalMasked(oldCfg, mask, opts...)
	if err != nil {
		return nil
	}
	newVals, err := marshalMasked(newCfg, mask, opts...)
	if err != nil {
		return nil
	}

	// A secret that changed reads "****" on both sides, so compare the real
	// values to decide whether to report it, and show only the masked ones.
	oldReal, err1 := MarshalMap(oldCfg, opts...)
	newReal, err2 := MarshalMap(newCfg, opts...)
	if err1 != nil || err2 != nil {
		oldReal, newReal = oldVals, newVals
	}

	keys := make([]string, 0, len(oldVals)+len(newVals))
	seen := make(map[string]bool, len(oldVals)+len(newVals))
	for _, m := range []map[string]string{oldVals, newVals} {
		for k := range m {
			if !seen[k] {
				seen[k], keys = true, append(keys, k)
			}
		}
	}
	sort.Strings(keys)

	var out []Change
	for _, k := range keys {
		ov, inOld := oldVals[k]
		nv, inNew := newVals[k]
		switch {
		case !inOld:
			out = append(out, Change{Key: k, New: nv, Added: true})
		case !inNew:
			out = append(out, Change{Key: k, Old: ov, Removed: true})
		case oldReal[k] != newReal[k]:
			out = append(out, Change{Key: k, Old: ov, New: nv})
		}
	}
	return out
}

// DiffString renders Diff as an aligned KEY / OLD / NEW table, or a single line
// saying the two configurations are identical.
func DiffString(oldCfg, newCfg any, opts ...Option) string {
	changes := Diff(oldCfg, newCfg, opts...)
	if len(changes) == 0 {
		return "no changes\n"
	}
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	_, _ = io.WriteString(tw, "KEY\tOLD\tNEW\n")
	for _, c := range changes {
		old, nw := displayValue(c.Old), displayValue(c.New)
		if c.Added {
			old = "(unset)"
		}
		if c.Removed {
			nw = "(unset)"
		}
		_, _ = io.WriteString(tw, c.Key+"\t"+old+"\t"+nw+"\n")
	}
	_ = tw.Flush()
	return b.String()
}

// Hash returns a short, stable fingerprint of a configuration: the first eight
// hex characters of the SHA-256 of its sorted KEY=value form. Two runs with the
// same effective configuration produce the same fingerprint, so a deploy can
// print it and an operator can tell at a glance whether anything changed.
//
//	log.Printf("config hash: %s", oneenv.Hash(cfg)) // config hash: 7d2d4d19
//
// The hash covers the real values, including secrets, so that a changed
// password does change the fingerprint — but it is one-way, and eight hex
// characters carry too little of the input to recover anything from.
func Hash(v any, opts ...Option) string {
	return HashFull(v, opts...)[:8]
}

// HashFull is Hash without the truncation: the full SHA-256, hex encoded. Use
// it when a fingerprint has to be collision-resistant rather than readable.
func HashFull(v any, opts ...Option) string {
	m, err := MarshalMap(v, opts...)
	if err != nil {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		// The NUL separators keep "AB"+"C" from hashing like "A"+"BC".
		_, _ = io.WriteString(h, k)
		_, _ = h.Write([]byte{0})
		_, _ = io.WriteString(h, m[k])
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
