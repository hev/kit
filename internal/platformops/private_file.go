package platformops

import (
	"errors"
	"io"
)

var errPrivateFile = errors.New("expected a protected regular file")

// ReadPrivateFile checks the opened object before reading any bytes. Errors
// intentionally carry neither the path nor the file contents.
func ReadPrivateFile(path string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, errPrivateFile
	}
	file, err := openPrivateFile(path)
	if err != nil {
		return nil, errPrivateFile
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, errPrivateFile
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes {
		return nil, errPrivateFile
	}
	return data, nil
}
