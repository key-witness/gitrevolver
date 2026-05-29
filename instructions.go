package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/key-witness/gitrevolver/internal/config"
)

const (
	instructionBegin = "<!-- BEGIN GitRevolver -->"
	instructionEnd   = "<!-- END GitRevolver -->"
)

func installLibraryTemplates() error {
	templates := map[string]string{
		config.AgentsTemplate: agentsInstructionBody(),
		config.ClaudeTemplate: claudeInstructionBody(),
	}
	for name, content := range templates {
		path, err := config.GetTemplatePath(name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content+"\n"), 0600); err != nil {
			return err
		}
	}
	return nil
}

func installProjectInstructions(projectRoot string) error {
	root, err := expandPath(projectRoot)
	if err != nil {
		return err
	}
	files := map[string]string{
		"AGENTS.md": agentsInstructionBody(),
		"CLAUDE.md": claudeInstructionBody(),
	}
	for name, body := range files {
		if err := updateMarkedFile(filepath.Join(root, name), body); err != nil {
			return err
		}
	}
	return nil
}

func updateMarkedFile(path, body string) error {
	block := instructionBegin + "\n" + strings.TrimSpace(body) + "\n" + instructionEnd
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	current := string(data)
	start := strings.Index(current, instructionBegin)
	end := strings.Index(current, instructionEnd)
	if start >= 0 && end >= start {
		end += len(instructionEnd)
		current = current[:start] + block + current[end:]
	} else if strings.TrimSpace(current) == "" {
		current = block + "\n"
	} else {
		current = strings.TrimRight(current, "\n") + "\n\n" + block + "\n"
	}
	return os.WriteFile(path, []byte(current), 0644)
}

func agentsInstructionBody() string {
	return strings.Join([]string{
		"# GitRevolver identity instructions",
		"This repository uses GitRevolver for GitHub, Vercel, and Supabase identity routing.",
		"GitRevolver stores provider tokens locally in secure platform storage. On macOS it uses the login Keychain; token values must not be pasted into chat, committed, logged, or written to YAML.",
		"",
		"Before credentialed project work, run these checks:",
		"",
		"```bash",
		"gitrevolver resolve . --json",
		"gitrevolver agent doctor .",
		"gitrevolver agent doctor . --online",
		"```",
		"",
		"If GitRevolver is not initialized on this machine, show the user this local setup flow and ask them to run token commands in their own terminal:",
		"",
		"```bash",
		"gitrevolver setup",
		"gitrevolver init",
		"gitrevolver identity add",
		"gitrevolver secret set <identity> github-token",
		"gitrevolver secret set <identity> vercel-token",
		"gitrevolver secret set <identity> supabase-token",
		"gitrevolver project scan ~/code",
		"gitrevolver agent install-instructions .",
		"```",
		"",
		"Launch tools that need project credentials through GitRevolver:",
		"",
		"```bash",
		"gitrevolver exec . -- codex",
		"gitrevolver exec . -- gh pr create",
		"gitrevolver exec . -- vercel deploy",
		"gitrevolver exec . -- supabase db push",
		"```",
		"",
		"Do not run `gh auth switch`, `vercel login`, `supabase login`, or global Git author edits for this project.",
		"For risky actions, run `gitrevolver agent guard . <action>` first.",
	}, "\n")
}

func claudeInstructionBody() string {
	return fmt.Sprintf(`# GitRevolver identity instructions
Use GitRevolver before credentialed work in this repository.
GitRevolver stores provider tokens locally in secure platform storage. On macOS it uses the login Keychain. Never ask the user to paste tokens into chat, logs, YAML, or docs.
Resolve with %s, validate with %s, and prefer %s for GitHub, Vercel, and Supabase CLI commands.
If setup is missing, show the user %s, then have them run %s locally for each provider token and confirm before writing AGENTS.md or CLAUDE.md with %s.
Never switch global provider auth for a GitRevolver project.
Guard risky actions with %s.`, "`gitrevolver resolve . --json`", "`gitrevolver agent doctor . --online`", "`gitrevolver exec . -- <command>`", "`gitrevolver setup`", "`gitrevolver secret set <identity> <github-token|vercel-token|supabase-token>`", "`gitrevolver agent install-instructions .`", "`gitrevolver agent guard . <action>`")
}
