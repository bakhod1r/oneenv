// Command oneenv is a small CLI around the oneenv package.
//
// Usage:
//
//	oneenv [flags] [-- command args...]
//
// With a command, it loads the .env files into the environment and execs the
// command (like `dotenv`). Without a command, it prints the merged variables.
//
//	oneenv                        # print merged .env as KEY=VALUE
//	oneenv -f .env -f .env.local  # merge several files
//	oneenv -json                  # print as JSON
//	oneenv -example               # write .env.example (keys only, values stripped)
//	oneenv -- go run ./...        # run a command with the env loaded
//
// It also has subcommands for working on the files themselves:
//
//	oneenv doctor                 # check files, duplicates, empty and shadowed keys
//	oneenv lint                   # report syntax and naming problems, non-zero exit
//	oneenv format [-w]            # sort keys, keeping the comments above them
//	oneenv explain KEY            # where one key's value comes from
//	oneenv init                   # create a starter .env and .env.example
//	oneenv migrate [-w] OLD=NEW   # rename keys across the files
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bakhod1r/oneenv"
)

type fileList []string

func (f *fileList) String() string     { return strings.Join(*f, ",") }
func (f *fileList) Set(v string) error { *f = append(*f, v); return nil }

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "oneenv:", err)
		os.Exit(1)
	}
}

// subcommands are dispatched before flag parsing, so `oneenv doctor -f .env`
// reads naturally. Anything starting with '-' is a flag for the default mode.
func runSubcommand(name string, args []string) (bool, error) {
	switch name {
	case "doctor", "lint", "format", "explain", "init", "migrate":
	default:
		return false, nil
	}

	var (
		files fileList
		write bool
	)
	fs := flag.NewFlagSet("oneenv "+name, flag.ContinueOnError)
	fs.Var(&files, "f", "env file to read (repeatable)")
	fs.BoolVar(&write, "w", false, "write changes back to the files")
	// Parse repeatedly so flags may appear after the positional arguments:
	// `oneenv explain PORT -f .env` reads more naturally than the reverse.
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			return true, err
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		rest = append(rest, args[0])
		args = args[1:]
	}
	if len(files) == 0 {
		files = fileList{".env", ".env.local"}
		if name == "init" {
			files = fileList{".env"}
		}
	}

	switch name {
	case "doctor":
		return true, doctor(os.Stdout, files)
	case "lint":
		return true, lintFiles(os.Stdout, files)
	case "format":
		return true, formatFiles(os.Stdout, files, write)
	case "init":
		return true, initFiles(os.Stdout, files)
	case "explain":
		if len(rest) != 1 {
			return true, fmt.Errorf("usage: oneenv explain KEY")
		}
		return true, explain(os.Stdout, files, rest[0])
	default: // migrate
		renames := make(map[string]string, len(rest))
		for _, pair := range rest {
			from, to, ok := strings.Cut(pair, "=")
			if !ok {
				return true, fmt.Errorf("migrate: expected OLD=NEW, got %q", pair)
			}
			renames[from] = to
		}
		return true, migrate(os.Stdout, files, renames, write)
	}
}

func run(args []string) error {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		if handled, err := runSubcommand(args[0], args[1:]); handled {
			return err
		}
	}

	var (
		files    fileList
		asJSON   bool
		override bool
		example  bool
		exOut    string
	)

	// Split flags from the optional "-- command...".
	var cmd []string
	if i := indexOf(args, "--"); i >= 0 {
		cmd = args[i+1:]
		args = args[:i]
	}

	fs := flag.NewFlagSet("oneenv", flag.ContinueOnError)
	fs.Var(&files, "f", "env file to read (repeatable)")
	fs.BoolVar(&asJSON, "json", false, "print merged variables as JSON")
	fs.BoolVar(&override, "override", false, "let file values override the environment")
	fs.BoolVar(&example, "example", false, "write a value-stripped example of the .env files")
	fs.StringVar(&exOut, "o", ".env.example", `output path for -example ("-" for stdout)`)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(files) == 0 {
		files = fileList{".env"}
	}

	if example {
		return writeExample(files, exOut)
	}

	if len(cmd) > 0 {
		return execWith(files, override, cmd)
	}

	vals, err := oneenv.Read(files...)
	if err != nil {
		return err
	}
	return printVals(vals, asJSON)
}

// writeExample reads the .env files and writes them back with every value
// stripped, producing a shareable .env.example. dst of "-" means stdout.
func writeExample(files fileList, dst string) error {
	vals, err := oneenv.Read(files...)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	comments := readComments(files)

	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('\n')
		}
		for _, c := range comments[k] {
			sb.WriteString(c)
			sb.WriteByte('\n')
		}
		sb.WriteString("# type: ")
		sb.WriteString(inferType(vals[k]))
		sb.WriteString("\n# required: this field\n")
		sb.WriteString(k)
		sb.WriteString("=\n")
	}
	if dst == "-" {
		_, err := os.Stdout.WriteString(sb.String())
		return err
	}
	return os.WriteFile(dst, []byte(sb.String()), 0o644)
}

// readComments scans the raw .env files and returns, per key, the comment
// lines that sit directly above it. Later files win for a repeated key.
func readComments(files fileList) map[string][]string {
	out := make(map[string][]string)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var pending []string
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			switch {
			case line == "":
				pending = nil
			case strings.HasPrefix(line, "#"):
				pending = append(pending, line)
			default:
				kv := strings.TrimPrefix(line, "export ")
				if k, _, ok := strings.Cut(kv, "="); ok {
					k = strings.TrimSpace(k)
					if len(pending) > 0 {
						out[k] = pending
					}
				}
				pending = nil
			}
		}
	}
	return out
}

// inferType guesses a value's type from its shape, for the -example comments.
func inferType(v string) string {
	switch {
	case v == "":
		return "string"
	case v == "true" || v == "false":
		return "bool"
	default:
		if _, err := strconv.Atoi(v); err == nil {
			return "int"
		}
		if _, err := strconv.ParseFloat(v, 64); err == nil {
			return "float"
		}
		if _, err := time.ParseDuration(v); err == nil {
			return "duration"
		}
		if strings.Contains(v, "://") {
			return "url"
		}
		return "string"
	}
}

func execWith(files fileList, override bool, cmd []string) error {
	load := oneenv.LoadEnv
	if override {
		load = oneenv.Overload
	}
	if err := load(files...); err != nil {
		return err
	}
	c := exec.Command(cmd[0], cmd[1:]...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func printVals(vals map[string]string, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(vals)
	}
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s=%s\n", k, vals[k])
	}
	return nil
}

func indexOf(s []string, target string) int {
	for i, v := range s {
		if v == target {
			return i
		}
	}
	return -1
}
