package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/uptrace/bun"

	"github.com/gumeniukcom/contactshq/internal/config"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

// Exit codes. Anything an operator might script against gets a stable meaning.
const (
	exitOK          = 0
	exitFailure     = 1
	exitUsage       = 2
	exitNoSuchUser  = 3
	exitNoDatabase  = 4
	exitNotMigrated = 5
)

// subcommands is the whitelist. An unrecognised first argument is a usage error, never a
// silent fallthrough to starting the server — an operator who typos `set-passwrd` must not
// end up with a running server and a password they think they changed.
var subcommands = map[string]func(args []string, stdin io.Reader, stdout, stderr io.Writer) int{
	"set-password":    runSetPassword,
	"reencode-vcards": runReencodeVCards,
	"version":         runVersion,
	"help":            runHelp,
}

// looksLikeSubcommand reports whether argv asks for a subcommand rather than the server.
// A leading dash is a flag, so `contactshq -foo` stays server invocation.
func looksLikeSubcommand(args []string) bool {
	return len(args) > 1 && !strings.HasPrefix(args[1], "-")
}

// runCLI dispatches a subcommand and returns the process exit code. It returns handled=false
// when argv names no subcommand, in which case the caller starts the server as before.
func runCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) (handled bool, code int) {
	if !looksLikeSubcommand(args) {
		return false, exitOK
	}

	name := args[1]
	cmd, ok := subcommands[name]
	if !ok {
		fmt.Fprintf(stderr, "unknown command %q\n\n", name)
		printUsage(stderr)
		return true, exitUsage
	}
	return true, cmd(args[2:], stdin, stdout, stderr)
}

// parseInterleaved parses flags that may appear after positional arguments.
//
// flag.Parse stops at the first non-flag token, so `set-password you@example.com --stdin` —
// the form anyone would actually type, and the one in the README — would otherwise be read
// as an unknown extra argument with --stdin silently ignored.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positionals []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positionals, nil
		}
		positionals = append(positionals, fs.Arg(0))
		rest = fs.Args()[1:]
	}
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `ContactsHQ

Usage:
  contactshq                          start the server
  contactshq set-password <email>     set a user's password (interactive)
  contactshq reencode-vcards          rewrite stored vCards with the current encoder
  contactshq version                  print version information
  contactshq help                     print this message

Run a subcommand with -h for its own flags.
`)
}

func runHelp(_ []string, _ io.Reader, stdout, _ io.Writer) int {
	printUsage(stdout)
	return exitOK
}

func runVersion(_ []string, _ io.Reader, stdout, _ io.Writer) int {
	fmt.Fprintf(stdout, "contactshq %s (built %s)\n", Version, BuildTime)
	return exitOK
}

// openCLIDatabase loads configuration, connects, and refuses to proceed on an unmigrated
// database. The loaded configuration is returned as well: a subcommand that reports on
// runtime behaviour must quote the settings this process actually read, not a literal.
//
// Migrations are deliberately not run here. The server applies them inside a transaction at
// startup; a second process doing the same concurrently is a race with no upside, and on
// SQLite it would contend for the single connection the server holds.
func openCLIDatabase(stderr io.Writer) (*bun.DB, *config.Config, int) {
	cfg, err := config.LoadForCLI()
	if err != nil {
		fmt.Fprintf(stderr, "failed to load config: %v\n", err)
		return nil, nil, exitFailure
	}

	db, err := repository.NewDB(cfg.Database)
	if err != nil {
		fmt.Fprintf(stderr, "failed to connect to the database: %v\n", err)
		return nil, nil, exitNoDatabase
	}

	var applied int
	row := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM schema_migrations")
	if err := row.Scan(&applied); err != nil || applied == 0 {
		_ = db.Close()
		fmt.Fprintln(stderr, "the database has no schema yet — start the server once so it can run migrations, then retry")
		return nil, nil, exitNotMigrated
	}

	return db, cfg, exitOK
}
