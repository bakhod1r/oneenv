package oneenv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// errWriter fails every Write, to exercise writer-error paths.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

// --- init / unset -----------------------------------------------------------

func TestInitField(t *testing.T) {
	type Config struct {
		P *int              `env:"P,init"`
		S []string          `env:"S,init"`
		M map[string]string `env:"M,init"`
		N int               `env:"N,init"` // non-nilable: init is a no-op
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{})); err != nil {
		t.Fatal(err)
	}
	if cfg.P == nil || cfg.S == nil || cfg.M == nil {
		t.Fatalf("init did not allocate: %+v", cfg)
	}
	// Calling initValue on an already non-nil value is a no-op.
	initValue(reflect.ValueOf(&cfg).Elem().Field(0))
}

func TestUnset(t *testing.T) {
	t.Setenv("SECRET_TOKEN", "abc")
	type Config struct {
		Token string `env:"SECRET_TOKEN,unset"`
	}
	var cfg Config
	if err := Load(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "abc" {
		t.Fatalf("got %q", cfg.Token)
	}
	if _, ok := os.LookupEnv("SECRET_TOKEN"); ok {
		t.Fatal("expected SECRET_TOKEN to be unset")
	}
}

// --- lookupers --------------------------------------------------------------

func TestOSLookuper(t *testing.T) {
	t.Setenv("OSLOOKUP_KEY", "v")
	if v, ok := (OSLookuper{}).Lookup("OSLOOKUP_KEY"); !ok || v != "v" {
		t.Fatalf("got %q %v", v, ok)
	}
	if _, ok := (OSLookuper{}).Lookup("OSLOOKUP_MISSING"); ok {
		t.Fatal("unexpected hit")
	}
}

func TestMultiLookuper(t *testing.T) {
	m := multiLookuper{MapLookuper{"A": "1"}, MapLookuper{"B": "2"}}
	if v, _ := m.Lookup("B"); v != "2" {
		t.Fatalf("got %q", v)
	}
	if _, ok := m.Lookup("C"); ok {
		t.Fatal("unexpected hit")
	}
}

// --- Parse / Unmarshal / context -------------------------------------------

func TestParseError(t *testing.T) {
	type Config struct {
		Host string `env:"HOST,required"`
	}
	if _, err := Parse[Config](WithLookuper(MapLookuper{})); !errors.Is(err, ErrRequired) {
		t.Fatalf("want ErrRequired, got %v", err)
	}
}

func TestParseContext(t *testing.T) {
	type Config struct {
		Name string `env:"NAME"`
	}
	cfg, err := ParseContext[Config](context.Background(),
		WithLookuper(MapLookuper{"NAME": "x"}),
		WithMutator(func(ctx context.Context, _, v string) (string, error) {
			if ctx == nil {
				t.Fatal("nil ctx")
			}
			return v, nil
		}),
	)
	if err != nil || cfg.Name != "x" {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

func TestUnmarshalParseError(t *testing.T) {
	var cfg struct{}
	if err := Unmarshal([]byte("=novalue\n"), &cfg); err == nil {
		t.Fatal("want parse error")
	}
}

func TestUnmarshalNotAStruct(t *testing.T) {
	x := 5
	if err := Unmarshal([]byte("A=1"), &x); !errors.Is(err, ErrNotAStruct) {
		t.Fatalf("want ErrNotAStruct, got %v", err)
	}
}

func TestLoadNotAStruct(t *testing.T) {
	x := 5
	if err := Load(&x); !errors.Is(err, ErrNotAStruct) {
		t.Fatalf("want ErrNotAStruct, got %v", err)
	}
	var nilp *struct{}
	if err := Load(nilp); !errors.Is(err, ErrNotAStruct) {
		t.Fatalf("want ErrNotAStruct for nil, got %v", err)
	}
}

func TestLoadMissingExplicitFile(t *testing.T) {
	var cfg struct{}
	if err := Load(&cfg, WithFiles("does-not-exist.env")); err == nil {
		t.Fatal("want error for missing explicit file")
	}
}

// --- loadEnv / Overload -----------------------------------------------------

func TestLoadEnvAndOverloadCov(t *testing.T) {
	f := writeTemp(t, ".env", "LE_A=fromfile\nLE_B=x\n")
	t.Setenv("LE_A", "fromenv")

	if err := LoadEnv(f); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("LE_A") != "fromenv" { // existing wins
		t.Fatalf("LoadEnv overwrote existing: %q", os.Getenv("LE_A"))
	}
	if os.Getenv("LE_B") != "x" {
		t.Fatalf("LE_B=%q", os.Getenv("LE_B"))
	}

	if err := Overload(f); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("LE_A") != "fromfile" { // .env wins
		t.Fatalf("Overload did not override: %q", os.Getenv("LE_A"))
	}
}

func TestLoadEnvMissingFile(t *testing.T) {
	if err := LoadEnv("nope.env"); err == nil {
		t.Fatal("want error")
	}
}

func TestReadDefaults(t *testing.T) {
	// Read/LoadEnv with no args default to ".env"; run in an empty dir.
	dir := t.TempDir()
	old, _ := os.Getwd()
	defer func() { _ = os.Chdir(old) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := Read(); err != nil { // missing default .env is OK
		t.Fatalf("Read() default: %v", err)
	}
	if err := LoadEnv(); err != nil {
		t.Fatalf("LoadEnv() default: %v", err)
	}
	if err := Overload(); err != nil {
		t.Fatalf("Overload() default: %v", err)
	}
}

// --- parser edge cases ------------------------------------------------------

func TestParserErrors(t *testing.T) {
	cases := []string{
		"=novalue\n",       // empty key
		"NOEQUALS\n",       // missing '='
		"KEY",              // EOF before '='
		"BAD!KEY=1\n",      // illegal char in key
		"K='unterminated",  // unterminated single quote
		"K=\"unterminated", // unterminated double quote
	}
	for _, src := range cases {
		out := map[string]string{}
		if err := parse("t.env", []byte(src), false, out); err == nil {
			t.Errorf("expected error for %q", src)
		}
	}
}

func TestParserValues(t *testing.T) {
	out := map[string]string{}
	src := "A='raw $NO'\n" +
		"B=\"esc \\n\\r\\t\\\" \\x end\"\n" +
		"C=bare # comment\n" +
		"D=\n" + // empty value
		"E=\"multi\nline\"\n" +
		"export F=exported\n"
	if err := parse("", []byte(src), false, out); err != nil {
		t.Fatal(err)
	}
	if out["A"] != "raw $NO" {
		t.Errorf("A=%q", out["A"])
	}
	if out["B"] != "esc \n\r\t\" x end" {
		t.Errorf("B=%q", out["B"])
	}
	if out["C"] != "bare" || out["D"] != "" || out["F"] != "exported" {
		t.Errorf("C=%q D=%q F=%q", out["C"], out["D"], out["F"])
	}
	if out["E"] != "multi\nline" {
		t.Errorf("E=%q", out["E"])
	}
}

func TestDoubleQuoteTrailingBackslash(t *testing.T) {
	out := map[string]string{}
	if err := parse("", []byte("K=\"ab\\"), false, out); err == nil {
		t.Fatal("want unterminated error")
	}
}

// --- expansion --------------------------------------------------------------

func TestExpansion(t *testing.T) {
	t.Setenv("EXP_HOME", "/home/x")
	out := map[string]string{}
	src := "BASE=b\n" +
		"A=${BASE}/sub\n" +
		"B=$BASE/tail\n" +
		"C=${EXP_HOME}\n" +
		"D=$$literal\n" +
		"E=trailing$\n" +
		"F=${UNTERMINATED\n" +
		"G=noexpand\n"
	if err := parse("", []byte(src), true, out); err != nil {
		t.Fatal(err)
	}
	if out["A"] != "b/sub" || out["B"] != "b/tail" || out["C"] != "/home/x" {
		t.Errorf("A=%q B=%q C=%q", out["A"], out["B"], out["C"])
	}
	if out["D"] != "$literal" || out["E"] != "trailing$" {
		t.Errorf("D=%q E=%q", out["D"], out["E"])
	}
}

func TestResolveMissing(t *testing.T) {
	if got := resolve("DEFINITELY_MISSING_VAR_XYZ", nil); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestUnescapeAll(t *testing.T) {
	for in, want := range map[byte]byte{'n': '\n', 'r': '\r', 't': '\t', 'x': 'x'} {
		if got := unescape(in); got != want {
			t.Errorf("unescape(%q)=%q want %q", in, got, want)
		}
	}
}

// --- schema / setters -------------------------------------------------------

func TestUnsupportedType(t *testing.T) {
	type Config struct {
		Ch chan int `env:"CH"`
	}
	var cfg Config
	err := Load(&cfg, WithLookuper(MapLookuper{"CH": "x"}))
	if !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("want ErrUnsupportedType, got %v", err)
	}
}

func TestUnsupportedMapKey(t *testing.T) {
	type Config struct {
		M map[int]string `env:"M"`
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{"M": "1:a"})); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("want ErrUnsupportedType, got %v", err)
	}
}

func TestSkippedAndUnexported(t *testing.T) {
	type Config struct {
		hidden string //nolint:unused
		Skip   string `env:"-"`
		Keep   string `env:"KEEP"`
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{"KEEP": "y", "Skip": "z"})); err != nil {
		t.Fatal(err)
	}
	if cfg.Keep != "y" || cfg.Skip != "" {
		t.Fatalf("%+v", cfg)
	}
}

func TestAllScalarTypesAndErrors(t *testing.T) {
	type Config struct {
		I    int           `env:"I"`
		U    uint          `env:"U"`
		F    float64       `env:"F"`
		B    bool          `env:"B"`
		P    *int          `env:"P"`
		Dur  time.Duration `env:"DUR"`
		Sl   []int         `env:"SL"`
		EmSl []int         `env:"EMSL"`
	}
	var cfg Config
	err := Load(&cfg, WithLookuper(MapLookuper{
		"I": "-3", "U": "4", "F": "1.5", "B": "true",
		"P": "7", "DUR": "2s", "SL": "1,2,3", "EMSL": "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.I != -3 || cfg.U != 4 || cfg.F != 1.5 || !cfg.B || *cfg.P != 7 || cfg.Dur != 2*time.Second {
		t.Fatalf("%+v", cfg)
	}
	if len(cfg.Sl) != 3 || len(cfg.EmSl) != 0 {
		t.Fatalf("slices: %+v", cfg)
	}

	// Parse errors for each scalar kind.
	for k, bad := range map[string]string{"I": "x", "U": "x", "F": "x", "B": "x", "DUR": "x", "SL": "x", "P": "x"} {
		var c Config
		if err := Load(&c, WithLookuper(MapLookuper{k: bad})); err == nil {
			t.Errorf("expected error for %s=%s", k, bad)
		}
	}
}

func TestMapErrors(t *testing.T) {
	type Config struct {
		M map[string]int `env:"M"`
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{"M": "noconlon"})); err == nil {
		t.Fatal("want invalid entry error")
	}
	if err := Load(&cfg, WithLookuper(MapLookuper{"M": "k:notint"})); err == nil {
		t.Fatal("want elem parse error")
	}
	var ok Config
	if err := Load(&ok, WithLookuper(MapLookuper{"M": ""})); err != nil || ok.M == nil {
		t.Fatalf("empty map: %+v %v", ok, err)
	}
}

func TestTimeLayoutError(t *testing.T) {
	type Config struct {
		T time.Time `env:"T" layout:"2006-01-02"`
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{"T": "not-a-date"})); err == nil {
		t.Fatal("want time parse error")
	}
}

func TestSetterForDirect(t *testing.T) {
	if _, err := setterFor(reflect.TypeOf(make(chan int)), nil); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("want ErrUnsupportedType, got %v", err)
	}
	// Struct case in setterFor: time.Time resolves to a setter.
	if _, err := setterFor(timeType, nil); err != nil {
		t.Fatalf("timeType: %v", err)
	}
	// Nested slice of unsupported element type propagates the error.
	if _, err := setterFor(reflect.TypeOf([]chan int{}), nil); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("slice elem: %v", err)
	}
	if _, err := setterFor(reflect.TypeOf(map[string]chan int{}), nil); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("map elem: %v", err)
	}
	if _, err := setterFor(reflect.TypeOf((*chan int)(nil)), nil); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("ptr elem: %v", err)
	}
}

// --- TextUnmarshaler --------------------------------------------------------

type upperText struct{ s string }

func (u *upperText) UnmarshalText(b []byte) error {
	u.s = strings.ToUpper(string(b))
	return nil
}
func (u upperText) MarshalText() ([]byte, error) { return []byte(u.s), nil }

func TestTextUnmarshaler(t *testing.T) {
	type Config struct {
		V upperText  `env:"V"`
		P *upperText `env:"P"`
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{"V": "hi", "P": "yo"})); err != nil {
		t.Fatal(err)
	}
	if cfg.V.s != "HI" || cfg.P.s != "YO" {
		t.Fatalf("%+v", cfg)
	}
}

// --- Marshal ----------------------------------------------------------------

func TestMarshalTypes(t *testing.T) {
	n := 7
	type Config struct {
		Name    string         `env:"NAME"`
		Nptr    *int           `env:"NPTR"`
		Nilptr  *int           `env:"NILPTR"`
		Tags    []string       `env:"TAGS" separator:","`
		Labels  map[string]int `env:"LABELS"`
		Dur     time.Duration  `env:"DUR"`
		When    time.Time      `env:"WHEN" layout:"2006-01-02"`
		Special string         `env:"SPECIAL"`
	}
	cfg := Config{
		Name: "app", Nptr: &n, Tags: []string{"a", "b"},
		Labels: map[string]int{"x": 1}, Dur: 90 * time.Second,
		When:    time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC),
		Special: "has space#and$dollar",
	}
	data, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"NAME=app", "NPTR=7", "NILPTR=", "TAGS=a,b", "LABELS=x:1", "DUR=1m30s", "WHEN=2020-01-02", `SPECIAL="has space#and$dollar"`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

func TestMarshalMapErrors(t *testing.T) {
	if _, err := MarshalMap(5); !errors.Is(err, ErrNotAStruct) {
		t.Fatalf("non-struct: %v", err)
	}
	var nilp *struct{}
	if _, err := MarshalMap(nilp); !errors.Is(err, ErrNotAStruct) {
		t.Fatalf("nil ptr: %v", err)
	}
	type Bad struct {
		Ch chan int `env:"CH"`
	}
	if _, err := Marshal(Bad{}); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("bad field Marshal: %v", err)
	}
	if _, err := MarshalMap(&Bad{}); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("bad field MarshalMap: %v", err)
	}
}

func TestMarshalPointerDeref(t *testing.T) {
	type Config struct {
		Name string `env:"NAME"`
	}
	m, err := MarshalMap(&Config{Name: "z"})
	if err != nil || m["NAME"] != "z" {
		t.Fatalf("m=%v err=%v", m, err)
	}
}

func TestTimeFormatterNonTime(t *testing.T) {
	// Direct call with a non-time value exercises the defensive branch.
	if got := timeFormatter(time.RFC3339)(reflect.ValueOf(5), ""); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestQuoteValue(t *testing.T) {
	cases := map[string]string{"": "", "plain": "plain", "a b": `"a b"`, "a\"b": `"a\"b"`, "l1\nl2": `"l1\nl2"`}
	for in, want := range cases {
		if got := quoteValue(in); got != want {
			t.Errorf("quoteValue(%q)=%q want %q", in, got, want)
		}
	}
}

// --- Usage ------------------------------------------------------------------

func TestUsageNested(t *testing.T) {
	type DB struct {
		Port int `env:"PORT" desc:"db port"`
	}
	type Config struct {
		Host string `env:"HOST,required"`
		DB   DB     `envPrefix:"DB_"`
	}
	var b strings.Builder
	if err := Usage[Config](&b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "DB_PORT") || !strings.Contains(b.String(), "db port") {
		t.Fatalf("nested usage missing:\n%s", b.String())
	}
}

func TestUsageNotAStruct(t *testing.T) {
	if err := Usage[int](&strings.Builder{}); !errors.Is(err, ErrNotAStruct) {
		t.Fatalf("want ErrNotAStruct, got %v", err)
	}
}

func TestUsageWriterError(t *testing.T) {
	type Config struct {
		Host string `env:"HOST"`
	}
	if err := Usage[Config](errWriter{}); err == nil {
		t.Fatal("want writer error")
	}
}

func TestUsageBadStruct(t *testing.T) {
	type Bad struct {
		Ch chan int `env:"CH"`
	}
	if err := Usage[Bad](&strings.Builder{}); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("want ErrUnsupportedType, got %v", err)
	}
	// Nested bad struct exercises the recursive error return.
	type Outer struct {
		Inner Bad `envPrefix:"I_"`
	}
	if err := Usage[Outer](&strings.Builder{}); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("nested bad: %v", err)
	}
}

// --- errors -----------------------------------------------------------------

func TestParseErrorNoFile(t *testing.T) {
	e := &ParseError{Line: 3, Msg: "boom"}
	if !strings.Contains(e.Error(), "<source>:3") {
		t.Fatalf("got %q", e.Error())
	}
}

func TestFieldErrorUnwrap(t *testing.T) {
	fe := &FieldError{Field: "X", Key: "X", Err: ErrRequired}
	if !errors.Is(fe, ErrRequired) {
		t.Fatal("unwrap failed")
	}
	if !strings.Contains(fe.Error(), "X") {
		t.Fatalf("got %q", fe.Error())
	}
}

// --- misc option paths ------------------------------------------------------

func TestNilOptionAndDefaults(t *testing.T) {
	type Config struct {
		Port int `env:"PORT" default:"8080"`
	}
	var cfg Config
	// nil option is skipped; empty tag key keeps default; nil lookuper ignored.
	if err := Load(&cfg, nil, WithTagKey(""), WithLookuper(nil), WithLookuper(MapLookuper{})); err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8080 {
		t.Fatalf("port=%d", cfg.Port)
	}
}

func TestMultipleTypeParsers(t *testing.T) {
	type Config struct {
		A string `env:"A"`
		B string `env:"B"`
	}
	var cfg Config
	err := Load(&cfg, WithLookuper(MapLookuper{"A": "a", "B": "b"}),
		WithTypeParser(func(s string) (string, error) { return "<" + s + ">", nil }),
		WithMutator(nil),   // nil mutator ignored
		WithValidator(nil), // nil validator: no-op
	)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.A != "<a>" || cfg.B != "<b>" {
		t.Fatalf("%+v", cfg)
	}
}

func TestExpandNoDollarFastPath(t *testing.T) {
	if got := expand("plain", nil); got != "plain" {
		t.Fatalf("got %q", got)
	}
}

func TestTypeParserError(t *testing.T) {
	type Config struct {
		A string `env:"A"`
	}
	boom := errors.New("bad parse")
	var cfg Config
	err := Load(&cfg, WithLookuper(MapLookuper{"A": "x"}),
		WithTypeParser(func(string) (string, error) { return "", boom }))
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
}

func TestMarshalNested(t *testing.T) {
	type DB struct {
		Host string `env:"HOST"`
	}
	type Config struct {
		DB DB `envPrefix:"DB_"`
	}
	m, err := MarshalMap(Config{DB: DB{Host: "h"}})
	if err != nil || m["DB_HOST"] != "h" {
		t.Fatalf("m=%v err=%v", m, err)
	}
}

func TestMarshalNestedError(t *testing.T) {
	type Bad struct {
		Ch chan int `env:"CH"`
	}
	type Outer struct {
		Inner Bad `envPrefix:"I_"`
	}
	if _, err := Marshal(Outer{}); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("want ErrUnsupportedType from nested marshal, got %v", err)
	}
}

func TestTimeDefaultLayout(t *testing.T) {
	type Config struct {
		T time.Time `env:"T"` // no layout tag → RFC3339
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{"T": "2020-01-02T03:04:05Z"})); err != nil {
		t.Fatal(err)
	}
	if cfg.T.Year() != 2020 {
		t.Fatalf("got %v", cfg.T)
	}
}

func TestKeyWithInternalSpace(t *testing.T) {
	out := map[string]string{}
	if err := parse("", []byte("A B=1\n"), false, out); err != nil {
		t.Fatal(err)
	}
	if out["A B"] != "1" {
		t.Fatalf("got %v", out)
	}
}

func TestValueEOFAfterEquals(t *testing.T) {
	out := map[string]string{}
	if err := parse("", []byte("K="), false, out); err != nil {
		t.Fatal(err)
	}
	if v, ok := out["K"]; !ok || v != "" {
		t.Fatalf("got %q %v", v, ok)
	}
}

func TestSingleQuoteMultiline(t *testing.T) {
	out := map[string]string{}
	if err := parse("", []byte("K='a\nb'\n"), false, out); err != nil {
		t.Fatal(err)
	}
	if out["K"] != "a\nb" {
		t.Fatalf("got %q", out["K"])
	}
}

func TestReadFilesParseError(t *testing.T) {
	f := writeTemp(t, "bad.env", "=novalue\n")
	if err := LoadEnv(f); err == nil {
		t.Fatal("want parse error from readFiles")
	}
}

func TestFieldNoTagAndEmptyTag(t *testing.T) {
	type Config struct {
		Plain string `env:""` // empty tag → name defaults to field name
		Bare  string // no env tag at all → name defaults to field name
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{"Plain": "p", "Bare": "b"})); err != nil {
		t.Fatal(err)
	}
	if cfg.Plain != "p" || cfg.Bare != "b" {
		t.Fatalf("%+v", cfg)
	}
}

func TestLoadEnvSetenvError(t *testing.T) {
	orig := setenv
	defer func() { setenv = orig }()
	boom := errors.New("setenv failed")
	setenv = func(string, string) error { return boom }

	f := writeTemp(t, ".env", "SOMEKEY=v\n")
	if err := LoadEnv(f); !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
}

func TestFilePrefixed(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s")
	if err := os.WriteFile(p, []byte("val"), 0o600); err != nil {
		t.Fatal(err)
	}
	type Config struct {
		S string `env:"S,file"`
	}
	var cfg Config
	if err := Load(&cfg, WithPrefix("APP_"), WithLookuper(MapLookuper{"APP_S": p})); err != nil {
		t.Fatal(err)
	}
	if cfg.S != "val" {
		t.Fatalf("got %q", cfg.S)
	}
}
