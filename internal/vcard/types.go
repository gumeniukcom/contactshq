// Package vcard provides parsing and building of vCard 3.0/4.0 contacts.
// It wraps github.com/emersion/go-vcard and exposes a clean, structured
// ParsedContact type used across all layers of the application.
package vcard

// ParsedContact holds all fields extracted from a single vCard entry.
// It is the canonical in-memory representation of a contact record.
type ParsedContact struct {
	// Name
	FN         string // FN property — human-readable display name
	FirstName  string // N[1] given name
	LastName   string // N[0] family name
	MiddleName string // N[2] additional/middle name
	NamePrefix string // N[3] honorific prefix: Dr., Mr., Prof.
	NameSuffix string // N[4] honorific suffix: Jr., III, PhD
	Nickname   string // NICKNAME (first value)

	// Primary access (pref=1 or first found) — for fast display
	PrimaryEmail string
	PrimaryPhone string

	// Multi-value fields
	Emails    []Field   // EMAIL (all, with type and pref)
	Phones    []Field   // TEL (all, with type and pref)
	Addresses []Address // ADR (all)
	URLs      []Field   // URL (all)
	IMs       []Field   // IMPP instant messaging (all)
	Dates     []Date    // BDAY, ANNIVERSARY (and future custom dates)

	// Organization
	Org        string // ORG first component (company name)
	Department string // ORG second component (organizational unit)
	Title      string // TITLE — job title (first)
	Role       string // ROLE — functional role (first)

	// Other fields
	Note       string   // NOTE (first)
	Gender     string   // GENDER sex component (M/F/O/N/U or "")
	GenderText string   // GENDER gender identity text
	TZ         string   // TZ (first, text form)
	Geo        string   // GEO URI (geo:lat,lon)
	PhotoURI   string   // PHOTO URI (first; binary photos not stored)
	Rev        string   // REV last-revision timestamp
	Categories []string // CATEGORIES tags
	UID        string   // UID globally unique identifier
	ProdID     string   // PRODID creator product ID
}

// Field represents a single value from a multi-value vCard property
// (EMAIL, TEL, URL, IMPP), carrying type and preference metadata.
type Field struct {
	Value string
	Type  string // comma-joined lowercase types: "work", "home", "cell,work", etc.
	Pref  int    // 0 = not set; 1 = most preferred (PREF=1); other values per RFC 6350
	Label string // free-form LABEL parameter
}

// Address represents a structured ADR vCard property.
type Address struct {
	Type       string // comma-joined lowercase types: "work", "home", etc.
	Pref       int
	Label      string
	POBox      string // post office box
	Extended   string // extended address: apartment, suite, floor
	Street     string // street address
	City       string // locality (city)
	Region     string // state or province
	PostalCode string
	Country    string
}

// Date represents a date-type vCard field (BDAY, ANNIVERSARY, or custom).
type Date struct {
	Kind  string // "bday", "anniversary", "other"
	Value string // raw vCard date string (e.g. "19900101" or "1990-01-01")
	Label string // label for custom/other dates
}
