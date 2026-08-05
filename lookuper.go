package oneenv

import "os"

// Lookuper is the source of environment values consulted by the decoder.
// The decoder never touches os.Getenv directly; everything flows through a
// Lookuper, which makes tests hermetic and parallel-safe.
type Lookuper interface {
	// Lookup returns the value for key and whether it was present.
	Lookup(key string) (value string, ok bool)
}

// OSLookuper reads from the process environment via os.LookupEnv.
type OSLookuper struct{}

// Lookup implements Lookuper.
func (OSLookuper) Lookup(key string) (string, bool) { return os.LookupEnv(key) }

// MapLookuper reads from an in-memory map. Handy in tests.
type MapLookuper map[string]string

// Lookup implements Lookuper.
func (m MapLookuper) Lookup(key string) (string, bool) {
	v, ok := m[key]
	return v, ok
}

// rawLookuper is implemented by sources that can also return the literal,
// pre-expansion form of a value. The decoder uses it for fields that opt out of
// expansion, so a secret containing '$' is decoded exactly as written.
type rawLookuper interface {
	LookupRaw(key string) (value string, ok bool)
}

// lookupRaw returns the literal value for key when src can provide one, falling
// back to the ordinary lookup otherwise.
func lookupRaw(src Lookuper, key string) (string, bool) {
	if r, ok := src.(rawLookuper); ok {
		return r.LookupRaw(key)
	}
	return src.Lookup(key)
}

// PrefixLookuper strips a fixed prefix off every key before delegating.
type PrefixLookuper struct {
	Prefix string
	Next   Lookuper
}

// Lookup implements Lookuper.
func (p PrefixLookuper) Lookup(key string) (string, bool) {
	return p.Next.Lookup(p.Prefix + key)
}

// LookupRaw implements rawLookuper, propagating the request down the chain so a
// prefixed or nested field can still opt out of expansion.
func (p PrefixLookuper) LookupRaw(key string) (string, bool) {
	return lookupRaw(p.Next, p.Prefix+key)
}

// multiLookuper consults each Lookuper in order and returns the first hit.
// The first source wins, so callers put higher-priority sources first.
type multiLookuper []Lookuper

func (m multiLookuper) Lookup(key string) (string, bool) {
	for _, l := range m {
		if v, ok := l.Lookup(key); ok {
			return v, true
		}
	}
	return "", false
}
