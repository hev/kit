package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// executePlugin only handles unknown top-level commands. Cobra owns all built-ins.
func executePlugin(root *cobra.Command, args []string, in io.Reader, out, errout io.Writer) (bool, int) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return false, 0
	}
	for _, c := range root.Commands() {
		if c.Name() == args[0] || c.HasAlias(args[0]) {
			return false, 0
		}
	}
	if args[0] == "help" || args[0] == "completion" {
		return false, 0
	}
	path, err := exec.LookPath("hev-" + args[0])
	if err != nil {
		return false, 0
	}
	code, err := runPlugin(path, args[1:], in, out, errout)
	if err != nil {
		fmt.Fprintln(errout, err)
		return true, 1
	}
	return true, code
}

func discoveredPlugins(root *cobra.Command) []string {
	names := map[string]bool{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			n := entry.Name()
			if entry.IsDir() || !strings.HasPrefix(n, "hev-") {
				continue
			}
			if runtime.GOOS == "windows" {
				ext := strings.ToLower(filepath.Ext(n))
				if ext != ".exe" && ext != ".cmd" {
					continue
				}
				n = strings.TrimSuffix(n, filepath.Ext(n))
			}
			n = strings.TrimPrefix(n, "hev-")
			if n == "" {
				continue
			}
			if _, err := exec.LookPath("hev-" + n); err != nil {
				continue
			}
			builtin := n == "help" || n == "completion"
			for _, c := range root.Commands() {
				builtin = builtin || c.Name() == n || c.HasAlias(n)
			}
			if !builtin {
				names[n] = true
			}
		}
	}
	result := []string{}
	for n := range names {
		result = append(result, n)
	}
	sort.Strings(result)
	return result
}

func installPluginHelp(root *cobra.Command) {
	original := root.HelpFunc()
	root.SetHelpFunc(func(c *cobra.Command, args []string) {
		original(c, args)
		if c == root {
			if names := discoveredPlugins(root); len(names) > 0 {
				fmt.Fprintln(c.OutOrStdout(), "\nPlugins:")
				for _, n := range names {
					fmt.Fprintln(c.OutOrStdout(), "  "+n)
				}
			}
		}
	})
}
