package daemon

import (
	"os"
	"path/filepath"

	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Credentials returns a credential provider that works for the local MinIO
// default while also supporting standard AWS env/profile setups for real S3.
func S3Credentials(cfg *Config) *credentials.Credentials {
	if cfg.S3Profile != "" {
		if home, err := os.UserHomeDir(); err == nil {
			return credentials.NewFileAWSCredentials(filepath.Join(home, ".aws", "credentials"), cfg.S3Profile)
		}
	}

	if cfg.S3Key != "" || cfg.S3Secret != "" || cfg.S3SessionToken != "" {
		return credentials.NewStaticV4(cfg.S3Key, cfg.S3Secret, cfg.S3SessionToken)
	}

	return credentials.NewChainCredentials([]credentials.Provider{
		&credentials.EnvAWS{},
		&credentials.EnvMinio{},
	})
}
