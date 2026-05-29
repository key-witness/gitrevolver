package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/key-witness/gitrevolver/internal/config"
	"github.com/key-witness/gitrevolver/internal/keychain"
)

func TestParseGitHubRemote(t *testing.T) {
	tests := []struct {
		remote string
		owner  string
		repo   string
		host   string
	}{
		{"git@github-client:client/app.git", "client", "app", "github-client"},
		{"git@github.com:owner/repo.git", "owner", "repo", "github.com"},
		{"https://github.com/openai/codex.git", "openai", "codex", "github.com"},
		{"ssh://git@github-personal/me/site.git", "me", "site", "github-personal"},
	}
	for _, test := range tests {
		owner, repo, host, ok := parseGitHubRemote(test.remote)
		if !ok || owner != test.owner || repo != test.repo || host != test.host {
			t.Fatalf("parseGitHubRemote(%q) = %q %q %q %v", test.remote, owner, repo, host, ok)
		}
	}
	if _, _, _, ok := parseGitHubRemote("git@gitlab.com:owner/repo.git"); ok {
		t.Fatal("expected non-GitHub remote to fail")
	}
}

func TestResolveProjectByDomainAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := config.InitLibrary(filepath.Join(home, "code")); err != nil {
		t.Fatal(err)
	}
	if err := config.AddIdentity(config.Identity{ID: "client"}); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(home, "code", "app")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.UpsertProject(config.Project{
		Slug:     "client-app",
		Name:     "Client App",
		Aliases:  []string{"client.example", "client-app.example"},
		Identity: "client",
		Path:     projectPath,
	}); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveProject("https://www.client.example/portfolio")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Project.Slug != "client-app" || resolved.Project.Path != projectPath {
		t.Fatalf("unexpected resolved project: %#v", resolved.Project)
	}
}

func TestRuntimeEnvInjectsOptionalSecrets(t *testing.T) {
	originalGetSecret := getSecret
	getSecret = func(identity, name string) (string, error) {
		return map[string]string{
			keychain.GitHubTokenSecret:   "github-secret",
			keychain.VercelTokenSecret:   "vercel-secret",
			keychain.SupabaseTokenSecret: "supabase-secret",
		}[name], nil
	}
	defer func() { getSecret = originalGetSecret }()

	env := runtimeEnv(&ResolvedProject{
		Project: config.Project{
			Slug:     "client-app",
			GitHub:   config.ProjectGitHub{Owner: "client", Repo: "app"},
			Vercel:   config.ProjectVercel{Mode: "token-cli", OrgID: "team", ProjectID: "project"},
			Supabase: config.ProjectSupabase{ProjectRef: "ref"},
		},
		Identity: config.Identity{
			ID:     "client",
			GitHub: config.GitHubIdentity{SSHKeyPath: "/tmp/client-key"},
		},
	})
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "VERCEL_TOKEN", "SUPABASE_ACCESS_TOKEN", "GIT_SSH_COMMAND"} {
		if env[key] == "" {
			t.Fatalf("expected %s to be set", key)
		}
	}
	if env["GITREVOLVER_PROJECT"] != "client-app" || env["GITHUB_OWNER"] != "client" {
		t.Fatalf("unexpected env map: %#v", env)
	}
}

func TestRuntimeEnvQuotesSSHKeyPath(t *testing.T) {
	env := runtimeEnv(&ResolvedProject{
		Project: config.Project{Slug: "client-app"},
		Identity: config.Identity{
			ID:     "client",
			GitHub: config.GitHubIdentity{SSHKeyPath: "/tmp/client key's/id_ed25519"},
		},
	})
	want := "ssh -i '/tmp/client key'\\''s/id_ed25519' -o IdentitiesOnly=yes"
	if env["GIT_SSH_COMMAND"] != want {
		t.Fatalf("GIT_SSH_COMMAND = %q, want %q", env["GIT_SSH_COMMAND"], want)
	}
}

func TestPrintableRuntimeEnvRequiresExplicitSecrets(t *testing.T) {
	originalGetSecret := getSecret
	getSecret = func(identity, name string) (string, error) {
		return "secret-" + name, nil
	}
	defer func() { getSecret = originalGetSecret }()

	resolved := &ResolvedProject{
		Project: config.Project{
			Slug:     "client-app",
			GitHub:   config.ProjectGitHub{Owner: "client", Repo: "app"},
			Vercel:   config.ProjectVercel{ProjectID: "project"},
			Supabase: config.ProjectSupabase{ProjectRef: "ref"},
		},
		Identity: config.Identity{ID: "client"},
	}
	withoutSecrets := printableRuntimeEnv(resolved, false)
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "VERCEL_TOKEN", "SUPABASE_ACCESS_TOKEN"} {
		if _, ok := withoutSecrets[key]; ok {
			t.Fatalf("default printable env included %s", key)
		}
	}
	withSecrets := printableRuntimeEnv(resolved, true)
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "VERCEL_TOKEN", "SUPABASE_ACCESS_TOKEN"} {
		if withSecrets[key] == "" {
			t.Fatalf("printable env with secrets omitted %s", key)
		}
	}
}

func TestResolveAndBindTempRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := config.InitLibrary(filepath.Join(home, "code")); err != nil {
		t.Fatal(err)
	}
	identity := config.Identity{
		ID:          "client",
		DisplayName: "Client",
		Git:         config.GitIdentity{Name: "Client Dev", Email: "dev@client.example"},
		GitHub:      config.GitHubIdentity{Username: "client-gh", HostAlias: "github-client"},
	}
	if err := config.AddIdentity(identity); err != nil {
		t.Fatal(err)
	}
	repoPath := filepath.Join(home, "code", "app")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "init")
	runGit(t, repoPath, "remote", "add", "origin", "git@github.com:client/app.git")
	project := config.Project{
		Slug:     "client-app",
		Name:     "Client App",
		Identity: "client",
		Path:     repoPath,
		GitHub:   config.ProjectGitHub{Owner: "client", Repo: "app"},
	}
	if err := config.UpsertProject(project); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveProject(filepath.Join(repoPath, "nested"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Project.Slug != "client-app" {
		t.Fatalf("resolved slug = %s", resolved.Project.Slug)
	}
	if err := bindProject(&project); err != nil {
		t.Fatal(err)
	}
	if got := gitText(t, repoPath, "config", "--local", "--get", "user.email"); got != identity.Git.Email {
		t.Fatalf("bound email = %q", got)
	}
	if got := gitText(t, repoPath, "remote", "get-url", "origin"); got != "git@github-client:client/app.git" {
		t.Fatalf("bound remote = %q", got)
	}
}

func TestExecProjectDoesNotRestoreBranch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := config.InitLibrary(filepath.Join(home, "code")); err != nil {
		t.Fatal(err)
	}
	if err := config.AddIdentity(config.Identity{ID: "client"}); err != nil {
		t.Fatal(err)
	}
	repoPath := filepath.Join(home, "code", "app")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "init")
	runGit(t, repoPath, "branch", "-M", "main")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "init")
	runGit(t, repoPath, "switch", "-c", "weekly")
	runGit(t, repoPath, "switch", "main")
	if err := config.UpsertProject(config.Project{
		Slug:     "client-app",
		Name:     "Client App",
		Identity: "client",
		Path:     repoPath,
	}); err != nil {
		t.Fatal(err)
	}

	if err := execProject("client-app", []string{"git", "switch", "weekly"}); err != nil {
		t.Fatal(err)
	}
	if got := gitText(t, repoPath, "branch", "--show-current"); got != "weekly" {
		t.Fatalf("execProject restored branch, got %q", got)
	}
}

func TestInstallHookCallsPushGuard(t *testing.T) {
	repoPath := t.TempDir()
	runGit(t, repoPath, "init")
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)
	if err := os.Chdir(repoPath); err != nil {
		t.Fatal(err)
	}
	if err := installHook(); err != nil {
		t.Fatal(err)
	}
	hook, err := os.ReadFile(filepath.Join(repoPath, ".git", "hooks", "pre-push"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hook), "gitrevolver agent guard . git-push --yes") {
		t.Fatalf("pre-push hook does not call GitRevolver guard:\n%s", hook)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func gitText(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}
