package vcard_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/vcard"
)

// A file cut off mid-card used to yield one card fewer and no error, so an import reported
// success while losing a contact — and a backup restore did the same after deleting the
// originals. The truncated card is now returned so the caller parses it, fails, and counts it.
func TestSplitVCards_KeepsATruncatedFinalCard(t *testing.T) {
	data := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Complete\r\nUID:a\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Truncated\r\n"

	cards := vcard.SplitVCards(data)
	require.Len(t, cards, 2, "the truncated card must not be dropped in silence")
	require.Contains(t, cards[1], "Truncated")

	_, err := vcard.ParseVCard(cards[1])
	require.Error(t, err, "and the caller must be able to reject it")
}

func TestSplitVCards_UnaffectedWhenEveryCardIsClosed(t *testing.T) {
	data := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:A\r\nUID:a\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:B\r\nUID:b\r\nEND:VCARD\r\n"
	require.Len(t, vcard.SplitVCards(data), 2)
}
