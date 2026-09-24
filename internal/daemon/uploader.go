package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

type Uploader struct {
	client *minio.Client
	bucket string
	logger *slog.Logger
}

func NewUploader(ctx context.Context, cfg *Config, logger *slog.Logger) (*Uploader, error) {
	u, err := url.Parse(cfg.S3Endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}
	host := u.Host
	if host == "" {
		host = strings.TrimPrefix(strings.TrimPrefix(cfg.S3Endpoint, "http://"), "https://")
	}
	useSSL := u.Scheme == "https"

	client, err := minio.New(host, &minio.Options{
		Creds:  S3Credentials(cfg),
		Secure: useSSL,
		Region: cfg.S3Region,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	// Verify bucket exists or create it.
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	exists, err := client.BucketExists(pingCtx, cfg.S3Bucket)
	if err != nil {
		return nil, fmt.Errorf("ping bucket %s: %w", cfg.S3Bucket, err)
	}
	if !exists {
		if err := client.MakeBucket(pingCtx, cfg.S3Bucket, minio.MakeBucketOptions{Region: cfg.S3Region}); err != nil {
			return nil, fmt.Errorf("make bucket %s: %w", cfg.S3Bucket, err)
		}
		logger.Info("created bucket", "bucket", cfg.S3Bucket)
	}

	return &Uploader{client: client, bucket: cfg.S3Bucket, logger: logger}, nil
}

// UploadClaudeRawBody writes a raw API-body envelope under a source-specific
// prefix so multiple request/response files in a session cannot overwrite each
// other.
func (u *Uploader) UploadClaudeRawBody(ctx context.Context, envelope *TraceEnvelope, dt string, name string) (string, error) {
	base := strings.TrimSuffix(safeObjectName(name), ".json")
	key := fmt.Sprintf("raw/dt=%s/source=claude-raw-api-bodies/%s.json", dt, base)
	return u.uploadEnvelope(ctx, envelope, key)
}

// UploadCodexRollout writes a Codex rollout envelope under a source-specific
// prefix. Rollout files are append-only while a session is active, so later
// scans intentionally overwrite the same object when the hash changes.
func (u *Uploader) UploadCodexRollout(ctx context.Context, envelope *TraceEnvelope, dt string, name string) (string, error) {
	base := strings.TrimSuffix(safeObjectName(name), ".jsonl")
	key := fmt.Sprintf("raw/dt=%s/source=codex-rollouts/%s.json", dt, base)
	return u.uploadEnvelope(ctx, envelope, key)
}

func (u *Uploader) uploadEnvelope(ctx context.Context, envelope *TraceEnvelope, key string) (string, error) {
	body, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("marshal envelope: %w", err)
	}

	_, err = u.client.PutObject(ctx, u.bucket, key,
		bytes.NewReader(body), int64(len(body)),
		minio.PutObjectOptions{ContentType: "application/json"})
	if err != nil {
		return "", fmt.Errorf("put %s: %w", key, err)
	}

	return key, nil
}

func safeObjectName(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	name = replacer.Replace(name)
	if name == "" || name == "." || name == ".." {
		return "unknown"
	}
	return name
}
