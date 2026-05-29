package keychain

import (
	"fmt"
)

const (
	ServiceName         = "gitrevolver"
	GitHubTokenSecret   = "github-token"
	VercelTokenSecret   = "vercel-token"
	SupabaseTokenSecret = "supabase-token"
	SSHPassphraseSecret = "ssh-passphrase"
)

func DeleteAllSecrets(identityAlias string) error {
	// Note: keyring doesn't support listing easily, so we delete known keys
	keys := []string{GitHubTokenSecret, VercelTokenSecret, SupabaseTokenSecret, SSHPassphraseSecret, "pat"}
	for _, key := range keys {
		DeleteSecret(identityAlias, key) // Ignore errors
	}
	return nil
}

func SecretKey(identityAlias, key string) string {
	return fmt.Sprintf("gitrevolver/%s/%s", identityAlias, key)
}
