package preval

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxPayload = 1 << 20
const providerTimeout = 10 * time.Second

type Provider struct {
	Repository
	// Name is the GitHub repository slug used for API routing. The numeric
	// repository ID is checked independently on every hook and API reply.
	Name       string
	HookSecret string
	HookUser   string
	Token      string
	client     *http.Client
	apiBase    string
}

type githubPR struct {
	Number      int64
	State       string
	Merged      bool
	Updated     time.Time `json:"updated_at"`
	MergeCommit string    `json:"merge_commit_sha"`
	Head        struct{ SHA string }
	Base        struct{ Repo githubRepo }
}

type githubRepo struct {
	ID    int64
	Owner struct{ Login string }
}

type azurePR struct {
	ID         int64 `json:"pullRequestId"`
	Status     string
	Closed     time.Time `json:"closedDate"`
	Created    time.Time `json:"creationDate"`
	Repository struct {
		ID      string
		Project struct{ ID string }
	}
	LastMergeSourceCommit struct {
		ID string `json:"commitId"`
	}
	LastMergeCommit struct {
		ID string `json:"commitId"`
	}
}

func (p Provider) githubRepoMatches(repo githubRepo) bool {
	return repo.ID > 0 && strconv.FormatInt(repo.ID, 10) == p.Repository.Repository &&
		strings.EqualFold(repo.Owner.Login, p.Organization)
}

func (p Provider) azureRepoMatches(pr azurePR) bool {
	return strings.EqualFold(pr.Repository.ID, p.Repository.Repository) &&
		strings.EqualFold(pr.Repository.Project.ID, p.Project)
}

func (p Provider) hookAuthorized(r *http.Request, body []byte) bool {
	if p.HookSecret == "" {
		return false
	}
	if p.Repository.Provider == GitHub {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !strings.HasPrefix(sig, "sha256=") {
			return false
		}
		sum, err := hex.DecodeString(strings.TrimPrefix(sig, "sha256="))
		mac := hmac.New(sha256.New, []byte(p.HookSecret))
		mac.Write(body)
		return err == nil && hmac.Equal(sum, mac.Sum(nil))
	}
	user, password, ok := r.BasicAuth()
	return p.HookUser != "" && ok && hmac.Equal([]byte(user), []byte(p.HookUser)) && hmac.Equal([]byte(password), []byte(p.HookSecret))
}

// Hook accepts only configured repository identities. The callback reconciles
// an existing attribution and never treats the payload itself as an outcome.
func (p Provider) Hook(reconcile func(context.Context, PullRequest, time.Time) error) http.Handler {
	slots := make(chan struct{}, 8)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPayload))
		if err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		if !p.Repository.valid() || !p.hookAuthorized(r, body) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if p.Repository.Provider == GitHub && r.Header.Get("X-GitHub-Event") == "ping" {
			var ping struct{ Repository githubRepo }
			if json.Unmarshal(body, &ping) != nil {
				w.WriteHeader(http.StatusBadRequest)
			} else if !p.githubRepoMatches(ping.Repository) {
				w.WriteHeader(http.StatusForbidden)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}
		id, eventAt, status := p.hookIdentity(r, body)
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), providerTimeout)
		defer cancel()
		if err := reconcile(ctx, PullRequest{p.Repository, id}, eventAt); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func (p Provider) hookIdentity(r *http.Request, body []byte) (int64, time.Time, int) {
	if p.Repository.Provider == GitHub {
		var event struct {
			Action      string
			Number      int64
			Repository  githubRepo
			PullRequest githubPR `json:"pull_request"`
		}
		if r.Header.Get("X-GitHub-Event") != "pull_request" || json.Unmarshal(body, &event) != nil ||
			event.Number <= 0 || event.Number != event.PullRequest.Number || !identifier.MatchString(event.Action) {
			return 0, time.Time{}, http.StatusBadRequest
		}
		if !p.githubRepoMatches(event.Repository) || !p.githubRepoMatches(event.PullRequest.Base.Repo) {
			return 0, time.Time{}, http.StatusForbidden
		}
		return event.Number, time.Time{}, 0
	}

	var event struct {
		EventType   string
		ID          string
		PublisherID string
		CreatedDate time.Time
		Resource    azurePR
	}
	if json.Unmarshal(body, &event) != nil || event.Resource.ID <= 0 || !identifier.MatchString(event.ID) ||
		event.PublisherID != "tfs" {
		return 0, time.Time{}, http.StatusBadRequest
	}
	switch event.EventType {
	case "git.pullrequest.created", "git.pullrequest.updated", "git.pullrequest.merged":
	default:
		return 0, time.Time{}, http.StatusBadRequest
	}
	// Azure credentials are scoped to one organization and repository
	// route. The event repository and project IDs must also match.
	if !p.azureRepoMatches(event.Resource) {
		return 0, time.Time{}, http.StatusForbidden
	}
	if event.CreatedDate.After(time.Now().UTC()) {
		return 0, time.Time{}, http.StatusBadRequest
	}
	return event.Resource.ID, event.CreatedDate, 0
}

var errLookup = errors.New("pull request provider lookup failed")

func (p Provider) fetch(ctx context.Context, path string, result any) error {
	base := p.apiBase
	if base == "" {
		base = "https://api.github.com"
		if p.Repository.Provider == AzureDevOps {
			base = "https://dev.azure.com"
		}
	}
	ctx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return errLookup
	}
	if p.Repository.Provider == AzureDevOps {
		req.SetBasicAuth("", p.Token)
	} else {
		req.Header.Set("Authorization", "Bearer "+p.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	client := http.Client{}
	if p.client != nil {
		client = *p.client
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return errLookup
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errLookup
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPayload+1))
	if err != nil || len(body) > maxPayload || json.Unmarshal(body, result) != nil {
		return errLookup
	}
	return nil
}

func (p Provider) Lookup(ctx context.Context, pr PullRequest) (Observation, error) {
	if pr.Repository != p.Repository || !p.Repository.valid() || pr.ID <= 0 || p.Token == "" {
		return Observation{}, errInvalid
	}
	// Start time orders overlapping lookups even when an older response arrives
	// after a newer one. An observed provider change can advance that time.
	result := Observation{Source: Source{PullRequest: pr}, ObservedAt: time.Now().UTC()}
	if p.Repository.Provider == GitHub {
		if !identifier.MatchString(p.Name) {
			return Observation{}, errInvalid
		}
		var record githubPR
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d", p.Organization, p.Name, pr.ID)
		if err := p.fetch(ctx, path, &record); err != nil || record.Number != pr.ID ||
			!p.githubRepoMatches(record.Base.Repo) || record.Updated.IsZero() {
			return Observation{}, errLookup
		}
		result.LastActivity = record.Updated
		result.Source.Revision = record.Head.SHA
		switch {
		case record.State == "closed" && record.Merged:
			result.Outcome, result.Source.Revision = Merged, record.MergeCommit
		case record.State == "closed":
			result.Outcome = Closed
		case record.State == "open" && !record.Merged:
			result.Outcome = Active
		default:
			return Observation{}, errLookup
		}
	} else {
		var record azurePR
		path := fmt.Sprintf("/%s/%s/_apis/git/repositories/%s/pullRequests/%d",
			p.Organization, p.Project, p.Repository.Repository, pr.ID)
		if err := p.fetch(ctx, path+"?api-version=7.1", &record); err != nil ||
			record.ID != pr.ID || !p.azureRepoMatches(record) {
			return Observation{}, errLookup
		}
		result.Source.Revision = record.LastMergeSourceCommit.ID
		switch record.Status {
		case "completed":
			result.Outcome, result.Source.Revision = Merged, record.LastMergeCommit.ID
		case "abandoned":
			result.Outcome = Closed
		case "active":
			result.Outcome = Active
			var iterations struct {
				Count int
				Value []struct {
					Updated time.Time `json:"updatedDate"`
				}
			}
			var threads struct {
				Count int
				Value []struct {
					Updated time.Time `json:"lastUpdatedDate"`
				}
			}
			if p.fetch(ctx, path+"/iterations?api-version=7.1", &iterations) == nil &&
				p.fetch(ctx, path+"/threads?api-version=7.1", &threads) == nil &&
				iterations.Count == len(iterations.Value) && threads.Count == len(threads.Value) {
				result.LastActivity = record.Created
				for _, iteration := range iterations.Value {
					if iteration.Updated.After(result.LastActivity) {
						result.LastActivity = iteration.Updated
					}
				}
				for _, thread := range threads.Value {
					if thread.Updated.After(result.LastActivity) {
						result.LastActivity = thread.Updated
					}
				}
			}
		default:
			return Observation{}, errLookup
		}
		if result.Outcome != Active {
			result.LastActivity = record.Closed
		}
	}
	if !identifier.MatchString(result.Source.Revision) {
		return Observation{}, errLookup
	}
	if result.LastActivity.After(time.Now().UTC()) {
		return Observation{}, errLookup
	}
	if result.LastActivity.After(result.ObservedAt) {
		result.ObservedAt = result.LastActivity
	}
	id := sha256.Sum256([]byte(result.Outcome + "\x00" + result.Source.Revision + "\x00" + result.LastActivity.Format(time.RFC3339Nano)))
	result.Source.EventID = "api_" + hex.EncodeToString(id[:])
	return result, nil
}
