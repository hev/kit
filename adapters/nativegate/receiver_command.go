package nativegate

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	a "github.com/hev/factory/pkg/nativeadmission"
	n "github.com/hev/kit/pkg/nativejob"
)

// ReceiverConfig is installed by the authenticated receiver owner, never read
// from a model action. Provenance maps this one enrolled journal to its existing
// workspace/parent and admission journals. Only credential references are stored.
type ReceiverConfig struct {
	Version          string         `json:"version"`
	JournalRoot      string         `json:"journal_root"`
	Enrollment       n.Enrollment   `json:"enrollment"`
	JournalReference string         `json:"journal_reference,omitempty"`
	Workspace        a.Workspace    `json:"workspace,omitempty"`
	Parent           a.Parent       `json:"parent,omitempty"`
	BusinessDir      string         `json:"business_dir,omitempty"`
	MaintenanceDir   string         `json:"maintenance_dir,omitempty"`
	MaintenanceOwner a.Lease        `json:"maintenance_owner,omitempty"`
	Connection       a.DBReferences `json:"connection,omitempty"`
	TokenFile        string         `json:"token_file,omitempty"`
	TimeoutSeconds   int            `json:"timeout_seconds"`
}

func DecodeReceiverConfig(r io.Reader) (ReceiverConfig, error) {
	var c ReceiverConfig
	if _, err := decodeExact(r, &c); err != nil {
		return c, ErrReceiverCall
	}
	if c.Version != ReceiverVersion || !canonicalPath(c.JournalRoot) || c.Enrollment.HostID == "" || c.Enrollment.ExecutorID == "" || c.TimeoutSeconds < 1 || c.TimeoutSeconds > 45 {
		return c, ErrReceiverCall
	}
	if c.TokenFile != "" && !canonicalPath(c.TokenFile) {
		return c, ErrReceiverCall
	}
	return c, nil
}
func canonicalPath(p string) bool { return filepath.IsAbs(p) && filepath.Clean(p) == p }

// LoadReceiverConfig requires an explicit private existing file. No cwd,
// environment DSN, default enrollment or discovered journal is selected.
func LoadReceiverConfig(path string) (ReceiverConfig, error) {
	if !canonicalPath(path) {
		return ReceiverConfig{}, ErrReceiverCall
	}
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return ReceiverConfig{}, ErrReceiverCall
	}
	f, err := os.Open(path)
	if err != nil {
		return ReceiverConfig{}, ErrReceiverCall
	}
	defer f.Close()
	return DecodeReceiverConfig(f)
}

type pinnedAuthority struct{ check a.Check }

func (p pinnedAuthority) Current(context.Context, string) (a.OrderState, error) {
	return a.OrderState{}, a.ErrHeld
}
func (p pinnedAuthority) Authorize(ctx context.Context, c a.Check) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c != p.check {
		return a.ErrBinding
	}
	return nil
}

// RunReceiver handles one explicit local-owner request. Start is synchronous:
// it waits outside admission locks for completion. The executable enforces its
// hard process lifetime; on timeout inspect/recover the original journal instead
// of retrying. This is not a detached supervisor or a scheduler.
func RunReceiver(ctx context.Context, cfg ReceiverConfig, call ReceiverCall) (Inspection, error) {
	var empty Inspection
	if cfg.Version != ReceiverVersion || !canonicalPath(cfg.JournalRoot) || call.Version != ReceiverVersion || call.JobID == "" {
		return empty, ErrReceiverCall
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	// Opening a missing journal would create an empty store. This route only
	// consumes the owner's existing original journal, never initializes one.
	st, err := os.Lstat(filepath.Join(cfg.JournalRoot, "journal.sqlite"))
	if err != nil || !st.Mode().IsRegular() {
		return empty, ErrReceiverCall
	}
	store, err := n.Open(cfg.JournalRoot, cfg.Enrollment)
	if err != nil {
		return empty, err
	}
	defer store.Close()
	switch call.Operation {
	case "inspect":
		if call.Claim != nil || call.Binding != nil || call.Identity != nil || call.EventID != "" || call.ResultDigest != "" {
			return empty, ErrReceiverCall
		}
	case "ack":
		if call.Identity == nil || call.Claim != nil || call.Binding != nil {
			return empty, ErrReceiverCall
		}
		if err = Acknowledge(ctx, store, call.JobID, *call.Identity, call.EventID, call.ResultDigest); err != nil {
			return empty, err
		}
	case "start":
		if call.Claim == nil || call.Binding == nil || call.Binding.JobID != call.JobID || call.Identity != nil || call.EventID != "" || call.ResultDigest != "" {
			return empty, ErrReceiverCall
		}
		j, err := store.Query(call.JobID)
		if err != nil {
			return empty, err
		}
		if j.ID != call.JobID || j.State != "Pending" || j.Request.StartAuthority == nil {
			return empty, a.ErrBinding
		}
		i := j.Request.Identity
		fence, err := store.IssuedFence(i.OrderUID, i.ChefID, i.Fence)
		if err != nil {
			return empty, err
		}
		p := Provenance{Workspace: cfg.Workspace, Parent: cfg.Parent, OriginalFence: fence, JournalReference: cfg.JournalReference}
		check, err := MapCheck(n.StartCheck{Version: n.StartVersion, JobID: j.ID, Request: j.Request}, p)
		if err != nil {
			return empty, err
		}
		// SQL authenticates the complete original request digest and canonical
		// generation/scope. Trusted configuration supplies workspace/parent access;
		// actual business and maintenance guards verify that provenance at spawn.
		binding := call.Binding
		if binding.OrderUID != check.Identity.OrderUID || binding.ScopeDigest != check.Identity.ScopeDigest || binding.RequestDigest != check.PayloadDigest || binding.Generation < 1 {
			return empty, a.ErrBinding
		}
		authority := pinnedAuthority{check: check}
		pool, err := a.OpenDB(ctx, cfg.Connection, a.DBOPResolver{TokenFile: cfg.TokenFile})
		if err != nil {
			return empty, errors.New("native receiver database unavailable")
		}
		defer pool.Close()
		facade, err := a.NewDB(a.DBConfig{Pool: pool, BusinessDir: cfg.BusinessDir, MaintenanceDir: cfg.MaintenanceDir, MaintenanceOwner: cfg.MaintenanceOwner, Authority: authority})
		if err != nil {
			return empty, err
		}
		if _, err = Start(ctx, store, facade, *call.Claim, *call.Binding, p); err != nil {
			return empty, err
		}
	default:
		return empty, ErrReceiverCall
	}
	return Inspect(ctx, store, call.JobID)
}
