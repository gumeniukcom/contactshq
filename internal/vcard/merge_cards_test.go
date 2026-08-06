package vcard

import (
	"strings"
	"testing"

	gvcard "github.com/emersion/go-vcard"
	"github.com/stretchr/testify/require"
)

const (
	cardWinner = "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:w1\r\nFN:Ada Lovelace\r\n" +
		"N:Lovelace;Ada;;;\r\n" +
		"EMAIL;TYPE=work:ada.work@example.com\r\n" +
		"TEL;TYPE=cell:+15550001\r\n" +
		"ORG:Analytical Engines\r\n" +
		"END:VCARD\r\n"
	cardLoser = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:l1\r\nFN:Ada L\r\n" +
		"N:L;Ada;;;\r\n" +
		"EMAIL;TYPE=home:ada.home@example.com\r\n" +
		"TEL;TYPE=cell:+15550001\r\n" +
		"NOTE:met at a conference\r\n" +
		"END:VCARD\r\n"
)

func idsFor(t *testing.T, refs []ValueRef, property string, values ...string) []ValueID {
	t.Helper()
	var out []ValueID
	for _, want := range values {
		found := false
		for _, r := range refs {
			if r.Property == property && r.Value == want {
				out = append(out, r.ID)
				found = true
				break
			}
		}
		require.True(t, found, "no candidate %s=%q", property, want)
	}
	return out
}

func decodeMerged(t *testing.T, card string) gvcard.Card {
	t.Helper()
	decoded, err := gvcard.NewDecoder(strings.NewReader(card)).Decode()
	require.NoError(t, err)
	return decoded
}

// The whole point of 4.4: per-value choice. Whole-property replacement could not express
// "the work address from one card and the home address from the other".
func TestMergeCards_KeepsValuesFromBothSides(t *testing.T) {
	refs, err := Candidates(cardWinner, cardLoser)
	require.NoError(t, err)

	sel := Selection{
		"EMAIL": idsFor(t, refs, "EMAIL", "ada.work@example.com", "ada.home@example.com"),
	}

	merged, err := MergeCards(cardWinner, cardLoser, sel)
	require.NoError(t, err)

	emails := decodeMerged(t, merged).Values(gvcard.FieldEmail)
	require.ElementsMatch(t, []string{"ada.work@example.com", "ada.home@example.com"}, emails,
		"both selected emails must survive")
}

// Parameters travel with the value: an email is "the work one" because of its TYPE.
func TestMergeCards_PreservesParametersOfKeptValues(t *testing.T) {
	refs, err := Candidates(cardWinner, cardLoser)
	require.NoError(t, err)

	sel := Selection{"EMAIL": idsFor(t, refs, "EMAIL", "ada.home@example.com")}
	merged, err := MergeCards(cardWinner, cardLoser, sel)
	require.NoError(t, err)

	require.Contains(t, merged, "EMAIL;TYPE=home:ada.home@example.com")
	require.NotContains(t, merged, "ada.work@example.com")
}

// UID and VERSION identify the record that survives; taking either from the loser would
// rename a contact every CardDAV client already knows.
func TestMergeCards_IdentityAlwaysComesFromTheWinner(t *testing.T) {
	merged, err := MergeCards(cardWinner, cardLoser, Selection{})
	require.NoError(t, err)

	card := decodeMerged(t, merged)
	require.Equal(t, "w1", card.Value(gvcard.FieldUID))
	require.Equal(t, "4.0", card.Value(gvcard.FieldVersion))
}

// A property nobody spoke about keeps the winner's values, so an empty selection means
// "keep this record" rather than "discard everything".
func TestMergeCards_EmptySelectionYieldsTheWinner(t *testing.T) {
	merged, err := MergeCards(cardWinner, cardLoser, Selection{})
	require.NoError(t, err)

	card := decodeMerged(t, merged)
	require.Equal(t, "Ada Lovelace", card.Value(gvcard.FieldFormattedName))
	require.Equal(t, "ada.work@example.com", card.Value(gvcard.FieldEmail))
	require.Equal(t, "Analytical Engines", card.Value(gvcard.FieldOrganization))
	require.Empty(t, card.Value(gvcard.FieldNote), "the loser's note was not asked for")
}

// A property mentioned with an empty list is an explicit "drop this", unlike one left out.
func TestMergeCards_ExplicitEmptyListDropsTheProperty(t *testing.T) {
	merged, err := MergeCards(cardWinner, cardLoser, Selection{"ORG": {}})
	require.NoError(t, err)

	card := decodeMerged(t, merged)
	require.Empty(t, card.Value(gvcard.FieldOrganization))
	require.Equal(t, "Ada Lovelace", card.Value(gvcard.FieldFormattedName), "other properties are untouched")
}

// Identical values on both cards collapse to one candidate and must not be emitted twice.
func TestMergeCards_IdenticalValueOnBothSidesAppearsOnce(t *testing.T) {
	refs, err := Candidates(cardWinner, cardLoser)
	require.NoError(t, err)

	var phoneRefs []ValueRef
	for _, r := range refs {
		if r.Property == "TEL" {
			phoneRefs = append(phoneRefs, r)
		}
	}
	require.Len(t, phoneRefs, 1, "the same number with the same parameters is one candidate")
	require.Equal(t, "both", phoneRefs[0].Side)

	merged, err := MergeCards(cardWinner, cardLoser, Selection{"TEL": {phoneRefs[0].ID}})
	require.NoError(t, err)
	require.Len(t, decodeMerged(t, merged).Values(gvcard.FieldTelephone), 1)
}

// The identifier must follow the value, not its position: a card edited between loading the
// screen and submitting it would otherwise silently select something else.
func TestMergeCards_ValueIDsAreContentBasedNotPositional(t *testing.T) {
	reordered := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:w1\r\nFN:Ada Lovelace\r\n" +
		"EMAIL;TYPE=work:ada.work@example.com\r\n" +
		"EMAIL;TYPE=home:added.later@example.com\r\n" +
		"END:VCARD\r\n"

	first, err := Candidates(cardWinner, cardLoser)
	require.NoError(t, err)
	second, err := Candidates(reordered, cardLoser)
	require.NoError(t, err)

	require.Equal(t,
		idsFor(t, first, "EMAIL", "ada.work@example.com"),
		idsFor(t, second, "EMAIL", "ada.work@example.com"),
		"the same value must keep the same id across loads")
}

// Candidates is what a UI renders; it must not offer the identity properties as choices.
func TestCandidates_ExcludesIdentityProperties(t *testing.T) {
	refs, err := Candidates(cardWinner, cardLoser)
	require.NoError(t, err)

	for _, r := range refs {
		require.NotEqual(t, "UID", r.Property)
		require.NotEqual(t, "VERSION", r.Property)
	}
}

func TestCandidates_MarksWhichSideEachValueCameFrom(t *testing.T) {
	refs, err := Candidates(cardWinner, cardLoser)
	require.NoError(t, err)

	sides := map[string]string{}
	for _, r := range refs {
		sides[r.Property+"="+r.Value] = r.Side
	}

	require.Equal(t, "winner", sides["EMAIL=ada.work@example.com"])
	require.Equal(t, "loser", sides["EMAIL=ada.home@example.com"])
	require.Equal(t, "both", sides["TEL=+15550001"])
}

func TestMergeCards_RejectsUnreadableInput(t *testing.T) {
	_, err := MergeCards("not a vcard", cardLoser, Selection{})
	require.Error(t, err)
}

// A merge result has to be re-mergeable: the ids of the output must still resolve.
func TestMergeCards_OutputIsItselfAValidInput(t *testing.T) {
	refs, err := Candidates(cardWinner, cardLoser)
	require.NoError(t, err)

	merged, err := MergeCards(cardWinner, cardLoser, Selection{
		"EMAIL": idsFor(t, refs, "EMAIL", "ada.work@example.com", "ada.home@example.com"),
	})
	require.NoError(t, err)

	again, err := MergeCards(merged, cardLoser, Selection{})
	require.NoError(t, err)
	require.Len(t, decodeMerged(t, again).Values(gvcard.FieldEmail), 2)
}
