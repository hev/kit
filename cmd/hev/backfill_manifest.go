package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/trace"
	"sort"
)

const backfillFormat = "session-projections-go-json-v2"
const backfillPolicy = "session-linkage-outcomes-v2"

type backfillTarget struct {
	Endpoint  string                     `json:"endpoint"`
	Store     string                     `json:"store"`
	Account   string                     `json:"account"` // Owner-supplied nonsecret identifier, not authentication proof.
	Namespace string                     `json:"namespace"`
	Schema    map[string]json.RawMessage `json:"schema"`
}

type backfillPublisher struct {
	Host              string  `json:"host"`
	BinaryRevision    *string `json:"binary_revision"`
	ObservedAt        string  `json:"observed_at"`
	EvidenceReference string  `json:"evidence_reference"`
}

// Provenance declarations bind receipts without asserting source/account/fence
// verification. Contract revisions are distinct from running publisher revisions.
type backfillProvenance struct {
	EvidenceStatus             string              `json:"evidence_status"`
	SourceRevision             *string             `json:"source_revision"`
	CaptureRevision            *string             `json:"capture_revision"`
	EnrichmentRevision         *string             `json:"enrichment_revision"`
	HighWater                  *string             `json:"high_water"`
	Publishers                 []backfillPublisher `json:"publishers"`
	PublisherInventoryComplete bool                `json:"publisher_inventory_complete"`
	ContractReferences         map[string]string   `json:"contract_references"`
}

type backfillManifest struct {
	Provenance         *backfillProvenance             `json:"provenance,omitempty"`
	Format             string                          `json:"format"`
	Policy             string                          `json:"policy"`
	Target             backfillTarget                  `json:"target"`
	Since              int64                           `json:"since"`
	Until              int64                           `json:"until"`
	After              string                          `json:"export_after"`
	Complete           bool                            `json:"export_complete"`
	SnapshotConsistent bool                            `json:"snapshot_consistent"`
	Rows               []trace.SessionRow              `json:"rows"`
	Fingerprints       map[string]backfillFingerprints `json:"fingerprints"`
}

func backfillHash(v any) string {
	b, _ := json.Marshal(v)
	var normalized any
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	if decoder.Decode(&normalized) == nil {
		b, _ = json.Marshal(normalized)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func (m *backfillManifest) validate() error {
	if m.Provenance != nil && (m.Provenance.EvidenceStatus != "declared-unverified" || m.Provenance.PublisherInventoryComplete) {
		return fmt.Errorf("this format cannot verify declared provenance or complete publisher coverage")
	}
	if m.SnapshotConsistent {
		return fmt.Errorf("this export format cannot claim snapshot consistency")
	}
	if m.Format != backfillFormat || m.Policy != backfillPolicy {
		return fmt.Errorf("unsupported manifest format or policy")
	}
	if m.Since <= 0 || m.Until < m.Since {
		return fmt.Errorf("invalid fixed manifest UTC interval")
	}
	if m.Target.Endpoint == "" || m.Target.Namespace == "" || m.Target.Store == "" || m.Target.Account == "" || m.Target.Schema == nil {
		return fmt.Errorf("manifest requires endpoint/store/nonsecret account/namespace/schema")
	}
	sort.Slice(m.Rows, func(i, j int) bool { return m.Rows[i].ID < m.Rows[j].ID })
	for i, r := range m.Rows {
		if r.ID == "" || (i > 0 && m.Rows[i-1].ID == r.ID) {
			return fmt.Errorf("invalid/duplicate cohort ID")
		}
		if r.End < m.Since || r.Start > m.Until {
			return fmt.Errorf("row outside fixed cohort interval")
		}
	}
	if len(m.Fingerprints) != len(m.Rows) {
		return fmt.Errorf("manifest source fingerprints incomplete")
	}
	for _, r := range m.Rows {
		if m.Fingerprints[r.ID] != fingerprintBackfill(r) {
			return fmt.Errorf("manifest fingerprint mismatch for %s", r.ID)
		}
	}
	return nil
}

type backfillFingerprints struct {
	Analyzer   string `json:"analyzer"`
	Preserved  string `json:"preserved"`
	Enrichment string `json:"enrichment"`
}

func fingerprintBackfill(r trace.SessionRow) backfillFingerprints {
	return backfillFingerprints{backfillSourceHash(r), backfillPreservedHash(r), backfillEnrichmentHash(r)}
}
func (m *backfillManifest) seal() {
	m.Fingerprints = map[string]backfillFingerprints{}
	for _, r := range m.Rows {
		m.Fingerprints[r.ID] = fingerprintBackfill(r)
	}
}

// Analyzer inputs have their own revision, unaffected by outcome observations.
func backfillSourceHash(r trace.SessionRow) string {
	return backfillHash(struct {
		ID        string `json:"id"`
		SessionID string `json:"session_id"`
		Harness   string `json:"harness"`
		Host      string `json:"host"`
		RepoURL   string `json:"repo_url"`
		Start     int64  `json:"start"`
		End       int64  `json:"end"`
		ToolCount int64  `json:"tool_count"`
	}{r.ID, r.SessionID, r.Harness, r.Host, r.RepoURL, r.Start, r.End, r.ToolCount})
}

func backfillEnrichmentHash(r trace.SessionRow) string {
	return backfillHash(struct {
		Outcomes trace.SessionOutcomes `json:"outcomes"`
		Workdir  string                `json:"workdir"`
		Branch   string                `json:"branch"`
		PR       string                `json:"pr"`
		Commits  trace.StringList      `json:"commits"`
	}{r.SessionOutcomes, r.Workdir, r.Branch, r.PR, r.Commits})
}

type backfillReconciliation struct {
	BaselineState        string   `json:"baseline_state"`
	ComparisonComplete   bool     `json:"comparison_complete"`
	PreservationVerified bool     `json:"preservation_verified"`
	EvidenceAffected     bool     `json:"evidence_affected"`
	Added                []string `json:"added"`
	Removed              []string `json:"removed"`
	SourceChanged        []string `json:"source_changed"`
	PreservedChanged     []string `json:"preserved_changed"`
	EnrichmentChanged    []string `json:"enrichment_changed"`
	ProvenanceChanged    bool     `json:"provenance_changed"`
	SchemaChanged        bool     `json:"schema_changed"`
	SnapshotConsistent   bool     `json:"snapshot_consistent"`
}

func reconcileBackfill(before, after backfillManifest) (backfillReconciliation, error) {
	out := backfillReconciliation{BaselineState: "not_compared"}
	if err := before.validate(); err != nil {
		return out, err
	}
	if err := after.validate(); err != nil {
		return out, err
	}
	if !before.Complete || !after.Complete {
		return out, fmt.Errorf("reconciliation requires two complete ID-zero enumerations")
	}
	a, b := before.Target, after.Target
	a.Schema = nil
	b.Schema = nil
	if backfillHash(a) != backfillHash(b) || before.Since != after.Since || before.Until != after.Until {
		return out, fmt.Errorf("reconciliation target/selection mismatch")
	}
	old := map[string]trace.SessionRow{}
	newRows := map[string]trace.SessionRow{}
	for _, r := range before.Rows {
		old[r.ID] = r
	}
	for _, r := range after.Rows {
		newRows[r.ID] = r
	}
	for _, r := range after.Rows {
		previous, ok := old[r.ID]
		if !ok {
			out.Added = append(out.Added, r.ID)
			continue
		}
		if backfillSourceHash(previous) != backfillSourceHash(r) {
			out.SourceChanged = append(out.SourceChanged, r.ID)
		}
		if backfillPreservedHash(previous) != backfillPreservedHash(r) {
			out.PreservedChanged = append(out.PreservedChanged, r.ID)
		}
		if backfillEnrichmentHash(previous) != backfillEnrichmentHash(r) {
			out.EnrichmentChanged = append(out.EnrichmentChanged, r.ID)
		}
	}
	for _, r := range before.Rows {
		if _, ok := newRows[r.ID]; !ok {
			out.Removed = append(out.Removed, r.ID)
		}
	}
	out.ProvenanceChanged = backfillHash(before.Provenance) != backfillHash(after.Provenance)
	out.SchemaChanged = backfillHash(before.Target.Schema) != backfillHash(after.Target.Schema)
	out.BaselineState = "observed_complete_projection"
	out.ComparisonComplete = true
	out.EvidenceAffected = len(out.Added)+len(out.Removed)+len(out.SourceChanged)+len(out.PreservedChanged) > 0 || out.ProvenanceChanged || out.SchemaChanged
	// Matching scans are observed stability, never verified preservation or a transactional snapshot.
	return out, nil
}

// The exported preservation projection additionally covers summaries and all
// known non-enrichment session attributes. Prompts/vectors and raw blocks are
// excluded from reads and are never written by this replay.
func backfillPreservedHash(r trace.SessionRow) string {
	r.SessionOutcomes = trace.SessionOutcomes{}
	r.Workdir = ""
	r.Branch = ""
	r.PR = ""
	r.Commits = nil
	return backfillHash(r)
}
