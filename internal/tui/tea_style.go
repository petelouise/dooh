package tui

type teaStyleTokens struct {
	TitleFG string
	TitleBG string
	PanelFG string
	PanelBG string
}

func teaTokensFromTheme(t Theme) teaStyleTokens {
	return teaStyleTokens{
		TitleFG: t.Colors["text"],
		TitleBG: t.Colors["panel"],
		PanelFG: t.Colors["muted"],
		PanelBG: t.Colors["background"],
	}
}
