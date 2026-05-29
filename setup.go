package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var setupProject string
var setupIdentity string

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Print a reproducible GitRevolver setup checklist",
	RunE: func(cmd *cobra.Command, args []string) error {
		printSetupGuide(os.Stdout, setupIdentity, setupProject)
		return nil
	},
}

func init() {
	setupCmd.Flags().StringVar(&setupIdentity, "identity", "client-a", "Identity slug to use in example commands")
	setupCmd.Flags().StringVar(&setupProject, "project", "client-a-app", "Project slug or path to use in example commands")
	rootCmd.AddCommand(setupCmd)
}

func printSetupGuide(out io.Writer, identity, project string) {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		identity = "client-a"
	}
	project = strings.TrimSpace(project)
	if project == "" {
		project = "client-a-app"
	}

	fmt.Fprintln(out, "GitRevolver setup checklist")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Install:")
	fmt.Fprintln(out, "  go install github.com/key-witness/gitrevolver@latest")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Create the local library:")
	fmt.Fprintln(out, "  gitrevolver init")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Create an identity and register projects:")
	fmt.Fprintln(out, "  gitrevolver identity add")
	fmt.Fprintln(out, "  gitrevolver project scan ~/code")
	fmt.Fprintf(out, "  gitrevolver project bind %s --identity %s\n", project, identity)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Store provider tokens locally:")
	fmt.Fprintf(out, "  gitrevolver secret set %s github-token\n", identity)
	fmt.Fprintf(out, "  gitrevolver secret set %s vercel-token\n", identity)
	fmt.Fprintf(out, "  gitrevolver secret set %s supabase-token\n", identity)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Token storage disclaimer:")
	fmt.Fprintln(out, "  Tokens are stored on this machine in secure platform storage.")
	fmt.Fprintln(out, "  On macOS, GitRevolver uses the login Keychain and opens a hidden Keychain prompt.")
	fmt.Fprintln(out, "  Token values are not written to GitRevolver YAML files, examples, logs, or normal command output.")
	fmt.Fprintln(out, "  Do not paste real token values into Codex, Claude, issue comments, or shell history.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Confirm and add Codex/Claude project context:")
	fmt.Fprintf(out, "  gitrevolver agent install-instructions %s\n", project)
	fmt.Fprintln(out, "  # In non-interactive automation, pass --yes only after reviewing the files it will update:")
	fmt.Fprintf(out, "  gitrevolver agent install-instructions %s --yes\n", project)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Verify and use:")
	fmt.Fprintf(out, "  gitrevolver agent doctor %s\n", project)
	fmt.Fprintf(out, "  gitrevolver agent doctor %s --online\n", project)
	fmt.Fprintf(out, "  gitrevolver exec %s -- codex\n", project)
	fmt.Fprintf(out, "  gitrevolver exec %s -- claude\n", project)
	fmt.Fprintf(out, "  gitrevolver exec %s -- gh pr status\n", project)
	fmt.Fprintf(out, "  gitrevolver exec %s -- vercel whoami\n", project)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Before risky actions:")
	fmt.Fprintf(out, "  gitrevolver agent guard %s git-push --yes\n", project)
	fmt.Fprintf(out, "  gitrevolver agent guard %s vercel-prod-deploy --yes\n", project)
	fmt.Fprintf(out, "  gitrevolver agent guard %s supabase-db-push --yes\n", project)
}
