//go:build !darwin

package keychain

import (
	"fmt"
	"strings"

	"github.com/99designs/keyring"
)

func getKeyring() (keyring.Keyring, error) {
	return keyring.Open(keyring.Config{
		ServiceName: ServiceName,
	})
}

func StoreSecret(identityAlias, key, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("secret value cannot be empty")
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("secret values cannot contain newlines")
	}
	ring, err := getKeyring()
	if err != nil {
		return fmt.Errorf("failed to open keyring: %w", err)
	}
	return ring.Set(keyring.Item{
		Key:  SecretKey(identityAlias, key),
		Data: []byte(value),
	})
}

func SupportsInteractiveStore() bool {
	return false
}

func StoreSecretInteractive(identityAlias, key string) error {
	return fmt.Errorf("interactive keyring storage is not supported on this platform")
}

func GetSecret(identityAlias, key string) (string, error) {
	ring, err := getKeyring()
	if err != nil {
		return "", fmt.Errorf("failed to open keyring: %w", err)
	}
	item, err := ring.Get(SecretKey(identityAlias, key))
	if err != nil {
		return "", err
	}
	return string(item.Data), nil
}

func DeleteSecret(identityAlias, key string) error {
	ring, err := getKeyring()
	if err != nil {
		return fmt.Errorf("failed to open keyring: %w", err)
	}
	return ring.Remove(SecretKey(identityAlias, key))
}
