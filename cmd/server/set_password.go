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

	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
	"golang.org/x/term"
)

// set-password lives apart from the dispatch table because it belongs to the identity domain
// (spec 001), while the subcommand machinery beside it belongs to runtime and delivery
// (spec 008). One file could not be claimed by both — see constitution Principle VII.

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
