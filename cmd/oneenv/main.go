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
//	oneenv -- go run ./...        # run a command with the env loaded
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

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

func run(args []string) error {
	var (
		files    fileList
		asJSON   bool
		override bool
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(files) == 0 {
		files = fileList{".env"}
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
