package main

import (
	"fmt"
	"strings"
)

// Batch files cannot represent arbitrary argv safely. Refuse shell syntax rather
// than letting cmd expand user arguments or executable paths.
func cmdPluginLine(path string, args []string) (string, error) {
	parts := append([]string{path}, args...)
	for i, part := range parts {
		if strings.ContainsAny(part, "\"^&|<>()%!\r\n") {
			return "", fmt.Errorf("unsafe metacharacter in batch plugin path or argument")
		}
		parts[i] = "\"" + part + "\""
	}
	return "\"" + strings.Join(parts, " ") + "\"", nil
}
