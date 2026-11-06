package peers

import "github.com/hev/kit/internal/platformops"

// Store is the append-only journal boundary for inbox messages and receipts,
// delivery history, and relay queue records. The caller holds Lock across a
// read/validate/append transaction. Append must durably retain existing bytes.
// Paths are logical journal keys within Engine.State. FileStore is the only
// implementation; no alternative transport or cloud storage is enabled.
type Store interface {
	Rows(path string) ([]Message, error)
	Append(path string, record any) error
	Lock(path string) (func(), error)
}

type FileStore struct{}

func (FileStore) Rows(path string) ([]Message, error)  { return rows(path) }
func (FileStore) Append(path string, record any) error { return appendRow(path, record) }
func (FileStore) Lock(path string) (func(), error)     { return platformops.LockPath(path, false) }
func (e *Engine) store() Store {
	if e.Storage != nil {
		return e.Storage
	}
	return FileStore{}
}
