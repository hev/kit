package secrets

import (
	"fmt"
	"os"
	"strings"
)

// ProtectedTokenStore is the credential-storage seam, separate from secret
// providers. Keychain or a Linux credential store can implement this later;
// only Windows DPAPI is implemented now. No decrypted token is persisted.
type ProtectedTokenStore struct {
	Path    string
	Scope   string
	Entropy []byte
}

func configuredTokenStore() ProtectedTokenStore {
	scope := os.Getenv("HEV_OP_DPAPI_SCOPE")
	if scope == "" {
		scope = "CurrentUser"
	}
	return ProtectedTokenStore{Path: os.Getenv("HEV_OP_DPAPI_FILE"), Scope: scope, Entropy: []byte(os.Getenv("HEV_OP_DPAPI_ENTROPY"))}
}

// TokenStoreConfig returns the configured Windows store (entropy is UTF-8).
func TokenStoreConfig() ProtectedTokenStore { return configuredTokenStore() }

func tokenCredential() (string, string, error) {
	if t := os.Getenv("OP_SERVICE_ACCOUNT_TOKEN"); t != "" {
		return t, "environment", nil
	}
	s := configuredTokenStore()
	if s.Path != "" {
		t, err := s.Read()
		if err != nil {
			return "", "", err
		}
		return t, "DPAPI protected file", nil
	}
	t := plaintextToken()
	if t == "" {
		return "", "", fmt.Errorf("1Password service-account token is required; desktop unlock disabled")
	}
	return t, "token file", nil
}

func validToken(data []byte) (string, error) {
	t := strings.TrimSpace(string(data))
	if t == "" || strings.ContainsAny(t, "\x00\r\n") {
		return "", fmt.Errorf("token must be a nonempty single line")
	}
	return t, nil
}
