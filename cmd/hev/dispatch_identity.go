package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/hev/kit/pkg/workidentity"
	"github.com/spf13/cobra"
)

func newDispatchIdentityCommand() *cobra.Command {
	var workspace string
	identity := &cobra.Command{
		Use:   "identity",
		Short: "Read or write shared work identity in Layer",
		Long: "Read or write shared work identity in Layer and print JSON. " +
			"Set HEV_WORK_IDENTITY_ENDPOINT to the Layer URL and HEV_WORK_IDENTITY_KEY to a bearer key with an explicit workspace grant. " +
			"Use resolve to turn an address or alias into the canonical ask ID before writing.",
	}
	identity.PersistentFlags().StringVar(&workspace, "workspace", "", "Layer work workspace UUID")
	client := func() (*workidentity.Client, error) {
		id, err := workidentity.ParseUUID(workspace)
		if err != nil {
			return nil, fmt.Errorf("--workspace: %w", err)
		}
		return workidentity.New(os.Getenv("HEV_WORK_IDENTITY_ENDPOINT"), os.Getenv("HEV_WORK_IDENTITY_KEY"), id)
	}
	add := func(use, short string, argc int, run func(context.Context, *workidentity.Client, []string) (any, error)) *cobra.Command {
		cmd := &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(argc)}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			c, err := client()
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
			link := workidentity.Link{Kind: workidentity.LinkKind(a[0]), Reference: workidentity.UUID(a[1])}
			return c.Link(ctx, workidentity.LinkRequest{Target: target, Link: link})
		})
	addWrite("accept ID REVISION SCOPE_REVISION EVIDENCE_REF", "Accept this ask's current scope with its own linked evidence", 2,
		func(ctx context.Context, c *workidentity.Client, target workidentity.Target, a []string) (any, error) {
			scope, err := strconv.ParseInt(a[0], 10, 64)
			if err != nil || scope <= 0 {
				return nil, fmt.Errorf("expected a positive SCOPE_REVISION")
			}
			acceptance := workidentity.Acceptance{ScopeRevision: scope, EvidenceRef: workidentity.UUID(a[1])}
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
