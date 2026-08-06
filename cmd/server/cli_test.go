package main

import (
	"bytes"
	"flag"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
// hanging or silently reading a line.
func TestSetPassword_NonTerminalWithoutStdinFlagIsRefused(t *testing.T) {
	_, code, _, stderr := runArgs(t, "some-password\n", "set-password", "owner@example.com")
	require.Equal(t, exitUsage, code)
	require.Contains(t, stderr, "--stdin")
}

func TestSetPassword_EmptyStdinIsRefused(t *testing.T) {
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
	// Only the text is under test here; drive it through the usage path so no database is
	// needed, then assert on the constant the success path prints.
	require.Contains(t, setPasswordEpilogue, "Access tokens remain valid")
	require.Contains(t, setPasswordEpilogue, "CardDAV")
	require.Contains(t, setPasswordEpilogue, "CHQ_AUTH_JWT_SECRET")
}
