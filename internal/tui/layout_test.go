package tui

import "testing"

func TestStripANSIRemovesCSISequences(t *testing.T) {
	in := "\x1b[38;2;255;122;89mhello\x1b[0m world"
	got := stripANSI(in)
	if got != "hello world" {
		t.Fatalf("unexpected strip result: %q", got)
	}
}

func TestPadANSIUsesVisibleWidth(t *testing.T) {
	colored := "\x1b[38;2;255;122;89mabc\x1b[0m"
	got := padANSI(colored, 6)
	plain := stripANSI(got)
	if len(plain) != 6 {
		t.Fatalf("expected visible len 6, got %d (%q)", len(plain), plain)
	}
}
