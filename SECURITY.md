# Security

Report security issues privately to the repository owner before public disclosure.

## Token Handling

GitRevolver stores GitHub, Vercel, and Supabase tokens locally in secure platform storage. On macOS it talks to the login Keychain through the built-in `security` CLI and uses the hidden Keychain prompt for token writes, so token values are not placed in command-line arguments. Do not place real tokens in YAML config, docs, examples, issues, test fixtures, agent chat, or logs.

On macOS, non-interactive token writes are intentionally rejected because the built-in `security` CLI would otherwise require the token value as a process argument. Run `gitrevolver secret set <identity> <github-token|vercel-token|supabase-token>` in a terminal and enter the value in the Keychain prompt.

`gitrevolver env` excludes token values by default. `gitrevolver env --include-secrets` explicitly prints them. Prefer `gitrevolver exec` or `gitrevolver shell` when a human does not need to inspect token-bearing environment values.
