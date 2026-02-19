package tui

import (
	"fmt"
	"strconv"
	"strings"
)

func bar(v int, total int, hex string) string {
	if total <= 0 {
		total = 1
	}
	width := 12
	filled := (v * width) / total
	if filled < 1 && v > 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	r, g, b := hexToRGB(hex)
	seg := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m %d", r, g, b, seg, v)
}

func hexToRGB(hex string) (int, int, int) {
	h := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(h) != 6 {
		return 255, 255, 255
	}
	r, _ := strconv.ParseInt(h[0:2], 16, 64)
	g, _ := strconv.ParseInt(h[2:4], 16, 64)
	b, _ := strconv.ParseInt(h[4:6], 16, 64)
	return int(r), int(g), int(b)
}
