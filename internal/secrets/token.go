package secrets

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// TokenFile is where a host keeps its service-account token when no launcher
// exports OP_SERVICE_ACCOUNT_TOKEN: HEV_OP_TOKEN_FILE, else ~/.config/op/token.
// kit's Layer-key reader already used this file; the cache uses the same one so
// that hevd serves the endpoint without the token in its unit or environment.
func TokenFile() string {
	if p := os.Getenv("HEV_OP_TOKEN_FILE"); p != "" {
		return filepath.Clean(p)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "op", "token")
}

// serviceToken returns the selected credential for endpoint fingerprinting.
// Invalid configured stores yield no fingerprint; actual reads and status
// retain the diagnostic through tokenCredential. Tokens are read at use time.
func serviceToken() string {
	t, _, _ := tokenCredential()
	return t
}

func plaintextToken() string {
	p := TokenFile()
	if p == "" {
		return ""
	}
	st, err := os.Stat(p)
	if err != nil || !st.Mode().IsRegular() || (runtime.GOOS != "windows" && st.Mode().Perm()&0o077 != 0) {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// opEnv is the environment for an op subprocess: the caller's environment,
// desktop unlock disabled, and exactly the selected credential. A rotated
// file credential replaces the startup token in the child's environment.
func opEnv(token string) []string {
	env := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "OP_SERVICE_ACCOUNT_TOKEN=") && !strings.HasPrefix(v, "OP_BIOMETRIC_UNLOCK_ENABLED=") {
			env = append(env, v)
		}
	}
	env = append(env, "OP_BIOMETRIC_UNLOCK_ENABLED=false")
	if token != "" {
		env = append(env, "OP_SERVICE_ACCOUNT_TOKEN="+token)
	}
	return env
}

func (c *Cache) tokenCredential() (string, string, error) {
	if t := c.rotatedToken.Load(); t != nil {
		return *t, "rotated token file", nil
	}
	return tokenCredential()
}
