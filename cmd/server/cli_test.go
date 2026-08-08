package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/config"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

func runArgs(t *testing.T, stdin string, argv ...string) (handled bool, code int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	handled, code = runCLI(append([]string{"contactshq"}, argv...), strings.NewReader(stdin), &out, &errBuf)
	return handled, code, out.String(), errBuf.String()
}

// No subcommand means "run the server"; the dispatcher must stay out of the way.
func TestRunCLI_NoArgumentsFallsThroughToTheServer(t *testing.T) {
	handled, _, _, _ := runArgs(t, "")
	require.False(t, handled, "a bare invocation must start the server")
}

// A leading dash is a server flag, not a subcommand.
func TestRunCLI_FlagsFallThroughToTheServer(t *testing.T) {
	handled, _, _, _ := runArgs(t, "", "-config", "/etc/chq.yaml")
	require.False(t, handled)
}

// A typo must never silently start a server while the operator believes a password changed.
func TestRunCLI_UnknownSubcommandIsAUsageError(t *testing.T) {
	handled, code, _, stderr := runArgs(t, "", "set-passwrd", "owner@example.com")
	require.True(t, handled, "an unknown subcommand must not fall through to the server")
	require.Equal(t, exitUsage, code)
	require.Contains(t, stderr, "unknown command")
	require.Contains(t, stderr, "Usage:")
}

func TestRunCLI_VersionAndHelp(t *testing.T) {
	handled, code, stdout, _ := runArgs(t, "", "version")
	require.True(t, handled)
	require.Equal(t, exitOK, code)
	require.Contains(t, stdout, "contactshq")

	handled, code, stdout, _ = runArgs(t, "", "help")
	require.True(t, handled)
	require.Equal(t, exitOK, code)
	require.Contains(t, stdout, "set-password")
}

// The password must never be accepted as an argument: argv is visible in ps, the shell
// history and docker inspect.
func TestSetPassword_RefusesAPasswordArgument(t *testing.T) {
	_, code, _, _ := runArgs(t, "", "set-password", "owner@example.com", "hunter2")
	require.Equal(t, exitUsage, code, "a second positional argument must not be taken as the password")
}

func TestSetPassword_RequiresAnEmail(t *testing.T) {
	_, code, _, _ := runArgs(t, "", "set-password")
	require.Equal(t, exitUsage, code)
}

// Reading from a pipe requires --stdin; without it the command must say so rather than
// hanging or silently reading a line. A usable database is set up first because the command
// now checks the schema before it asks for a password.
func TestSetPassword_NonTerminalWithoutStdinFlagIsRefused(t *testing.T) {
	seedCLIUser(t, newCLIDatabaseEnv(t), "owner@example.com")

	_, code, _, stderr := runArgs(t, "some-password\n", "set-password", "owner@example.com")
	require.Equal(t, exitUsage, code)
	require.Contains(t, stderr, "--stdin")
}

func TestSetPassword_EmptyStdinIsRefused(t *testing.T) {
	seedCLIUser(t, newCLIDatabaseEnv(t), "owner@example.com")

	_, code, _, stderr := runArgs(t, "\n", "set-password", "owner@example.com", "--stdin")
	require.Equal(t, exitUsage, code)
	require.Contains(t, stderr, "no password")
}

// flag.Parse stops at the first positional, so a flag written after the email — the form the
// README shows and the one anyone would type — used to be dropped.
func TestParseInterleaved_AcceptsFlagsAfterPositionals(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(&bytes.Buffer{})
	stdinFlag := fs.Bool("stdin", false, "")

	positionals, err := parseInterleaved(fs, []string{"owner@example.com", "--stdin"})
	require.NoError(t, err)
	require.Equal(t, []string{"owner@example.com"}, positionals)
	require.True(t, *stdinFlag, "a flag after a positional argument must still be parsed")
}

func TestParseInterleaved_AcceptsFlagsBeforePositionals(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(&bytes.Buffer{})
	stdinFlag := fs.Bool("stdin", false, "")

	positionals, err := parseInterleaved(fs, []string{"--stdin", "owner@example.com"})
	require.NoError(t, err)
	require.Equal(t, []string{"owner@example.com"}, positionals)
	require.True(t, *stdinFlag)
}

// The success message has to state what the operator would otherwise assume wrongly: that
// changing the password signs everyone out.
func TestSetPassword_WarningsAreSpelledOut(t *testing.T) {
	text := setPasswordEpilogue(time.Hour, 168*time.Hour)
	require.Contains(t, text, "Access tokens remain valid")
	require.Contains(t, text, "CardDAV")
	require.Contains(t, text, "CHQ_AUTH_JWT_SECRET")
	// The numbers are only trustworthy for the configuration this process read; a server
	// started with a different environment may be using others. Saying so is what keeps the
	// message from trading one confident wrong number for another.
	require.Contains(t, text, "THIS process")
}

func TestHumanTTL(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{time.Hour, "1h"},
		{168 * time.Hour, "168h"},
		{90 * time.Minute, "1h30m"},
		{30 * time.Minute, "30m"},
		{0, "0s"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, humanTTL(tc.in), "humanTTL(%v)", tc.in)
	}
}

// newCLIDatabaseEnv points the CLI's own config loading at a fresh SQLite file and returns
// its path. Nothing is migrated: a caller that needs a schema asks for it.
func newCLIDatabaseEnv(t *testing.T) string {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "contactshq.db")
	t.Setenv("CHQ_DATABASE_DRIVER", "sqlite")
	t.Setenv("CHQ_DATABASE_DSN", dsn)
	return dsn
}

// seedCLIUser migrates the database the CLI will open and puts one user in it.
func seedCLIUser(t *testing.T, dsn, email string) {
	t.Helper()
	db, err := repository.NewDB(config.DatabaseConfig{Driver: "sqlite", DSN: dsn})
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	require.NoError(t, repository.Migrate(context.Background(), db))

	user := &domain.User{
		ID:           uuid.NewString(),
		Email:        email,
		PasswordHash: "placeholder",
		Role:         "admin",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	require.NoError(t, repository.NewBunUserRepository(db).Create(context.Background(), user))
}

// The epilogue exists to tell an operator how long a compromised session outlives the reset,
// so a hardcoded number that drifts away from the defaults is the message failing. Only
// driving the whole command can catch that: asserting on the formatter alone stays green if
// the call site passes constants.
func TestSetPassword_EpilogueQuotesTheRunningConfiguration(t *testing.T) {
	dsn := newCLIDatabaseEnv(t)
	t.Setenv("CHQ_AUTH_TOKEN_TTL", "4h")
	t.Setenv("CHQ_AUTH_REFRESH_TTL", "48h")
	seedCLIUser(t, dsn, "owner@example.com")

	_, code, stdout, stderr := runArgs(t, "correct-horse\n", "set-password", "owner@example.com", "--stdin")
	require.Equal(t, exitOK, code, "stderr: %s", stderr)

	require.Contains(t, stdout, "4h", "the epilogue must quote the configured access token lifetime")
	require.Contains(t, stdout, "48h", "the epilogue must quote the configured refresh token lifetime")
	require.NotContains(t, stdout, "24h", "the pre-0.4.0 default must not be printed")
	require.NotContains(t, stdout, "720h", "the pre-0.4.0 default must not be printed")
}

// countingReader records whether anything read from it at all.
type countingReader struct {
	r    io.Reader
	read bool
}

func (c *countingReader) Read(p []byte) (int, error) {
	c.read = true
	return c.r.Read(p)
}

// Asking for a password and only then discovering there is no schema makes the operator type
// a secret twice for nothing.
func TestSetPassword_RefusesAnUnmigratedDatabaseBeforePrompting(t *testing.T) {
	newCLIDatabaseEnv(t)

	stdin := &countingReader{r: strings.NewReader("correct-horse\n")}
	var out, errBuf bytes.Buffer
	handled, code := runCLI(
		[]string{"contactshq", "set-password", "owner@example.com", "--stdin"},
		stdin, &out, &errBuf,
	)

	require.True(t, handled)
	require.Equal(t, exitNotMigrated, code)
	require.Contains(t, errBuf.String(), "no schema yet")
	require.False(t, stdin.read, "the password must not be read before the database is known to be usable")
}
