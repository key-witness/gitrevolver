package main

import (
	"fmt"
	"os"

	"github.com/key-witness/gitrevolver/internal/config"
	"github.com/spf13/cobra"
)

var initProjectsDir string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the GitRevolver identity library",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(os.Stdout, welcomeDescription)
		fmt.Fprintln(os.Stdout)
		if err := config.InitLibrary(initProjectsDir); err != nil {
			return err
		}
		if err := installLibraryTemplates(); err != nil {
			return err
		}
		dir, _ := config.GetConfigDir()
		fmt.Fprintf(os.Stdout, "Initialized GitRevolver library at %s\n", dir)
		fmt.Fprintln(os.Stdout, "Secrets are stored locally in secure platform storage, not in YAML.")
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "Next steps:")
		fmt.Fprintln(os.Stdout, "  gitrevolver setup")
		fmt.Fprintln(os.Stdout, "  gitrevolver identity add")
		fmt.Fprintln(os.Stdout, "  gitrevolver project scan ~/code")
		return nil
	},
}

const welcomeDescription = `Welcome to GitRevolver.

GitRevolver is a Go/Cobra CLI for running multiple Codex and Claude agents under different GitHub identities in parallel without switching global gh auth state.

Set up your local project library, install the Codex or Claude instruction block, and registered projects can be matched to the right GitHub identity from their path and remote metadata. V1 is optimized for Vercel and Supabase stacks.`

func init() {
	initCmd.Flags().StringVar(&initProjectsDir, "projects-dir", "~/code", "Default directory scanned for projects")
	rootCmd.AddCommand(initCmd)
}
