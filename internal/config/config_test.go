package config

import (
	"os"
	"testing"
)

func TestConfigDir(t *testing.T) {
	dir, err := GetConfigDir()
	if err != nil {
		t.Fatalf("GetConfigDir() error = %v", err)
	}
	if dir == "" {
		t.Fatal("GetConfigDir() returned empty directory")
	}
}

func TestSaveAndLoadIdentities(t *testing.T) {
	withConfigDir(t, func() {
		store := &IdentityStore{
			Version: Version,
			Identities: map[string]Identity{
				"test": {
					DisplayName: "Test",
					Git:         GitIdentity{Name: "Test User", Email: "test@example.com"},
					GitHub:      GitHubIdentity{Username: "testuser", HostAlias: "github-test", SSHKeyPath: "~/.ssh/test"},
				},
			},
		}
		if err := SaveConfig(store); err != nil {
			t.Fatalf("SaveConfig() error = %v", err)
		}
		loaded, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if got := loaded.Identities["test"].Git.Email; got != "test@example.com" {
			t.Fatalf("Git.Email = %q", got)
		}
		if got := loaded.Identities["test"].ID; got != "test" {
			t.Fatalf("identity ID = %q", got)
		}
	})
}

func TestAddAndRemoveIdentity(t *testing.T) {
	withConfigDir(t, func() {
		identity := Identity{
			ID:          "work",
			DisplayName: "Work",
			Git:         GitIdentity{Name: "Work User", Email: "work@example.com"},
			GitHub:      GitHubIdentity{Username: "workuser"},
		}
		if err := AddIdentity(identity); err != nil {
			t.Fatalf("AddIdentity() error = %v", err)
		}
		found, err := FindIdentityByAlias("work")
		if err != nil {
			t.Fatalf("FindIdentityByAlias() error = %v", err)
		}
		if found.Git.Name != "Work User" {
			t.Fatalf("Git.Name = %q", found.Git.Name)
		}
		if err := RemoveIdentity("work"); err != nil {
			t.Fatalf("RemoveIdentity() error = %v", err)
		}
		if _, err := FindIdentityByAlias("work"); err == nil {
			t.Fatal("expected removed identity lookup to fail")
		}
	})
}

func TestValidateSlugRejectsUnsafeCharacters(t *testing.T) {
	for _, slug := range []string{"client app", "client/app", "client\nHost injected", ""} {
		if err := ValidateSlug("identity", slug); err == nil {
			t.Fatalf("ValidateSlug(%q) returned nil", slug)
		}
	}
	for _, slug := range []string{"client-a", "client_a", "client.a"} {
		if err := ValidateSlug("identity", slug); err != nil {
			t.Fatalf("ValidateSlug(%q) error = %v", slug, err)
		}
	}
}

func TestSaveAndLoadProjects(t *testing.T) {
	withConfigDir(t, func() {
		project := Project{
			Slug:     "client-app",
			Name:     "Client App",
			Identity: "work",
			Path:     "~/code/client/app",
			GitHub:   ProjectGitHub{Owner: "client", Repo: "app"},
		}
		if err := UpsertProject(project); err != nil {
			t.Fatalf("UpsertProject() error = %v", err)
		}
		found, err := FindProject("client-app")
		if err != nil {
			t.Fatalf("FindProject() error = %v", err)
		}
		if found.GitHub.Owner != "client" {
			t.Fatalf("GitHub.Owner = %q", found.GitHub.Owner)
		}
	})
}

func withConfigDir(t *testing.T, run func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "gitrevolver-config-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	original := getConfigDirFunc
	getConfigDirFunc = func() (string, error) { return tmpDir, nil }
	defer func() { getConfigDirFunc = original }()
	run()
}
