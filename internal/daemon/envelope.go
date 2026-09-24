package daemon

import (
	"bufio"
	"bytes"
	"encoding/json"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// TraceEnvelope is the raw envelope written to S3-compatible object storage.
type TraceEnvelope struct {
	IngestID           string  `json:"ingest_id"`
	IngestTS           string  `json:"ingest_ts"`
	IngestSource       string  `json:"ingest_source"`
	IngestAgent        string  `json:"ingest_agent,omitempty"`
	IngestAgentVersion string  `json:"ingest_agent_version,omitempty"`
	SourceTS           string  `json:"source_ts"`
	Host               string  `json:"host"`
	GHUser             string  `json:"gh_user,omitempty"`
	Harness            string  `json:"harness"`
	HarnessVersion     *string `json:"harness_version"`
	SessionID          string  `json:"session_id"`
	EventKind          string  `json:"event_kind"`
	SignalType         string  `json:"signal_type"`
	PayloadRaw         string  `json:"payload_raw"`
	PayloadSchemaRef   string  `json:"payload_schema_ref"`
	Resource           *string `json:"resource,omitempty"`
	Repo               string  `json:"repo,omitempty"`
	Branch             string  `json:"branch,omitempty"`
}

const ingestAgent = "hev-kit"

// ingestAgentVersion is the short build SHA Go embeds via vcs.revision when
// the binary is built from inside a git working tree. Marked dirty when the
// tree had uncommitted changes at build time. Cached because ReadBuildInfo
// walks the binary header.
var (
	ingestAgentVersionOnce sync.Once
	ingestAgentVersionVal  string
)

func IngestAgentVersion() string {
	ingestAgentVersionOnce.Do(func() {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}
		var revision string
		var modified bool
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				revision = s.Value
			case "vcs.modified":
				modified = s.Value == "true"
			}
		}
		if revision == "" {
			return
		}
		if len(revision) > 7 {
			revision = revision[:7]
		}
		if modified {
			revision += "-dirty"
		}
		ingestAgentVersionVal = revision
	})
	return ingestAgentVersionVal
}

// WrapClaudeRawBody builds an envelope around a raw Anthropic Messages API
// request/response body emitted by Claude Code's raw-body OTel logging.
func WrapClaudeRawBody(tf RawBodyFile, host, ghUser string) (*TraceEnvelope, error) {
	name := filepath.Base(tf.Path)
	bodyID := strings.TrimSuffix(name, ".json")

	resource := map[string]string{
		"artifact.path": tf.Path,
		"artifact.kind": "claude_raw_api_body",
		"body.file":     name,
		"body.kind":     tf.Kind,
	}
	addResource(resource, "gh.user", ghUser)
	resourceJSON, _ := json.Marshal(resource)

	eventKind := "api_body_file"
	schemaRef := "anthropic.messages.raw_body.v1"
	if tf.Kind != "" {
		eventKind = "api_" + tf.Kind + "_body_file"
		schemaRef = "anthropic.messages." + tf.Kind + ".raw_body.v1"
	}
	resourceRaw := string(resourceJSON)

	env := &TraceEnvelope{
		IngestID:           ulid.Make().String(),
		IngestTS:           time.Now().UTC().Format(time.RFC3339),
		IngestSource:       "claude_raw_api_body_file",
		IngestAgent:        ingestAgent,
		IngestAgentVersion: IngestAgentVersion(),
		SourceTS:           tf.ModTime.UTC().Format(time.RFC3339Nano),
		Host:               host,
		GHUser:             ghUser,
		Harness:            "claude_code",
		SessionID:          bodyID,
		EventKind:          eventKind,
		SignalType:         "log",
		PayloadRaw:         string(tf.Raw),
		PayloadSchemaRef:   schemaRef,
		Resource:           &resourceRaw,
	}
	return env, nil
}

type codexRolloutMeta struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		ID            string `json:"id"`
		Timestamp     string `json:"timestamp"`
		CWD           string `json:"cwd"`
		Originator    string `json:"originator"`
		CLIVersion    string `json:"cli_version"`
		Source        string `json:"source"`
		ThreadSource  string `json:"thread_source"`
		ModelProvider string `json:"model_provider"`
	} `json:"payload"`
	Git struct {
		CommitHash    string `json:"commit_hash"`
		Branch        string `json:"branch"`
		RepositoryURL string `json:"repository_url"`
	} `json:"git"`
}

// WrapCodexRollout builds an envelope around a Codex CLI rollout JSONL file.
func WrapCodexRollout(tf RolloutFile, host, ghUser string) (*TraceEnvelope, error) {
	name := filepath.Base(tf.Path)
	meta := parseCodexRolloutMeta(tf.Raw)
	sessionID := meta.Payload.ID
	if sessionID == "" {
		sessionID = strings.TrimSuffix(name, ".jsonl")
	}

	sourceTS := firstNonEmpty(meta.Payload.Timestamp, meta.Timestamp)
	if sourceTS == "" {
		sourceTS = tf.ModTime.UTC().Format(time.RFC3339Nano)
	}

	version := codexHarnessVersion(meta.Payload.CLIVersion)
	schemaVersion := "v1"
	if meta.Payload.CLIVersion != "" {
		schemaVersion = "v" + strings.TrimPrefix(meta.Payload.CLIVersion, "codex-")
	}

	resource := map[string]string{
		"artifact.path": tf.Path,
		"artifact.kind": "codex_rollout_jsonl",
		"rollout.file":  name,
	}
	addResource(resource, "codex.cwd", meta.Payload.CWD)
	addResource(resource, "codex.originator", meta.Payload.Originator)
	addResource(resource, "codex.source", meta.Payload.Source)
	addResource(resource, "codex.thread_source", meta.Payload.ThreadSource)
	addResource(resource, "codex.model_provider", meta.Payload.ModelProvider)
	addResource(resource, "codex.cli_version", meta.Payload.CLIVersion)
	addResource(resource, "git.commit_hash", meta.Git.CommitHash)
	addResource(resource, "git.repository_url", meta.Git.RepositoryURL)
	addResource(resource, "gh.user", ghUser)
	resourceJSON, _ := json.Marshal(resource)
	resourceRaw := string(resourceJSON)

	env := &TraceEnvelope{
		IngestID:           ulid.Make().String(),
		IngestTS:           time.Now().UTC().Format(time.RFC3339),
		IngestSource:       "codex_rollout_jsonl",
		IngestAgent:        ingestAgent,
		IngestAgentVersion: IngestAgentVersion(),
		SourceTS:           sourceTS,
		Host:               host,
		GHUser:             ghUser,
		Harness:            "codex_cli",
		HarnessVersion:     version,
		SessionID:          sessionID,
		EventKind:          "session_rollout_file",
		SignalType:         "log",
		PayloadRaw:         string(tf.Raw),
		PayloadSchemaRef:   "codex.rollout_jsonl." + schemaVersion,
		Resource:           &resourceRaw,
		Repo:               meta.Git.RepositoryURL,
		Branch:             meta.Git.Branch,
	}
	return env, nil
}

func parseCodexRolloutMeta(raw []byte) codexRolloutMeta {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var meta codexRolloutMeta
		if err := json.Unmarshal(line, &meta); err != nil {
			continue
		}
		if meta.Type == "session_meta" {
			return meta
		}
	}
	return codexRolloutMeta{}
}

func codexHarnessVersion(cliVersion string) *string {
	if cliVersion == "" {
		return nil
	}
	version := cliVersion
	if !strings.HasPrefix(version, "codex-") {
		version = "codex-" + version
	}
	return &version
}

func addResource(resource map[string]string, key, value string) {
	if value != "" {
		resource[key] = value
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// PartitionDateFromTime returns the YYYY-MM-DD date for S3 partitioning.
func PartitionDateFromTime(t time.Time) string {
	if t.IsZero() {
		return time.Now().UTC().Format("2006-01-02")
	}
	return t.UTC().Format("2006-01-02")
}
