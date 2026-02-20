package tui

// teaKeyMap defines the displayed key hints for the Bubble Tea path.
type teaKeyMap struct {
	UpDown string
	Expand string
	Filter string
	Facets string
	Views  string
	Theme  string
	Quit   string
}

func defaultTeaKeyMap() teaKeyMap {
	return teaKeyMap{
		UpDown: "↑/↓ move",
		Expand: "Enter expand",
		Filter: "f text",
		Facets: "g tags · a assignee",
		Views:  "1-5 views",
		Theme:  "t theme",
		Quit:   "q quit",
	}
}
