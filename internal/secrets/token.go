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

// serviceToken returns the environment token, else the token file's contents
// when the file is private to its owner. It is read at use time and never
// stored outside the call.
func serviceToken() string {
	if t := os.Getenv("OP_SERVICE_ACCOUNT_TOKEN"); t != "" {
		return t
	}
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
// desktop unlock disabled, and the token from the token file when the
// environment has none. The token travels only in the child's environment.
func opEnv(token string) []string {
	env := append(os.Environ(), "OP_BIOMETRIC_UNLOCK_ENABLED=false")
	if os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") == "" && token != "" {
		env = append(env, "OP_SERVICE_ACCOUNT_TOKEN="+token)
	}
	return env
}
