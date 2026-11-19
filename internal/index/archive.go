package index

import (
	"reflect"

	"github.com/hev/kit/internal/archive"
	"github.com/hev/kit/internal/layer"
)

// Archive is the storage surface the indexer writes through:
// archive.Writer, implemented by *layer.Client and *filearchive.Store.
type Archive = archive.Writer

var _ Archive = (*layer.Client)(nil)

// absent reports a missing archive, including a nil *layer.Client held in
// the interface (a dry run has no client).
func absent(cl Archive) bool {
	if cl == nil {
		return true
	}
	v := reflect.ValueOf(cl)
	return v.Kind() == reflect.Pointer && v.IsNil()
}
