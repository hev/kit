package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/evidence"
	"github.com/hev/kit/pkg/kitapi"
	"github.com/hev/kit/pkg/workidentity"
	"github.com/spf13/cobra"
)

func newDispatchIdentityCommand() *cobra.Command {
	var workspace string
	identity := &cobra.Command{
		Use:   "identity",
		Short: "Read or write shared work identity in Layer",
		Long: "Read or write shared work identity in Layer and print JSON. " +
			"Uses the signed-in hevmind.com service or Layer configuration by default. " +
			"Set HEV_WORK_IDENTITY_ENDPOINT and HEV_WORK_IDENTITY_KEY to use a different endpoint and bearer key. " +
			"Use resolve to turn an address or alias into the canonical ask ID before writing.",
	}
	identity.PersistentFlags().StringVar(&workspace, "workspace", "", "Layer work workspace UUID")
	add := func(use, short string, argc int, run func(context.Context, *workidentity.Client, []string) (any, error)) *cobra.Command {
		cmd := &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(argc)}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			c, err := dispatchIdentityClient(cmd.Context(), workspace, cmd.Flags().Changed("workspace"))
			if err != nil {
				return err
			}
			result, err := run(cmd.Context(), c, args)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		identity.AddCommand(cmd)
		return cmd
	}

	var createParent string
	create := add("create KEY REQUEST_REF SCOPE_REF", "Create an ask with a caller-supplied idempotency key", 3,
		func(ctx context.Context, c *workidentity.Client, a []string) (any, error) {
			return c.Create(ctx, workidentity.CreateRequest{
				IdempotencyKey: workidentity.UUID(a[0]),
				RequestRef:     workidentity.UUID(a[1]),
				ScopeRef:       workidentity.UUID(a[2]),
				ParentID:       identityParent(createParent),
			})
		})
	create.Flags().StringVar(&createParent, "parent", "", "Parent ask UUID (omit for a root)")
	add("resolve ADDRESS", "Resolve a current address or old alias", 1,
		func(ctx context.Context, c *workidentity.Client, a []string) (any, error) {
			return c.Resolve(ctx, workidentity.Address(a[0]))
		})
	add("get ID", "Read an ask by UUID", 1,
		func(ctx context.Context, c *workidentity.Client, a []string) (any, error) {
			return c.Get(ctx, workidentity.UUID(a[0]))
		})

	addWrite := func(use, short string, extra int, run func(context.Context, *workidentity.Client, workidentity.Target, []string) (any, error)) *cobra.Command {
		return add(use, short, extra+2, func(ctx context.Context, c *workidentity.Client, a []string) (any, error) {
			revision, err := strconv.ParseInt(a[1], 10, 64)
			if err != nil || revision <= 0 {
				return nil, fmt.Errorf("expected a positive REVISION")
			}
			return run(ctx, c, workidentity.Target{ID: workidentity.UUID(a[0]), ExpectedRevision: revision}, a[2:])
		})
	}
	var moveParent string
	move := addWrite("move ID REVISION SEGMENT", "Move or renumber an ask and its descendants", 1,
		func(ctx context.Context, c *workidentity.Client, target workidentity.Target, a []string) (any, error) {
			segment, err := strconv.ParseInt(a[0], 10, 32)
			if err != nil || segment <= 0 {
				return nil, fmt.Errorf("expected a positive SEGMENT")
			}
			return c.Move(ctx, workidentity.MoveRequest{Target: target, Segment: int32(segment), ParentID: identityParent(moveParent)})
		})
	move.Flags().StringVar(&moveParent, "parent", "", "Destination parent ask UUID (omit to move to the root)")
	addWrite("revise ID REVISION SCOPE_REF", "Replace an ask's scope and clear its acceptance", 1,
		func(ctx context.Context, c *workidentity.Client, target workidentity.Target, a []string) (any, error) {
			return c.Revise(ctx, workidentity.ReviseRequest{Target: target, ScopeRef: workidentity.UUID(a[0])})
		})
	addWrite("link ID REVISION KIND REFERENCE", "Link an attempt, pr, execution, or artifact", 2,
		func(ctx context.Context, c *workidentity.Client, target workidentity.Target, a []string) (any, error) {
			ref, err := identityReference(a[1])
			if err != nil {
				return nil, err
			}
			link := workidentity.Link{Kind: workidentity.LinkKind(a[0]), Reference: ref}
			return c.Link(ctx, workidentity.LinkRequest{Target: target, Link: link})
		})
	addWrite("accept ID REVISION SCOPE_REVISION EVIDENCE_REF", "Accept this ask's current scope with its own linked evidence", 2,
		func(ctx context.Context, c *workidentity.Client, target workidentity.Target, a []string) (any, error) {
			scope, err := strconv.ParseInt(a[0], 10, 64)
			if err != nil || scope <= 0 {
				return nil, fmt.Errorf("expected a positive SCOPE_REVISION")
			}
			ref, err := identityReference(a[1])
			if err != nil {
				return nil, err
			}
			acceptance := workidentity.Acceptance{ScopeRevision: scope, EvidenceRef: ref}
			return c.Accept(ctx, workidentity.AcceptRequest{Target: target, Acceptance: acceptance})
		})

	for _, op := range []string{"list", "history"} {
		page := workidentity.PageRequest{Limit: 25}
		cmd := add(op, "Read one page of work identity "+op, 0,
			func(ctx context.Context, c *workidentity.Client, _ []string) (any, error) {
				if page.Limit < 1 || page.Limit > 100 {
					return nil, fmt.Errorf("--limit must be from 1 to 100")
				}
				if op == "list" {
					return c.List(ctx, page)
				}
				return c.History(ctx, page)
			})
		cmd.Flags().IntVar(&page.Limit, "limit", 25, "Page size (1–100)")
		cmd.Flags().Int64Var(&page.After, "after", 0, "Previous next_after cursor")
	}
	return identity
}

func identityParent(value string) *workidentity.UUID {
	if value == "" {
		return nil
	}
	id := workidentity.UUID(value)
	return &id
}

func identityReference(value string) (workidentity.UUID, error) {
	if evidence.ValidID(value) {
		return workidentity.UUID(uuid.NewSHA1(uuid.NameSpaceURL, []byte("urn:hev:evidence:"+value)).String()), nil
	}
	ref, err := workidentity.ParseUUID(value)
	if err != nil {
		return "", fmt.Errorf("expected a canonical UUID or evidence package ID")
	}
	return ref, nil
}

func dispatchIdentityClient(ctx context.Context, workspace string, explicitWorkspace bool) (*workidentity.Client, error) {
	var id workidentity.UUID
	if explicitWorkspace {
		var err error
		id, err = workidentity.ParseUUID(workspace)
		if err != nil {
			return nil, fmt.Errorf("--workspace: %w", err)
		}
	}
	endpoint, key := os.Getenv("HEV_WORK_IDENTITY_ENDPOINT"), os.Getenv("HEV_WORK_IDENTITY_KEY")
	if endpoint == "" || key == "" {
		configEndpoint, configKey, err := identityConfigCredentials()
		if err != nil {
			return nil, err
		}
		if endpoint == "" {
			endpoint = configEndpoint
		}
		if key == "" {
			// A new endpoint must never receive an implicit credential from
			// a different service.
			if strings.TrimRight(endpoint, "/") != strings.TrimRight(configEndpoint, "/") {
				return nil, fmt.Errorf("set HEV_WORK_IDENTITY_KEY when overriding the work identity endpoint")
			}
			key = configKey
		}
	}
	if key == "" {
		return nil, fmt.Errorf("sign in to the hevmind.com service, configure Layer, or set HEV_WORK_IDENTITY_KEY")
	}
	if !explicitWorkspace {
		if !kitapi.ValidBaseURL(endpoint) {
			return nil, fmt.Errorf("work identity requires an HTTPS endpoint or loopback HTTP")
		}
		var who kitapi.Whoami
		if err := kitRequest(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/v1/whoami", key, nil, &who); err != nil {
			return nil, fmt.Errorf("discover work workspace from /v1/whoami: %w", err)
		}
		var err error
		id, err = workidentity.ParseUUID(who.WorkWorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("/v1/whoami did not return a valid work_workspace_id; specify --workspace")
		}
	}
	return workidentity.New(endpoint, key, id)
}

func identityConfigCredentials() (string, string, error) {
	kit, err := daemon.ReadKitConfig()
	if err != nil {
		return "", "", err
	}
	if kit.SignedIn() {
		return kit.API, kit.Key, nil
	}
	cfg, err := daemon.LoadConfig()
	if err != nil {
		return "", "", err
	}
	return daemon.LayerWorkIdentityCredentials(cfg)
}
