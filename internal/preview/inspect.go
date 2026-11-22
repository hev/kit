package preview

import (
	"os"
	"path/filepath"
	"strings"
)

// Summary is the row printed by `hev preview ls`.
type Summary struct {
	Name           string `json:"name"`
	Bind           string `json:"bind"`
	Port           int    `json:"port"`
	URL            string `json:"url"`
	Mode           string `json:"mode"`
	State          string `json:"state"`
	Ref            string `json:"ref,omitempty"`
	SHA            string `json:"sha,omitempty"`
	EnvFingerprint string `json:"env_fingerprint,omitempty"`
	BuiltAt        string `json:"built_at,omitempty"`
	Builds         int    `json:"builds"`
	KeepBuilds     int    `json:"keep_builds"`
}

// Details is the full view printed by `hev preview show`.
type Details struct {
	Summary
	Repo                string    `json:"repo,omitempty"`
	Subdir              string    `json:"subdir,omitempty"`
	OutDir              string    `json:"out_dir,omitempty"`
	BuildCmd            []string  `json:"build_cmd,omitempty"`
	Timeout             string    `json:"timeout"`
	EnvKeys             []string  `json:"env_keys,omitempty"`
	ConfigEnvDigest     string    `json:"config_env_fingerprint"`
	BuildID             string    `json:"build_id,omitempty"`
	Current             *Metadata `json:"current,omitempty"`
	EnvChangedSinceSync bool      `json:"env_changed_since_sync,omitempty"`
}

// Inspect reads the local preview state without performing network or git calls.
func (r Runner) Inspect(cfg Config) (Details, error) {
	if err := cfg.Validate(); err != nil {
		return Details{}, err
	}
	cfgFP := EnvFingerprint(cfg.Env)
	d := Details{
		Summary: Summary{
			Name:       cfg.Name,
			Bind:       cfg.Bind,
			Port:       cfg.Port,
			URL:        cfg.URL(),
			Mode:       cfg.Mode(),
			KeepBuilds: cfg.EffectiveKeepBuilds(),
		},
		Timeout:         cfg.EffectiveTimeout().String(),
		EnvKeys:         cfg.EnvKeys(),
		ConfigEnvDigest: cfgFP,
	}

	if cfg.Mode() == "static" {
		root := cfg.ExpandedRoot(r.Home)
		if st, err := os.Stat(root); err == nil && st.IsDir() {
			d.State = "ready"
		} else {
			d.State = "missing_root"
		}
		return d, nil
	}

	d.Ref = cfg.Ref
	d.Repo = SanitizeRepo(cfg.Repo)
	d.Subdir = cfg.Subdir
	d.OutDir = cfg.EffectiveOutDir()
	if len(cfg.BuildCmd) > 0 {
		d.BuildCmd = append([]string(nil), cfg.BuildCmd...)
	}

	previewDir := PreviewDir(r.Home, cfg.Name)
	d.Builds = countBuilds(filepath.Join(previewDir, "builds"))

	curDir, curID, err := resolveCurrent(previewDir)
	if err != nil {
		d.State = "not_built"
		return d, nil
	}
	d.BuildID = curID
	d.State = "ready"
	if meta, err := readMetadata(filepath.Join(curDir, "preview.json")); err == nil {
		d.Current = &meta
		d.SHA = meta.SHA
		d.EnvFingerprint = meta.EnvFingerprint
		d.BuiltAt = meta.BuiltAt
		if meta.EnvFingerprint != "" && meta.EnvFingerprint != cfgFP {
			d.EnvChangedSinceSync = true
		}
	}
	return d, nil
}

func countBuilds(buildsDir string) int {
	entries, err := os.ReadDir(buildsDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && !strings.HasSuffix(e.Name(), ".tmp") {
			n++
		}
	}
	return n
}
