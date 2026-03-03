package service

import (
	"testing"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/stretchr/testify/assert"
)

func makeContact(firstName, lastName, email, phone string) *domain.Contact {
	return &domain.Contact{
		FirstName: firstName,
		LastName:  lastName,
		Email:     email,
		Phone:     phone,
	}
}

func TestScoreContacts_ExactEmailMatch(t *testing.T) {
	a := makeContact("Alice", "Smith", "alice@example.com", "+1234567890")
	b := makeContact("Alice", "Smith", "alice@example.com", "+1234567890")
	score, reasons := scoreContacts(a, b)
	assert.Equal(t, 1.0, score)
	assert.Contains(t, reasons, "email_match")
}

func TestScoreContacts_DifferentEmail_PhoneMatch(t *testing.T) {
	a := makeContact("Alice", "Smith", "alice@a.com", "+1 (234) 567-890")
	b := makeContact("Ali", "Smith", "ali@b.com", "1234567890")
	score, reasons := scoreContacts(a, b)
	assert.Equal(t, 0.8, score)
	assert.Contains(t, reasons, "phone_match")
}

func TestScoreContacts_NameExactMatch(t *testing.T) {
	a := makeContact("Alice", "Smith", "", "")
	b := makeContact("Alice", "Smith", "", "")
	score, reasons := scoreContacts(a, b)
	assert.Equal(t, 0.7, score)
	assert.Contains(t, reasons, "name_exact")
}

func TestScoreContacts_NameSimilar(t *testing.T) {
	a := makeContact("Alice", "Smyth", "", "")
	b := makeContact("Alice", "Smith", "", "")
	score, reasons := scoreContacts(a, b)
	assert.Equal(t, 0.5, score)
	assert.Contains(t, reasons, "name_similar")
}

func TestScoreContacts_NoMatch(t *testing.T) {
	a := makeContact("Alice", "Smith", "alice@a.com", "+111")
	b := makeContact("Bob", "Jones", "bob@b.com", "+999")
	score, _ := scoreContacts(a, b)
	assert.Less(t, score, duplicateScoreThreshold)
}

func TestLevenshtein(t *testing.T) {
	assert.Equal(t, 0, levenshtein("abc", "abc"))
	assert.Equal(t, 1, levenshtein("abc", "ab"))
	assert.Equal(t, 1, levenshtein("abc", "axc"))
	assert.Equal(t, 3, levenshtein("abc", "xyz"))
}

func TestNormalizePhone(t *testing.T) {
	assert.Equal(t, "1234567890", normalizePhone("+1 (234) 567-890"))
	assert.Equal(t, "1234567890", normalizePhone("1234567890"))
	assert.Equal(t, "", normalizePhone(""))
}
