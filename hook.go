package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var installHookCmd = &cobra.Command{
	Use:   "install-hook",
	Short: "Install pre-push hook",
	Long:  "Install a pre-push hook that calls the GitRevolver push guard before Git sends commits.",
	Run: func(cmd *cobra.Command, args []string) {
		if err := installHook(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var uninstallHookCmd = &cobra.Command{
	Use:   "uninstall-hook",
	Short: "Uninstall pre-push hook",
	Long:  "Remove the GitRevolver pre-push hook.",
	Run: func(cmd *cobra.Command, args []string) {
		if err := uninstallHook(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(installHookCmd)
	rootCmd.AddCommand(uninstallHookCmd)
}

func installHook() error {
	// Check if we're in a git repo
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("not a git repository")
	}

	gitDir := strings.TrimSpace(string(output))
	hooksDir := filepath.Join(gitDir, "hooks")
	hookPath := filepath.Join(hooksDir, "pre-push")

	// Check if hook already exists
	if _, err := os.Stat(hookPath); err == nil {
		// Check if it's our hook
		data, _ := os.ReadFile(hookPath)
		if strings.Contains(string(data), "GitRevolver") {
			fmt.Println("GitRevolver pre-push hook already installed")
			return nil
		}
		// Existing hook - we should merge or warn
		fmt.Println("Warning: pre-push hook already exists. GitRevolver hook not installed.")
		fmt.Println("You can manually merge the GitRevolver check into your existing hook.")
		return nil
	}

	// Create hooks directory if it doesn't exist
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}

	hookContent := `#!/bin/sh
# GitRevolver pre-push hook
# Verifies the registered project identity before Git sends commits.

if ! command -v gitrevolver >/dev/null 2>&1; then
  echo "Error: GitRevolver is required by this pre-push hook but was not found in PATH."
  exit 1
fi

# The push command is already an explicit action, so the hook confirms that
# guard action while retaining remote, author, identity, and branch checks.
exec gitrevolver agent guard . git-push --yes
`

	if err := os.WriteFile(hookPath, []byte(hookContent), 0755); err != nil {
		return fmt.Errorf("failed to write hook: %w", err)
	}

	fmt.Println("✓ Pre-push hook installed")
	return nil
}

func uninstallHook() error {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("not a git repository")
	}

	gitDir := strings.TrimSpace(string(output))
	hookPath := filepath.Join(gitDir, "hooks", "pre-push")

	// Check if hook exists and is ours
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		fmt.Println("No GitRevolver pre-push hook found")
		return nil
	}

	data, err := os.ReadFile(hookPath)
	if err != nil {
		return err
	}

	if !strings.Contains(string(data), "GitRevolver") {
		fmt.Println("Pre-push hook exists but is not a GitRevolver hook")
		return nil
	}

	if err := os.Remove(hookPath); err != nil {
		return fmt.Errorf("failed to remove hook: %w", err)
	}

	fmt.Println("✓ Pre-push hook uninstalled")
	return nil
}
