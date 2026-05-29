package ssh

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestValidateSSHConfigPropagatesParserFailure(t *testing.T) {
	original := runSSHConfigValidation
	defer func() { runSSHConfigValidation = original }()

	runSSHConfigValidation = func(path string) ([]byte, error) {
		return []byte("bad configuration option"), errors.New("exit status 255")
	}

	err := validateSSHConfig("/tmp/ssh-config")
	if err == nil || !strings.Contains(err.Error(), "bad configuration option") {
		t.Fatalf("validateSSHConfig() error = %v", err)
	}
}

func TestValidateSSHConfigAllowsParserSuccess(t *testing.T) {
	original := runSSHConfigValidation
	defer func() { runSSHConfigValidation = original }()

	runSSHConfigValidation = func(path string) ([]byte, error) {
		return []byte("host gitrevolver-validation-host"), nil
	}

	if err := validateSSHConfig("/tmp/ssh-config"); err != nil {
		t.Fatalf("validateSSHConfig() error = %v", err)
	}
}

func TestConcurrentAddSSHConfigEntryKeepsAllAliases(t *testing.T) {
	configPath := tempSSHConfig(t)
	stubSSHValidation(t, nil)

	aliases := []string{"github-a", "github-b", "github-c", "github-d", "github-e"}
	var wg sync.WaitGroup
	errs := make(chan error, len(aliases))
	start := make(chan struct{})

	for _, alias := range aliases {
		alias := alias
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- AddSSHConfigEntry(alias, "/tmp/"+alias)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("AddSSHConfigEntry() error = %v", err)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, alias := range aliases {
		if !strings.Contains(content, "Host "+alias+"\n") {
			t.Fatalf("final SSH config missing alias %q:\n%s", alias, content)
		}
	}
	if count := strings.Count(content, SSHConfigMarkerBegin); count != 1 {
		t.Fatalf("managed block begin count = %d, want 1:\n%s", count, content)
	}
	assertNoSSHMutationScratchFiles(t, filepath.Dir(configPath))
}

func TestConcurrentAddSameAliasDoesNotDuplicateHost(t *testing.T) {
	configPath := tempSSHConfig(t)
	stubSSHValidation(t, nil)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- AddSSHConfigEntry("github-same", fmt.Sprintf("/tmp/key-%d", i))
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("AddSSHConfigEntry() error = %v", err)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(data), "Host github-same\n"); count != 1 {
		t.Fatalf("Host github-same count = %d, want 1:\n%s", count, string(data))
	}
}

func TestAddSSHConfigEntryRejectsUnsafeAliasAndPath(t *testing.T) {
	tempSSHConfig(t)
	stubSSHValidation(t, nil)

	if err := AddSSHConfigEntry("github bad\nHost injected", "/tmp/key"); err == nil || !strings.Contains(err.Error(), "unsupported characters") {
		t.Fatalf("AddSSHConfigEntry unsafe alias error = %v", err)
	}
	if err := AddSSHConfigEntry("github-safe", "relative/key"); err == nil || !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("AddSSHConfigEntry relative path error = %v", err)
	}
	if err := AddSSHConfigEntry("github-safe", "/tmp/key\nProxyCommand bad"); err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("AddSSHConfigEntry unsafe path error = %v", err)
	}
}

func TestConcurrentAddAndRemoveSSHConfigEntries(t *testing.T) {
	configPath := tempSSHConfig(t)
	stubSSHValidation(t, nil)

	for _, alias := range []string{"github-a", "github-b", "github-c"} {
		if err := AddSSHConfigEntry(alias, "/tmp/"+alias); err != nil {
			t.Fatalf("seed AddSSHConfigEntry(%q) error = %v", alias, err)
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	start := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		errs <- RemoveSSHConfigEntry("github-b")
	}()
	go func() {
		defer wg.Done()
		<-start
		errs <- AddSSHConfigEntry("github-d", "/tmp/github-d")
	}()
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent SSH config mutation error = %v", err)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "Host github-b\n") {
		t.Fatalf("removed alias still present:\n%s", content)
	}
	if !strings.Contains(content, "Host github-d\n") {
		t.Fatalf("added alias missing:\n%s", content)
	}
	assertNoSSHMutationScratchFiles(t, filepath.Dir(configPath))
}

func TestSSHConfigLockTimeout(t *testing.T) {
	configPath := tempSSHConfig(t)
	configDir := filepath.Dir(configPath)
	lockPath := filepath.Join(configDir, sshConfigLockName)
	if err := os.WriteFile(lockPath, []byte("locked"), 0600); err != nil {
		t.Fatal(err)
	}

	restoreLockTiming(t, 20*time.Millisecond, time.Millisecond, time.Hour)
	_, err := acquireSSHConfigLock(configDir)
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for SSH config lock") {
		t.Fatalf("acquireSSHConfigLock() error = %v", err)
	}
}

func TestSSHConfigStaleLockIsRemoved(t *testing.T) {
	configPath := tempSSHConfig(t)
	configDir := filepath.Dir(configPath)
	lockPath := filepath.Join(configDir, sshConfigLockName)
	if err := os.WriteFile(lockPath, []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	staleTime := time.Now().Add(-3 * time.Minute)
	if err := os.Chtimes(lockPath, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}

	restoreLockTiming(t, time.Second, time.Millisecond, time.Minute)
	unlock, err := acquireSSHConfigLock(configDir)
	if err != nil {
		t.Fatalf("acquireSSHConfigLock() error = %v", err)
	}
	unlock()
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock file exists after unlock, stat error = %v", err)
	}
}

func TestValidationFailurePreservesOriginalSSHConfig(t *testing.T) {
	configPath := tempSSHConfig(t)
	original := "Host github-original\n  HostName github.com\n"
	if err := os.WriteFile(configPath, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	stubSSHValidation(t, errors.New("exit status 255"))

	err := AddSSHConfigEntry("github-new", "/tmp/github-new")
	if err == nil || !strings.Contains(err.Error(), "invalid SSH config") {
		t.Fatalf("AddSSHConfigEntry() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("SSH config changed after validation failure:\n%s", string(data))
	}
	assertNoSSHMutationScratchFiles(t, filepath.Dir(configPath))
}

func tempSSHConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(sshDir, "config")
	if err := os.WriteFile(configPath, []byte("Host github-existing\n  HostName github.com\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func stubSSHValidation(t *testing.T, err error) {
	t.Helper()
	original := runSSHConfigValidation
	runSSHConfigValidation = func(path string) ([]byte, error) {
		if err != nil {
			return []byte("bad configuration option"), err
		}
		return []byte("host gitrevolver-validation-host"), nil
	}
	t.Cleanup(func() { runSSHConfigValidation = original })
}

func restoreLockTiming(t *testing.T, timeout, retry, stale time.Duration) {
	t.Helper()
	originalTimeout := sshConfigLockTimeout
	originalRetry := sshConfigLockRetry
	originalStale := sshConfigLockStale
	sshConfigLockTimeout = timeout
	sshConfigLockRetry = retry
	sshConfigLockStale = stale
	t.Cleanup(func() {
		sshConfigLockTimeout = originalTimeout
		sshConfigLockRetry = originalRetry
		sshConfigLockStale = originalStale
	})
}

func assertNoSSHMutationScratchFiles(t *testing.T, dir string) {
	t.Helper()
	for _, pattern := range []string{
		filepath.Join(dir, ".gitrevolver-ssh-config-*.tmp"),
		filepath.Join(dir, "config.tmp"),
		filepath.Join(dir, sshConfigLockName),
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) > 0 {
			t.Fatalf("unexpected scratch files for %s: %v", pattern, matches)
		}
	}
}
