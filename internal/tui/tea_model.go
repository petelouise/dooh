package tui

// teaModelState is retained for migration bookkeeping and test hooks.
type teaModelState struct {
	Selected   int
	ExpandedID string
	Filter     FilterState
	View       string
	ThemeID    string
}
