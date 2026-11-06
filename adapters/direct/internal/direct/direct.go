// Package direct is an explicit-assignment coordination client. The transport is
// an owner-selected executable; it consumes SQL on stdin and returns rows JSON.
package direct

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed schema.sql
var Schema string

type Client struct{ Transport string }
type Row map[string]any

func (c Client) Query(ctx context.Context, sql string) ([]Row, error) {
	if !filepath.IsAbs(c.Transport) {
		return nil, errors.New("select an absolute SQL transport executable")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Transport)
	cmd.Stdin = strings.NewReader(sql)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, errors.New("coordination transport failed; inspect transport health before retry")
	}
	var reply struct {
		Rows   []Row  `json:"rows"`
		Status string `json:"status"`
	}
	if json.Unmarshal(stdout.Bytes(), &reply) != nil || reply.Status != "ok" {
		return nil, errors.New("invalid coordination transport response")
	}
	return reply.Rows, nil
}
func literal(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
func (c Client) Change(ctx context.Context, action string, p Row) (Row, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	rows, err := c.Query(ctx, "SELECT hev_03.change("+literal(action)+","+literal(string(b))+"::jsonb) AS record")
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, errors.New("mutation returned no record")
	}
	r, ok := rows[0]["record"].(map[string]any)
	if !ok {
		return nil, errors.New("mutation returned invalid record")
	}
	return r, nil
}
func (c Client) List(ctx context.Context, resource, id string) ([]Row, error) {
	var q string
	switch resource {
	case "orders":
		q = "SELECT to_jsonb(x) AS record FROM hev_03.orders x"
	case "runs":
		q = "SELECT to_jsonb(x) AS record FROM hev_03.runs x"
	case "events":
		q = "SELECT to_jsonb(x) AS record FROM hev_03.events x"
	default:
		return nil, errors.New("unsupported resource")
	}
	if id != "" {
		if resource == "events" {
			q += " WHERE resource=" + literal(id)
		} else {
			q += " WHERE id=" + literal(id)
		}
	}
	if resource == "events" {
		q += " ORDER BY sequence DESC LIMIT 100"
	} else {
		q += " ORDER BY id LIMIT 100"
	}
	rows, err := c.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	out := []Row{}
	for _, r := range rows {
		v, ok := r["record"].(map[string]any)
		if !ok {
			return nil, errors.New("invalid record")
		}
		out = append(out, v)
	}
	return out, nil
}
func ID(kind string, value any) string {
	b, _ := json.Marshal(value)
	h := sha256.Sum256(b)
	return kind + "-" + hex.EncodeToString(h[:16])
}
