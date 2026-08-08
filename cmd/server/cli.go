package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"golang.org/x/term"

	"github.com/gumeniukcom/contactshq/internal/config"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
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

func runSetPassword(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("set-password", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fromStdin := fs.Bool("stdin", false, "read the new password from stdin instead of prompting")
	fs.Usage = func() {
		fmt.Fprint(stderr, `Usage: contactshq set-password <email> [--stdin]

Sets a user's password without needing the old one. The password is never taken as a
command-line argument: it would be visible in ps, the shell history and docker inspect.
`)
		fs.PrintDefaults()
	}

	positionals, err := parseInterleaved(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(positionals) != 1 {
		fs.Usage()
		return exitUsage
	}
	email := strings.TrimSpace(positionals[0])
	if email == "" {
		fs.Usage()
		return exitUsage
	}

	// The database is opened first on purpose: an operator whose database has no schema would
	// otherwise type a password twice and only then be told the command cannot proceed.
	db, cfg, code := openCLIDatabase(stderr)
	if code != exitOK {
		return code
	}
	defer func() { _ = db.Close() }()

	password, code := readNewPassword(*fromStdin, stdin, stdout, stderr)
	if code != exitOK {
		return code
	}

	userSvc := service.NewUserService(repository.NewBunUserRepository(db))

	switch err := userSvc.SetPassword(context.Background(), email, password); {
	case err == nil:
	case errors.Is(err, service.ErrUserNotFound):
		fmt.Fprintf(stderr, "no user with email %s\n", email)
		return exitNoSuchUser
	case errors.Is(err, service.ErrPasswordTooShort):
		fmt.Fprintf(stderr, "%v\n", err)
		return exitUsage
	default:
		fmt.Fprintf(stderr, "failed to set password: %v\n", err)
		return exitFailure
	}

	// One audit line, carrying neither the password nor the hash.
	fmt.Fprintf(stderr, "password updated for %s\n", email)

	fmt.Fprint(stdout, setPasswordEpilogue(cfg.Auth.TokenTTL, cfg.Auth.RefreshTTL))
	return exitOK
}

// setPasswordEpilogue states the two things an operator would otherwise assume wrongly. Both
// mean "the old credential still works for a while", and neither is visible from the outside.
//
// The lifetimes are quoted from the configuration this process loaded rather than written as
// literals. Hardcoded numbers went stale once already: the text still said 24h/720h after
// v0.4.0 moved the defaults to 1h/168h, and being wrong by 24x about how long a compromised
// session survives is this message failing at the only job it has.
func setPasswordEpilogue(tokenTTL, refreshTTL time.Duration) string {
	return fmt.Sprintf(`
Password updated.

Two things this does NOT do:
  * Existing sessions stay signed in. Access tokens remain valid for their full lifetime
    (%s) and refresh tokens for theirs (%s). To cut them off, rotate
    CHQ_AUTH_JWT_SECRET and restart — that signs everyone out.
  * This command runs in its own process, so a running server keeps its cached CardDAV
    authentication verdicts: a client mid-session may keep working for up to 5 minutes.
    Restart the server to drop that cache at once. (A password changed through the web UI
    takes effect for CardDAV immediately.)

Those lifetimes are the ones THIS process read, from its own environment and working
directory. A server started with a different environment is using different ones.
`, humanTTL(tokenTTL), humanTTL(refreshTTL))
}

// humanTTL prints a duration the way an operator writes it in configuration: "1h", not
// "1h0m0s". Only whole trailing units are dropped, so 90m still reads "1h30m".
func humanTTL(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// readNewPassword obtains the new password without ever putting it in argv.
func readNewPassword(fromStdin bool, stdin io.Reader, stdout, stderr io.Writer) (string, int) {
	if fromStdin {
		reader := bufio.NewReader(stdin)
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintf(stderr, "failed to read the password from stdin: %v\n", err)
			return "", exitFailure
		}
		password := strings.TrimRight(line, "\r\n")
		if password == "" {
			fmt.Fprintln(stderr, "no password on stdin")
			return "", exitUsage
		}
		return password, exitOK
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		fmt.Fprintln(stderr, "stdin is not a terminal — pass --stdin to read the password from a pipe")
		return "", exitUsage
	}

	fmt.Fprint(stdout, "New password: ")
	first, err := term.ReadPassword(fd)
	fmt.Fprintln(stdout)
	if err != nil {
		fmt.Fprintf(stderr, "failed to read the password: %v\n", err)
		return "", exitFailure
	}

	fmt.Fprint(stdout, "Repeat password: ")
	second, err := term.ReadPassword(fd)
	fmt.Fprintln(stdout)
	if err != nil {
		fmt.Fprintf(stderr, "failed to read the password: %v\n", err)
		return "", exitFailure
	}

	if string(first) != string(second) {
		fmt.Fprintln(stderr, "the two passwords do not match")
		return "", exitUsage
	}
	return string(first), exitOK
}
