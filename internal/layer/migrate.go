package layer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/trace"
)

// SessionMigrationFile, under ~/.hev, holds the session rows of a migration in
// progress.
const SessionMigrationFile = "sessions-migration.json"

// MigrateSessionLists rebuilds a sessions namespace whose list columns are
// declared as strings: the one kit v0.3.0 wrote on Layer 0.7.0's Postgres,
// which had no array types. The store will not change an attribute's type in
// place (a 400, "incompatible schema change for tool_names"), so on such a
// namespace every session write this kit makes fails, and the dashboard's
// ContainsAny on tool_names is a 400 too.
//
// The rows are read back whole (StringList and UintList read the string form),
// kept in backup, and the namespace is deleted and written again with array
// types. Nothing is re-parsed and nothing is embedded: the chunk and block
// namespaces have no list columns and are left alone, and a session whose
// transcript has since aged off disk keeps its row. A migration stopped after
// the delete resumes from backup on the next call. It returns the number of
// rows rewritten, zero when there was nothing to do.
//
// The caller stops every other writer first: an older daemon writing string
// lists between the read and the delete would recreate the old schema. `hev up`
// stops hevd before it calls this, and hevd calls it before each scan, so an
// upgrade needs no command of its own. A Postgres gateway older than 0.7.1 is
// never migrated: it would take the delete and then refuse the array rewrite.
func (c *Client) MigrateSessionLists(backup string) (int, error) {
	unlock, err := redact.LockArchive()
	if err != nil {
		return 0, err
	}
	defer unlock()

	if !c.Caps.Arrays() || !c.Caps.ReadSide() {
		return 0, nil
	}
	scrubber, err := redact.Load()
	if err != nil {
		return 0, err
	}
	rows, err := readSessionBackup(backup)
	if err != nil {
		return 0, err
	}
	legacy, err := c.SessionListsLegacy()
	if err != nil {
		return 0, err
	}
	if legacy || rows != nil {
		if ok, err := c.servesPostgresArrays(); err != nil || !ok {
			return 0, err
		}
	}
	if legacy {
		current, err := c.ListSessionRows(-1, nil)
		if err != nil {
			return 0, fmt.Errorf("read sessions to migrate: %w", err)
		}
		rows = mergeSessionRows(rows, current)
		if err := writeSessionBackup(backup, rows); err != nil {
			return 0, err
		}
		if err := c.do("DELETE", "/v2/namespaces/"+c.Namespace+"-sessions", nil, nil); err != nil && !isNamespaceMissing(err) {
			return 0, fmt.Errorf("delete the string-typed sessions namespace: %w", err)
		}
	} else if rows == nil {
		return 0, nil
	}
	// An interrupted older schema migration may retain raw prompts/titles.
	// Scrub before replay so it cannot resurrect them after archive cleanup.
	for i := range rows {
		rows[i].FirstPrompt, _ = scrubber.Text(rows[i].FirstPrompt)
		rows[i].Summary, _ = scrubber.Text(rows[i].Summary)
	}
	for start := 0; start < len(rows); start += 200 {
		end := min(start+200, len(rows))
		if _, err := c.WriteSessions(rows[start:end]); err != nil {
			return 0, fmt.Errorf("rewrite sessions with array types (%s keeps them; run `hev up` again): %w", backup, err)
		}
	}
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	return len(rows), nil
}

// servesPostgresArrays reports whether the gateway is new enough to take array
// attributes on Postgres (0.7.1, LYR-138). Other stores always had arrays.
func (c *Client) servesPostgresArrays() (bool, error) {
	return c.postgresGatewayAtLeast(7, 1)
}

// postgresGatewayAtLeast reports whether a Postgres gateway's /health version
// is at least 0.minor.patch; the edge mirror's -dev versions count as their
// release. A version it cannot read, like a hand-built gateway's, is not a
// yes. Other stores are always a yes: the version gates only what the
// pgvector row gained in a given release.
func (c *Client) postgresGatewayAtLeast(minor, patch int) (bool, error) {
	return c.postgresGatewayAtLeastContext(context.Background(), minor, patch)
}
func (c *Client) postgresGatewayAtLeastContext(ctx context.Context, minor, patch int) (bool, error) {
	if c.Caps.Store.Kind != StorePgvector {
		return true, nil
	}
	h, err := c.healthContext(ctx)
	if err != nil {
		return false, err
	}
	var major, gotMinor, gotPatch int
	if _, err := fmt.Sscanf(h.Version, "%d.%d.%d", &major, &gotMinor, &gotPatch); err != nil {
		return false, nil
	}
	return major > 0 || gotMinor > minor || (gotMinor == minor && gotPatch >= patch), nil
}

// SessionListsLegacy reads the sessions schema and reports whether a list
// column is declared as a string. A namespace not written yet is not legacy.
func (c *Client) SessionListsLegacy() (bool, error) {
	var schema map[string]json.RawMessage
	if err := c.do("GET", "/v1/namespaces/"+c.Namespace+"-sessions/schema", nil, &schema); err != nil {
		if isNamespaceMissing(err) {
			return false, nil
		}
		return false, fmt.Errorf("read the sessions schema: %w", err)
	}
	for _, field := range listColumns {
		var attr struct {
			Type string `json:"type"`
		}
		if raw, ok := schema[field]; ok && json.Unmarshal(raw, &attr) == nil && attr.Type == "string" {
			return true, nil
		}
	}
	return false, nil
}

// mergeSessionRows keeps one row per id, the one whose session ended later:
// the rule WriteSessions' upsert condition applies in the store.
func mergeSessionRows(a, b []trace.SessionRow) []trace.SessionRow {
	byID := make(map[string]int, len(a)+len(b))
	out := make([]trace.SessionRow, 0, len(a)+len(b))
	for _, row := range append(append([]trace.SessionRow{}, a...), b...) {
		if i, ok := byID[row.ID]; ok {
			if row.End >= out[i].End {
				out[i] = row
			}
			continue
		}
		byID[row.ID] = len(out)
		out = append(out, row)
	}
	return out
}

func readSessionBackup(path string) ([]trace.SessionRow, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rows []trace.SessionRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if rows == nil {
		rows = []trace.SessionRow{}
	}
	return rows, nil
}

func writeSessionBackup(path string, rows []trace.SessionRow) error {
	raw, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
