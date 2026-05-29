//go:build darwin

package keychain

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

var runSecurity = func(args []string, stdin string) ([]byte, error) {
	cmd := exec.Command("security", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	return cmd.CombinedOutput()
}

func StoreSecret(identityAlias, key, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("secret value cannot be empty")
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("secret values cannot contain newlines")
	}

	return fmt.Errorf("non-interactive macOS Keychain storage would expose the secret in process arguments; run `gitrevolver secret set %s %s` in a terminal and enter the value in the hidden Keychain prompt", identityAlias, key)
}

func SupportsInteractiveStore() bool {
	return true
}

func StoreSecretInteractive(identityAlias, key string) error {
	account := SecretKey(identityAlias, key)
	output, err := runSecurity([]string{"add-generic-password", "-a", account, "-s", ServiceName, "-U", "-w"}, "")
	if err != nil {
		return securityError("store", output, err)
	}
	return nil
}

func GetSecret(identityAlias, key string) (string, error) {
	account := SecretKey(identityAlias, key)
	output, err := runSecurity([]string{"find-generic-password", "-a", account, "-s", ServiceName, "-w"}, "")
	if err != nil {
		return "", securityError("read", output, err)
	}
	return strings.TrimSuffix(string(output), "\n"), nil
}

func DeleteSecret(identityAlias, key string) error {
	account := SecretKey(identityAlias, key)
	output, err := runSecurity([]string{"delete-generic-password", "-a", account, "-s", ServiceName}, "")
	if err != nil {
		return securityError("delete", output, err)
	}
	return nil
}

func securityError(action string, output []byte, err error) error {
	message := string(bytes.TrimSpace(output))
	if message == "" {
		return fmt.Errorf("failed to %s macOS Keychain secret: %w", action, err)
	}
	return fmt.Errorf("failed to %s macOS Keychain secret: %s: %w", action, message, err)
}
