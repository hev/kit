// Package evidencemedia exposes kit's byte-based evidence validation.
package evidencemedia

import (
	"errors"

	"github.com/hev/kit/internal/evidence"
)

type Media = evidence.Media

// Inspect verifies the container, modality, size and duration.
func Inspect(data []byte) (Media, error) {
	return evidence.Inspect(data, evidence.DefaultLimits)
}

// TooLarge reports whether the evidence exceeds a modality limit.
func TooLarge(err error) bool {
	return errors.Is(err, evidence.ErrTooLarge)
}
