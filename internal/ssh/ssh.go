package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	SSHConfigMarkerBegin = "# BEGIN gitrevolver managed"
	SSHConfigMarkerEnd   = "# END gitrevolver managed"
	sshConfigLockName    = ".gitrevolver-config.lock"
)

var hostAliasPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

var (
	sshConfigLockTimeout = 10 * time.Second
	sshConfigLockRetry   = 50 * time.Millisecond
	sshConfigLockStale   = 2 * time.Minute
)

func GetSSHConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".ssh", "config"), nil
}

func GenerateSSHKey(identityAlias string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	sshDir := filepath.Join(homeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	keyPath := filepath.Join(sshDir, fmt.Sprintf("gitrevolver_%s", identityAlias))

	// Check if key already exists
	if _, err := os.Stat(keyPath); err == nil {
		return keyPath, nil
	}

	// Generate SSH key
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "", "-C", fmt.Sprintf("gitrevolver-%s", identityAlias))
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to generate SSH key: %w", err)
	}

	return keyPath, nil
}

// SSHIdentity represents an SSH host alias and key path pair
type SSHIdentity struct {
	HostAlias string
	KeyPath   string
}

// AddSSHConfigEntry adds or updates an SSH config entry, preserving managed entries.
func AddSSHConfigEntry(hostAlias, keyPath string) error {
	if err := validateHostAlias(hostAlias); err != nil {
		return err
	}
	if err := validateIdentityFilePath(keyPath); err != nil {
		return err
	}
	configPath, err := GetSSHConfigPath()
	if err != nil {
		return err
	}

	return withSSHConfigLock(configPath, func() error {
		// Backup before making changes
		if _, err := BackupSSHConfig(); err != nil {
			return fmt.Errorf("failed to backup SSH config: %w", err)
		}

		// Read existing config
		var existingContent string
		if data, err := os.ReadFile(configPath); err == nil {
			existingContent = string(data)
		}

		// Parse existing managed entries
		existingIdentities := parseManagedBlock(existingContent)

		// Add or update the new identity
		found := false
		for i, id := range existingIdentities {
			if id.HostAlias == hostAlias {
				existingIdentities[i].KeyPath = keyPath
				found = true
				break
			}
		}
		if !found {
			existingIdentities = append(existingIdentities, SSHIdentity{
				HostAlias: hostAlias,
				KeyPath:   keyPath,
			})
		}

		// Remove existing managed block
		existingContent = removeManagedBlock(existingContent)

		// Build new managed block with all identities
		managedBlock := buildManagedBlockFromIdentities(existingIdentities)

		// Append managed block
		newContent := existingContent
		if !strings.HasSuffix(newContent, "\n") && newContent != "" {
			newContent += "\n"
		}
		newContent += managedBlock + "\n"

		return writeValidatedSSHConfig(configPath, newContent)
	})
}

func withSSHConfigLock(configPath string, fn func() error) error {
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create SSH config directory: %w", err)
	}

	unlock, err := acquireSSHConfigLock(configDir)
	if err != nil {
		return err
	}
	defer unlock()

	return fn()
}

func buildManagedBlock(hostAlias, keyPath string) string {
	block := fmt.Sprintf("%s\n", SSHConfigMarkerBegin)
	block += fmt.Sprintf("Host %s\n", hostAlias)
	block += fmt.Sprintf("  HostName github.com\n")
	block += fmt.Sprintf("  User git\n")
	block += fmt.Sprintf("  IdentityFile %s\n", keyPath)
	block += fmt.Sprintf("  IdentitiesOnly yes\n")
	block += fmt.Sprintf("%s\n", SSHConfigMarkerEnd)
	return block
}

func buildManagedBlockFromIdentities(identities []SSHIdentity) string {
	block := fmt.Sprintf("%s\n", SSHConfigMarkerBegin)
	for _, id := range identities {
		block += fmt.Sprintf("Host %s\n", id.HostAlias)
		block += fmt.Sprintf("  HostName github.com\n")
		block += fmt.Sprintf("  User git\n")
		block += fmt.Sprintf("  IdentityFile %s\n", id.KeyPath)
		block += fmt.Sprintf("  IdentitiesOnly yes\n")
		block += "\n"
	}
	block += fmt.Sprintf("%s\n", SSHConfigMarkerEnd)
	return block
}

func validateHostAlias(hostAlias string) error {
	if strings.TrimSpace(hostAlias) == "" {
		return fmt.Errorf("SSH host alias cannot be empty")
	}
	if !hostAliasPattern.MatchString(hostAlias) {
		return fmt.Errorf("SSH host alias %q contains unsupported characters", hostAlias)
	}
	return nil
}

func validateIdentityFilePath(keyPath string) error {
	if strings.TrimSpace(keyPath) == "" {
		return fmt.Errorf("SSH key path cannot be empty")
	}
	if strings.ContainsAny(keyPath, "\r\n\x00") {
		return fmt.Errorf("SSH key path contains unsupported control characters")
	}
	if strings.HasPrefix(keyPath, "~"+string(filepath.Separator)) || filepath.IsAbs(keyPath) {
		return nil
	}
	return fmt.Errorf("SSH key path must be absolute or start with ~/")
}

func parseManagedBlock(content string) []SSHIdentity {
	var identities []SSHIdentity
	lines := strings.Split(content, "\n")
	inManagedBlock := false
	var currentHost string
	var currentKeyPath string

	for i, line := range lines {
		if strings.Contains(line, SSHConfigMarkerBegin) {
			inManagedBlock = true
			continue
		}
		if strings.Contains(line, SSHConfigMarkerEnd) {
			// Save last identity if any
			if currentHost != "" && currentKeyPath != "" {
				identities = append(identities, SSHIdentity{
					HostAlias: currentHost,
					KeyPath:   currentKeyPath,
				})
			}
			inManagedBlock = false
			currentHost = ""
			currentKeyPath = ""
			continue
		}
		if inManagedBlock {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Host ") {
				// Save previous identity if any
				if currentHost != "" && currentKeyPath != "" {
					identities = append(identities, SSHIdentity{
						HostAlias: currentHost,
						KeyPath:   currentKeyPath,
					})
				}
				currentHost = strings.TrimPrefix(line, "Host ")
				currentKeyPath = ""
			} else if strings.HasPrefix(line, "IdentityFile ") {
				currentKeyPath = strings.TrimPrefix(line, "IdentityFile ")
			}
		}
		_ = i // avoid unused variable
	}

	return identities
}

func removeManagedBlock(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inManagedBlock := false

	for _, line := range lines {
		if strings.Contains(line, SSHConfigMarkerBegin) {
			inManagedBlock = true
			continue
		}
		if strings.Contains(line, SSHConfigMarkerEnd) {
			inManagedBlock = false
			continue
		}
		if !inManagedBlock {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

var runSSHConfigValidation = func(configPath string) ([]byte, error) {
	cmd := exec.Command("ssh", "-F", configPath, "-G", "gitrevolver-validation-host")
	return cmd.CombinedOutput()
}

func validateSSHConfig(configPath string) error {
	output, err := runSSHConfigValidation(configPath)
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		return err
	}
	return fmt.Errorf("%s: %w", message, err)
}

func acquireSSHConfigLock(configDir string) (func(), error) {
	lockPath := filepath.Join(configDir, sshConfigLockName)
	deadline := time.Now().Add(sshConfigLockTimeout)

	for {
		file, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			if _, err := fmt.Fprintf(file, "pid=%d\ncreated=%s\n", os.Getpid(), time.Now().Format(time.RFC3339Nano)); err != nil {
				file.Close()
				os.Remove(lockPath)
				return nil, fmt.Errorf("failed to write SSH config lock: %w", err)
			}
			if err := file.Close(); err != nil {
				os.Remove(lockPath)
				return nil, fmt.Errorf("failed to close SSH config lock: %w", err)
			}
			return func() {
				_ = os.Remove(lockPath)
			}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("failed to create SSH config lock: %w", err)
		}

		info, statErr := os.Stat(lockPath)
		switch {
		case os.IsNotExist(statErr):
			continue
		case statErr != nil:
			return nil, fmt.Errorf("failed to inspect SSH config lock: %w", statErr)
		case time.Since(info.ModTime()) > sshConfigLockStale:
			if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to remove stale SSH config lock: %w", err)
			}
			continue
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for SSH config lock %s", lockPath)
		}
		time.Sleep(sshConfigLockRetry)
	}
}

func writeValidatedSSHConfig(configPath, content string) error {
	tempFile, err := os.CreateTemp(filepath.Dir(configPath), ".gitrevolver-ssh-config-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp config: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if err := tempFile.Chmod(0600); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to set temp config permissions: %w", err)
	}
	if _, err := tempFile.WriteString(content); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write temp config: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp config: %w", err)
	}

	if err := validateSSHConfig(tempPath); err != nil {
		return fmt.Errorf("invalid SSH config: %w", err)
	}

	if err := os.Rename(tempPath, configPath); err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	return nil
}

func RemoveSSHConfigEntry(hostAlias string) error {
	if err := validateHostAlias(hostAlias); err != nil {
		return err
	}
	configPath, err := GetSSHConfigPath()
	if err != nil {
		return err
	}

	return withSSHConfigLock(configPath, func() error {
		// Backup before making changes
		if _, err := BackupSSHConfig(); err != nil {
			return fmt.Errorf("failed to backup SSH config: %w", err)
		}

		data, err := os.ReadFile(configPath)
		if err != nil {
			return err
		}

		// Parse existing managed entries
		existingIdentities := parseManagedBlock(string(data))

		// Remove the specified host alias
		filteredIdentities := []SSHIdentity{}
		for _, id := range existingIdentities {
			if id.HostAlias != hostAlias {
				filteredIdentities = append(filteredIdentities, id)
			}
		}

		// Remove existing managed block
		content := removeManagedBlock(string(data))

		// If there are remaining identities, rebuild the managed block
		if len(filteredIdentities) > 0 {
			managedBlock := buildManagedBlockFromIdentities(filteredIdentities)
			if !strings.HasSuffix(content, "\n") && content != "" {
				content += "\n"
			}
			content += managedBlock + "\n"
		}

		return writeValidatedSSHConfig(configPath, content)
	})
}
