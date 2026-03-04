package sync

import (
	"strings"

	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
	"google.golang.org/api/people/v1"
)

// allPersonFields is the set of fields requested from the People API.
const allPersonFields = "names,emailAddresses,phoneNumbers,addresses,organizations,biographies,birthdays,nicknames,urls,imClients,events,genders,photos,memberships,metadata"

// allUpdatePersonFields is the set of fields that can be written via updateContact.
const allUpdatePersonFields = "names,emailAddresses,phoneNumbers,addresses,organizations,biographies,birthdays,nicknames,urls,imClients,events,genders"

// PersonToVCard converts a Google People API Person to a vCard 4.0 string.
func PersonToVCard(person *people.Person) (string, error) {
	p := &vcardpkg.ParsedContact{}

	// UID — use resourceName as stable identifier
	p.UID = person.ResourceName

	// Name
	if len(person.Names) > 0 {
		name := person.Names[0]
		p.FirstName = name.GivenName
		p.LastName = name.FamilyName
		p.MiddleName = name.MiddleName
		p.NamePrefix = name.HonorificPrefix
		p.NameSuffix = name.HonorificSuffix
		p.FN = name.DisplayName
	}

	// Nickname
	if len(person.Nicknames) > 0 {
		p.Nickname = person.Nicknames[0].Value
	}

	// Email
	for _, email := range person.EmailAddresses {
		f := vcardpkg.Field{
			Value: email.Value,
			Type:  normalizeGoogleType(email.Type),
		}
		if email.Metadata != nil && email.Metadata.Primary {
			f.Pref = 1
		}
		p.Emails = append(p.Emails, f)
		if p.PrimaryEmail == "" || f.Pref == 1 {
			p.PrimaryEmail = f.Value
		}
	}

	// Phone
	for _, phone := range person.PhoneNumbers {
		f := vcardpkg.Field{
			Value: phone.Value,
			Type:  normalizeGoogleType(phone.Type),
		}
		if phone.Metadata != nil && phone.Metadata.Primary {
			f.Pref = 1
		}
		p.Phones = append(p.Phones, f)
		if p.PrimaryPhone == "" || f.Pref == 1 {
			p.PrimaryPhone = f.Value
		}
	}

	// Addresses
	for _, addr := range person.Addresses {
		a := vcardpkg.Address{
			Type:       normalizeGoogleType(addr.Type),
			Street:     addr.StreetAddress,
			City:       addr.City,
			Region:     addr.Region,
			PostalCode: addr.PostalCode,
			Country:    addr.Country,
			Extended:   addr.ExtendedAddress,
			POBox:      addr.PoBox,
		}
		p.Addresses = append(p.Addresses, a)
	}

	// Organization
	if len(person.Organizations) > 0 {
		org := person.Organizations[0]
		p.Org = org.Name
		p.Title = org.Title
		p.Department = org.Department
		p.Role = org.JobDescription
	}

	// Birthday
	if len(person.Birthdays) > 0 {
		bday := person.Birthdays[0]
		if bday.Date != nil {
			p.Dates = append(p.Dates, vcardpkg.Date{
				Kind:  "bday",
				Value: googleDateToString(bday.Date),
			})
		} else if bday.Text != "" {
			p.Dates = append(p.Dates, vcardpkg.Date{Kind: "bday", Value: bday.Text})
		}
	}

	// Events (anniversary)
	for _, event := range person.Events {
		if strings.EqualFold(event.Type, "anniversary") && event.Date != nil {
			p.Dates = append(p.Dates, vcardpkg.Date{
				Kind:  "anniversary",
				Value: googleDateToString(event.Date),
			})
		}
	}

	// URLs
	for _, u := range person.Urls {
		p.URLs = append(p.URLs, vcardpkg.Field{
			Value: u.Value,
			Type:  normalizeGoogleType(u.Type),
		})
	}

	// IM clients
	for _, im := range person.ImClients {
		proto := im.Protocol
		if proto == "" {
			proto = im.Type
		}
		p.IMs = append(p.IMs, vcardpkg.Field{
			Value: im.Username,
			Type:  proto,
		})
	}

	// Notes
	if len(person.Biographies) > 0 {
		p.Note = person.Biographies[0].Value
	}

	// Gender
	if len(person.Genders) > 0 {
		g := person.Genders[0]
		switch strings.ToLower(g.Value) {
		case "male":
			p.Gender = "M"
		case "female":
			p.Gender = "F"
		case "unspecified":
			p.Gender = "U"
		default:
			p.Gender = "O"
			p.GenderText = g.Value
		}
	}

	// Photos — URL only
	if len(person.Photos) > 0 {
		p.PhotoURI = person.Photos[0].Url
	}

	// Categories from contact group memberships
	for _, m := range person.Memberships {
		if m.ContactGroupMembership != nil {
			groupName := m.ContactGroupMembership.ContactGroupResourceName
			// Strip "contactGroups/" prefix for readability
			groupName = strings.TrimPrefix(groupName, "contactGroups/")
			if groupName != "" && groupName != "myContacts" {
				p.Categories = append(p.Categories, groupName)
			}
		}
	}

	// REV from metadata
	if person.Metadata != nil && len(person.Metadata.Sources) > 0 {
		p.Rev = person.Metadata.Sources[0].UpdateTime
	}

	return vcardpkg.BuildVCard(p)
}

// ParsedContactToPerson converts a ParsedContact to a Google People API Person.
func ParsedContactToPerson(p *vcardpkg.ParsedContact) *people.Person {
	person := &people.Person{}

	// Name
	if p.FirstName != "" || p.LastName != "" || p.MiddleName != "" {
		person.Names = []*people.Name{{
			GivenName:       p.FirstName,
			FamilyName:      p.LastName,
			MiddleName:      p.MiddleName,
			HonorificPrefix: p.NamePrefix,
			HonorificSuffix: p.NameSuffix,
		}}
	}

	// Nickname
	if p.Nickname != "" {
		person.Nicknames = []*people.Nickname{{Value: p.Nickname}}
	}

	// Emails
	for _, e := range p.Emails {
		person.EmailAddresses = append(person.EmailAddresses, &people.EmailAddress{
			Value: e.Value,
			Type:  toGoogleType(e.Type),
		})
	}

	// Phones
	for _, ph := range p.Phones {
		person.PhoneNumbers = append(person.PhoneNumbers, &people.PhoneNumber{
			Value: ph.Value,
			Type:  toGoogleType(ph.Type),
		})
	}

	// Addresses
	for _, a := range p.Addresses {
		person.Addresses = append(person.Addresses, &people.Address{
			Type:            toGoogleType(a.Type),
			StreetAddress:   a.Street,
			City:            a.City,
			Region:          a.Region,
			PostalCode:      a.PostalCode,
			Country:         a.Country,
			ExtendedAddress: a.Extended,
			PoBox:           a.POBox,
		})
	}

	// Organization
	if p.Org != "" || p.Title != "" || p.Department != "" {
		person.Organizations = []*people.Organization{{
			Name:           p.Org,
			Title:          p.Title,
			Department:     p.Department,
			JobDescription: p.Role,
		}}
	}

	// Birthday
	for _, d := range p.Dates {
		if d.Kind == "bday" {
			date := parseGoogleDate(d.Value)
			if date != nil {
				person.Birthdays = []*people.Birthday{{Date: date}}
			}
		}
		if d.Kind == "anniversary" {
			date := parseGoogleDate(d.Value)
			if date != nil {
				person.Events = append(person.Events, &people.Event{
					Date: date,
					Type: "anniversary",
				})
			}
		}
	}

	// URLs
	for _, u := range p.URLs {
		person.Urls = append(person.Urls, &people.Url{
			Value: u.Value,
			Type:  toGoogleType(u.Type),
		})
	}

	// IM clients
	for _, im := range p.IMs {
		person.ImClients = append(person.ImClients, &people.ImClient{
			Username: im.Value,
			Protocol: im.Type,
		})
	}

	// Notes
	if p.Note != "" {
		person.Biographies = []*people.Biography{{Value: p.Note}}
	}

	// Gender
	if p.Gender != "" {
		var genderVal string
		switch p.Gender {
		case "M":
			genderVal = "male"
		case "F":
			genderVal = "female"
		case "U":
			genderVal = "unspecified"
		default:
			genderVal = p.GenderText
			if genderVal == "" {
				genderVal = "other"
			}
		}
		person.Genders = []*people.Gender{{Value: genderVal}}
	}

	return person
}

// VCardToPerson parses vCard data and converts to a Google Person.
func VCardToPerson(vcardData string) (*people.Person, error) {
	parsed, err := vcardpkg.ParseVCard(vcardData)
	if err != nil {
		return nil, err
	}
	return ParsedContactToPerson(parsed), nil
}

// normalizeGoogleType maps Google contact types to lowercase vCard types.
func normalizeGoogleType(t string) string {
	switch strings.ToLower(t) {
	case "home":
		return "home"
	case "work":
		return "work"
	case "mobile":
		return "cell"
	case "homefax":
		return "home,fax"
	case "workfax":
		return "work,fax"
	case "other", "":
		return ""
	default:
		return strings.ToLower(t)
	}
}

// toGoogleType maps vCard types back to Google People API type strings.
func toGoogleType(t string) string {
	switch strings.ToLower(t) {
	case "home":
		return "home"
	case "work":
		return "work"
	case "cell":
		return "mobile"
	case "home,fax":
		return "homeFax"
	case "work,fax":
		return "workFax"
	case "":
		return "other"
	default:
		return "other"
	}
}

// googleDateToString formats a Google Date to YYYY-MM-DD or MMDD if no year.
func googleDateToString(d *people.Date) string {
	if d == nil {
		return ""
	}
	if d.Year > 0 {
		return strings.TrimSpace(strings.Replace(
			strings.Replace(
				strings.Replace("%04d-%02d-%02d", "%04d", padInt(int(d.Year), 4), 1),
				"%02d", padInt(int(d.Month), 2), 1),
			"%02d", padInt(int(d.Day), 2), 1))
	}
	// No year — use --MM-DD (vCard 4.0 truncated date)
	return "--" + padInt(int(d.Month), 2) + "-" + padInt(int(d.Day), 2)
}

func padInt(n, width int) string {
	s := strings.Repeat("0", width)
	v := string(rune('0' + n%10)) // single digit
	if n >= 10 {
		// simple formatting for 1-4 digit numbers
		result := ""
		for n > 0 {
			result = string(rune('0'+n%10)) + result
			n /= 10
		}
		for len(result) < width {
			result = "0" + result
		}
		return result
	}
	return s[:width-1] + v
}

// parseGoogleDate parses a date string like "1990-01-15" or "19900115" into a Google Date.
func parseGoogleDate(s string) *people.Date {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	// Handle --MM-DD (no year)
	if strings.HasPrefix(s, "--") {
		s = strings.TrimPrefix(s, "--")
		parts := strings.SplitN(s, "-", 2)
		if len(parts) == 2 {
			return &people.Date{
				Month: parseInt64(parts[0]),
				Day:   parseInt64(parts[1]),
			}
		}
		if len(s) == 4 {
			return &people.Date{
				Month: parseInt64(s[:2]),
				Day:   parseInt64(s[2:]),
			}
		}
		return nil
	}

	// YYYY-MM-DD
	if len(s) == 10 && s[4] == '-' && s[7] == '-' {
		return &people.Date{
			Year:  parseInt64(s[:4]),
			Month: parseInt64(s[5:7]),
			Day:   parseInt64(s[8:10]),
		}
	}

	// YYYYMMDD
	if len(s) == 8 {
		return &people.Date{
			Year:  parseInt64(s[:4]),
			Month: parseInt64(s[4:6]),
			Day:   parseInt64(s[6:8]),
		}
	}

	return nil
}

func parseInt64(s string) int64 {
	var n int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int64(c-'0')
		}
	}
	return n
}
