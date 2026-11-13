//go:build !windows

package secrets

import "fmt"

func (s ProtectedTokenStore) Read() (string, error) {
	return "", fmt.Errorf("DPAPI token stores require Windows")
}
func (s ProtectedTokenStore) Write(data []byte) error {
	return fmt.Errorf("DPAPI token stores require Windows")
}
func (s ProtectedTokenStore) Inspect() ([]string, error) {
	return nil, fmt.Errorf("DPAPI token stores require Windows")
}
