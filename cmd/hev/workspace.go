package main

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/workspace"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newWorkspaceCmd(), newFOHCmd(), newPickCmd())
}

func newWorkspaceCmd() *cobra.Command {
	root := &cobra.Command{Use: "workspace", Short: "Manage front-of-house workspaces"}
	root.AddCommand(&cobra.Command{Use: "ls", Short: "List configured workspaces", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := workspace.Read(layerenv.ConfigPath())
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "WORKSPACE\tDIRECTORY\tLOGIN\tSTATE\tHOSTS\tPROJECTS")
		for _, name := range slices.Sorted(maps.Keys(cfg.Workspaces)) {
			row := cfg.Workspaces[name]
			login, state := row.Login, "active"
			if login == "" {
				login = "shared"
			}
			if row.Archived {
				state = "archived"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", name, row.Dir, login, state, strings.Join(row.Hosts, ","), strings.Join(slices.Sorted(maps.Keys(row.Projects)), ","))
		}
		return w.Flush()
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
			maps.Copy(current, entries)
			return nil
		})
	}}
	importCmd.Flags().StringVar(&themes, "themes", "", "Path to config/themes.json")
	importCmd.Flags().StringVar(&lists, "lists", "", "Path to tmux-config.sh")
	root.AddCommand(importCmd)
	return root
}

func xdgPath(env, fallback string) string {
	if path := os.Getenv(env); path != "" {
		return path
	}
	return workspace.Expand(fallback)
}
