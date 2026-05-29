//go:build darwin

package keychain

import (
	"reflect"
	"strings"
	"testing"
)

func TestDarwinStoreRejectsProgrammaticStorage(t *testing.T) {
	original := runSecurity
	defer func() { runSecurity = original }()

	called := false
	runSecurity = func(args []string, stdin string) ([]byte, error) {
		called = true
		return nil, nil
	}

	err := StoreSecret("client", GitHubTokenSecret, "token-value")
	if err == nil || !strings.Contains(err.Error(), "would expose the secret in process arguments") {
		t.Fatalf("StoreSecret() error = %v", err)
	}
	if called {
		t.Fatal("StoreSecret called security with a secret-bearing path")
	}
}

func TestDarwinInteractiveStoreUsesSecurityPrompt(t *testing.T) {
	original := runSecurity
	defer func() { runSecurity = original }()

	var gotArgs []string
	var gotStdin string
	runSecurity = func(args []string, stdin string) ([]byte, error) {
		gotArgs = append([]string(nil), args...)
		gotStdin = stdin
		return nil, nil
	}

	if err := StoreSecretInteractive("client", VercelTokenSecret); err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"add-generic-password", "-a", "gitrevolver/client/vercel-token", "-s", ServiceName, "-U", "-w"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("security args = %#v, want %#v", gotArgs, wantArgs)
	}
	if gotStdin != "" {
		t.Fatalf("security stdin = %q", gotStdin)
	}
}

func TestDarwinStoreRejectsNewlines(t *testing.T) {
	if err := StoreSecret("client", VercelTokenSecret, "line1\nline2"); err == nil || !strings.Contains(err.Error(), "newlines") {
		t.Fatalf("StoreSecret() error = %v", err)
	}
}
