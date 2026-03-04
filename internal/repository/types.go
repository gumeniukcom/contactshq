package repository

// ListFilters holds sort/filter parameters for contact list queries.
type ListFilters struct {
	SortBy   string   // "name"|"email"|"org"|"created_at"|"updated_at" (default: "name")
	SortDir  string   // "asc"|"desc" (default: "asc")
	Category []string // OR-match on contact_categories.value
	Org      string   // exact match on contacts.org
	HasEmail *bool
	HasPhone *bool
}

// ContactFacets holds aggregated filter facet data.
type ContactFacets struct {
	Categories []string `json:"categories"`
	Orgs       []string `json:"orgs"`
	Total      int      `json:"total"`
}
