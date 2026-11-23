package foh

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
)

// MaxFavorites bounds the favorites file.
const MaxFavorites = 200

// FavoritesPath is ~/.hev/foh-favorites.json.
func FavoritesPath() string { return filepath.Join(homeDir(), ".hev", "foh-favorites.json") }

// Favorites is the starred model list, in the order they were starred.
type Favorites struct {
	Path string
	IDs  []string
}

// LoadFavorites reads path (FavoritesPath when empty). A missing or invalid
// file is an empty list.
func LoadFavorites(path string) *Favorites {
	if path == "" {
		path = FavoritesPath()
	}
	f := &Favorites{Path: path}
	file, err := os.Open(path)
	if err != nil {
		return f
	}
	defer file.Close()
	var doc struct {
		Models []string `json:"models"`
	}
	if json.NewDecoder(io.LimitReader(file, 1<<20)).Decode(&doc) == nil {
		for _, id := range doc.Models {
			if id != "" && !slices.Contains(f.IDs, id) && len(f.IDs) < MaxFavorites {
				f.IDs = append(f.IDs, id)
			}
		}
	}
	return f
}

// Has reports whether id is starred.
func (f *Favorites) Has(id string) bool { return slices.Contains(f.IDs, id) }

// Toggle stars or unstars id, persists the list, and reports the new state.
func (f *Favorites) Toggle(id string) (bool, error) {
	if i := slices.Index(f.IDs, id); i >= 0 {
		f.IDs = slices.Delete(f.IDs, i, i+1)
		return false, f.save()
	}
	f.IDs = append(f.IDs, id)
	if len(f.IDs) > MaxFavorites {
		f.IDs = f.IDs[len(f.IDs)-MaxFavorites:]
	}
	return true, f.save()
}

func (f *Favorites) save() error {
	return writeFileAtomic(f.Path, struct {
		Models []string `json:"models"`
	}{append([]string{}, f.IDs...)})
}
