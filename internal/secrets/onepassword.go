package secrets

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// onePassword reads op:// references with one `op inject` per batch, through
// a service account only (desktop unlock disabled). Behaviour is the cache's
// original 1Password path, moved behind the Provider seam.
type onePassword struct{ c *Cache }

func init() {
	register(providerSpec{
		scheme:   "op",
		validate: func(uri string) bool { return strings.HasPrefix(uri, "op://") },
		build:    func(c *Cache) Provider { return onePassword{c} },
	})
}

func (onePassword) Scheme() string { return "op" }
func (onePassword) Name() string   { return "1Password" }
func (onePassword) Kind() string   { return "inject" }

func (onePassword) Credential() (string, error) {
	if os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") != "" {
		return "service-account token (environment)", nil
	}
	if serviceToken() != "" {
		return "service-account token (token file)", nil
	}
	return "", errors.New("1Password service-account token is required; desktop unlock disabled")
}

// Fingerprint is the SHA-256 of the service-account token, as the endpoint's
// account check has always used.
func (onePassword) Fingerprint() string {
	if token := serviceToken(); token != "" {
		return fingerprint(token)
	}
	return ""
}

func (onePassword) quotaNote() string { return "account quota below" }

// Read resolves every reference with one op inject. A random boundary frames
// arbitrary multiline values without escaping. The template and injected
// result travel only through anonymous pipes.
func (p onePassword) Read(ctx context.Context, uris []string) ([]string, int, error) {
	token := serviceToken()
	if token == "" {
		return nil, 0, errors.New("1Password service-account token is required; desktop unlock disabled")
	}
	boundary := fmt.Sprintf("HEV-%016x-%016x", rand.Uint64(), rand.Uint64())
	var template strings.Builder
	for _, u := range uris {
		template.WriteString("{{ " + u + " }}" + boundary)
	}
	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(opCtx, p.c.op, "inject")
	cmd.Env = opEnv(token)
	cmd.Stdin = strings.NewReader(template.String())
	var stderr strings.Builder
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		// Never return raw op output: it can echo injected values or references.
		lower := strings.ToLower(opRefText.ReplaceAllString(stderr.String(), ""))
		if rl := opRateLimit(lower); rl != nil {
			return nil, 1, rl
		}
		return nil, 1, errors.New("1Password secret resolution failed (service account required; desktop unlock disabled)")
	}
	parts := strings.Split(string(data), boundary)
	if len(parts) != len(uris)+1 || strings.TrimSpace(parts[len(parts)-1]) != "" {
		return nil, 1, errors.New("1Password returned invalid batch framing")
	}
	return parts[:len(uris)], 1, nil
}

var (
	opRefText  = regexp.MustCompile(`op://[^\s"']+`)
	resetStamp = regexp.MustCompile(`(?i)(?:reset|retry)(?: at| after| time)?[:= ]+(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)`)
	retryIn    = regexp.MustCompile(`(?:try again|retry) in (\d{1,6}) ?(seconds?|secs?|s|minutes?|mins?|m|hours?|h)\b`)
)

// opRateLimit classifies a sanitized, lower-cased op diagnostic. It returns
// nil when the answer is not a rate limit. Only explicit reset/retry wording
// is read; the cache bounds it by the class.
func opRateLimit(lower string) *RateLimit {
	if !strings.Contains(lower, "rate limit") && !strings.Contains(lower, "too many requests") && !strings.Contains(lower, "rate-limited") {
		return nil
	}
	rl := &RateLimit{Class: "rate limit"}
	if strings.Contains(lower, "hourly limit") || strings.Contains(lower, "hourly rate limit") {
		rl.Class = "hourly rate limit"
	}
	if strings.Contains(lower, "daily limit") || strings.Contains(lower, "daily rate limit") {
		rl.Class = "daily rate limit"
	}
	if m := resetStamp.FindStringSubmatch(lower); len(m) == 2 {
		if reset, e := time.Parse(time.RFC3339, strings.ToUpper(m[1])); e == nil {
			rl.Reset = reset
		} else {
			rl.Reset = time.Unix(0, 0) // matched but unusable: no hint at all
		}
	} else if m := retryIn.FindStringSubmatch(lower); len(m) == 3 {
		n, _ := strconv.Atoi(m[1])
		unit := time.Second
		switch m[2][0] {
		case 'm':
			unit = time.Minute
		case 'h':
			unit = time.Hour
		}
		rl.RetryIn = time.Duration(n) * unit
	}
	return rl
}
