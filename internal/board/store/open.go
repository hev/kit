package store

import (
	"github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/storagemode"
)

// Open is the store `hev board serve` runs on. storagemode decides: with a
// Layer API key the board is the Layer namespace it always was; with none it
// lives in files under settings.Dir(). A key that is configured but cannot
// be read is an error, never a quiet switch to files.
func Open() (*Store, error) {
	r, err := storagemode.Resolve(layer.KeyOptions)
	if err != nil {
		return nil, err
	}
	if r.Mode == storagemode.Layer {
		cl, err := layer.WithKey(r.Key)
		if err != nil {
			return nil, err
		}
		st := New(cl)
		st.info.KeySource = r.Source
		return st, nil
	}
	return NewFiles(settings.Dir())
}
