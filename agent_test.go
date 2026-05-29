package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/key-witness/gitrevolver/internal/config"
	"github.com/key-witness/gitrevolver/internal/keychain"
)

func TestVercelDoctorUsesIdentityLevelToken(t *testing.T) {
	restore := stubSecretGetter(func(identity, name string) (string, error) {
		if identity == "client-a" && name == keychain.VercelTokenSecret {
			return "vercel-token", nil
		}
		return "", errors.New("missing")
	})
	defer restore()

	checks := vercelDoctorChecks(config.Project{}, config.Identity{ID: "client-a"})

	if !hasCheck(checks, "Vercel token", "OK") {
		t.Fatalf("expected identity-level Vercel token OK, got %#v", checks)
	}
	if !hasCheck(checks, "Vercel mode", "SKIP") {
		t.Fatalf("expected project mode to be skipped, got %#v", checks)
	}
	if hasCheck(checks, "Vercel", "SKIP") {
		t.Fatalf("unexpected project-level Vercel not linked check: %#v", checks)
	}
}

func TestVercelDoctorWarnsWhenIdentityTokenMissing(t *testing.T) {
	restore := stubSecretGetter(func(identity, name string) (string, error) {
		return "", errors.New("missing")
	})
	defer restore()

	checks := vercelDoctorChecks(config.Project{}, config.Identity{ID: "client-a"})

	if !hasCheck(checks, "Vercel token", "WARN") {
		t.Fatalf("expected missing identity token warning, got %#v", checks)
	}
	if hasCheck(checks, "Vercel", "SKIP") {
		t.Fatalf("unexpected project-level Vercel not linked check: %#v", checks)
	}
}

func TestVercelProdGuardUsesIdentityLevelToken(t *testing.T) {
	restore := stubSecretGetter(func(identity, name string) (string, error) {
		if identity == "client-a" && name == keychain.VercelTokenSecret {
			return "vercel-token", nil
		}
		return "", errors.New("missing")
	})
	defer restore()

	result := guard(&ResolvedProject{
		Project: config.Project{Slug: "client-app"},
		Identity: config.Identity{
			ID: "client-a",
		},
	}, "vercel-prod-deploy", true, false, false)

	if !result.Allowed {
		t.Fatalf("expected guard to allow identity-level Vercel token, got %#v", result.Checks)
	}
	if hasCheck(result.Checks, "Vercel metadata", "BLOCK") {
		t.Fatalf("guard should not require project-level Vercel metadata: %#v", result.Checks)
	}
}

func TestVercelProdGuardBlocksProductionBranchMismatch(t *testing.T) {
	restore := stubSecretGetter(func(identity, name string) (string, error) {
		if name == keychain.VercelTokenSecret {
			return "vercel-token", nil
		}
		return "", errors.New("missing")
	})
	defer restore()

	repoPath := t.TempDir()
	runGit(t, repoPath, "init")
	runGit(t, repoPath, "branch", "-M", "feature")
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := guard(&ResolvedProject{
		Project: config.Project{
			Slug: "client-app",
			Path: repoPath,
			Vercel: config.ProjectVercel{
				ProductionBranch: "main",
			},
		},
		Identity: config.Identity{ID: "client"},
	}, "vercel-prod-deploy", true, false, false)

	if result.Allowed {
		t.Fatalf("expected guard to block branch mismatch, got %#v", result.Checks)
	}
	if !hasCheck(result.Checks, "production branch", "BLOCK") {
		t.Fatalf("expected production branch BLOCK, got %#v", result.Checks)
	}

	result = guard(&ResolvedProject{
		Project: config.Project{
			Slug: "client-app",
			Path: repoPath,
			Vercel: config.ProjectVercel{
				ProductionBranch: "main",
			},
		},
		Identity: config.Identity{ID: "client"},
	}, "vercel-prod-deploy", true, false, true)
	if !result.Allowed {
		t.Fatalf("expected override to allow branch mismatch, got %#v", result.Checks)
	}
}

func TestSummarizeProviderOutputRedactsInjectedTokens(t *testing.T) {
	got := summarizeProviderOutput(
		[]byte("authentication failed for secret-token\ntry again\nextra line"),
		errors.New("exit status 1"),
		[]string{"VERCEL_TOKEN=secret-token"},
	)
	if got != "authentication failed for [redacted] try again" {
		t.Fatalf("summary = %q", got)
	}
}

func TestSupabaseDoctorUsesIdentityLevelToken(t *testing.T) {
	restore := stubSecretGetter(func(identity, name string) (string, error) {
		if identity == "client-a" && name == keychain.SupabaseTokenSecret {
			return "supabase-token", nil
		}
		return "", errors.New("missing")
	})
	defer restore()

	checks := supabaseDoctorChecks(config.Project{}, config.Identity{ID: "client-a"})

	if !hasCheck(checks, "Supabase token", "OK") {
		t.Fatalf("expected identity-level Supabase token OK, got %#v", checks)
	}
	if !hasCheck(checks, "Supabase project ref", "SKIP") {
		t.Fatalf("expected project ref to be skipped, got %#v", checks)
	}
	if hasCheck(checks, "Supabase", "SKIP") {
		t.Fatalf("unexpected project-level Supabase not linked check: %#v", checks)
	}
}

func TestSupabaseDoctorWarnsWhenIdentityTokenMissing(t *testing.T) {
	restore := stubSecretGetter(func(identity, name string) (string, error) {
		return "", errors.New("missing")
	})
	defer restore()

	checks := supabaseDoctorChecks(config.Project{}, config.Identity{ID: "client-a"})

	if !hasCheck(checks, "Supabase token", "WARN") {
		t.Fatalf("expected missing identity token warning, got %#v", checks)
	}
	if hasCheck(checks, "Supabase", "SKIP") {
		t.Fatalf("unexpected project-level Supabase not linked check: %#v", checks)
	}
}

func TestSupabaseGuardUsesIdentityLevelToken(t *testing.T) {
	restore := stubSecretGetter(func(identity, name string) (string, error) {
		if identity == "client-a" && name == keychain.SupabaseTokenSecret {
			return "supabase-token", nil
		}
		return "", errors.New("missing")
	})
	defer restore()

	result := guard(&ResolvedProject{
		Project: config.Project{Slug: "client-app"},
		Identity: config.Identity{
			ID: "client-a",
		},
	}, "supabase-db-push", true, false, false)

	if !result.Allowed {
		t.Fatalf("expected guard to allow identity-level Supabase token, got %#v", result.Checks)
	}
	if hasCheck(result.Checks, "Supabase project ref", "BLOCK") {
		t.Fatalf("guard should not require project-level Supabase ref: %#v", result.Checks)
	}
}

func stubSecretGetter(fn func(identity, name string) (string, error)) func() {
	original := getSecret
	getSecret = fn
	return func() {
		getSecret = original
	}
}

func hasCheck(checks []CheckResult, name, status string) bool {
	for _, check := range checks {
		if check.Name == name && check.Status == status {
			return true
		}
	}
	return false
}
