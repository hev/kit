package evidence

import (
	"context"

	"github.com/hev/kit/internal/hooks"
)

// ItemRef is an item as an evidence-* hook event carries it: metadata only,
// never the bytes or the text.
type ItemRef struct {
	ID         string `json:"id"`
	Seq        int    `json:"seq"`
	Modality   string `json:"modality"`
	Name       string `json:"name"`
	MIME       string `json:"mime"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	DurationMs *int64 `json:"duration_ms,omitempty"`
}

// Refs is the hook view of p's items.
func Refs(p *Package) []ItemRef {
	out := make([]ItemRef, 0, len(p.Items))
	for _, it := range p.Items {
		out = append(out, ItemRef{ID: it.ID, Seq: it.Seq, Modality: it.Modality, Name: it.Name, MIME: it.MIME,
			SHA256: it.SHA256, Size: it.Size, Width: it.Width, Height: it.Height, DurationMs: it.DurationMs})
	}
	return out
}

// HookObserver raises evidence-created, evidence-updated and evidence-sealed
// through kit's hook layer. Set it as Store.Observe.
func HookObserver(ctx context.Context) func(string, *Package, []string, []string) {
	return func(event string, p *Package, items, claims []string) {
		hooks.ObserveEvidence(ctx, event, hooks.EvidenceEvent{Package: p.Summarize(), Items: Refs(p), AddedItemIDs: items, AddedClaimIDs: claims})
	}
}
