package secrets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// Vault-neutral references (RFC 0009, "Vault-neutral references"):
//
//	secret://NAME
//
// names a secret without naming a store. The host's secret manager, set once
// per host, decides where NAME is read from:
//
//	1Password        op://VAULT/NAME/FIELD   (FIELD defaults to "password")
//	Azure Key Vault  azkv://VAULT/NAME       (VAULT may be VAULT.SUFFIX)
//
// NAME is 1-127 lowercase letters, digits and hyphens, valid as a Key Vault
// secret name and as a 1Password item title. A neutral reference is expanded
// to its explicit form in the resolving process before validation, the cache,
// the read log or any IPC, so a value read as secret://NAME and as its
// explicit form is one cache entry and one request, and a daemon of any age
// only ever sees op:// or azkv://.

// NeutralScheme is the vault-neutral reference scheme, without "://".
const NeutralScheme = "secret"

const neutralPrefix = NeutralScheme + "://"

// Managers a host may select.
const (
	ManagerOnePassword = "1password"
	ManagerKeyVault    = "azure-key-vault"
)

// Environment overrides for the host setting. When HEV_SECRETS_MANAGER is set
// the environment is the whole setting for that process and the config file's
// [secrets] table is not read.
const (
	ManagerEnv = "HEV_SECRETS_MANAGER"
	VaultEnv   = "HEV_SECRETS_VAULT"
	FieldEnv   = "HEV_SECRETS_FIELD"
)

// defaultOPField is the 1Password field a neutral NAME reads when the host
// sets none: the field of a Password item.
const defaultOPField = "password"

var neutralName = regexp.MustCompile(`^[a-z0-9-]{1,127}$`)

// Manager is a host's secret manager setting. It names stores and where the
// setting came from, never a value or credential.
type Manager struct {
	// Kind is ManagerOnePassword, ManagerKeyVault, or "" when none is set.
	Kind string `json:"manager,omitempty"`
	// Vault is the 1Password vault or the Key Vault name (VAULT or
	// VAULT.SUFFIX).
	Vault string `json:"vault,omitempty"`
	// Field is the 1Password field read for every NAME.
	Field string `json:"field,omitempty"`
	// Source is where the setting was read: "environment (HEV_SECRETS_MANAGER)"
	// or "PATH [secrets]"; with no setting, the config file that would hold it.
	Source string `json:"source"`
}

// Name is the manager's human name.
func (m Manager) Name() string {
	switch m.Kind {
	case ManagerOnePassword:
		return "1Password"
	case ManagerKeyVault:
		return "Azure Key Vault"
	}
	return ""
}

// ConfigPath is kit's config file, where a host's [secrets] table lives:
// HEV_CONFIG, or ~/.hev/config.toml (the same file hevd reads).
func ConfigPath() string {
	if p := os.Getenv("HEV_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(home, ".hev", "config.toml")
}

// setupHint names exactly what to set for a neutral reference.
func setupHint(path string) string {
	return fmt.Sprintf(`set this host's secret manager in %s: [secrets] manager = "%s" or "%s" and vault = "VAULT" (or %s and %s in the environment)`,
		path, ManagerOnePassword, ManagerKeyVault, ManagerEnv, VaultEnv)
}

// HostManager reads this host's secret manager setting at use time. A host
// with no setting returns Kind "" and no error; a malformed setting is an
// error naming what to fix. The manager is never guessed from which
// credentials happen to be present.
func HostManager() (Manager, error) {
	var m Manager
	path := ConfigPath()
	if kind := strings.TrimSpace(os.Getenv(ManagerEnv)); kind != "" {
		m = Manager{Kind: kind, Vault: strings.TrimSpace(os.Getenv(VaultEnv)), Field: strings.TrimSpace(os.Getenv(FieldEnv)),
			Source: "environment (" + ManagerEnv + ")"}
	} else {
		m.Source = path
		var file struct {
			Secrets struct {
				Manager string `toml:"manager"`
				Vault   string `toml:"vault"`
				Field   string `toml:"field"`
			} `toml:"secrets"`
		}
		if _, err := toml.DecodeFile(path, &file); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return m, nil
			}
			// The decoder's message can quote file content; name the file only.
			return m, fmt.Errorf("secret manager: cannot read %s as TOML", path)
		}
		s := file.Secrets
		if s.Manager == "" {
			if s.Vault != "" || s.Field != "" {
				return m, fmt.Errorf(`secret manager: [secrets] in %s has no manager; set manager = "%s" or "%s"`, path, ManagerOnePassword, ManagerKeyVault)
			}
			return m, nil
		}
		m = Manager{Kind: strings.TrimSpace(s.Manager), Vault: strings.TrimSpace(s.Vault), Field: strings.TrimSpace(s.Field), Source: path + " [secrets]"}
	}
	switch m.Kind {
	case ManagerOnePassword:
		if m.Field == "" {
			m.Field = defaultOPField
		}
		if m.Vault == "" || strings.ContainsAny(m.Vault+m.Field, "/{}\r\n\x00") || strings.ContainsAny(m.Field, " ") {
			return m, fmt.Errorf("secret manager 1password (%s) needs vault, a 1Password vault name, and field, a field label without '/', '{', '}' or spaces", m.Source)
		}
	case ManagerKeyVault:
		if m.Field != "" {
			return m, fmt.Errorf("secret manager azure-key-vault (%s) takes no field; remove it", m.Source)
		}
		if _, ok := parseKV(azkvScheme + "://" + m.Vault + "/x"); !ok {
			return m, fmt.Errorf("secret manager azure-key-vault (%s) needs vault, a Key Vault name (VAULT, or VAULT.SUFFIX for a sovereign cloud or one listed in HEV_AZKV_DNS_SUFFIXES)", m.Source)
		}
	default:
		return m, fmt.Errorf(`secret manager (%s) must be "%s" or "%s"`, m.Source, ManagerOnePassword, ManagerKeyVault)
	}
	return m, nil
}

// IsNeutral reports whether uri is written in the secret:// scheme, valid or
// not.
func IsNeutral(uri string) bool { return strings.HasPrefix(uri, neutralPrefix) }

// ValidNeutral reports whether uri is a well-formed secret://NAME.
func ValidNeutral(uri string) bool {
	name, ok := strings.CutPrefix(uri, neutralPrefix)
	return ok && neutralName.MatchString(name)
}

// errNeutralName refuses a NAME that would not be portable between managers.
var errNeutralName = errors.New("invalid secret:// reference: NAME must be 1-127 lowercase letters, digits and hyphens (secret://stripe-api-key)")

// Expand returns uri in its explicit form under m. An explicit reference is
// returned unchanged.
func (m Manager) Expand(uri string) (string, error) {
	if !IsNeutral(uri) {
		return uri, nil
	}
	name := strings.TrimPrefix(uri, neutralPrefix)
	if !neutralName.MatchString(name) {
		return "", errNeutralName
	}
	switch m.Kind {
	case ManagerOnePassword:
		return "op://" + m.Vault + "/" + name + "/" + m.Field, nil
	case ManagerKeyVault:
		return azkvScheme + "://" + m.Vault + "/" + name, nil
	}
	return "", errors.New("secret:// needs this host's secret manager, and none is set; " + setupHint(m.Source))
}

// ExpandRef expands one reference through this host's manager. An explicit
// reference is returned unchanged without reading the setting.
func ExpandRef(uri string) (string, error) {
	if !IsNeutral(uri) {
		return uri, nil
	}
	if !ValidNeutral(uri) {
		return "", errNeutralName
	}
	m, err := HostManager()
	if err != nil {
		return "", err
	}
	return m.Expand(uri)
}

// ExpandRefs returns refs with every secret:// reference in its explicit
// form. The setting is read once, and only when a neutral reference is
// present; refs without one are returned as is.
func ExpandRefs(refs []Ref) ([]Ref, error) {
	var m *Manager
	var out []Ref
	for i, r := range refs {
		if !IsNeutral(r.URI) {
			if out != nil {
				out = append(out, r)
			}
			continue
		}
		if !ValidNeutral(r.URI) {
			return nil, errNeutralName
		}
		if m == nil {
			hm, err := HostManager()
			if err != nil {
				return nil, err
			}
			m = &hm
		}
		u, err := m.Expand(r.URI)
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = append(make([]Ref, 0, len(refs)), refs[:i]...)
		}
		r.URI = u
		out = append(out, r)
	}
	if out == nil {
		return refs, nil
	}
	return out, nil
}

// ManagerStatus is the host's manager as status shows it: the manager, its
// vault and field, where it is configured, and what to fix when it is not
// usable. Never a value or credential.
type ManagerStatus struct {
	Manager
	ManagerName string `json:"name,omitempty"`
	Configured  bool   `json:"configured"`
	Setup       string `json:"setup,omitempty"`
}

// HostManagerStatus reads the setting for status.
func HostManagerStatus() *ManagerStatus {
	m, err := HostManager()
	s := &ManagerStatus{Manager: m, ManagerName: m.Name()}
	switch {
	case err != nil:
		s.Setup = err.Error()
	case m.Kind == "":
		s.Setup = setupHint(m.Source)
	default:
		s.Configured = true
	}
	return s
}
