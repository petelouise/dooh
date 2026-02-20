package tui

// teaModelState captures task-view state intended for Bubble Tea migration.
// It is kept compile-safe in this build and used by future renderer phases.
type teaModelState struct {
	Selected   int
	ExpandedID string
	Filter     FilterState
}
