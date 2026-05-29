# GitRevolver

GitRevolver is a local identity runtime for developers and coding agents working across multiple GitHub, Vercel, and Supabase identities.

It extends per-repository Git identity management with project resolution, secure token storage, process-local environment injection, and safety checks. The core workflow is:

```bash
go install github.com/key-witness/gitrevolver@latest
gitrevolver init
gitrevolver setup
gitrevolver identity add
gitrevolver secret set client-a github-token
gitrevolver secret set client-a vercel-token
gitrevolver secret set client-a supabase-token
gitrevolver project scan ~/code
gitrevolver project bind client-a-app --identity client-a
gitrevolver agent install-instructions client-a-app
gitrevolver agent doctor client-a-app
gitrevolver agent doctor client-a-app --online
gitrevolver exec client-a-app -- codex
```

## Welcome

GitRevolver is a Go/Cobra CLI for running multiple Codex and Claude agents under different GitHub identities in parallel without switching global `gh` auth state.

Set up your local project library, install the Codex or Claude instruction block, and registered projects can be matched to the right GitHub identity from their path and remote metadata. V1 is optimized for Vercel and Supabase stacks.

## Why

Parallel agents should not depend on one global active GitHub, Vercel, or Supabase login. GitRevolver resolves a project to its Git author, GitHub SSH alias, provider metadata, and keyring-backed tokens, then injects that context only into the child process it launches.

GitRevolver is designed for workflows such as:

- one Codex session on a personal repo
- one Codex session on a client repo
- a Vercel preview shell
- a Supabase migration shell

Those processes can run at the same time without `gh auth switch`, `vercel login`, `supabase login`, or global Git author edits.

## Install From Source

Install the published CLI:

```bash
go install github.com/key-witness/gitrevolver@latest
gitrevolver setup
```

Or build from a checkout:

```bash
git clone https://github.com/key-witness/gitrevolver.git
cd gitrevolver
make test
make build
```

GitRevolver currently targets macOS first. Git, SSH, and Go 1.21 or newer are expected.
The Makefile builds with cgo disabled by default, so a Mac does not need Xcode or Xcode Command Line Tools for GitRevolver. On macOS, secrets go to the login Keychain through the built-in `security` CLI. Non-macOS builds keep the `99designs/keyring` backend.

## Reproducible Setup

Ask Codex or Claude to install GitRevolver, then have it show the local setup checklist:

```bash
gitrevolver setup
```

The intended first-run flow is:

```bash
gitrevolver init
gitrevolver identity add
gitrevolver secret set client-a github-token
gitrevolver secret set client-a vercel-token
gitrevolver secret set client-a supabase-token
gitrevolver project scan ~/code
gitrevolver project bind client-a-app --identity client-a
gitrevolver agent install-instructions client-a-app
gitrevolver agent doctor client-a-app
gitrevolver agent doctor client-a-app --online
gitrevolver exec client-a-app -- codex
```

`gitrevolver agent install-instructions` asks before updating `AGENTS.md` and `CLAUDE.md`. For reviewed non-interactive automation, pass `--yes`.

Provider tokens are stored locally in secure platform storage. On macOS, `gitrevolver secret set` opens a hidden login Keychain prompt; token values are not placed in command-line arguments, committed to YAML, or printed by normal status/doctor output. Do not paste real tokens into agent chat.

## Library Model

Non-secret state lives under `~/.gitrevolver/`:

```text
config.yml
identities.yml
projects.yml
AGENTS.template.md
CLAUDE.template.md
```

Provider tokens are stored in secure platform storage with identity-specific keys. YAML files store only metadata and secret references.

## Commands

Identity and secrets:

```bash
gitrevolver init
gitrevolver setup
gitrevolver identity add
gitrevolver identity list
gitrevolver identity show client-a --json
gitrevolver identity edit client-a
gitrevolver secret set client-a github-token
gitrevolver secret set client-a vercel-token
gitrevolver secret set client-a supabase-token
gitrevolver secret check client-a
```

Projects:

```bash
gitrevolver project add
gitrevolver project scan ~/code
gitrevolver project bind client-a-app --identity client-a
gitrevolver project link-vercel client-a-app --org-id team_xxx --project-id prj_xxx
gitrevolver project link-supabase client-a-app --project-ref abcdxyz
```

Runtime:

```bash
gitrevolver resolve . --json
gitrevolver env client-a-app --format shell
gitrevolver env client-a-app --format shell --include-secrets
gitrevolver shell client-a-app
gitrevolver exec client-a-app -- gh pr create
gitrevolver exec client-a-app -- codex
```

Agent safety:

```bash
gitrevolver agent doctor .
gitrevolver agent doctor . --online
gitrevolver agent guard . git-push --yes
gitrevolver agent guard . vercel-prod-deploy --yes
gitrevolver agent guard . supabase-db-push --yes
gitrevolver agent install-instructions .
gitrevolver agent install-instructions . --yes
```

## Security Model

- Git operations use per-identity SSH aliases and repo-local Git author config.
- `exec` and `shell` inject provider tokens into child processes. `env` output excludes token values unless the user passes `--include-secrets`.
- On macOS, non-interactive token storage is rejected because it would expose secrets in process arguments; use `gitrevolver secret set` in a terminal and enter the value in the hidden Keychain prompt.
- The optional pre-push hook delegates to `gitrevolver agent guard . git-push --yes` so registered identity, remote, Git author, and branch checks run before Git sends commits.
- SSH config changes are validated with `ssh -G` before GitRevolver replaces the candidate config file.
- Missing optional GitHub or Vercel tokens do not block SSH-only work.
- `gitrevolver agent doctor --online` validates stored provider tokens with safe provider CLI calls when the matching CLI is installed.
- Guard checks block configured risky actions until branch, remote, metadata, token, and confirmation checks pass. Vercel production deploy checks block production branch mismatches unless explicitly overridden.
- Normal status and doctor output do not print token values.

Run the security checks before publishing:

```bash
go test ./...
go test -race ./...
go vet ./...
govulncheck ./...
gitleaks detect --source . --verbose
```

## Agent Integration

`gitrevolver agent install-instructions .` updates marked GitRevolver blocks in `AGENTS.md` and `CLAUDE.md`. This repo also ships a Codex skill in `.agents/skills/gitrevolver/` for GitRevolver setup and usage.

## Limitations

- GitHub Git transport is intentionally SSH-first. HTTPS credential-helper switching is not the default safety model.
- Vercel and Supabase support is provider context plus guards, not a wrapper around every provider CLI command.
- `project scan` uses GitHub remote and SSH alias heuristics; unknown repos still require user confirmation.

## Upstream

GitRevolver is derived from `csawai/git-identity-switcher`, an MIT-licensed per-repository Git identity CLI. See `NOTICE.md`.
