package vcard

import (
	"strings"
)

// splitLines breaks data on CRLF, LF or CR, dropping the terminators.
//
// This deliberately avoids bufio.Scanner: its default 64 KiB token limit silently
// truncates the scan at the first longer line — an embedded PHOTO easily exceeds it —
// and the resulting error is invisible unless the caller inspects scanner.Err(). Every
// vCard after such a line would simply vanish, which in a backup restore means losing
// contacts. The input is already a string in memory, so there is nothing to stream.
func splitLines(data string) []string {
	if data == "" {
		return nil
	}

	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// SplitVCards splits a string containing one or more vCards into individual
// vCard strings. Correctly handles BEGIN:VCARD / END:VCARD delimiters.
func SplitVCards(data string) []string {
	var cards []string
	var current strings.Builder
	inCard := false

	for _, line := range splitLines(data) {
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
	for _, line := range splitLines(data) {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(line)), "UID:") {
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
