package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/key-witness/gitrevolver/internal/config"
	"github.com/key-witness/gitrevolver/internal/ssh"
	"github.com/key-witness/gitrevolver/internal/ui"
	"github.com/spf13/cobra"
)

var bindDryRun bool

func init() {
	bindCmd.Flags().BoolVar(&bindDryRun, "dry-run", false, "Show what would be changed without making changes")
}

var bindCmd = &cobra.Command{
	Use:   "bind [identity]",
	Short: "Bind repository to an identity",
	Long:  "Bind the current repository to a specific identity, updating user.name, user.email, and remote URL.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := bindIdentity(args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func bindIdentity(alias string) error {
	// Load identity
	identity, err := config.FindIdentityByAlias(alias)
	if err != nil {
		return err
	}

	// Check if we're in a git repo
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("not a git repository")
	}

	if bindDryRun {
		currentName, _ := getGitConfig("user.name")
		currentEmail, _ := getGitConfig("user.email")
		currentRemote, _ := getRemoteURL()

		fmt.Println("[DRY RUN] Would make the following changes:")
		fmt.Printf("  user.name: '%s' -> '%s'\n", currentName, identity.Git.Name)
		fmt.Printf("  user.email: '%s' -> '%s'\n", currentEmail, identity.Git.Email)
		if identity.GitHub.HostAlias != "" {
			newRemote := strings.Replace(currentRemote, "git@github.com:", fmt.Sprintf("git@%s:", identity.GitHub.HostAlias), 1)
			fmt.Printf("  remote URL: '%s' -> '%s'\n", currentRemote, newRemote)
		}
		return nil
	}

	// Set user.name
	if err := setGitConfig("user.name", identity.Git.Name); err != nil {
		return fmt.Errorf("failed to set user.name: %w", err)
	}

	// Set user.email
	if err := setGitConfig("user.email", identity.Git.Email); err != nil {
		return fmt.Errorf("failed to set user.email: %w", err)
	}

	if err := setGitConfig("gitrevolver.bound", alias); err != nil {
		return fmt.Errorf("failed to set gitrevolver.bound marker: %w", err)
	}

	// Ensure SSH config entry exists for SSH identities
	if identity.GitHub.HostAlias != "" && identity.GitHub.SSHKeyPath != "" {
		if err := ssh.AddSSHConfigEntry(identity.GitHub.HostAlias, identity.GitHub.SSHKeyPath); err != nil {
			return fmt.Errorf("failed to update SSH config: %w", err)
		}
	}

	// Update remote URL based on auth method
	if identity.GitHub.HostAlias != "" {
		if err := updateRemoteURL(identity.GitHub.HostAlias); err != nil {
			return fmt.Errorf("failed to update remote URL: %w", err)
		}
	}

	fmt.Println(ui.Celebration(fmt.Sprintf("Repository bound to identity '%s'", alias)))
	return nil
}

func setGitConfig(key, value string) error {
	cmd := exec.Command("git", "config", "--local", key, value)
	return cmd.Run()
}

func updateRemoteURL(hostAlias string) error {
	// Get current remote URL
	cmd := exec.Command("git", "remote", "get-url", "origin")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get remote URL: %w", err)
	}

	currentURL := strings.TrimSpace(string(output))

	// Convert to use host alias
	var newURL string
	if strings.HasPrefix(currentURL, "git@github.com:") {
		// SSH format: git@github.com:org/repo.git -> git@github.com-work:org/repo.git
		newURL = strings.Replace(currentURL, "git@github.com:", fmt.Sprintf("git@%s:", hostAlias), 1)
	} else if strings.HasPrefix(currentURL, "https://github.com/") {
		// HTTPS format: https://github.com/org/repo.git -> git@github.com-work:org/repo.git
		parts := strings.TrimPrefix(currentURL, "https://github.com/")
		newURL = fmt.Sprintf("git@%s:%s", hostAlias, parts)
	} else {
		// Already using a host alias or custom format
		return nil
	}

	// Update remote
	cmd = exec.Command("git", "remote", "set-url", "origin", newURL)
	return cmd.Run()
}
