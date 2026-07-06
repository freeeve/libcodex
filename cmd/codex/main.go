// Command codex is a small toolkit for inspecting and converting the
// bibliographic records libcodex reads and writes. It wires the library's format
// codecs behind four subcommands:
//
//	codex cat       [-i fmt] [-t tags] [-n N] [--json] [file...]
//	codex convert   [-i fmt] -o fmt [file...]
//	codex validate  [-i fmt] [file...]
//	codex stats     [-i fmt] [file...]
//
// Input format is auto-detected when -i is omitted. With no file arguments each
// subcommand reads standard input.
package main

import (
	"fmt"
	"io"
	"os"
)

// version is the build version, stamped at release time via
// -ldflags "-X main.version=<tag>"; it is "dev" for a plain go build/install.
var version = "dev"

// usageText is the top-level help, listing the registered input/output formats.
func usageText() string {
	return `codex -- inspect and convert bibliographic records

usage:
  codex cat       [-i fmt] [-t tags] [-n N] [--json] [file...]   readable dump
  codex convert   [-i fmt] -o fmt [file...]                      transcode
  codex validate  [-i fmt] [file...]                             structural check
  codex stats     [-i fmt] [file...]                             field/leader report

  -i is the input format (auto-detected when omitted).
  input formats:  ` + formatNames(readers) + `
  output formats: ` + formatNames(writers) + `

With no files, reads standard input.`
}

// main dispatches to the named subcommand and maps its error to an exit code.
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usageText())
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	if err := run(cmd, args, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "codex "+cmd+": "+err.Error())
		os.Exit(1)
	}
}

// run executes one subcommand against stdout, returning any error. It is split
// from main so tests can drive each subcommand directly.
func run(cmd string, args []string, stdout io.Writer) error {
	switch cmd {
	case "cat":
		return runCat(args, stdout)
	case "convert":
		return runConvert(args, stdout)
	case "validate":
		return runValidate(args, stdout)
	case "stats":
		return runStats(args, stdout)
	case "help", "-h", "--help":
		fmt.Fprintln(stdout, usageText())
		return nil
	case "version", "-v", "--version":
		fmt.Fprintln(stdout, "codex "+version)
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q (try: cat, convert, validate, stats)", cmd)
	}
}
