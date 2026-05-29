package main

import (
	"strings"
	"testing"
)

func TestSetupGuideIncludesLocalTokenDisclaimerAndAgentContext(t *testing.T) {
	var out strings.Builder
	printSetupGuide(&out, "client-a", "client-a-app")
	text := out.String()

	for _, want := range []string{
		"gitrevolver init",
		"gitrevolver secret set client-a github-token",
		"gitrevolver secret set client-a vercel-token",
		"gitrevolver secret set client-a supabase-token",
		"Tokens are stored on this machine in secure platform storage.",
		"login Keychain",
		"Do not paste real token values into Codex, Claude",
		"gitrevolver agent install-instructions client-a-app",
		"gitrevolver agent doctor client-a-app",
		"gitrevolver agent doctor client-a-app --online",
		"gitrevolver exec client-a-app -- codex",
		"gitrevolver exec client-a-app -- claude",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("setup guide missing %q:\n%s", want, text)
		}
	}
}
