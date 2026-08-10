package vcard_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/vcard"
)

func card(fn, uid string) string {
	return "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:" + fn + "\r\nUID:" + uid + "\r\nEND:VCARD"
}

// The defect this file exists for: import and restore stored cards with the terminator
// stripped, export concatenated them, and the result read back as one contact — or none.
func TestTerminated_ConcatenatedCardsStillSplit(t *testing.T) {
	var raw, fixed strings.Builder
	for _, c := range []struct{ fn, uid string }{{"Alpha", "a"}, {"Beta", "b"}, {"Gamma", "c"}} {
		raw.WriteString(card(c.fn, c.uid))
		fixed.WriteString(vcard.Terminated(card(c.fn, c.uid)))
	}

	require.Contains(t, raw.String(), "END:VCARDBEGIN:VCARD",
		"guard: without Terminated the cards really do run together")
	require.Len(t, vcard.SplitVCards(raw.String()), 1,
		"guard: and the splitter really does lose the rest")

	require.Len(t, vcard.SplitVCards(fixed.String()), 3)
}

func TestTerminated(t *testing.T) {
	tests := map[string]string{
		"END:VCARD":         "END:VCARD\r\n",
		"END:VCARD\r\n":     "END:VCARD\r\n",
		"END:VCARD\n":       "END:VCARD\r\n",
		"END:VCARD\r\n\r\n": "END:VCARD\r\n",
		"":                  "",
		"\r\n":              "",
	}
	for in, want := range tests {
		require.Equal(t, want, vcard.Terminated(in), "input %q", in)
	}
}
