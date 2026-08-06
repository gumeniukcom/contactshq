package vcard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	gvcard "github.com/emersion/go-vcard"
)

// Merging two cards used to mean picking a whole property from one side or the other:
// chqsync.ApplyResolution replaces card[EMAIL] entirely with the winner's or the loser's
// list. "The work address from A and the home address from B" was not expressible, and the
// UI that offered per-field choices sent keys the resolver never looked at, so the winner
// always won regardless of what the user clicked.
//
// MergeCards works at the level of individual values instead. Every value on either side gets
// a stable identifier derived from its content, and the caller says which identifiers survive.
// Identifiers are content hashes rather than positions: an index into a slice means something
// different the moment the underlying card is edited by another client mid-review.
//
// chqsync.ApplyResolution is deliberately left alone — its other caller, SyncConflictService,
// genuinely does resolve whole properties, and its keys really are vCard property names.

// alwaysFromWinner are the properties that identify the surviving record rather than describe
// the person. Taking them from the loser would rename the contact CardDAV clients already
// know, or downgrade the card's version.
var alwaysFromWinner = map[string]bool{
	gvcard.FieldVersion: true,
	gvcard.FieldUID:     true,
}

// ValueID identifies one value of one property, stable across requests.
type ValueID string

// Selection lists the value identifiers to keep, per property name.
//
// A property absent from the map keeps the winner's values, which makes the zero Selection
// behave like "keep the winner" — the safe default for a caller that says nothing.
type Selection map[string][]ValueID

// ValueRef describes one candidate value offered to the caller.
type ValueRef struct {
	ID       ValueID           `json:"id"`
	Property string            `json:"property"`
	Value    string            `json:"value"`
	Params   map[string]string `json:"params,omitempty"`
	// Side is "winner", "loser", or "both" when the same value appears on both cards.
	Side string `json:"side"`
}

// valueID derives the identifier of a value from its content and parameters, so the same
// value on both cards collapses to one entry.
func valueID(property string, f *gvcard.Field) ValueID {
	h := sha256.New()
	h.Write([]byte(strings.ToUpper(property)))
	h.Write([]byte{0})
	h.Write([]byte(f.Value))

	keys := make([]string, 0, len(f.Params))
	for k := range f.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte{0})
		h.Write([]byte(k))
		for _, v := range f.Params[k] {
			h.Write([]byte{1})
			h.Write([]byte(v))
		}
	}

	return ValueID(hex.EncodeToString(h.Sum(nil)[:12]))
}

// Candidates lists every value of both cards, collapsing identical ones.
//
// The order is stable: properties alphabetically, and within a property the winner's values
// first, so a UI built on it does not reshuffle between loads.
func Candidates(winner, loser string) ([]ValueRef, error) {
	winnerCard, err := decodeCard(winner)
	if err != nil {
		return nil, fmt.Errorf("parse winner vcard: %w", err)
	}
	loserCard, err := decodeCard(loser)
	if err != nil {
		return nil, fmt.Errorf("parse loser vcard: %w", err)
	}

	seen := map[ValueID]int{}
	var refs []ValueRef

	collect := func(card gvcard.Card, side string) {
		properties := make([]string, 0, len(card))
		for k := range card {
			properties = append(properties, k)
		}
		sort.Strings(properties)

		for _, property := range properties {
			if alwaysFromWinner[strings.ToUpper(property)] {
				continue
			}
			for _, f := range card[property] {
				if strings.TrimSpace(f.Value) == "" {
					continue
				}
				id := valueID(property, f)
				if idx, ok := seen[id]; ok {
					refs[idx].Side = "both"
					continue
				}
				seen[id] = len(refs)
				refs = append(refs, ValueRef{
					ID:       id,
					Property: strings.ToUpper(property),
					Value:    f.Value,
					Params:   flattenParams(f.Params),
					Side:     side,
				})
			}
		}
	}

	collect(winnerCard, "winner")
	collect(loserCard, "loser")

	return refs, nil
}

func flattenParams(p gvcard.Params) map[string]string {
	if len(p) == 0 {
		return nil
	}
	out := make(map[string]string, len(p))
	for k, v := range p {
		out[k] = strings.Join(v, ",")
	}
	return out
}

// MergeCards builds a card from the values the selection keeps.
//
// UID and VERSION always come from the winner: they identify the record that survives, and
// every CardDAV client already knows the contact by that UID.
func MergeCards(winner, loser string, sel Selection) (string, error) {
	winnerCard, err := decodeCard(winner)
	if err != nil {
		return "", fmt.Errorf("parse winner vcard: %w", err)
	}
	loserCard, err := decodeCard(loser)
	if err != nil {
		return "", fmt.Errorf("parse loser vcard: %w", err)
	}

	merged := gvcard.Card{}

	// Identity first, always from the winner.
	if f := winnerCard[gvcard.FieldVersion]; len(f) > 0 {
		merged[gvcard.FieldVersion] = f
	} else {
		merged[gvcard.FieldVersion] = []*gvcard.Field{{Value: "4.0"}}
	}
	if f := winnerCard[gvcard.FieldUID]; len(f) > 0 {
		merged[gvcard.FieldUID] = f
	}

	for _, property := range propertyUnion(winnerCard, loserCard) {
		if alwaysFromWinner[property] {
			continue
		}

		keep, chosen := sel[property]
		if !chosen {
			// Nothing said about this property: the winner keeps it. A selection that
			// mentions nothing therefore yields the winner's card, which is what a caller
			// asking for "keep this one" means.
			if f := fieldsFor(winnerCard, property); len(f) > 0 {
				merged[property] = f
			}
			continue
		}

		wanted := make(map[ValueID]bool, len(keep))
		for _, id := range keep {
			wanted[id] = true
		}

		var fields []*gvcard.Field
		// Winner's values first so the surviving card keeps its own ordering, then the
		// loser's additions — this is what makes "work email from A, home from B" possible.
		for _, card := range []gvcard.Card{winnerCard, loserCard} {
			for _, f := range fieldsFor(card, property) {
				id := valueID(property, f)
				if !wanted[id] {
					continue
				}
				delete(wanted, id) // the same value on both cards is emitted once
				fields = append(fields, f)
			}
		}
		if len(fields) > 0 {
			merged[property] = fields
		}
	}

	var sb strings.Builder
	if err := EncodeCard(&sb, merged); err != nil {
		return "", fmt.Errorf("encode merged vcard: %w", err)
	}
	return sb.String(), nil
}

// fieldsFor returns a property's fields regardless of the case it was stored in.
func fieldsFor(card gvcard.Card, property string) []*gvcard.Field {
	if f, ok := card[property]; ok {
		return f
	}
	for k, f := range card {
		if strings.EqualFold(k, property) {
			return f
		}
	}
	return nil
}

// propertyUnion lists every property present on either card, upper-cased and sorted.
func propertyUnion(cards ...gvcard.Card) []string {
	seen := map[string]bool{}
	for _, card := range cards {
		for k := range card {
			seen[strings.ToUpper(k)] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func decodeCard(data string) (gvcard.Card, error) {
	if strings.TrimSpace(data) == "" {
		return gvcard.Card{}, nil
	}
	return gvcard.NewDecoder(strings.NewReader(data)).Decode()
}
