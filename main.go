package main

import (
	"fmt"
	"os"

	"github.com/key-witness/gitrevolver/internal/ui"
	"github.com/spf13/cobra"
)

func showBanner() {
	fmt.Println(ui.Banner())
	fmt.Println(ui.Subtitle("Project identity runtime for parallel agents"))
	fmt.Println()
}

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "gitrevolver",
	Short: "Resolve project identities without switching global auth state",
	Long: `GitRevolver binds projects to identities and launches child processes
with project-scoped GitHub, Vercel, Supabase, and SSH context.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Show banner and help if no command provided
		showBanner()
		cmd.Help()
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show GitRevolver version",
	Run: func(cmd *cobra.Command, args []string) {
		showBanner()
		versionInfo := fmt.Sprintf(`Version: %s
Commit:  %s
Built:   %s`, version, commit, buildDate)
		fmt.Println(ui.InfoBox.Render(versionInfo))
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(bindCmd)
	rootCmd.AddCommand(unbindCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		// Use styled error box
		errorMsg := ui.ErrorBox.Render(fmt.Sprintf("❌ Error: %v", err))
		fmt.Fprintf(os.Stderr, "%s\n", errorMsg)
		os.Exit(1)
	}
}
