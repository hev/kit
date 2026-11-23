package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/evidence"
	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/hevpaths"
	"github.com/hev/kit/internal/preval"
	"github.com/hev/kit/internal/prevallearn"
	"github.com/spf13/cobra"
	"path/filepath"
	"strings"
)

type prProviderConfig struct {
	preval.Repository
	Name          string `json:"name,omitempty"`
	HookUser      string `json:"hook_user,omitempty"`
	HookSecretEnv string `json:"hook_secret_env"`
	TokenEnv      string `json:"token_env"`
}

type prHostedConfig struct {
	BaseURL      string `json:"base_url"`
	OrderPath    string `json:"order_path"`
	TracePath    string `json:"trace_path"`
	EvidencePath string `json:"evidence_path"`
	TokenEnv     string `json:"token_env"`
}

type prEvaluationConfig struct {
	Providers      []prProviderConfig    `json:"providers"`
	Links          []preval.Registration `json:"links"`
	StaleAfter     string                `json:"stale_after"`
	TraceNamespace string                `json:"trace_namespace"`
	Hosted         *prHostedConfig       `json:"hosted,omitempty"`
	LearningModel  string                `json:"approved_claude_model,omitempty"`
}

type localEvaluationDocuments struct {
	traces   *filearchive.Store
	evidence *evidence.Store
}

func (d localEvaluationDocuments) ReadOrder(_ context.Context, p preval.Provenance) (*preval.Evaluation, error) {
	return dispatch.ReadEvaluation(p.OrderID, p.AttemptID)
}
func (d localEvaluationDocuments) WriteOrder(_ context.Context, e preval.Evaluation) error {
	return dispatch.SetEvaluation(e)
}
func (d localEvaluationDocuments) WriteTrace(_ context.Context, id string, e preval.Evaluation) error {
	return d.traces.SetEvaluation(id, e)
}
func (d localEvaluationDocuments) WriteEvidence(_ context.Context, id string, e preval.Evaluation) error {
	return d.evidence.SetEvaluation(id, e)
}

func loadPREvaluation(path string) (*preval.Reconciler, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open pull request evaluation policy")
	}
	defer file.Close()
	const configLimit = 256 << 10
	body, err := io.ReadAll(io.LimitReader(file, configLimit+1))
	if err != nil || len(body) > configLimit {
		return nil, errors.New("cannot read pull request evaluation policy")
	}
	var config prEvaluationConfig
	if json.Unmarshal(body, &config) != nil {
		return nil, errors.New("invalid pull request evaluation policy")
	}
	var staleAfter time.Duration
	if config.StaleAfter != "" {
		staleAfter, err = time.ParseDuration(config.StaleAfter)
		if err != nil {
			return nil, errors.New("invalid pull request inactivity interval")
		}
	}
	providers := make([]preval.Provider, 0, len(config.Providers))
	for _, provider := range config.Providers {
		providers = append(providers, preval.Provider{Repository: provider.Repository, Name: provider.Name,
			HookUser: provider.HookUser, HookSecret: os.Getenv(provider.HookSecretEnv), Token: os.Getenv(provider.TokenEnv)})
	}
	var documents preval.Documents
	var local localEvaluationDocuments
	if hosted := config.Hosted; hosted != nil {
		if hosted.OrderPath == "" {
			hosted.OrderPath = "/v1/orders"
		}
		if hosted.TracePath == "" {
			hosted.TracePath = "/v1/traces/sessions"
		}
		if hosted.EvidencePath == "" {
			hosted.EvidencePath = "/v1/evidence"
		}
		documents = preval.HostedDocuments{BaseURL: hosted.BaseURL, OrderPath: hosted.OrderPath, TracePath: hosted.TracePath,
			EvidencePath: hosted.EvidencePath, Token: os.Getenv(hosted.TokenEnv)}
	} else {
		traces, err := filearchive.OpenLazy(filearchive.Root(""), config.TraceNamespace)
		if err != nil {
			return nil, errors.New("cannot open local trace archive")
		}
		ev, err := evidence.Open(evidence.Root())
		if err != nil {
			return nil, errors.New("cannot open local evidence store")
		}
		local = localEvaluationDocuments{traces: traces, evidence: ev}
		documents = local
	}
	reconciler, err := preval.NewReconciler(providers, config.Links, documents, staleAfter)
	if err != nil {
		return nil, err
	}
	if config.LearningModel != "" {
		if config.Hosted != nil {
			return nil, errors.New("native learning requires explicitly authorized local source and board storage")
		}
		learner, err := prevallearn.New(hevpaths.Dir(hevpaths.Board), config.LearningModel, local.learningSources)
		if err != nil {
			return nil, err
		}
		reconciler.EnableLearning(learner)
	}
	return reconciler, nil
}

func init() {
	var policy, address string
	cmd := &cobra.Command{
		Use:   "pr-evaluation",
		Short: "Reconcile explicitly attributed pull request outcomes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := loadPREvaluation(policy)
			if err != nil {
				return err
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
				return errors.New("pull request hook listener requires a loopback IP")
			}
			listener, err := net.Listen("tcp", address)
			if err != nil {
				return errors.New("cannot start pull request hook listener")
			}
			defer listener.Close()
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			server := &http.Server{Handler: r.Handler(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second,
				WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
			serverErr := make(chan error, 1)
			go func() { serverErr <- server.Serve(listener) }()
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			cursor := 0
			fmt.Fprintln(cmd.OutOrStdout(), "Pull request hook listener started on port "+strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
			for {
				select {
				case <-ctx.Done():
					shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					return server.Shutdown(shutdown)
				case <-serverErr:
					return errors.New("pull request hook listener stopped")
				case <-ticker.C:
					sweep, cancel := context.WithTimeout(ctx, 100*time.Second)
					var sweepErr error
					cursor, sweepErr = r.Sweep(sweep, cursor, 1)
					cancel()
					if sweepErr != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "Pull request reconciliation will retry on the next sweep.")
					}
				}
			}
		},
	}
	cmd.Flags().StringVar(&policy, "policy", "", "Workspace-owned attribution policy file")
	cmd.Flags().StringVar(&address, "listen", "127.0.0.1:7789", "Loopback address behind the workspace TLS ingress")
	rootCmd.AddCommand(cmd)
}

func readPREvalExcerpt(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, 4800))
	return string(body), err
}

func (d localEvaluationDocuments) learningSources(e preval.Evaluation) ([]preval.Excerpt, error) {
	p := e.Provenance
	doc, err := readPREvalExcerpt(filepath.Join(dispatch.Root(), p.OrderID, "order.md"))
	if err != nil {
		return nil, err
	}
	sources := []preval.Excerpt{{Reference: "order:" + p.OrderID, Text: doc}}
	for _, id := range p.TraceSessionIDs[:min(5, len(p.TraceSessionIDs))] {
		sample, err := d.traces.EvaluationSample(id)
		if err != nil {
			return nil, err
		}
		sources = append(sources, preval.Excerpt{Reference: "trace:" + id, Text: sample})
	}
	for _, id := range p.EvidenceIDs[:min(5, len(p.EvidenceIDs))] {
		pkg, err := d.evidence.Load(id)
		if err != nil {
			return nil, err
		}
		var text strings.Builder
		text.WriteString(pkg.Summary)
		for _, claim := range pkg.Claims {
			text.WriteString("\n" + claim.Criterion + ": " + claim.Text)
			if text.Len() > 4800 {
				break
			}
		}
		sources = append(sources, preval.Excerpt{Reference: "evidence:" + id, Text: text.String()})
	}
	return sources, nil
}
