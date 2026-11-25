package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/peers/compat"
	"github.com/hev/kit/internal/workspace"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&workspaceFlag, "workspace", "w", "", "Select workspace")
	rootCmd.AddCommand(newWorkspaceCmd())
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if cmd.DisableFlagParsing {
			return nil
		}
		// A subcommand may use the same flag name for a different kind of workspace.
		flag := rootCmd.PersistentFlags().Lookup("workspace")
		if flag.Changed && workspaceFlag == "" {
			return fmt.Errorf("workspace name must not be empty")
		}
		if workspaceFlag != "" || os.Getenv("HEV_WORKSPACE") != "" {
			_, err := currentWorkspace()
			return err
		}
		return nil
	}
}

func newWorkspaceCmd() *cobra.Command {
	root := &cobra.Command{Use: "workspace", Short: "Manage front-of-house workspaces"}
	for _, action := range []string{"ls", "current", "show"} {
		args := cobra.NoArgs
		if action == "show" {
			args = cobra.ExactArgs(1)
		}
		c := &cobra.Command{Use: action, Args: args, RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := workspace.Read(layerenv.ConfigPath())
			if err != nil {
				return err
			}
			selected, err := currentWorkspace()
			if err != nil {
				return err
			}
			isCurrent := true
			if action == "show" {
				row, ok := cfg.Workspaces[args[0]]
				if !ok {
					return fmt.Errorf("unknown workspace %q", args[0])
				}
				isCurrent = args[0] == selected.Name
				selected = workspace.Selection{Name: args[0], Workspace: row}
			}
			rows := []workspaceView{viewWorkspace(selected, isCurrent)}
			if action == "ls" {
				rows = nil
				for _, name := range slices.Sorted(maps.Keys(cfg.Workspaces)) {
					row := workspace.Selection{Name: name, Workspace: cfg.Workspaces[name]}
					if name == selected.Name {
						row.Source = selected.Source
					}
					rows = append(rows, viewWorkspace(row, name == selected.Name))
				}
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				if action == "ls" {
					return enc.Encode(rows)
				}
				return enc.Encode(rows[0])
			}
			out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(out, "WORKSPACE\tDIR\tLOGIN\tHOSTS\tTRACES\tBOARD\tSTATE\tSOURCE")
			for _, row := range rows {
				marker := " "
				if row.Current {
					marker = "*"
				}
				login, state := row.Login, "active"
				if login == "" {
					login = "shared"
				}
				if row.Archived {
					state = "archived"
				}
				fmt.Fprintf(out, "%s %s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", marker, row.Name, row.Dir, login, strings.Join(row.Hosts, ","), row.Traces, row.Board, state, row.Source)
			}
			return out.Flush()
		}}
		c.Flags().Bool("json", false, "Print JSON")
		root.AddCommand(c)
	}
	root.AddCommand(&cobra.Command{Use: "use NAME", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		return workspace.Use(layerenv.ConfigPath(), args[0])
	}})
	var added workspace.Workspace
	add := &cobra.Command{Use: "add NAME", Short: "Add a workspace", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		name := args[0]
		if !workspace.ValidName(name) || added.Dir == "" || (added.Login != "" && added.Login != "own") || (added.Color != "" && added.Color != workspace.Color(added.Color)) {
			return fmt.Errorf("provide a valid name and directory, optional hex color, and login mode own or empty")
		}
		return workspace.Update(layerenv.ConfigPath(), func(current map[string]workspace.Workspace) error {
			if _, exists := current[name]; exists {
				return fmt.Errorf("workspace %q already exists", name)
			}
			current[name] = added
			return nil
		})
	}}
	add.Flags().StringVar(&added.Dir, "dir", "", "Workspace directory")
	add.Flags().StringVar(&added.Icon, "icon", "", "Workspace icon")
	add.Flags().StringVar(&added.Color, "color", "", "Hex color")
	add.Flags().StringVar(&added.Login, "login", "", "Set to own for a separate login")
	add.Flags().StringVar(&added.Layer.TracesNamespace, "traces-namespace", "", "Hosted traces namespace")
	add.Flags().StringVar(&added.Layer.BoardNamespace, "board-namespace", "", "Hosted board namespace")
	add.Flags().StringSliceVar(&added.Hosts, "hosts", nil, "Dispatch host labels")
	root.AddCommand(add, &cobra.Command{Use: "archive NAME", Short: "Move a workspace to the archive", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		return workspace.Update(layerenv.ConfigPath(), func(current map[string]workspace.Workspace) error {
			row, ok := current[args[0]]
			if !ok || args[0] == workspace.Default {
				return fmt.Errorf("cannot archive workspace %q", args[0])
			}
			row.Archived = true
			current[args[0]] = row
			return nil
		})
	}})
	var themes, lists string
	importCmd := &cobra.Command{Use: "import", Short: "Import the shell picker's themes and client lists", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		entries, err := workspace.Import(themes, lists)
		if err != nil {
			return err
		}
		return workspace.Update(layerenv.ConfigPath(), func(current map[string]workspace.Workspace) error {
			for name := range entries {
				if _, exists := current[name]; exists && name != workspace.Default {
					return fmt.Errorf("workspace %q already exists", name)
				}
			}
			if main := current[workspace.Default]; len(main.Projects) > 0 {
				return fmt.Errorf("default workspace already has projects")
			}
			main := current[workspace.Default]
			imported := entries[workspace.Default]
			main.Projects = imported.Projects
			if main.Color == "" || main.Color == workspace.Color("") {
				main.Color = imported.Color
			}
			entries[workspace.Default] = main
			maps.Copy(current, entries)
			return nil
		})
	}}
	importCmd.Flags().StringVar(&themes, "themes", "", "Path to config/themes.json")
	importCmd.Flags().StringVar(&lists, "lists", "", "Path to tmux-config.sh")
	root.AddCommand(importCmd)
	return root
}

var workspaceFlag string

func currentWorkspace() (workspace.Selection, error) {
	cwd, _ := os.Getwd()
	return workspace.Resolve(workspaceFlag, cwd)
}

type workspaceView struct {
	workspace.Selection
	Current bool   `json:"current"`
	Traces  string `json:"traces"`
	Board   string `json:"board"`
}

func viewWorkspace(s workspace.Selection, current bool) workspaceView {
	s.Dir = workspace.Expand(s.Dir)
	traces, board := s.Namespaces()
	if s.TraceSettings().KeyRef == "" && s.IsolatedStorage() {
		traces = filepath.Join(s.Root(), "archive")
		if s.TraceCapture.ArchiveDir != "" {
			traces = workspace.Expand(s.TraceCapture.ArchiveDir)
		}
	}
	if s.Layer.KeyRef == "" && s.IsolatedStorage() {
		board = filepath.Join(s.Root(), "board")
	}
	if s.Layer.BoardURL != "" {
		board = s.Layer.BoardURL
	}
	return workspaceView{Selection: s, Current: current, Traces: traces, Board: board}
}

// Board and peers own their argument parsers, so extract the persistent
// workspace option before forwarding the remaining arguments.
func workspaceRawArgs(args []string) ([]string, error) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return append(out, args[i:]...), nil
		}
		switch {
		case arg == "-w" || arg == "--workspace":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("%s needs a workspace name", arg)
			}
			workspaceFlag = args[i]
		case strings.HasPrefix(arg, "--workspace="):
			workspaceFlag = strings.TrimPrefix(arg, "--workspace=")
		case strings.HasPrefix(arg, "-w") && len(arg) > 2:
			workspaceFlag = strings.TrimPrefix(arg[2:], "=")
		default:
			out = append(out, arg)
			continue
		}
		if workspaceFlag == "" {
			return nil, fmt.Errorf("workspace name must not be empty")
		}
	}
	return out, nil
}

func peersWorkspaceScope(w workspace.Selection) (compat.Scope, error) {
	cfg, err := workspace.Read(layerenv.ConfigPath())
	if err != nil {
		return compat.Scope{}, err
	}
	return compat.Scope{Workspace: w, Enabled: len(cfg.Workspaces) > 1 || len(w.Hosts) > 0, Workspaces: cfg.Workspaces}, nil
}
