package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/key-witness/gitrevolver/internal/config"
	"github.com/key-witness/gitrevolver/internal/keychain"
	"github.com/key-witness/gitrevolver/internal/ssh"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type CheckResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type DoctorResult struct {
	Project string        `json:"project"`
	Checks  []CheckResult `json:"checks"`
}

type GuardResult struct {
	Project string        `json:"project"`
	Action  string        `json:"action"`
	Allowed bool          `json:"allowed"`
	Checks  []CheckResult `json:"checks"`
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent diagnostics, safety guards, and instructions",
}

var doctorJSON bool
var doctorOnline bool

var doctorCmd = &cobra.Command{
	Use:   "doctor <project-or-path>",
	Short: "Check project identity and provider context",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		resolved, err := resolveProject(args[0])
		if err != nil {
			return err
		}
		result := doctor(resolved, doctorOnline)
		if doctorJSON {
			return writeJSON(os.Stdout, result)
		}
		printChecks("GitRevolver Doctor: "+result.Project, result.Checks)
		return nil
	},
}

var guardJSON bool
var guardYes bool
var guardAllowProtected bool
var guardAllowProductionBranchMismatch bool
var installInstructionsYes bool

var guardCmd = &cobra.Command{
	Use:   "guard <project-or-path> <git-push|vercel-prod-deploy|supabase-db-push>",
	Short: "Preflight a risky project action",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		resolved, err := resolveProject(args[0])
		if err != nil {
			return err
		}
		result := guard(resolved, args[1], guardYes, guardAllowProtected, guardAllowProductionBranchMismatch)
		if guardJSON {
			if err := writeJSON(os.Stdout, result); err != nil {
				return err
			}
		} else {
			printChecks("GitRevolver Guard: "+result.Action, result.Checks)
		}
		if !result.Allowed {
			return fmt.Errorf("%s blocked for %s", result.Action, result.Project)
		}
		return nil
	},
}

var installInstructionsCmd = &cobra.Command{
	Use:   "install-instructions <project-or-path>",
	Short: "Install or update GitRevolver AGENTS and Claude instruction blocks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		resolved, err := resolveProject(args[0])
		if err != nil {
			return err
		}
		if !installInstructionsYes {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return fmt.Errorf("refusing to update AGENTS.md and CLAUDE.md without confirmation; pass --yes after review")
			}
			if !yes(bufio.NewReader(os.Stdin), "Update AGENTS.md and CLAUDE.md with GitRevolver context?", true) {
				fmt.Fprintln(os.Stdout, "Cancelled.")
				return nil
			}
		}
		if err := installProjectInstructions(resolved.Project.Path); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Updated GitRevolver instruction blocks in %s and %s\n",
			filepath.Join(resolved.Project.Path, "AGENTS.md"), filepath.Join(resolved.Project.Path, "CLAUDE.md"))
		return nil
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "Render machine-readable output")
	doctorCmd.Flags().BoolVar(&doctorOnline, "online", false, "Validate stored provider tokens with safe provider CLI calls")
	guardCmd.Flags().BoolVar(&guardJSON, "json", false, "Render machine-readable output")
	guardCmd.Flags().BoolVar(&guardYes, "yes", false, "Confirm configured guard actions")
	guardCmd.Flags().BoolVar(&guardAllowProtected, "allow-protected", false, "Allow protected branch git push checks")
	guardCmd.Flags().BoolVar(&guardAllowProductionBranchMismatch, "allow-production-branch-mismatch", false, "Allow Vercel production deploy checks from a non-production branch")
	installInstructionsCmd.Flags().BoolVar(&installInstructionsYes, "yes", false, "Update AGENTS.md and CLAUDE.md without an interactive prompt")
	agentCmd.AddCommand(doctorCmd, guardCmd, installInstructionsCmd)
	rootCmd.AddCommand(agentCmd)
}

func doctor(resolved *ResolvedProject, online bool) DoctorResult {
	project := resolved.Project
	identity := resolved.Identity
	checks := []CheckResult{
		okCheck("project", project.Path),
		okCheck("identity", identity.ID),
		pathCheck("repo path", project.Path),
	}
	checks = append(checks, gitAuthorChecks(project.Path, identity)...)
	checks = append(checks, remoteCheck(project)...)
	checks = append(checks, sshChecks(identity)...)
	if optionalSecret(identity.ID, keychain.GitHubTokenSecret) == "" {
		checks = append(checks, skipCheck("GitHub API token", "optional"))
	} else {
		checks = append(checks, okCheck("GitHub API token", "present"))
	}
	checks = append(checks, vercelDoctorChecks(project, identity)...)
	checks = append(checks, supabaseDoctorChecks(project, identity)...)
	if online {
		checks = append(checks, onlineProviderChecks(resolved)...)
	}
	if branch := currentBranch(project.Path); branch != "" {
		if protected(project.Safety.ProtectedBranches, branch) {
			checks = append(checks, warnCheck("branch", branch+" is protected"))
		} else {
			checks = append(checks, okCheck("branch", branch))
		}
	}
	checks = append(checks, okCheck("runtime env", "available"))
	return DoctorResult{Project: project.Slug, Checks: checks}
}

func onlineProviderChecks(resolved *ResolvedProject) []CheckResult {
	env := mergeEnv(os.Environ(), runtimeEnv(resolved))
	checks := []CheckResult{}
	if optionalSecret(resolved.Identity.ID, keychain.GitHubTokenSecret) != "" {
		checks = append(checks, providerCommandCheck("GitHub token online", resolved.Project.Path, env, "gh", "auth", "status", "-h", "github.com"))
	}
	if optionalSecret(resolved.Identity.ID, keychain.VercelTokenSecret) != "" {
		checks = append(checks, providerCommandCheck("Vercel token online", resolved.Project.Path, env, "vercel", "whoami"))
	}
	if optionalSecret(resolved.Identity.ID, keychain.SupabaseTokenSecret) != "" {
		checks = append(checks, providerCommandCheck("Supabase token online", resolved.Project.Path, env, "supabase", "projects", "list"))
	}
	if len(checks) == 0 {
		checks = append(checks, skipCheck("provider tokens online", "no provider tokens present"))
	}
	return checks
}

func providerCommandCheck(name, dir string, env []string, command string, args ...string) CheckResult {
	if _, err := exec.LookPath(command); err != nil {
		return warnCheck(name, command+" not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, command, args...)
	child.Dir = dir
	child.Env = env
	output, err := child.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return warnCheck(name, "timed out")
	}
	if err != nil {
		return warnCheck(name, summarizeProviderOutput(output, err, env))
	}
	return okCheck(name, "validated")
}

func summarizeProviderOutput(output []byte, err error, env []string) string {
	message := strings.TrimSpace(string(output))
	if message == "" {
		return err.Error()
	}
	message = redactEnvSecrets(message, env)
	lines := strings.Split(message, "\n")
	if len(lines) > 2 {
		lines = lines[:2]
	}
	message = strings.Join(lines, " ")
	if len(message) > 180 {
		message = message[:180] + "..."
	}
	return message
}

func redactEnvSecrets(message string, env []string) string {
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || value == "" {
			continue
		}
		switch key {
		case "GH_TOKEN", "GITHUB_TOKEN", "VERCEL_TOKEN", "SUPABASE_ACCESS_TOKEN":
			message = strings.ReplaceAll(message, value, "[redacted]")
		}
	}
	return message
}

func guard(resolved *ResolvedProject, action string, confirmed, allowProtected, allowProductionBranchMismatch bool) GuardResult {
	result := GuardResult{Project: resolved.Project.Slug, Action: action}
	switch action {
	case "git-push":
		result.Checks = append(result.Checks, pathCheck("repo path", resolved.Project.Path))
		result.Checks = append(result.Checks, currentDirCheck(resolved.Project.Path))
		result.Checks = append(result.Checks, gitAuthorChecks(resolved.Project.Path, resolved.Identity)...)
		result.Checks = append(result.Checks, remoteCheck(resolved.Project)...)
		branch := currentBranch(resolved.Project.Path)
		if branch != "" && protected(resolved.Project.Safety.ProtectedBranches, branch) && !allowProtected {
			result.Checks = append(result.Checks, blockCheck("branch", branch+" is protected; pass --allow-protected after review"))
		} else if branch != "" {
			result.Checks = append(result.Checks, okCheck("branch", branch))
		}
	case "vercel-prod-deploy":
		if optionalSecret(resolved.Identity.ID, keychain.VercelTokenSecret) == "" {
			result.Checks = append(result.Checks, blockCheck("Vercel token", "missing for "+resolved.Identity.ID))
		} else {
			result.Checks = append(result.Checks, okCheck("Vercel token", "present for "+resolved.Identity.ID))
		}
		if resolved.Project.Vercel.ProductionBranch != "" {
			branch := currentBranch(resolved.Project.Path)
			if branch != "" && branch != resolved.Project.Vercel.ProductionBranch && !allowProductionBranchMismatch {
				result.Checks = append(result.Checks, blockCheck("production branch", branch+" differs from "+resolved.Project.Vercel.ProductionBranch))
			} else if branch != "" && branch != resolved.Project.Vercel.ProductionBranch {
				result.Checks = append(result.Checks, warnCheck("production branch", branch+" differs from "+resolved.Project.Vercel.ProductionBranch))
			}
		}
	case "supabase-db-push":
		if optionalSecret(resolved.Identity.ID, keychain.SupabaseTokenSecret) == "" {
			result.Checks = append(result.Checks, blockCheck("Supabase token", "missing for "+resolved.Identity.ID))
		} else {
			result.Checks = append(result.Checks, okCheck("Supabase token", "present for "+resolved.Identity.ID))
		}
		if resolved.Project.Supabase.ProjectRef == "" {
			result.Checks = append(result.Checks, skipCheck("Supabase project ref", "identity-level token"))
		} else {
			result.Checks = append(result.Checks, okCheck("Supabase project ref", resolved.Project.Supabase.ProjectRef))
			result.Checks = append(result.Checks, linkedSupabaseRefCheck(resolved.Project))
		}
	default:
		result.Checks = append(result.Checks, blockCheck("action", "unsupported"))
	}
	if needsConfirm(resolved.Project.Safety.RequireConfirm, action) && !confirmed {
		result.Checks = append(result.Checks, blockCheck("confirmation", "pass --yes after review"))
	}
	result.Allowed = !hasBlocked(result.Checks)
	return result
}

func gitAuthorChecks(path string, identity config.Identity) []CheckResult {
	name, nameErr := gitOutput(path, "config", "--local", "--get", "user.name")
	email, emailErr := gitOutput(path, "config", "--local", "--get", "user.email")
	checks := []CheckResult{}
	if nameErr != nil || name != identity.Git.Name {
		checks = append(checks, blockCheck("git user.name", "expected "+identity.Git.Name))
	} else {
		checks = append(checks, okCheck("git user.name", name))
	}
	if emailErr != nil || email != identity.Git.Email {
		checks = append(checks, blockCheck("git user.email", "expected "+identity.Git.Email))
	} else {
		checks = append(checks, okCheck("git user.email", email))
	}
	return checks
}

func remoteCheck(project config.Project) []CheckResult {
	remote, err := gitOutput(project.Path, "remote", "get-url", "origin")
	if err != nil {
		return []CheckResult{blockCheck("origin", "missing")}
	}
	if project.GitHub.RemoteSSH != "" && remote != project.GitHub.RemoteSSH {
		return []CheckResult{blockCheck("origin", "expected "+project.GitHub.RemoteSSH)}
	}
	return []CheckResult{okCheck("origin", remote)}
}

func sshChecks(identity config.Identity) []CheckResult {
	checks := []CheckResult{}
	if identity.GitHub.SSHKeyPath == "" {
		return []CheckResult{blockCheck("SSH key", "missing")}
	}
	info, err := os.Stat(identity.GitHub.SSHKeyPath)
	if err != nil {
		checks = append(checks, blockCheck("SSH key", err.Error()))
	} else if info.Mode().Perm()&0077 != 0 {
		checks = append(checks, warnCheck("SSH key permissions", info.Mode().Perm().String()))
	} else {
		checks = append(checks, okCheck("SSH key", identity.GitHub.SSHKeyPath))
	}
	configPath, err := ssh.GetSSHConfigPath()
	if err != nil {
		return append(checks, warnCheck("SSH config", err.Error()))
	}
	data, err := os.ReadFile(configPath)
	if err != nil || !strings.Contains(string(data), "Host "+identity.GitHub.HostAlias) {
		checks = append(checks, warnCheck("SSH host alias", identity.GitHub.HostAlias+" not found"))
	} else {
		checks = append(checks, okCheck("SSH host alias", identity.GitHub.HostAlias))
	}
	return checks
}

func vercelDoctorChecks(project config.Project, identity config.Identity) []CheckResult {
	if optionalSecret(identity.ID, keychain.VercelTokenSecret) == "" {
		return []CheckResult{warnCheck("Vercel token", "missing for "+identity.ID)}
	}
	checks := []CheckResult{okCheck("Vercel token", "present for "+identity.ID)}
	if project.Vercel.Mode != "" {
		checks = append(checks, okCheck("Vercel mode", project.Vercel.Mode))
	} else {
		checks = append(checks, skipCheck("Vercel mode", "identity-level token"))
	}
	return checks
}

func supabaseDoctorChecks(project config.Project, identity config.Identity) []CheckResult {
	if optionalSecret(identity.ID, keychain.SupabaseTokenSecret) == "" {
		return []CheckResult{warnCheck("Supabase token", "missing for "+identity.ID)}
	}
	checks := []CheckResult{okCheck("Supabase token", "present for "+identity.ID)}
	if project.Supabase.ProjectRef != "" {
		checks = append(checks, okCheck("Supabase project ref", project.Supabase.ProjectRef))
	} else {
		checks = append(checks, skipCheck("Supabase project ref", "identity-level token"))
	}
	return checks
}

func linkedSupabaseRefCheck(project config.Project) CheckResult {
	linkedPath := filepath.Join(project.Path, "supabase", ".temp", "project-ref")
	data, err := os.ReadFile(linkedPath)
	if err != nil {
		return skipCheck("Supabase linked ref", "not discoverable")
	}
	ref := strings.TrimSpace(string(data))
	if ref != project.Supabase.ProjectRef {
		return blockCheck("Supabase linked ref", "expected "+project.Supabase.ProjectRef)
	}
	return okCheck("Supabase linked ref", ref)
}

func currentBranch(path string) string {
	branch, err := gitOutput(path, "branch", "--show-current")
	if err != nil {
		return ""
	}
	return branch
}

func pathCheck(name, path string) CheckResult {
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return blockCheck(name, "missing "+path)
	}
	return okCheck(name, path)
}

func currentDirCheck(projectRoot string) CheckResult {
	cwd, err := os.Getwd()
	if err != nil {
		return blockCheck("current directory", err.Error())
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return blockCheck("current directory", err.Error())
	}
	rel, err := filepath.Rel(root, cwd)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return blockCheck("current directory", "run from "+root+" or a child directory")
	}
	return okCheck("current directory", cwd)
}

func needsConfirm(actions []string, action string) bool {
	for _, configured := range actions {
		if configured == action {
			return true
		}
	}
	return false
}

func protected(branches []string, branch string) bool {
	for _, protectedBranch := range branches {
		if protectedBranch == branch {
			return true
		}
	}
	return false
}

func hasBlocked(checks []CheckResult) bool {
	for _, check := range checks {
		if check.Status == "BLOCK" {
			return true
		}
	}
	return false
}

func okCheck(name, message string) CheckResult {
	return CheckResult{Name: name, Status: "OK", Message: message}
}

func warnCheck(name, message string) CheckResult {
	return CheckResult{Name: name, Status: "WARN", Message: message}
}

func skipCheck(name, message string) CheckResult {
	return CheckResult{Name: name, Status: "SKIP", Message: message}
}

func blockCheck(name, message string) CheckResult {
	return CheckResult{Name: name, Status: "BLOCK", Message: message}
}

func printChecks(title string, checks []CheckResult) {
	fmt.Fprintln(os.Stdout, title)
	for _, check := range checks {
		fmt.Fprintf(os.Stdout, "%-24s %-5s %s\n", check.Name+":", check.Status, check.Message)
	}
}

func nonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// keep bufio linked in command file for future interactive guard confirmation.
var _ = bufio.NewReader
