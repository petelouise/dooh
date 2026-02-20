package tui

import "github.com/charmbracelet/lipgloss"

type teaStyleTokens struct {
	TitleFG string
	TitleBG string
	PanelFG string
	PanelBG string
}

func teaTokensFromTheme(t Theme) teaStyleTokens {
	return teaStyleTokens{
		TitleFG: fallbackHex(t.Colors["text"], "#F8FAFC"),
		TitleBG: fallbackHex(t.Colors["panel"], "#0F172A"),
		PanelFG: fallbackHex(t.Colors["muted"], "#CBD5E1"),
		PanelBG: fallbackHex(t.Colors["background"], "#111827"),
	}
}

func fallbackHex(v string, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func teaBaseStyle(tokens teaStyleTokens) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.PanelFG)).Background(lipgloss.Color(tokens.PanelBG))
}
