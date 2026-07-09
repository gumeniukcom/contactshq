package vcard_test

import (
	"strings"
	"testing"

	"github.com/gumeniukcom/contactshq/internal/vcard"
)

// A vCard with an embedded photo carries a single base64 line far longer than
// bufio.Scanner's 64 KiB default. Everything after it used to disappear silently.
func TestSplitVCards_LongPhotoLineDoesNotTruncate(t *testing.T) {
	photo := strings.Repeat("A", 200_000)

	data := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:with-photo\r\nFN:Photo Person\r\n" +
		"PHOTO:data:image/jpeg;base64," + photo + "\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:4.0\r\nUID:after-photo\r\nFN:Later Person\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:4.0\r\nUID:last\r\nFN:Last Person\r\nEND:VCARD\r\n"

	cards := vcard.SplitVCards(data)

	if len(cards) != 3 {
		t.Fatalf("SplitVCards returned %d cards, want 3 — contacts after a long line were dropped", len(cards))
	}
	if !strings.Contains(cards[0], photo) {
		t.Error("the photo payload was truncated")
	}
	if !strings.Contains(cards[1], "UID:after-photo") {
		t.Error("the card following the photo is missing")
	}
	if !strings.Contains(cards[2], "UID:last") {
		t.Error("the final card is missing")
	}
}

func TestSplitVCards_LineEndings(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"crlf", "BEGIN:VCARD\r\nUID:a\r\nEND:VCARD\r\n"},
		{"lf", "BEGIN:VCARD\nUID:a\nEND:VCARD\n"},
		{"no trailing newline", "BEGIN:VCARD\r\nUID:a\r\nEND:VCARD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cards := vcard.SplitVCards(tt.data)
			if len(cards) != 1 {
				t.Fatalf("got %d cards, want 1", len(cards))
			}
			if !strings.Contains(cards[0], "UID:a") {
				t.Errorf("card content lost: %q", cards[0])
			}
			if strings.Contains(cards[0], "\r\r") {
				t.Errorf("mangled line endings: %q", cards[0])
			}
		})
	}
}

func TestSplitVCards_Empty(t *testing.T) {
	if got := vcard.SplitVCards(""); len(got) != 0 {
		t.Fatalf("got %d cards from empty input, want 0", len(got))
	}
}

// InjectUID must see a UID that sits after a very long line, or it writes a second one.
func TestInjectUID_FindsUIDAfterLongLine(t *testing.T) {
	long := strings.Repeat("B", 200_000)
	data := "BEGIN:VCARD\r\nPHOTO:" + long + "\r\nUID:existing\r\nEND:VCARD\r\n"

	got := vcard.InjectUID(data, "generated")

	if strings.Contains(got, "UID:generated") {
		t.Error("InjectUID added a duplicate UID; it failed to see the existing one")
	}
	if strings.Count(got, "UID:") != 1 {
		t.Errorf("expected exactly one UID line, got %d", strings.Count(got, "UID:"))
	}
}

func TestInjectUID_AddsWhenMissing(t *testing.T) {
	got := vcard.InjectUID("BEGIN:VCARD\r\nFN:Nobody\r\nEND:VCARD\r\n", "generated")

	if !strings.Contains(got, "UID:generated") {
		t.Fatalf("UID was not injected: %q", got)
	}
	if strings.Index(got, "UID:generated") > strings.Index(got, "END:VCARD") {
		t.Error("UID must be inserted before END:VCARD")
	}
}
