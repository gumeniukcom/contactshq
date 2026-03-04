package sync

import (
	"strings"
	"testing"

	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/people/v1"
)

func TestPersonToVCard_BasicName(t *testing.T) {
	person := &people.Person{
		ResourceName: "people/c123",
		Etag:         "etag1",
		Names: []*people.Name{{
			GivenName:   "John",
			FamilyName:  "Doe",
			DisplayName: "John Doe",
		}},
	}

	vcard, err := PersonToVCard(person)
	require.NoError(t, err)
	assert.Contains(t, vcard, "John")
	assert.Contains(t, vcard, "Doe")
	assert.Contains(t, vcard, "people/c123") // UID
}

func TestPersonToVCard_EmailsAndPhones(t *testing.T) {
	person := &people.Person{
		ResourceName: "people/c456",
		EmailAddresses: []*people.EmailAddress{
			{Value: "john@work.com", Type: "work"},
			{Value: "john@home.com", Type: "home"},
		},
		PhoneNumbers: []*people.PhoneNumber{
			{Value: "+1234567890", Type: "mobile"},
			{Value: "+0987654321", Type: "work"},
		},
		Names: []*people.Name{{GivenName: "John", FamilyName: "Doe"}},
	}

	vcard, err := PersonToVCard(person)
	require.NoError(t, err)
	assert.Contains(t, vcard, "john@work.com")
	assert.Contains(t, vcard, "john@home.com")
	assert.Contains(t, vcard, "+1234567890")
	assert.Contains(t, vcard, "+0987654321")
}

func TestPersonToVCard_Address(t *testing.T) {
	person := &people.Person{
		ResourceName: "people/c789",
		Names:        []*people.Name{{GivenName: "Jane"}},
		Addresses: []*people.Address{{
			Type:          "home",
			StreetAddress: "123 Main St",
			City:          "Springfield",
			Region:        "IL",
			PostalCode:    "62701",
			Country:       "US",
		}},
	}

	vcard, err := PersonToVCard(person)
	require.NoError(t, err)
	assert.Contains(t, vcard, "123 Main St")
	assert.Contains(t, vcard, "Springfield")
}

func TestPersonToVCard_Organization(t *testing.T) {
	person := &people.Person{
		ResourceName: "people/c100",
		Names:        []*people.Name{{GivenName: "Alice"}},
		Organizations: []*people.Organization{{
			Name:       "Acme Corp",
			Title:      "Engineer",
			Department: "R&D",
		}},
	}

	vcard, err := PersonToVCard(person)
	require.NoError(t, err)
	assert.Contains(t, vcard, "Acme Corp")
	assert.Contains(t, vcard, "Engineer")
}

func TestPersonToVCard_Birthday(t *testing.T) {
	person := &people.Person{
		ResourceName: "people/c200",
		Names:        []*people.Name{{GivenName: "Bob"}},
		Birthdays: []*people.Birthday{{
			Date: &people.Date{Year: 1990, Month: 6, Day: 15},
		}},
	}

	vcard, err := PersonToVCard(person)
	require.NoError(t, err)
	assert.Contains(t, vcard, "1990-06-15")
}

func TestPersonToVCard_Nickname(t *testing.T) {
	person := &people.Person{
		ResourceName: "people/c300",
		Names:        []*people.Name{{GivenName: "Robert"}},
		Nicknames:    []*people.Nickname{{Value: "Bobby"}},
	}

	vcard, err := PersonToVCard(person)
	require.NoError(t, err)
	assert.Contains(t, vcard, "Bobby")
}

func TestPersonToVCard_Notes(t *testing.T) {
	person := &people.Person{
		ResourceName: "people/c400",
		Names:        []*people.Name{{GivenName: "Charlie"}},
		Biographies:  []*people.Biography{{Value: "Met at conference 2024"}},
	}

	vcard, err := PersonToVCard(person)
	require.NoError(t, err)
	assert.Contains(t, vcard, "Met at conference 2024")
}

func TestPersonToVCard_Gender(t *testing.T) {
	person := &people.Person{
		ResourceName: "people/c500",
		Names:        []*people.Name{{GivenName: "Dana"}},
		Genders:      []*people.Gender{{Value: "female"}},
	}

	vcard, err := PersonToVCard(person)
	require.NoError(t, err)
	assert.Contains(t, vcard, "F")
}

func TestParsedContactToPerson_BasicName(t *testing.T) {
	p := &vcardpkg.ParsedContact{
		FirstName: "John",
		LastName:  "Doe",
		Nickname:  "JD",
	}

	person := ParsedContactToPerson(p)
	require.Len(t, person.Names, 1)
	assert.Equal(t, "John", person.Names[0].GivenName)
	assert.Equal(t, "Doe", person.Names[0].FamilyName)
	require.Len(t, person.Nicknames, 1)
	assert.Equal(t, "JD", person.Nicknames[0].Value)
}

func TestParsedContactToPerson_EmailsAndPhones(t *testing.T) {
	p := &vcardpkg.ParsedContact{
		FirstName: "Jane",
		Emails: []vcardpkg.Field{
			{Value: "jane@work.com", Type: "work"},
			{Value: "jane@home.com", Type: "home"},
		},
		Phones: []vcardpkg.Field{
			{Value: "+1111111111", Type: "cell"},
		},
	}

	person := ParsedContactToPerson(p)
	require.Len(t, person.EmailAddresses, 2)
	assert.Equal(t, "jane@work.com", person.EmailAddresses[0].Value)
	assert.Equal(t, "work", person.EmailAddresses[0].Type)
	assert.Equal(t, "jane@home.com", person.EmailAddresses[1].Value)
	assert.Equal(t, "home", person.EmailAddresses[1].Type)

	require.Len(t, person.PhoneNumbers, 1)
	assert.Equal(t, "+1111111111", person.PhoneNumbers[0].Value)
	assert.Equal(t, "mobile", person.PhoneNumbers[0].Type)
}

func TestParsedContactToPerson_Organization(t *testing.T) {
	p := &vcardpkg.ParsedContact{
		FirstName:  "Alice",
		Org:        "BigCorp",
		Title:      "CTO",
		Department: "Engineering",
		Role:       "Technical Lead",
	}

	person := ParsedContactToPerson(p)
	require.Len(t, person.Organizations, 1)
	assert.Equal(t, "BigCorp", person.Organizations[0].Name)
	assert.Equal(t, "CTO", person.Organizations[0].Title)
	assert.Equal(t, "Engineering", person.Organizations[0].Department)
	assert.Equal(t, "Technical Lead", person.Organizations[0].JobDescription)
}

func TestVCardToPerson(t *testing.T) {
	vcard := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Test User\r\nN:User;Test;;;\r\nEMAIL;TYPE=work:test@example.com\r\nTEL;TYPE=cell:+1234567890\r\nEND:VCARD\r\n"

	person, err := VCardToPerson(vcard)
	require.NoError(t, err)
	require.Len(t, person.Names, 1)
	assert.Equal(t, "Test", person.Names[0].GivenName)
	assert.Equal(t, "User", person.Names[0].FamilyName)
	require.Len(t, person.EmailAddresses, 1)
	assert.Equal(t, "test@example.com", person.EmailAddresses[0].Value)
	require.Len(t, person.PhoneNumbers, 1)
	assert.Equal(t, "+1234567890", person.PhoneNumbers[0].Value)
}

func TestRoundTrip_PersonToVCardAndBack(t *testing.T) {
	original := &people.Person{
		ResourceName: "people/c999",
		Names: []*people.Name{{
			GivenName:  "Round",
			FamilyName: "Trip",
			MiddleName: "T",
		}},
		EmailAddresses: []*people.EmailAddress{
			{Value: "round@test.com", Type: "work"},
		},
		PhoneNumbers: []*people.PhoneNumber{
			{Value: "+9999999999", Type: "home"},
		},
		Organizations: []*people.Organization{{
			Name:  "TripCorp",
			Title: "Developer",
		}},
		Nicknames: []*people.Nickname{{Value: "RT"}},
	}

	// Person → vCard
	vcardStr, err := PersonToVCard(original)
	require.NoError(t, err)

	// vCard → Person
	restored, err := VCardToPerson(vcardStr)
	require.NoError(t, err)

	// Verify key fields survived
	require.Len(t, restored.Names, 1)
	assert.Equal(t, "Round", restored.Names[0].GivenName)
	assert.Equal(t, "Trip", restored.Names[0].FamilyName)
	assert.Equal(t, "T", restored.Names[0].MiddleName)

	require.Len(t, restored.EmailAddresses, 1)
	assert.Equal(t, "round@test.com", restored.EmailAddresses[0].Value)

	require.Len(t, restored.PhoneNumbers, 1)
	assert.Equal(t, "+9999999999", restored.PhoneNumbers[0].Value)

	require.Len(t, restored.Organizations, 1)
	assert.Equal(t, "TripCorp", restored.Organizations[0].Name)

	require.Len(t, restored.Nicknames, 1)
	assert.Equal(t, "RT", restored.Nicknames[0].Value)
}

func TestNormalizeGoogleType(t *testing.T) {
	tests := []struct{ input, expected string }{
		{"home", "home"},
		{"work", "work"},
		{"mobile", "cell"},
		{"homeFax", "home,fax"},
		{"workFax", "work,fax"},
		{"other", ""},
		{"", ""},
		{"custom", "custom"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, normalizeGoogleType(tt.input), "input: %s", tt.input)
	}
}

func TestToGoogleType(t *testing.T) {
	tests := []struct{ input, expected string }{
		{"home", "home"},
		{"work", "work"},
		{"cell", "mobile"},
		{"home,fax", "homeFax"},
		{"work,fax", "workFax"},
		{"", "other"},
		{"unknown", "other"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, toGoogleType(tt.input), "input: %s", tt.input)
	}
}

func TestGoogleDateToString(t *testing.T) {
	tests := []struct {
		name     string
		date     *people.Date
		expected string
	}{
		{"full date", &people.Date{Year: 1990, Month: 6, Day: 15}, "1990-06-15"},
		{"no year", &people.Date{Month: 12, Day: 25}, "--12-25"},
		{"nil", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, googleDateToString(tt.date))
		})
	}
}

func TestParseGoogleDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		year  int64
		month int64
		day   int64
	}{
		{"YYYY-MM-DD", "1990-06-15", 1990, 6, 15},
		{"YYYYMMDD", "19900615", 1990, 6, 15},
		{"--MM-DD", "--12-25", 0, 12, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := parseGoogleDate(tt.input)
			require.NotNil(t, d)
			assert.Equal(t, tt.year, d.Year)
			assert.Equal(t, tt.month, d.Month)
			assert.Equal(t, tt.day, d.Day)
		})
	}
}

func TestPersonToVCard_PhotoAndURL(t *testing.T) {
	person := &people.Person{
		ResourceName: "people/c600",
		Names:        []*people.Name{{GivenName: "Photo"}},
		Photos:       []*people.Photo{{Url: "https://example.com/photo.jpg"}},
		Urls:         []*people.Url{{Value: "https://example.com", Type: "home"}},
	}

	vcard, err := PersonToVCard(person)
	require.NoError(t, err)
	assert.True(t, strings.Contains(vcard, "https://example.com/photo.jpg") || strings.Contains(vcard, "PHOTO"))
	assert.Contains(t, vcard, "https://example.com")
}
