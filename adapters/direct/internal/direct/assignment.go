package direct

import (
	"errors"
	"path/filepath"
	"regexp"
)

var revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ValidateBrief checks the fixed execution inputs; skills and checks travel in brief.
func ValidateBrief(a Row) error {
	for _, k := range []string{"role", "model", "source_revision", "directory", "brief"} {
		if v, ok := a[k].(string); !ok || v == "" {
			return errors.New("assignment requires nonempty " + k)
		}
	}
	if a["role"] != "implementation" && a["role"] != "validation" {
		return errors.New("assignment role must be implementation or validation")
	}
	if !revisionPattern.MatchString(a["source_revision"].(string)) || !filepath.IsAbs(a["directory"].(string)) {
		return errors.New("assignment requires exact Git revision and absolute directory")
	}
	if a["role"] == "validation" {
		if v, ok := a["implementation_run"].(string); !ok || v == "" {
			return errors.New("validation assignment requires implementation_run")
		}

	}
	n, ok := a["max_seconds"].(float64)
	if !ok || n < 1 || n > 3600 || n != float64(int(n)) {
		return errors.New("max_seconds must be an integer in 1..3600")
	}

	return nil
}
