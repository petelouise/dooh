package tui

type teaKeyMap struct {
	Up       string
	Down     string
	Expand   string
	Collapse string
	Filter   string
	Quit     string
}

func defaultTeaKeyMap() teaKeyMap {
	return teaKeyMap{
		Up:       "up",
		Down:     "down",
		Expand:   "enter",
		Collapse: "left",
		Filter:   "/",
		Quit:     "q",
	}
}
