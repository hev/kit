package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/weave"
	"github.com/spf13/cobra"
)

func init() { rootCmd.AddCommand(newContextCommand()) }

// newContextCommand is Weave's CLI inside hev. Each subcommand hands its
// arguments to the ported implementation unchanged, so flags, errors and
// output match `weave COMMAND`. review also accepts --run NAME [--attempt N]
// to open the bundle a dispatch run on this host was given.
func newContextCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "context",
		Short: "Compose explicit agent context from a recipe",
		Long: `Compose explicit, reproducible agent context (ported from Weave).

  hev context assess  --root DIR --out NEW         find instructions and skills, report findings
  hev context extract --root DIR --out NEW         assess, and copy the sources into NEW/sources
  hev context build   --root DIR --recipe R.json --out NEW
  hev context diff    --before A --after B --out NEW
  hev context review  --bundle DIR [--addr 127.0.0.1:0]
  hev context review  --run NAME [--attempt N]     the bundle a dispatch run on this host was given

A recipe is JSON with a version, a task and an ordered list of sources:
{"version":1,"task":"...","sources":[{"path":"AGENTS.md","target":"AGENTS.md"}]}.
Outputs must be new directories. review binds loopback addresses only.
Name a recipe with "context: PATH" in an order's frontmatter and dispatch
builds the bundle at launch and gives it to the worker; see docs/context.md.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	for _, name := range []string{"assess", "extract", "build", "diff", "review"} {
		name := name
		c.AddCommand(&cobra.Command{
			Use:                name + " [flags]",
			Short:              contextShort[name],
			DisableFlagParsing: true,
			SilenceUsage:       true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if name == "review" {
					var err error
					if args, err = reviewRunArgs(args); err != nil {
						return err
					}
				}
				return weave.Run(append([]string{name}, args...), cmd.OutOrStdout())
			},
		})
	}
	return c
}

var contextShort = map[string]string{
	"assess":  "Discover instructions and skills and report deterministic findings",
	"extract": "Assess and copy discovered sources into a standalone tree",
	"build":   "Build a reproducible bundle from a recipe",
	"diff":    "Compare two bundles",
	"review":  "Serve a bundle's reviewer page on loopback",
}

// reviewRunArgs replaces --run NAME [--attempt N] with --bundle DIR for the
// run's recorded bundle. Other arguments pass through untouched.
func reviewRunArgs(args []string) ([]string, error) {
	var out []string
	run, attempt := "", 0
	for i := 0; i < len(args); i++ {
		a := args[i]
		key, value, inline := strings.Cut(a, "=")
		if key != "--run" && key != "--attempt" {
			out = append(out, a)
			continue
		}
		if !inline {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("%s requires a value", key)
			}
			i++
			value = args[i]
		}
		if key == "--run" {
			run = value
			continue
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("--attempt must be a positive attempt number")
		}
		attempt = n
	}
	if run == "" {
		if attempt != 0 {
			return nil, fmt.Errorf("--attempt requires --run")
		}
		return out, nil
	}
	for _, a := range out {
		if a == "--bundle" || strings.HasPrefix(a, "--bundle=") {
			return nil, fmt.Errorf("use --run or --bundle, not both")
		}
	}
	dir, err := dispatch.ContextBundleDir(run, attempt)
	if err != nil {
		return nil, err
	}
	return append(out, "--bundle", dir), nil
}
