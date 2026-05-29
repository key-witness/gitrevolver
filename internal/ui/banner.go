package ui

// Banner returns the GitRevolver banner.
func Banner() string {
	banner := `
  ____ _ _   ____                 _
 / ___(_) |_|  _ \ _____   _____ | |_   _____ _ __
| |  _| | __| |_) / _ \ \ / / _ \| \ \ / / _ \ '__|
| |_| | | |_|  _ <  __/\ V / (_) | |\ V /  __/ |
 \____|_|\__|_| \_\___| \_/ \___/|_| \_/ \___|_|
`
	return TitleStyle.Render(banner)
}

// SmallBanner returns a compact banner
func SmallBanner() string {
	return TitleStyle.Render("GitRevolver")
}

// Subtitle returns a styled subtitle
func Subtitle(text string) string {
	return MutedText.Render(text)
}

// SectionDivider returns a styled section divider
func SectionDivider() string {
	return MutedText.Render("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// Celebration returns a celebration message
func Celebration(message string) string {
	return SuccessBox.Render("🎉 " + message + " 🎊")
}
