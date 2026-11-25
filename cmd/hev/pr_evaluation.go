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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	boardlayer "github.com/hev/kit/internal/board/layer"
	boardsettings "github.com/hev/kit/internal/board/settings"
	boardstore "github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/evidence"
	"github.com/hev/kit/internal/hostlabel"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/preval"
	"github.com/hev/kit/internal/prevallearn"
	"github.com/hev/kit/internal/workspace"
	"github.com/spf13/cobra"
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
	workspace string
	traces    interface {
		SetEvaluationConditional(context.Context, string, string, preval.Evaluation) (string, error)
		ReadEvaluation(string) (*preval.Evaluation, string, error)
		EvaluationSample(context.Context, string) (string, error)
	}
	evidence *evidence.Store
}

func (d localEvaluationDocuments) ReadOrder(_ context.Context, p preval.Provenance) (*preval.Evaluation, error) {
	return dispatch.ReadEvaluation(d.workspace, p.OrderID, p.AttemptID)
}
func (d localEvaluationDocuments) WriteOrder(_ context.Context, e preval.Evaluation) error {
	return dispatch.SetEvaluation(d.workspace, e)
}
func (d localEvaluationDocuments) ClaimLearning(_ context.Context, e preval.Evaluation) (bool, error) {
	return dispatch.ClaimEvaluationLearning(d.workspace, e)
}
func (d localEvaluationDocuments) WriteTrace(ctx context.Context, id string, e preval.Evaluation) error {
	_, err := d.traces.SetEvaluationConditional(ctx, id, "", e)
	return err
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
	w, err := currentWorkspace()
	if err != nil || !w.AllowsHost(hostlabel.Self(os.Hostname)) {
		return nil, errors.New("cannot resolve pull request evaluation workspace on this host")
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
		target, err := openTarget(config.TraceNamespace, nil)
		if err != nil {
			return nil, errors.New("cannot open local trace archive")
		}
		ev, err := evidence.Open(evidenceWorkspaceRoot(w))
		if err != nil {
			return nil, errors.New("cannot open local evidence store")
		}
		local = localEvaluationDocuments{workspace: w.Name, evidence: ev}
		if target.Layer != nil {
			local.traces = target.Layer
		} else {
			local.traces = target.Files
		}
		documents = local
	}
	reconciler, err := preval.NewReconciler(providers, config.Links, documents, staleAfter)
	if err != nil {
		return nil, err
	}
	if config.LearningModel != "" {
		var learner *prevallearn.Learner
		if hosted, ok := documents.(preval.HostedDocuments); ok {
			learner, err = prevallearn.NewHosted(hosted, config.LearningModel)
		} else {
			board, root, openErr := prLearningBoard(w)
			if openErr != nil {
				return nil, openErr
			}
			learner, err = prevallearn.NewWithBoard(board, root, config.LearningModel, local.learningSources)
		}
		if err != nil {
			return nil, err
		}
		learner.Scope(w.Name, w.Profile("claude"), func(ctx context.Context) ([]string, []string, error) {
			return dispatch.PRLearningClaudeAuth(ctx, w)
		})
		reconciler.EnableLearning(learner)
	}
	return reconciler, nil
}

func prLearningBoard(w workspace.Selection) (*boardstore.Store, string, error) {
	unavailable := errors.New("cannot open the selected workspace's learning board")
	if w.IsolatedStorage() {
		if w.Layer.BoardURL != "" {
			return nil, "", unavailable
		}
		key, err := layerenv.WorkspaceKey(w)
		if err != nil {
			return nil, "", unavailable
		}
		if key != "" {
			_, namespace := w.Namespaces()
			endpoint := defaultString(w.Layer.Endpoint, layerenv.HostedEndpoint)
			return boardstore.New(boardlayer.New(endpoint, key, namespace, "")), "", nil
		}
		root := filepath.Join(w.Root(), "board")
		board, err := boardstore.NewFiles(root)
		if err != nil {
			return nil, "", unavailable
		}
		return board, root, nil
	}
	if err := boardsettings.Load(); err != nil || boardsettings.Current.URLSet || boardsettings.Current.ReadOnly {
		return nil, "", unavailable
	}
	board, err := boardstore.Open()
	if err != nil {
		return nil, "", unavailable
	}
	info := board.Info()
	if info.Mode == "filesystem" {
		return board, boardsettings.Dir(), nil
	}
	return board, "", nil
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

func evidenceWorkspaceRoot(w workspace.Selection) string {
	if w.IsolatedStorage() {
		return filepath.Join(w.Root(), "evidence")
	}
	return evidence.Root()
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

func (d localEvaluationDocuments) learningSources(ctx context.Context, e preval.Evaluation) ([]preval.Excerpt, error) {
	p := e.Provenance
	prior, err := d.ReadOrder(ctx, p)
	if err != nil || prior == nil || prior.ID != e.ID || !prior.ObservedAt.Equal(e.ObservedAt) {
		return nil, preval.ErrConflict
	}
	doc, err := readPREvalExcerpt(filepath.Join(dispatch.Root(), p.OrderID, "order.md"))
	if err != nil {
		return nil, err
	}
	sources := []preval.Excerpt{{Reference: "order:" + p.OrderID, Text: doc}}
	for _, id := range p.TraceSessionIDs[:min(5, len(p.TraceSessionIDs))] {
		current, _, err := d.traces.ReadEvaluation(id)
		if err != nil || current == nil || current.ID != e.ID || !current.ObservedAt.Equal(e.ObservedAt) {
			return nil, preval.ErrConflict
		}
		sample, err := d.traces.EvaluationSample(ctx, id)
		if err != nil {
			return nil, err
		}
		sources = append(sources, preval.Excerpt{Reference: "trace:" + id, Text: sample})
	}
	for _, id := range p.EvidenceIDs[:min(5, len(p.EvidenceIDs))] {
		pkg, err := d.evidence.Load(id)
		if err != nil || pkg.Evaluation == nil || pkg.Evaluation.ID != e.ID || !pkg.Evaluation.ObservedAt.Equal(e.ObservedAt) {
			return nil, preval.ErrConflict
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
