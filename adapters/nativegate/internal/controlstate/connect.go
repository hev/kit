package controlstate

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// References contains credential references and an optional nonsecret database name.
// Credential values are resolved in memory and never
// interpolated into a DSN, process arguments, logs or error messages.
type References struct {
	Host     string `json:"host_ref"`
	Database string `json:"database_ref,omitempty"`
	// DatabaseName is an explicitly nonsecret database identifier, mutually exclusive
	// with Database. Host, user and password remain references without exceptions.
	DatabaseName string `json:"database_name,omitempty"`
	User         string `json:"user_ref"`
	Password     string `json:"password_ref"`
	Port         uint16 `json:"port,omitempty"`
}
type Resolver interface {
	Resolve(context.Context, string) (string, error)
}

// OPResolver invokes the existing op executable. TokenFile is optional; when
// omitted op uses its existing environment. Output and failures are redacted.
type OPResolver struct{ TokenFile string }

func (r OPResolver) Resolve(ctx context.Context, ref string) (string, error) {
	if !validRef(ref) {
		return "", errors.New("invalid credential reference")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "op", "read", "--no-newline", ref)
	cmd.Env = append(os.Environ(), "OP_BIOMETRIC_UNLOCK_ENABLED=false")
	if r.TokenFile != "" && os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") == "" {
		b, err := os.ReadFile(filepath.Clean(r.TokenFile))
		if err != nil {
			return "", errors.New("credential token unavailable")
		}
		cmd.Env = append(cmd.Env, "OP_SERVICE_ACCOUNT_TOKEN="+strings.TrimSpace(string(b)))
	}
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return "", errors.New("credential resolution failed")
	}
	return string(out), nil
}
func validRef(s string) bool {
	return strings.HasPrefix(s, "op://") && len(s) > 5 && !strings.ContainsAny(s, "\r\n\x00")
}

// Open resolves only the supplied references. It requires verified TLS and
// checks api_version on every new connection, including after pool recovery.
// Close the returned pool when finished. Tests use New with an isolated pool.
func Open(ctx context.Context, refs References, r Resolver) (*Store, *pgxpool.Pool, error) {
	if r == nil {
		return nil, nil, errors.New("credential resolver required")
	}
	if (refs.Database == "") == (refs.DatabaseName == "") {
		return nil, nil, errors.New("select exactly one database reference or nonsecret database name")
	}
	if refs.DatabaseName != "" && (len(refs.DatabaseName) > 63 || strings.ContainsAny(refs.DatabaseName, " \t\r\n\x00/\\") || strings.Contains(refs.DatabaseName, ":")) {
		return nil, nil, errors.New("invalid nonsecret database name")
	}
	rr := []string{refs.Host, refs.Database, refs.User, refs.Password}
	for i, ref := range rr {
		if i == 1 && refs.DatabaseName != "" {
			continue
		}
		if !validRef(ref) {
			return nil, nil, errors.New("connection fields must be credential references")
		}
	}
	vals := make([]string, len(rr))
	for i, ref := range rr {
		if i == 1 && refs.DatabaseName != "" {
			vals[i] = refs.DatabaseName
			continue
		}
		v, err := r.Resolve(ctx, ref)
		if err != nil || v == "" {
			return nil, nil, errors.New("credential resolution failed")
		}
		vals[i] = v
	}
	host := vals[0]
	port := refs.Port
	if port == 0 {
		port = 5432
	}
	if h, p, e := net.SplitHostPort(host); e == nil {
		n, e := strconv.ParseUint(p, 10, 16)
		if e != nil || n == 0 {
			return nil, nil, errors.New("invalid database port")
		}
		host, port = h, uint16(n)
	}
	cfg, err := connectionConfig(host, port, vals[1], vals[2], vals[3])
	if err != nil {
		return nil, nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, nil, ErrTransport
	}
	s, err := New(ctx, pool)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return s, pool, nil
}

func checkVersion(ctx context.Context, c *pgx.Conn) error {
	var v int
	if err := c.QueryRow(ctx, "SELECT kitchen.api_version()", pgx.QueryExecModeExec).Scan(&v); err != nil {
		return safe(err)
	}
	if v != APIVersion {
		return &VersionError{v, APIVersion}
	}
	return nil
}

func connectionConfig(host string, port uint16, database, user, password string) (*pgxpool.Config, error) {
	if os.Getenv("PGSERVICE") != "" {
		return nil, errors.New("PGSERVICE must be unset for reference connections")
	}
	// A Unix-socket host disables TLS in PostgreSQL clients. A reference is not
	// permission to weaken transport security or to supply multiple fallback hosts.
	if host == "" || strings.ContainsAny(host, "/\\, \r\n\t\x00") || (strings.Contains(host, ":") && net.ParseIP(host) == nil) {
		return nil, errors.New("invalid database host")
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, errors.New("system certificates unavailable")
	}
	// Explicit placeholders prevent PG* variables selecting a service, identity or
	// TLS mode. Replace every identity field before any connection is opened.
	cfg, err := pgxpool.ParseConfig("host=localhost port=5432 user=placeholder password=placeholder dbname=placeholder sslmode=verify-full connect_timeout=10 passfile='' sslcert='' sslkey='' sslrootcert='' target_session_attrs=any")
	if err != nil {
		return nil, ErrTransport
	}
	cfg.ConnConfig.Host = host
	cfg.ConnConfig.Port = port
	cfg.ConnConfig.Database = database
	cfg.ConnConfig.User = user
	cfg.ConnConfig.Password = password
	cfg.ConnConfig.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: host}
	cfg.ConnConfig.Fallbacks = nil
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	cfg.ConnConfig.RuntimeParams = map[string]string{"application_name": "control-state"}
	cfg.ConnConfig.ConnectTimeout = 10 * time.Second
	cfg.MaxConns = 4
	cfg.AfterConnect = checkVersion
	return cfg, nil
}
