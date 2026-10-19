package nativegate

import (
	"errors"
	"github.com/hev/factory/pkg/argohost"
	"io"
)

// DecodeArgoTaskReference is a bounded reference codec only. It neither prepares
// nor launches work. The host must independently verify canonical parents,
// workflow/node identity, profile binding and existing native/DB gates.
func DecodeArgoTaskReference(input io.Reader) (argohost.TaskReference, error) {
	raw, err := io.ReadAll(io.LimitReader(input, argohost.TaskReferenceLimit+1))
	if err != nil || len(raw) > argohost.TaskReferenceLimit {
		return argohost.TaskReference{}, errors.New("invalid Argo task reference")
	}
	return argohost.ParseTaskReference(raw)
}
