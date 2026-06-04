package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/key-witness/gitrevolver/internal/config"
	"github.com/key-witness/gitrevolver/internal/keychain"
	"github.com/key-witness/gitrevolver/internal/ssh"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var identityCmd = &cobra.Command{
	Use:   "identity",
	Short: "Manage GitRevolver identities",
}

var identityAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a GitHub identity and optional provider secrets",
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(os.Stdin)
		slug, err := promptRequired(reader, "Identity slug")
		if err != nil {
			return err
		}
		if err := config.ValidateSlug("identity", slug); err != nil {
			return err
		}
		displayName, err := promptDefault(reader, "Display name", slug)
		if err != nil {
			return err
		}
		username, err := promptRequired(reader, "GitHub username")
		if err != nil {
			return err
		}
		name, err := promptRequired(reader, "Git commit name")
		if err != nil {
			return err
		}
		email, err := promptRequired(reader, "Git commit email")
		if err != nil {
			return err
		}
		hostAlias, err := promptDefault(reader, "SSH host alias", "github-"+slug)
		if err != nil {
			return err
		}
		keyPath, err := identitySSHPath(reader, slug)
		if err != nil {
			return err
		}
		identity := config.Identity{
			ID:          slug,
			DisplayName: displayName,
			Git:         config.GitIdentity{Name: name, Email: email},
			GitHub: config.GitHubIdentity{
				Username:       username,
				HostAlias:      hostAlias,
				SSHKeyPath:     keyPath,
				APITokenSecret: keychain.SecretKey(slug, keychain.GitHubTokenSecret),
			},
			Vercel: config.VercelIdentity{
				Mode:        "github-auto-deploy",
				TokenSecret: keychain.SecretKey(slug, keychain.VercelTokenSecret),
			},
			Supabase: config.SupabaseIdentity{
				TokenSecret: keychain.SecretKey(slug, keychain.SupabaseTokenSecret),
			},
		}
		if err := config.AddIdentity(identity); err != nil {
			return err
		}
		if keyPath != "" {
			if err := ssh.AddSSHConfigEntry(hostAlias, keyPath); err != nil {
				return fmt.Errorf("failed to update SSH config: %w", err)
			}
			fmt.Fprintf(os.Stdout, "SSH public key: %s\n", keyPath+".pub")
			fmt.Fprintln(os.Stdout, "Add that public key to the matching GitHub account before pushing.")
		}
		for _, secret := range []string{keychain.GitHubTokenSecret, keychain.VercelTokenSecret, keychain.SupabaseTokenSecret} {
			if yes(reader, "Store "+secret+" now?", false) {
				if err := storeSecretFromPrompt(reader, slug, secret); err != nil {
					return err
				}
			}
		}
		fmt.Fprintf(os.Stdout, "Added identity %s\n", slug)
		return nil
	},
}

var identityListCmd = &cobra.Command{
	Use:   "list",
	Short: "List identities without secrets",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := config.LoadConfig()
		if err != nil {
			return err
		}
		slugs := sortedIdentitySlugs(store)
		if len(slugs) == 0 {
			fmt.Fprintln(os.Stdout, "No identities configured. Run `gitrevolver identity add`.")
			return nil
		}
		for _, slug := range slugs {
			identity := store.Identities[slug]
			fmt.Fprintf(os.Stdout, "%s\t%s\t%s\t%s\n", slug, identity.DisplayName, identity.Git.Email, identity.GitHub.Username)
		}
		return nil
	},
}

var identityShowJSON bool

var identityShowCmd = &cobra.Command{
	Use:   "show <identity>",
	Short: "Show one identity without token values",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identity, err := config.FindIdentityByAlias(args[0])
		if err != nil {
			return err
		}
		if identityShowJSON {
			return writeJSON(os.Stdout, identity)
		}
		fmt.Fprintf(os.Stdout, "Identity: %s\nName: %s\nGit: %s <%s>\nGitHub: %s via %s\nSSH key: %s\n",
			identity.ID, identity.DisplayName, identity.Git.Name, identity.Git.Email, identity.GitHub.Username, identity.GitHub.HostAlias, identity.GitHub.SSHKeyPath)
		return nil
	},
}

var identityEditCmd = &cobra.Command{
	Use:   "edit <identity>",
	Short: "Edit identity display, Git author, and SSH metadata",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identity, err := config.FindIdentityByAlias(args[0])
		if err != nil {
			return err
		}
		reader := bufio.NewReader(os.Stdin)
		if identity.DisplayName, err = promptDefault(reader, "Display name", identity.DisplayName); err != nil {
			return err
		}
		if identity.Git.Name, err = promptDefault(reader, "Git commit name", identity.Git.Name); err != nil {
			return err
		}
		if identity.Git.Email, err = promptDefault(reader, "Git commit email", identity.Git.Email); err != nil {
			return err
		}
		if identity.GitHub.Username, err = promptDefault(reader, "GitHub username", identity.GitHub.Username); err != nil {
			return err
		}
		if identity.GitHub.HostAlias, err = promptDefault(reader, "SSH host alias", identity.GitHub.HostAlias); err != nil {
			return err
		}
		if identity.GitHub.SSHKeyPath, err = promptDefault(reader, "SSH key path", identity.GitHub.SSHKeyPath); err != nil {
			return err
		}
		if err := config.UpdateIdentity(*identity); err != nil {
			return err
		}
		if identity.GitHub.HostAlias != "" && identity.GitHub.SSHKeyPath != "" {
			return ssh.AddSSHConfigEntry(identity.GitHub.HostAlias, identity.GitHub.SSHKeyPath)
		}
		return nil
	},
}

var identityRemoveForce bool
var identityRemoveKeys bool

var identityRemoveCmd = &cobra.Command{
	Use:   "remove <identity>",
	Short: "Remove an identity, its stored tokens, and optional generated SSH files",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identity, err := config.FindIdentityByAlias(args[0])
		if err != nil {
			return err
		}
		if !identityRemoveForce && !yes(bufio.NewReader(os.Stdin), "Remove identity "+identity.ID+"?", false) {
			fmt.Fprintln(os.Stdout, "Cancelled.")
			return nil
		}
		if identity.GitHub.HostAlias != "" {
			_ = ssh.RemoveSSHConfigEntry(identity.GitHub.HostAlias)
		}
		_ = keychain.DeleteAllSecrets(identity.ID)
		if identityRemoveKeys && identity.GitHub.SSHKeyPath != "" {
			_ = os.Remove(identity.GitHub.SSHKeyPath)
			_ = os.Remove(identity.GitHub.SSHKeyPath + ".pub")
		}
		return config.RemoveIdentity(identity.ID)
	},
}

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Manage identity tokens in the OS keyring",
}

var storeSecret = keychain.StoreSecret

var secretSetCmd = &cobra.Command{
	Use:   "set <identity> <github-token|vercel-token|supabase-token>",
	Short: "Store a token without writing it to config",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.FindIdentityByAlias(args[0]); err != nil {
			return err
		}
		if !knownSecret(args[1]) {
			return fmt.Errorf("unsupported secret %q", args[1])
		}
		return storeSecretFromPrompt(bufio.NewReader(os.Stdin), args[0], args[1])
	},
}

var secretCheckCmd = &cobra.Command{
	Use:   "check <identity>",
	Short: "Check which provider tokens exist without printing values",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.FindIdentityByAlias(args[0]); err != nil {
			return err
		}
		for _, name := range []string{keychain.GitHubTokenSecret, keychain.VercelTokenSecret, keychain.SupabaseTokenSecret} {
			status := "missing"
			if token, err := keychain.GetSecret(args[0], name); err == nil && token != "" {
				status = "present"
			}
			fmt.Fprintf(os.Stdout, "%s\t%s\n", name, status)
		}
		return nil
	},
}

var secretRemoveCmd = &cobra.Command{
	Use:   "remove <identity> <secret>",
	Short: "Remove a token from the OS keyring",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !knownSecret(args[1]) {
			return fmt.Errorf("unsupported secret %q", args[1])
		}
		return keychain.DeleteSecret(args[0], args[1])
	},
}

// secretGetCmd prints a secret value to stdout for use in script injection.
// Use for runtime patterns like: TOKEN=$(gitrevolver secret get . github-token) ./script
// Never pipe or echo the output into chat, logs, or files.
var secretGetCmd = &cobra.Command{
	Use:   "get <identity-or-path> <secret-name>",
	Short: "Print a secret value to stdout for script injection (not for chat or logs)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		identityID := args[0]
		// Resolve project path (handles ".", absolute paths, and project slugs)
		if resolved, err := resolveProject(args[0]); err == nil {
			identityID = resolved.Identity.ID
		} else {
			if _, err := config.FindIdentityByAlias(args[0]); err != nil {
				return fmt.Errorf("could not resolve %q as identity or project: %w", args[0], err)
			}
		}
		token, err := keychain.GetSecret(identityID, args[1])
		if err != nil {
			return fmt.Errorf("secret %q not found for %s: %w", args[1], identityID, err)
		}
		fmt.Fprint(os.Stdout, token)
		return nil
	},
}

func init() {
	identityShowCmd.Flags().BoolVar(&identityShowJSON, "json", false, "Render machine-readable output")
	identityRemoveCmd.Flags().BoolVar(&identityRemoveForce, "force", false, "Skip removal confirmation")
	identityRemoveCmd.Flags().BoolVar(&identityRemoveKeys, "delete-keys", false, "Delete identity SSH key files")
	identityCmd.AddCommand(identityAddCmd, identityListCmd, identityShowCmd, identityEditCmd, identityRemoveCmd)
	secretCmd.AddCommand(secretSetCmd, secretGetCmd, secretCheckCmd, secretRemoveCmd)
	rootCmd.AddCommand(identityCmd, secretCmd)
}

func identitySSHPath(reader *bufio.Reader, slug string) (string, error) {
	if yes(reader, "Create a new SSH key?", true) {
		return ssh.GenerateSSHKey(slug)
	}
	path, err := promptDefault(reader, "Existing SSH key path", filepath.Join("~", ".ssh", "gitrevolver_"+slug))
	if err != nil {
		return "", err
	}
	return expandPath(path)
}

func sortedIdentitySlugs(store *config.IdentityStore) []string {
	slugs := make([]string, 0, len(store.Identities))
	for slug := range store.Identities {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	return slugs
}

func knownSecret(name string) bool {
	switch name {
	case keychain.GitHubTokenSecret, keychain.VercelTokenSecret, keychain.SupabaseTokenSecret:
		return true
	default:
		return false
	}
}

func storeSecretFromPrompt(reader *bufio.Reader, identity, name string) error {
	if keychain.SupportsInteractiveStore() && term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stdout, "Enter %s for %s in the hidden macOS Keychain prompt.\n", name, identity)
		if err := keychain.StoreSecretInteractive(identity, name); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Stored %s for %s\n", name, identity)
		return nil
	}

	token, err := promptSecret(reader, name+" value")
	if err != nil {
		return err
	}
	if err := storeSecret(identity, name, token); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Stored %s for %s\n", name, identity)
	return nil
}

func promptSecret(reader *bufio.Reader, label string) (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stdout, "%s: ", label)
		value, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stdout)
		if err != nil {
			return "", err
		}
		trimmed := strings.TrimSpace(string(value))
		if trimmed == "" {
			return "", fmt.Errorf("%s cannot be empty", label)
		}
		return trimmed, nil
	}
	return promptRequired(reader, label)
}

func promptRequired(reader *bufio.Reader, label string) (string, error) {
	value, err := promptDefault(reader, label, "")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s cannot be empty", label)
	}
	return strings.TrimSpace(value), nil
}

func promptDefault(reader *bufio.Reader, label, fallback string) (string, error) {
	if fallback == "" {
		fmt.Fprintf(os.Stdout, "%s: ", label)
	} else {
		fmt.Fprintf(os.Stdout, "%s [%s]: ", label, fallback)
	}
	value, err := reader.ReadString('\n')
	if err != nil && len(value) == 0 {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	return value, nil
}

func yes(reader *bufio.Reader, label string, fallback bool) bool {
	defaultText := "y/N"
	if fallback {
		defaultText = "Y/n"
	}
	answer, err := promptDefault(reader, label+" ("+defaultText+")", "")
	if err != nil || answer == "" {
		return fallback
	}
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes")
}

func writeJSON(file *os.File, value any) error {
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
