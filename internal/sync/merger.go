package sync

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/emersion/go-vcard"
)

// ErrRemoteCardEmpty is returned when a wildcard resolution asks for the remote card and
// there is no remote card to take.
var ErrRemoteCardEmpty = errors.New("cannot resolve to the remote card: it is empty")

// FieldConflict describes a per-field conflict between local and remote vCard.
type FieldConflict struct {
	Field  string `json:"field"`
	Base   string `json:"base"`
	Local  string `json:"local"`
	Remote string `json:"remote"`
}

// MergeResult holds the outcome of a three-way merge attempt.
type MergeResult struct {
	MergedVCard string
	Conflicts   []FieldConflict
	AutoMerged  bool
}

// skipFields are vCard properties that should not be compared/merged
// (they identify the card, not its content).
var skipFields = map[string]bool{
	vcard.FieldUID:     true,
	vcard.FieldVersion: true,
}

// MergeVCards performs a three-way merge of base, local, and remote vCard strings.
//
//   - If base == "", there is no reference point: if both sides differ from each other
//     a conflict is returned; otherwise the non-empty/changed side wins.
//   - Returns AutoMerged=true and a complete MergedVCard when no conflicts exist.
//   - Returns AutoMerged=false and a non-empty Conflicts slice otherwise.
func MergeVCards(base, local, remote string) (*MergeResult, error) {
	baseCard, err := parseCard(base)
	if err != nil {
		return nil, fmt.Errorf("parse base vcard: %w", err)
	}
	localCard, err := parseCard(local)
	if err != nil {
		return nil, fmt.Errorf("parse local vcard: %w", err)
	}
	remoteCard, err := parseCard(remote)
	if err != nil {
		return nil, fmt.Errorf("parse remote vcard: %w", err)
	}

	// Collect all field types across all three cards
	allFields := fieldUnion(baseCard, localCard, remoteCard)

	result := &MergeResult{}
	merged := vcard.Card{}

	// Always carry over UID and VERSION from local (canonical)
	if f := localCard.Get(vcard.FieldUID); f != nil {
		merged[vcard.FieldUID] = localCard[vcard.FieldUID]
	}
	if f := localCard.Get(vcard.FieldVersion); f != nil {
		merged[vcard.FieldVersion] = localCard[vcard.FieldVersion]
	} else {
		merged[vcard.FieldVersion] = []*vcard.Field{{Value: "3.0"}}
	}

	for _, field := range allFields {
		if skipFields[field] {
			continue
		}
		baseVal := serializeField(baseCard, field)
		localVal := serializeField(localCard, field)
		remoteVal := serializeField(remoteCard, field)

		var chosen []*vcard.Field

		switch {
		case base == "":
			// No base: if both sides agree → take local; if they differ → conflict
			if localVal == remoteVal {
				chosen = localCard[field]
			} else {
				result.Conflicts = append(result.Conflicts, FieldConflict{
					Field:  field,
					Base:   "",
					Local:  localVal,
					Remote: remoteVal,
				})
			}
		case localVal == baseVal && remoteVal == baseVal:
			// No change on either side
			chosen = baseCard[field]
		case localVal == baseVal && remoteVal != baseVal:
			// Only remote changed → accept remote
			chosen = remoteCard[field]
		case localVal != baseVal && remoteVal == baseVal:
			// Only local changed → keep local
			chosen = localCard[field]
		default:
			// Both changed → conflict
			result.Conflicts = append(result.Conflicts, FieldConflict{
				Field:  field,
				Base:   baseVal,
				Local:  localVal,
				Remote: remoteVal,
			})
		}

		if chosen != nil {
			merged[field] = chosen
		}
	}

	if len(result.Conflicts) == 0 {
		result.AutoMerged = true
		result.MergedVCard = cardToString(merged)
	}

	return result, nil
}

// wildcardField is the resolution key meaning "every field not named individually".
// The conflict UI sends `{"*": "remote"}` alone for "Apply remote (source wins)": that
// button only renders when the conflict carries no stored field diffs, so there are no
// per-field keys to send with it.
const wildcardField = "*"

// ApplyResolution builds a final vCard based on the user's field-level choices.
//
// resolution maps field type → "local" | "remote". Anything else, including a literal
// vCard line, is read as "local". The wildcardField key supplies the choice for every
// field the map does not name; without it, unnamed fields default to local.
//
// A choice of "remote" removes any property the remote card does not carry — that is what
// "the remote version won" means. Resolving the wildcard to "remote" against an empty
// remote card would leave nothing but UID and VERSION, so it is refused with
// ErrRemoteCardEmpty rather than collapsing the contact.
func ApplyResolution(base, local, remote string, resolution map[string]string) (string, error) {
	fallback := resolution[wildcardField]
	if fallback == "remote" && strings.TrimSpace(remote) == "" {
		return "", ErrRemoteCardEmpty
	}

	localCard, err := parseCard(local)
	if err != nil {
		return "", fmt.Errorf("parse local vcard: %w", err)
	}
	remoteCard, err := parseCard(remote)
	if err != nil {
		return "", fmt.Errorf("parse remote vcard: %w", err)
	}

	merged := vcard.Card{}

	// Always preserve UID and VERSION from local
	if f := localCard.Get(vcard.FieldUID); f != nil {
		merged[vcard.FieldUID] = localCard[vcard.FieldUID]
	}
	if f := localCard.Get(vcard.FieldVersion); f != nil {
		merged[vcard.FieldVersion] = localCard[vcard.FieldVersion]
	} else {
		merged[vcard.FieldVersion] = []*vcard.Field{{Value: "3.0"}}
	}

	allFields := fieldUnion(nil, localCard, remoteCard)
	for _, field := range allFields {
		if skipFields[field] {
			continue
		}
		choice, ok := resolution[field]
		if !ok {
			choice = fallback
		}
		switch choice {
		case "remote":
			if remoteCard[field] != nil {
				merged[field] = remoteCard[field]
			}
		default: // "local" or unset
			if localCard[field] != nil {
				merged[field] = localCard[field]
			}
		}
	}

	return cardToString(merged), nil
}

// parseCard parses a vCard string into a vcard.Card.
// Returns an empty card if data is empty.
func parseCard(data string) (vcard.Card, error) {
	if strings.TrimSpace(data) == "" {
		return vcard.Card{}, nil
	}
	card, err := vcard.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		return nil, err
	}
	return card, nil
}

// serializeField returns a stable string representation of all values for a field type.
// Values are sorted to make comparison order-independent.
func serializeField(card vcard.Card, field string) string {
	fields := card[field]
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, f.Value)
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

// fieldUnion returns sorted unique field types across all provided cards.
func fieldUnion(cards ...vcard.Card) []string {
	seen := map[string]bool{}
	for _, card := range cards {
		for k := range card {
			seen[k] = true
		}
	}
	result := make([]string, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}
