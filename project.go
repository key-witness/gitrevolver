package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/key-witness/gitrevolver/internal/config"
	"github.com/key-witness/gitrevolver/internal/ssh"
	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage registered projects",
}

var projectAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Register a project path and identity",
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(os.Stdin)
		slug, err := promptRequired(reader, "Project slug")
		if err != nil {
			return err
		}
		if err := config.ValidateSlug("project", slug); err != nil {
			return err
		}
		return addProjectPrompt(reader, slug, "")
	},
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := config.LoadProjects()
		if err != nil {
			return err
		}
		slugs := make([]string, 0, len(store.Projects))
		for slug := range store.Projects {
			slugs = append(slugs, slug)
		}
		sort.Strings(slugs)
		for _, slug := range slugs {
			project := store.Projects[slug]
			fmt.Fprintf(os.Stdout, "%s\t%s\t%s\t%s\n", slug, project.Identity, project.Name, project.Path)
		}
		if len(slugs) == 0 {
			fmt.Fprintln(os.Stdout, "No projects registered. Run `gitrevolver project add` or `gitrevolver project scan`.")
		}
		return nil
	},
}

var projectShowJSON bool

var projectShowCmd = &cobra.Command{
	Use:   "show <project>",
	Short: "Show one registered project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := config.FindProject(args[0])
		if err != nil {
			return err
		}
		if projectShowJSON {
			return writeJSON(os.Stdout, project)
		}
		fmt.Fprintf(os.Stdout, "Project: %s\nName: %s\nAliases: %s\nIdentity: %s\nPath: %s\nGitHub: %s/%s\nVercel: %s %s\nSupabase: %s\n",
			project.Slug, project.Name, strings.Join(project.Aliases, ", "), project.Identity, project.Path, project.GitHub.Owner, project.GitHub.Repo,
			project.Vercel.Mode, project.Vercel.ProjectID, project.Supabase.ProjectRef)
		return nil
	},
}

var projectScanCmd = &cobra.Command{
	Use:   "scan [path]",
	Short: "Scan directories for GitHub projects and offer to register them",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := ""
		if len(args) == 1 {
			root = args[0]
		} else {
			lib, err := config.LoadLibraryConfig()
			if err != nil {
				return err
			}
			root = lib.DefaultProjectsDir
		}
		root, err := expandPath(root)
		if err != nil {
			return err
		}
		reader := bufio.NewReader(os.Stdin)
		return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if !entry.IsDir() {
				return nil
			}
			if entry.Name() == "node_modules" || entry.Name() == ".cache" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			if entry.Name() != ".git" {
				return nil
			}
			repoPath := filepath.Dir(path)
			remote, err := gitOutput(repoPath, "remote", "get-url", "origin")
			if err != nil {
				return filepath.SkipDir
			}
			owner, repo, host, ok := parseGitHubRemote(remote)
			if !ok {
				return filepath.SkipDir
			}
			identity := identityForHost(host)
			fmt.Fprintf(os.Stdout, "Found %s (%s/%s via %s)\n", repoPath, owner, repo, host)
			if identity != "" {
				fmt.Fprintf(os.Stdout, "Matched identity: %s\n", identity)
			}
			if yes(reader, "Register this project?", identity != "") {
				slug, err := promptDefault(reader, "Project slug", repo)
				if err != nil {
					return err
				}
				if err := config.ValidateSlug("project", slug); err != nil {
					return err
				}
				if err := addScannedProject(reader, slug, repoPath, identity, owner, repo, remote); err != nil {
					return err
				}
			}
			return filepath.SkipDir
		})
	},
}

var projectBindIdentity string

var projectBindCmd = &cobra.Command{
	Use:   "bind <project>",
	Short: "Bind a registered repo to its identity SSH alias and local Git author",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := config.FindProject(args[0])
		if err != nil {
			return err
		}
		if projectBindIdentity != "" {
			project.Identity = projectBindIdentity
		}
		if err := bindProject(project); err != nil {
			return err
		}
		if err := config.UpsertProject(*project); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Bound %s to %s\n", project.Slug, project.Identity)
		return nil
	},
}

var (
	projectVercelMode       string
	projectVercelOrgID      string
	projectVercelProjectID  string
	projectVercelProdBranch string
)

var projectLinkVercelCmd = &cobra.Command{
	Use:   "link-vercel <project>",
	Short: "Attach Vercel metadata to a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := config.FindProject(args[0])
		if err != nil {
			return err
		}
		project.Vercel.Mode = projectVercelMode
		project.Vercel.OrgID = projectVercelOrgID
		project.Vercel.ProjectID = projectVercelProjectID
		project.Vercel.ProductionBranch = projectVercelProdBranch
		return config.UpsertProject(*project)
	},
}

var projectSupabaseRef string

var projectLinkSupabaseCmd = &cobra.Command{
	Use:   "link-supabase <project>",
	Short: "Attach a Supabase project ref to a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(projectSupabaseRef) == "" {
			return fmt.Errorf("--project-ref is required")
		}
		project, err := config.FindProject(args[0])
		if err != nil {
			return err
		}
		project.Supabase.ProjectRef = projectSupabaseRef
		return config.UpsertProject(*project)
	},
}

var projectAliasCmd = &cobra.Command{
	Use:   "alias <project> <alias...>",
	Short: "Attach local lookup aliases such as hosted domains to a project",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := config.FindProject(args[0])
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		aliases := []string{}
		for _, alias := range project.Aliases {
			normalized := normalizeProjectAlias(alias)
			if normalized == "" || seen[normalized] {
				continue
			}
			seen[normalized] = true
			aliases = append(aliases, alias)
		}
		for _, alias := range args[1:] {
			alias = strings.TrimSpace(alias)
			normalized := normalizeProjectAlias(alias)
			if normalized == "" || seen[normalized] {
				continue
			}
			seen[normalized] = true
			aliases = append(aliases, alias)
		}
		project.Aliases = aliases
		if err := config.UpsertProject(*project); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Aliases for %s: %s\n", project.Slug, strings.Join(project.Aliases, ", "))
		return nil
	},
}

func init() {
	projectShowCmd.Flags().BoolVar(&projectShowJSON, "json", false, "Render machine-readable output")
	projectBindCmd.Flags().StringVar(&projectBindIdentity, "identity", "", "Override the project's identity before binding")
	projectLinkVercelCmd.Flags().StringVar(&projectVercelMode, "mode", "github-auto-deploy", "Vercel mode")
	projectLinkVercelCmd.Flags().StringVar(&projectVercelOrgID, "org-id", "", "Vercel organization or team ID")
	projectLinkVercelCmd.Flags().StringVar(&projectVercelProjectID, "project-id", "", "Vercel project ID")
	projectLinkVercelCmd.Flags().StringVar(&projectVercelProdBranch, "production-branch", "main", "Branch allowed for production policy")
	projectLinkSupabaseCmd.Flags().StringVar(&projectSupabaseRef, "project-ref", "", "Supabase project ref")
	projectCmd.AddCommand(projectAddCmd, projectListCmd, projectShowCmd, projectScanCmd, projectBindCmd, projectAliasCmd, projectLinkVercelCmd, projectLinkSupabaseCmd)
	rootCmd.AddCommand(projectCmd)
}

func addProjectPrompt(reader *bufio.Reader, slug, discoveredPath string) error {
	name, err := promptDefault(reader, "Project name", slug)
	if err != nil {
		return err
	}
	identity, err := promptRequired(reader, "Identity slug")
	if err != nil {
		return err
	}
	path := discoveredPath
	if path == "" {
		path, err = promptRequired(reader, "Project path")
		if err != nil {
			return err
		}
	}
	path, err = expandPath(path)
	if err != nil {
		return err
	}
	if _, err := config.FindIdentityByAlias(identity); err != nil {
		return err
	}
	project := config.Project{
		Slug:     slug,
		Name:     name,
		Identity: identity,
		Path:     path,
		Safety: config.SafetyPolicy{
			ProtectedBranches: []string{"main"},
			RequireConfirm:    []string{"git-push", "vercel-prod-deploy", "supabase-db-push"},
		},
	}
	if remote, err := gitOutput(path, "remote", "get-url", "origin"); err == nil {
		project.GitHub = projectGitHub(remote, identity)
	}
	return config.UpsertProject(project)
}

func addScannedProject(reader *bufio.Reader, slug, path, identity, owner, repo, remote string) error {
	if identity == "" {
		var err error
		identity, err = promptRequired(reader, "Identity slug")
		if err != nil {
			return err
		}
	}
	if _, err := config.FindIdentityByAlias(identity); err != nil {
		return err
	}
	name, err := promptDefault(reader, "Project name", repo)
	if err != nil {
		return err
	}
	project := config.Project{
		Slug:     slug,
		Name:     name,
		Identity: identity,
		Path:     path,
		GitHub:   config.ProjectGitHub{Owner: owner, Repo: repo, RemoteSSH: projectGitHub(remote, identity).RemoteSSH},
		Safety: config.SafetyPolicy{
			ProtectedBranches: []string{"main"},
			RequireConfirm:    []string{"git-push", "vercel-prod-deploy", "supabase-db-push"},
		},
	}
	return config.UpsertProject(project)
}

func bindProject(project *config.Project) error {
	identity, err := config.FindIdentityByAlias(project.Identity)
	if err != nil {
		return err
	}
	path, err := expandPath(project.Path)
	if err != nil {
		return err
	}
	if err := gitRun(path, "rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("%s is not a git repo", path)
	}
	if err := gitRun(path, "config", "--local", "user.name", identity.Git.Name); err != nil {
		return err
	}
	if err := gitRun(path, "config", "--local", "user.email", identity.Git.Email); err != nil {
		return err
	}
	if err := gitRun(path, "config", "--local", "gitrevolver.bound", project.Identity); err != nil {
		return err
	}
	if identity.GitHub.HostAlias != "" && identity.GitHub.SSHKeyPath != "" {
		if err := ssh.AddSSHConfigEntry(identity.GitHub.HostAlias, identity.GitHub.SSHKeyPath); err != nil {
			return err
		}
	}
	remote, err := gitOutput(path, "remote", "get-url", "origin")
	if err != nil {
		return err
	}
	owner, repo, _, ok := parseGitHubRemote(remote)
	if !ok {
		return fmt.Errorf("origin is not a supported GitHub remote: %s", remote)
	}
	project.GitHub.Owner = owner
	project.GitHub.Repo = repo
	project.GitHub.RemoteSSH = remoteForIdentity(identity, owner, repo)
	if remote != project.GitHub.RemoteSSH {
		if err := gitRun(path, "remote", "set-url", "origin", project.GitHub.RemoteSSH); err != nil {
			return err
		}
	}
	project.Path = path
	return nil
}

func projectGitHub(remote, identitySlug string) config.ProjectGitHub {
	owner, repo, _, ok := parseGitHubRemote(remote)
	if !ok {
		return config.ProjectGitHub{}
	}
	if identity, err := config.FindIdentityByAlias(identitySlug); err == nil {
		return config.ProjectGitHub{Owner: owner, Repo: repo, RemoteSSH: remoteForIdentity(identity, owner, repo)}
	}
	return config.ProjectGitHub{Owner: owner, Repo: repo, RemoteSSH: remote}
}

func identityForHost(host string) string {
	store, err := config.LoadConfig()
	if err != nil {
		return ""
	}
	for slug, identity := range store.Identities {
		if host == identity.GitHub.HostAlias {
			return slug
		}
	}
	return ""
}

func remoteForIdentity(identity *config.Identity, owner, repo string) string {
	host := identity.GitHub.HostAlias
	if host == "" {
		host = "github.com"
	}
	return fmt.Sprintf("git@%s:%s/%s.git", host, owner, repo)
}

func gitOutput(path string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", path}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitRun(path string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", path}, args...)...)
	return cmd.Run()
}
