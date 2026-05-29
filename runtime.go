package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/key-witness/gitrevolver/internal/config"
	"github.com/key-witness/gitrevolver/internal/keychain"
	"github.com/spf13/cobra"
)

type ResolvedProject struct {
	Project  config.Project  `json:"project"`
	Identity config.Identity `json:"identity"`
}

var resolveJSON bool

var resolveCmd = &cobra.Command{
	Use:   "resolve <project-or-path>",
	Short: "Resolve a project slug, name, or local path",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		resolved, err := resolveProject(args[0])
		if err != nil {
			return err
		}
		if resolveJSON {
			return writeJSON(os.Stdout, resolved)
		}
		fmt.Fprintf(os.Stdout, "%s\t%s\t%s\n", resolved.Project.Slug, resolved.Identity.ID, resolved.Project.Path)
		return nil
	},
}

var envFormat string
var envIncludeSecrets bool

var envCmd = &cobra.Command{
	Use:   "env <project-or-path>",
	Short: "Emit project-scoped child-process environment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		resolved, err := resolveProject(args[0])
		if err != nil {
			return err
		}
		env := printableRuntimeEnv(resolved, envIncludeSecrets)
		if envFormat == "json" {
			return json.NewEncoder(os.Stdout).Encode(env)
		}
		for _, key := range sortedEnvKeys(env) {
			fmt.Fprintf(os.Stdout, "export %s=%s\n", key, shellQuote(env[key]))
		}
		return nil
	},
}

var shellCmd = &cobra.Command{
	Use:   "shell <project-or-path>",
	Short: "Start an interactive shell with project-scoped environment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/zsh"
		}
		return execProject(args[0], []string{shell})
	},
}

var execCmd = &cobra.Command{
	Use:   "exec <project-or-path> -- <command...>",
	Short: "Run a command with project-scoped credentials and SSH context",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return execProject(args[0], args[1:])
	},
}

func init() {
	resolveCmd.Flags().BoolVar(&resolveJSON, "json", false, "Render machine-readable output")
	envCmd.Flags().StringVar(&envFormat, "format", "shell", "Output format: shell or json")
	envCmd.Flags().BoolVar(&envIncludeSecrets, "include-secrets", false, "Include platform-stored tokens in explicit env output")
	rootCmd.AddCommand(resolveCmd, envCmd, shellCmd, execCmd)
}

func resolveProject(input string) (*ResolvedProject, error) {
	store, err := config.LoadProjects()
	if err != nil {
		return nil, err
	}
	if project, ok := store.Projects[input]; ok {
		return resolvedFor(project, input)
	}
	if project, ok := projectForAlias(store, input); ok {
		return resolvedFor(project, project.Slug)
	}
	if path, err := expandPath(input); err == nil {
		if match, ok := projectForPath(store, path); ok {
			return resolvedFor(match, match.Slug)
		}
	}
	needle := strings.ToLower(strings.TrimSpace(input))
	matches := []config.Project{}
	for slug, project := range store.Projects {
		project.Slug = slug
		if strings.Contains(strings.ToLower(project.Name), needle) ||
			strings.Contains(strings.ToLower(slug), needle) ||
			projectHasFuzzyAlias(project, needle) {
			matches = append(matches, project)
		}
	}
	if len(matches) == 1 {
		return resolvedFor(matches[0], matches[0].Slug)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("project %q is ambiguous", input)
	}
	return nil, fmt.Errorf("project %q not found", input)
}

func projectForAlias(store *config.ProjectStore, input string) (config.Project, bool) {
	needle := normalizeProjectAlias(input)
	if needle == "" {
		return config.Project{}, false
	}
	for slug, project := range store.Projects {
		project.Slug = slug
		for _, alias := range project.Aliases {
			if normalizeProjectAlias(alias) == needle {
				return project, true
			}
		}
	}
	return config.Project{}, false
}

func projectHasFuzzyAlias(project config.Project, needle string) bool {
	for _, alias := range project.Aliases {
		if strings.Contains(strings.ToLower(alias), needle) {
			return true
		}
	}
	return false
}

func normalizeProjectAlias(input string) string {
	alias := strings.ToLower(strings.TrimSpace(input))
	alias = strings.TrimPrefix(alias, "https://")
	alias = strings.TrimPrefix(alias, "http://")
	alias = strings.TrimPrefix(alias, "www.")
	if index := strings.Index(alias, "/"); index >= 0 {
		alias = alias[:index]
	}
	return strings.TrimSuffix(alias, ".")
}

func resolvedFor(project config.Project, slug string) (*ResolvedProject, error) {
	project.Slug = slug
	path, err := expandPath(project.Path)
	if err != nil {
		return nil, err
	}
	project.Path = path
	identity, err := config.FindIdentityByAlias(project.Identity)
	if err != nil {
		return nil, err
	}
	return &ResolvedProject{Project: project, Identity: *identity}, nil
}

func projectForPath(store *config.ProjectStore, path string) (config.Project, bool) {
	path, err := filepath.Abs(path)
	if err != nil {
		return config.Project{}, false
	}
	var best config.Project
	bestLen := -1
	for slug, project := range store.Projects {
		root, err := expandPath(project.Path)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if len(root) > bestLen {
				project.Slug = slug
				best = project
				bestLen = len(root)
			}
		}
	}
	return best, bestLen >= 0
}

func printableRuntimeEnv(resolved *ResolvedProject, includeSecrets bool) map[string]string {
	return buildRuntimeEnv(resolved, includeSecrets)
}

func runtimeEnv(resolved *ResolvedProject) map[string]string {
	return buildRuntimeEnv(resolved, true)
}

func buildRuntimeEnv(resolved *ResolvedProject, includeSecrets bool) map[string]string {
	env := map[string]string{
		"GITREVOLVER_IDENTITY": resolved.Identity.ID,
		"GITREVOLVER_PROJECT":  resolved.Project.Slug,
		"GITHUB_OWNER":         resolved.Project.GitHub.Owner,
		"GITHUB_REPO":          resolved.Project.GitHub.Repo,
	}
	if includeSecrets {
		if token := optionalSecret(resolved.Identity.ID, keychain.GitHubTokenSecret); token != "" {
			env["GH_TOKEN"] = token
			env["GITHUB_TOKEN"] = token
		}
	}
	if resolved.Project.Vercel.Mode != "" {
		env["VERCEL_MODE"] = resolved.Project.Vercel.Mode
	}
	if resolved.Project.Vercel.OrgID != "" {
		env["VERCEL_ORG_ID"] = resolved.Project.Vercel.OrgID
	}
	if resolved.Project.Vercel.ProjectID != "" {
		env["VERCEL_PROJECT_ID"] = resolved.Project.Vercel.ProjectID
	}
	if includeSecrets {
		if token := optionalSecret(resolved.Identity.ID, keychain.VercelTokenSecret); token != "" {
			env["VERCEL_TOKEN"] = token
		}
	}
	if resolved.Project.Supabase.ProjectRef != "" {
		env["SUPABASE_PROJECT_REF"] = resolved.Project.Supabase.ProjectRef
	}
	if includeSecrets {
		if token := optionalSecret(resolved.Identity.ID, keychain.SupabaseTokenSecret); token != "" {
			env["SUPABASE_ACCESS_TOKEN"] = token
		}
	}
	if resolved.Identity.GitHub.SSHKeyPath != "" {
		env["GIT_SSH_COMMAND"] = fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes", shellSingleQuote(resolved.Identity.GitHub.SSHKeyPath))
	}
	return compactEnv(env)
}

var getSecret = keychain.GetSecret

func optionalSecret(identity, name string) string {
	token, err := getSecret(identity, name)
	if err != nil {
		return ""
	}
	return token
}

func execProject(input string, command []string) error {
	resolved, err := resolveProject(input)
	if err != nil {
		return err
	}
	if len(command) == 0 {
		return fmt.Errorf("child command is required")
	}
	child := exec.Command(command[0], command[1:]...)
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Env = mergeEnv(os.Environ(), runtimeEnv(resolved))
	child.Dir = execDir(resolved.Project.Path)
	return child.Run()
}

func execDir(projectRoot string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return projectRoot
	}
	if rel, err := filepath.Rel(projectRoot, cwd); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return cwd
	}
	return projectRoot
}

func mergeEnv(base []string, injected map[string]string) []string {
	merged := map[string]string{}
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			merged[key] = value
		}
	}
	for key, value := range injected {
		merged[key] = value
	}
	keys := sortedEnvKeys(merged)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+merged[key])
	}
	return result
}

func compactEnv(env map[string]string) map[string]string {
	for key, value := range env {
		if value == "" {
			delete(env, key)
		}
	}
	return env
}

func sortedEnvKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func shellQuote(value string) string {
	return strconv.Quote(value)
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func expandPath(path string) (string, error) {
	if path == "." || path == "" {
		return filepath.Abs(path)
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return filepath.Abs(path)
}

func parseGitHubRemote(remote string) (owner, repo, host string, ok bool) {
	remote = strings.TrimSpace(remote)
	var path string
	switch {
	case strings.HasPrefix(remote, "git@"):
		rest := strings.TrimPrefix(remote, "git@")
		var found bool
		host, path, found = strings.Cut(rest, ":")
		if !found {
			return "", "", "", false
		}
	case strings.HasPrefix(remote, "ssh://git@"):
		rest := strings.TrimPrefix(remote, "ssh://git@")
		var found bool
		host, path, found = strings.Cut(rest, "/")
		if !found {
			return "", "", "", false
		}
	case strings.HasPrefix(remote, "https://github.com/"):
		host = "github.com"
		path = strings.TrimPrefix(remote, "https://github.com/")
	default:
		return "", "", "", false
	}
	if host != "github.com" && !strings.HasPrefix(host, "github") {
		return "", "", "", false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", false
	}
	return parts[0], parts[1], host, true
}
