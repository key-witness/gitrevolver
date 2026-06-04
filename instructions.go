package main

import (
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
		"## Bootstrap",
		"Before any credentialed work, verify the toolchain and install any missing tools automatically:",
		"",
		"```bash",
		"# Install gitrevolver if missing",
		"which gitrevolver || go install github.com/key-witness/gitrevolver@latest",
		"",
		"# Install missing provider CLIs",
		"which gh       || brew install gh",
		"which vercel   || npm i -g vercel",
		"which supabase || brew install supabase-cli",
		"",
		"# Verify project identity",
		"gitrevolver resolve . --json",
		"gitrevolver agent doctor .",
		"```",
		"",
		"If doctor fails, stop and report the mismatch. Do not attempt blind auth/login/token flows.",
		"",
		"## Running provider tools",
		"Always inject credentials through GitRevolver rather than global auth state:",
		"",
		"```bash",
		"gitrevolver exec . -- gh pr create",
		"gitrevolver exec . -- vercel deploy",
		"gitrevolver exec . -- supabase db push",
		"gitrevolver exec . -- codex        # Codex with project credentials",
		"gitrevolver exec . -- claude       # Claude Code with project credentials",
		"```",
		"",
		"For extended sessions, open a project shell: `gitrevolver shell .`",
		"",
		"Do not run `gh auth switch`, `vercel login`, `supabase login`, or global Git author edits for this project.",
		"For risky actions, run `gitrevolver agent guard . <action>` first.",
		"",
		"## Secrets",
		"Retrieve runtime secrets for script injection only — never echo or log values:",
		"",
		"```bash",
		"get-secret <alias>                          # reads /etc/secrets/keys.env (global)",
		"gitrevolver secret get <identity> <name>   # reads macOS Keychain",
		"# Inject pattern: API_KEY=$(get-secret mykey) ./script",
		"```",
		"",
		"## First-time setup",
		"If GitRevolver is not initialized on this machine, show the user this flow and ask them to run token commands in their own terminal:",
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
	}, "\n")
}

func claudeInstructionBody() string {
	return `# GitRevolver identity instructions
Use GitRevolver before credentialed work in this repository.
GitRevolver stores provider tokens locally in secure platform storage. On macOS it uses the login Keychain. Never ask the user to paste tokens into chat, logs, YAML, or docs.

## Bootstrap
Check the toolchain at session start and install anything missing before wandering:

` + "```bash" + `
which gitrevolver || go install github.com/key-witness/gitrevolver@latest
which gh          || brew install gh
which vercel      || npm i -g vercel
gitrevolver resolve . --json
gitrevolver agent doctor .
` + "```" + `

If doctor fails, stop and report the mismatch. Do not attempt blind auth/login/token flows.

## Working with this project
- Resolve: ` + "`gitrevolver resolve . --json`" + `
- Validate: ` + "`gitrevolver agent doctor . --online`" + `
- Run provider tools: ` + "`gitrevolver exec . -- <command>`" + ` — injects GH_TOKEN, VERCEL_TOKEN, SUPABASE_ACCESS_TOKEN, GIT_SSH_COMMAND automatically
- Launch sub-agents: ` + "`gitrevolver exec . -- codex`" + ` or ` + "`gitrevolver exec . -- claude`" + `
- Extended session: ` + "`gitrevolver shell .`" + `

Never run ` + "`gh auth switch`" + `, ` + "`vercel login`" + `, ` + "`supabase login`" + `, or global Git author edits for this project.
Guard risky actions with ` + "`gitrevolver agent guard . <action>`" + `.

## Secrets
Retrieve runtime secrets for script injection (never echo values into chat or logs):
- Global aliases: ` + "`get-secret <alias>`" + ` (reads /etc/secrets/keys.env)
- Keychain: ` + "`gitrevolver secret get <identity> <name>`" + `
- Inject pattern: ` + "`API_KEY=$(get-secret mykey) ./script`" + `

## First-time setup
If setup is missing, show the user ` + "`gitrevolver setup`" + `, then have them run ` + "`gitrevolver secret set <identity> <github-token|vercel-token|supabase-token>`" + ` locally for each provider token and confirm before writing AGENTS.md or CLAUDE.md with ` + "`gitrevolver agent install-instructions .`" + `.
Never switch global provider auth for a GitRevolver project.
Guard risky actions with ` + "`gitrevolver agent guard . <action>`" + `.`
}
