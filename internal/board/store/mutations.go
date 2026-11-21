package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/internal/platformops"
)

// ErrLocked remains an invalid request for legacy board callers.
var ErrLocked = invalid("thread is locked")
var ErrForbidden = errors.New("post belongs to another author")

type Edit struct {
	Title  *string
	Body   *string
	Tags   *[]string
	Pinned *bool
	Locked *bool
}

type Editor interface {
	EditThread(context.Context, int64, string, bool, Edit) error
	DeleteThread(context.Context, int64, string, bool) error
}

func (e Edit) content() bool { return e.Title != nil || e.Body != nil || e.Tags != nil }

func (e Edit) apply(title, body *string, tags *[]string) error {
	if e.Title != nil {
		*title = strings.TrimSpace(*e.Title)
	}
	if e.Body != nil {
		*body = strings.TrimSpace(*e.Body)
	}
	if *title == "" || *body == "" {
		return invalid("a post needs a title and body")
	}
	if e.Tags != nil {
		*tags = cleanTags(*e.Tags)
	}
	return nil
}

func (s *layerStore) EditThread(ctx context.Context, id int64, author string, admin bool, edit Edit) error {
	t, err := s.Thread(ctx, id)
	if err != nil {
		return err
	}
	if edit.content() && !admin && t.Author != author {
		return ErrForbidden
	}
	row := layer.Row{"id": fmt.Sprintf("t:%d", id)}
	if edit.Pinned != nil {
		row["pinned"] = *edit.Pinned
	}
	if edit.Locked != nil {
		row["locked"] = *edit.Locked
	}
	if !edit.content() {
		return s.cl.WithContext(ctx).Patch([]layer.Row{row})
	}
	if err := edit.apply(&t.Title, &t.body, &t.Tags); err != nil {
		return err
	}
	// Layer embeds text on upsert. Preserve the entire existing row,
	// including activity timestamps, votes and attachments.
	rows, err := s.query(ctx, []any{"id", "Eq", row["id"]}, nil, 1, s.threadAttrs())
	if err != nil {
		return err
	}
	if len(rows) == 0 || str(rows[0], "kind") != "thread" {
		return ErrNotFound
	}
	for k, v := range rows[0] {
		if _, set := row[k]; !set && v != nil && !strings.HasPrefix(k, "$") {
			row[k] = v
		}
	}
	row["n"] = 1
	row["title"], row["body"], row["tags"], row["text"] = t.Title, t.body, t.Tags, t.Title+"\n\n"+t.body
	if s.app {
		tags, _ := json.Marshal(t.Tags)
		row["tags"] = string(tags)
	}
	return s.cl.WithContext(ctx).Upsert([]layer.Row{row})
}

func (s *layerStore) DeleteThread(ctx context.Context, id int64, author string, admin bool) error {
	client := s.cl.WithContext(ctx)
	key := fmt.Sprintf("t:%d", id)
	rows, err := s.query(ctx, []any{"id", "Eq", key}, nil, 1, []string{"kind", "author"})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrNotFound
	}
	kind, owner := str(rows[0], "kind"), str(rows[0], "author")
	if kind != "thread" && kind != "deleting" {
		return ErrNotFound
	}
	if !admin && owner != author {
		return ErrForbidden
	}
	if kind == "thread" {
		if err := client.Upsert([]layer.Row{{"id": key, "kind": "deleting", "thread_id": id, "author": owner}}); err != nil {
			return err
		}
	}
	if err := client.DeleteWhere(and([]any{"thread_id", "Eq", id}, []any{"kind", "Eq", "post"})); err != nil {
		return err
	}
	return client.Upsert([]layer.Row{{"id": key, "kind": "deleted", "thread_id": id}})
}

func (f *fileStore) EditThread(_ context.Context, id int64, author string, admin bool, edit Edit) error {
	return f.locked(func() error {
		op, err := readPost(f.threadPath(id))
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, errSkip) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if edit.content() {
			if !admin && op.Author != author {
				return ErrForbidden
			}
			if err := edit.apply(&op.Title, &op.Body, &op.Tags); err != nil {
				return err
			}
			if err := writeOpeningPost(f.threadPath(id), *op); err != nil {
				return err
			}
		}
		if edit.Pinned == nil && edit.Locked == nil {
			return nil
		}
		flags, err := f.readFlags(id)
		if err != nil {
			return err
		}
		if edit.Pinned != nil {
			flags.Pinned = *edit.Pinned
		}
		if edit.Locked != nil {
			flags.Locked = *edit.Locked
		}
		return f.writeFlags(id, flags)
	})
}

func (f *fileStore) DeleteThread(_ context.Context, id int64, author string, admin bool) error {
	return f.locked(func() error {
		op, err := readPost(f.threadPath(id))
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, errSkip) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !admin && op.Author != author {
			return ErrForbidden
		}
		if err := writeOpeningPost(f.threadPath(id), filePost{ID: id, Deleted: true}); err != nil {
			return err
		}
		return os.RemoveAll(f.replyDir(id))
	})
}

func writeOpeningPost(path string, op filePost) error {
	raw, err := json.Marshal(op)
	if err != nil {
		return err
	}
	return platformops.WriteFileAtomic(path, append(raw, '\n'), 0o600)
}

func sortThreads(threads []Thread, pinnedFirst bool) {
	sort.Slice(threads, func(i, j int) bool {
		a, b := threads[i], threads[j]
		if pinnedFirst && a.Pinned != b.Pinned {
			return a.Pinned
		}
		if !a.Updated.Equal(b.Updated) {
			return a.Updated.After(b.Updated)
		}
		return a.ID > b.ID
	})
}

func hasTags(have, want []string) bool {
	for _, tag := range want {
		if !hasTag(have, tag) {
			return false
		}
	}
	return true
}
