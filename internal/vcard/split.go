package vcard

import (
	"bufio"
	"strings"
)

// SplitVCards splits a string containing one or more vCards into individual
// vCard strings. Correctly handles BEGIN:VCARD / END:VCARD delimiters.
func SplitVCards(data string) []string {
	var cards []string
	scanner := bufio.NewScanner(strings.NewReader(data))
	var current strings.Builder
	inCard := false

	for scanner.Scan() {
		line := scanner.Text()
		upper := strings.ToUpper(strings.TrimSpace(line))
		if strings.HasPrefix(upper, "BEGIN:VCARD") {
			inCard = true
			current.Reset()
		}
		if inCard {
			current.WriteString(line)
			current.WriteString("\r\n")
		}
		if strings.HasPrefix(upper, "END:VCARD") {
			if inCard {
				cards = append(cards, current.String())
			}
			inCard = false
		}
	}

	return cards
}

// InjectUID adds a UID field to a raw vCard string if one is not already
// present. Returns the string unchanged if UID already exists.
func InjectUID(data, uid string) string {
	// Fast check: scan for existing UID line
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		line := strings.ToUpper(strings.TrimSpace(scanner.Text()))
		if strings.HasPrefix(line, "UID:") {
			return data // already has UID
		}
	}
	// Insert before END:VCARD
	insertLine := "UID:" + uid + "\r\n"
	idx := strings.Index(strings.ToUpper(data), "END:VCARD")
	if idx >= 0 {
		return data[:idx] + insertLine + data[idx:]
	}
	return data + insertLine
}
