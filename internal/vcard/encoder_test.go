package vcard

import (
	"strings"
	"testing"

	gvcard "github.com/emersion/go-vcard"
	"github.com/stretchr/testify/require"
)

func cardWith(property, value string) gvcard.Card {
	card := make(gvcard.Card)
	card.SetValue(gvcard.FieldVersion, "4.0")
	card.SetValue(gvcard.FieldUID, "uid-1")
	card.SetValue(gvcard.FieldFormattedName, "Ada Lovelace")
	if property != "" {
		card.SetValue(property, value)
	}
	return card
}

func encodeToLines(t *testing.T, card gvcard.Card) []string {
	t.Helper()
	var sb strings.Builder
	require.NoError(t, EncodeCard(&sb, card))
	return strings.Split(strings.TrimSuffix(sb.String(), "\r\n"), "\r\n")
}

func lineFor(t *testing.T, card gvcard.Card, property string) string {
	t.Helper()
	for _, line := range encodeToLines(t, card) {
		if strings.HasPrefix(line, property+":") || strings.HasPrefix(line, property+";") {
			return line
		}
	}
	t.Fatalf("no %s line in the encoded card", property)
	return ""
}

func TestEncodeCard_EscapingIsValueTypeAware(t *testing.T) {
	tests := []struct {
		name     string
		property string
		value    string
		want     string
		why      string
	}{
		{
			name:     "URI keeps its comma",
			property: gvcard.FieldPhoto,
			value:    "data:image/jpeg;base64,/9j/4AAQ",
			want:     "PHOTO:data:image/jpeg;base64,/9j/4AAQ",
			why:      "an escaped comma corrupts the base64 payload and breaks photos on iOS",
		},
		{
			name:     "URL keeps its comma",
			property: gvcard.FieldURL,
			value:    "https://example.com/a,b",
			want:     "URL:https://example.com/a,b",
		},
		{
			name:     "CATEGORIES keeps the separator unescaped",
			property: gvcard.FieldCategories,
			value:    "work,friends",
			want:     "CATEGORIES:work,friends",
			why:      "escaping the separator turns two categories into one named \"work,friends\"",
		},
		{
			name:     "NICKNAME keeps the separator unescaped",
			property: gvcard.FieldNickname,
			value:    "Ada,Countess",
			want:     "NICKNAME:Ada,Countess",
		},
		{
			name:     "structured N keeps both separators",
			property: gvcard.FieldName,
			value:    "Lovelace;Ada;Augusta,Byron;;",
			want:     "N:Lovelace;Ada;Augusta,Byron;;",
		},
		{
			name:     "structured ADR keeps both separators",
			property: gvcard.FieldAddress,
			value:    ";;12 Main St,Apt 3;London;;SW1;UK",
			want:     "ADR:;;12 Main St,Apt 3;London;;SW1;UK",
		},
		{
			name:     "plain TEXT still escapes its comma",
			property: gvcard.FieldNote,
			value:    "hello, world",
			want:     `NOTE:hello\, world`,
			why:      "RFC 6350 §3.4 — a comma in a TEXT value is data and must be escaped",
		},
		{
			name:     "a backslash in TEXT is escaped",
			property: gvcard.FieldNote,
			value:    `a\b`,
			want:     `NOTE:a\\b`,
		},
		{
			name:     "a backslash inside a list item is escaped, the separator is not",
			property: gvcard.FieldCategories,
			value:    `a\b,c`,
			want:     `CATEGORIES:a\\b,c`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lineFor(t, cardWith(tt.property, tt.value), tt.property)
			require.Equal(t, tt.want, got, tt.why)
		})
	}
}

// A carriage return used to travel into the output untouched, producing a bare CR inside a
// line and a card no strict parser will accept.
func TestEncodeCard_NewlinesAreNormalisedAndEscaped(t *testing.T) {
	for _, input := range []string{"first\r\nsecond", "first\rsecond", "first\nsecond"} {
		card := cardWith(gvcard.FieldNote, input)
		var sb strings.Builder
		require.NoError(t, EncodeCard(&sb, card))
		out := sb.String()

		require.Contains(t, out, `NOTE:first\nsecond`)

		// The only CRs in the output are the line terminators.
		body := strings.ReplaceAll(out, "\r\n", "")
		require.NotContains(t, body, "\r", "a bare carriage return survived into the value")
	}
}

// A URI cannot contain a newline; if one is present the value is already broken, and emitting
// it would produce an unparseable card.
func TestEncodeCard_NewlineInAURIIsDropped(t *testing.T) {
	got := lineFor(t, cardWith(gvcard.FieldPhoto, "https://example.com/a\nb"), gvcard.FieldPhoto)
	require.Equal(t, "PHOTO:https://example.com/ab", got)
}

// The layout must match go-vcard's byte for byte, or every stored ETag changes and every
// CardDAV client re-downloads the whole address book.
func TestEncodeCard_LayoutMatchesGoVCardForUnaffectedValues(t *testing.T) {
	build := func() gvcard.Card {
		card := make(gvcard.Card)
		card.SetValue(gvcard.FieldVersion, "4.0")
		card.SetValue(gvcard.FieldUID, "uid-1")
		card.SetValue(gvcard.FieldFormattedName, "Ada Lovelace")
		card.SetValue(gvcard.FieldNote, "a plain note")
		card.SetValue(gvcard.FieldTitle, "Engineer")
		card.SetName(&gvcard.Name{FamilyName: "Lovelace", GivenName: "Ada"})
		card.Add(gvcard.FieldEmail, &gvcard.Field{
			Value:  "ada@example.com",
			Params: gvcard.Params{gvcard.ParamType: []string{"work"}},
		})
		card.Add(gvcard.FieldTelephone, &gvcard.Field{
			Value:  "+15550001",
			Params: gvcard.Params{gvcard.ParamType: []string{"cell", "voice"}},
		})
		return card
	}

	var ours, theirs strings.Builder
	require.NoError(t, EncodeCard(&ours, build()))
	require.NoError(t, gvcard.NewEncoder(&theirs).Encode(build()))

	require.Equal(t, theirs.String(), ours.String(),
		"a card with nothing to fix must serialise exactly as before")
}

// Whatever this encoder writes, the decoder this application reads with must give back the
// original values. That is the constraint that rules out escaping semicolons: go-vcard's
// decoder has no case for `\;`.
func TestEncodeCard_RoundTripsThroughTheDecoder(t *testing.T) {
	values := map[string]string{
		gvcard.FieldPhoto:      "data:image/jpeg;base64,/9j/4AAQ",
		gvcard.FieldURL:        "https://example.com/a,b",
		gvcard.FieldNote:       "hello, world",
		gvcard.FieldCategories: "work,friends",
		gvcard.FieldTitle:      "Engineer",
	}

	card := make(gvcard.Card)
	card.SetValue(gvcard.FieldVersion, "4.0")
	card.SetValue(gvcard.FieldUID, "uid-1")
	card.SetValue(gvcard.FieldFormattedName, "Ada Lovelace")
	for k, v := range values {
		card.SetValue(k, v)
	}

	var sb strings.Builder
	require.NoError(t, EncodeCard(&sb, card))

	decoded, err := gvcard.NewDecoder(strings.NewReader(sb.String())).Decode()
	require.NoError(t, err)

	for property, want := range values {
		require.Equal(t, want, decoded.Value(property),
			"%s did not survive a round trip", property)
	}

	// And the list value really is two categories after decoding — Card.Categories splits on
	// the comma, which is exactly what escaping the separator used to prevent.
	require.Equal(t, []string{"work", "friends"}, decoded.Categories())
}

// Escaping must not accumulate: encoding what was decoded has to be a fixed point, or every
// sync cycle would add another backslash.
func TestEncodeCard_IsIdempotentAcrossRoundTrips(t *testing.T) {
	card := cardWith(gvcard.FieldNote, "hello, world")
	card.SetValue(gvcard.FieldPhoto, "data:image/jpeg;base64,/9j/4AAQ")
	card.SetValue(gvcard.FieldCategories, "work,friends")

	var first strings.Builder
	require.NoError(t, EncodeCard(&first, card))

	decoded, err := gvcard.NewDecoder(strings.NewReader(first.String())).Decode()
	require.NoError(t, err)

	var second strings.Builder
	require.NoError(t, EncodeCard(&second, decoded))

	require.Equal(t, first.String(), second.String(), "escaping accumulated across a round trip")
}

func TestEncodeCard_MissingVersionIsAnError(t *testing.T) {
	card := make(gvcard.Card)
	card.SetValue(gvcard.FieldFormattedName, "Ada")

	var sb strings.Builder
	require.ErrorIs(t, EncodeCard(&sb, card), ErrMissingVersion)
}

// Parameters keep their existing treatment; nothing in this codebase was mis-serialising
// them, and changing it would move ETags for no benefit.
func TestEncodeCard_ParametersAreUnchanged(t *testing.T) {
	card := cardWith("", "")
	card.Add(gvcard.FieldEmail, &gvcard.Field{
		Value:  "ada@example.com",
		Params: gvcard.Params{gvcard.ParamType: []string{"work"}},
	})

	require.Equal(t, "EMAIL;TYPE=work:ada@example.com", lineFor(t, card, gvcard.FieldEmail))
}
