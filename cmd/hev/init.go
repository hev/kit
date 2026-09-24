package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hev/kit/internal/daemon"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use: "init", Short: "Interactively configure hev kit",
	Long: "Configure the Layer endpoint, API key, and namespace used by indexing and search.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runInteractiveInit(cmd.InOrStdin(), cmd.OutOrStdout())
	},
}

type initFileConfig struct {
	Layer    initLayerConfig `toml:"layer"`
	source   []byte
	exists   bool
	hasLocal bool
	Capture  struct {
		ScanInterval string `toml:"scan_interval"`
	} `toml:"capture"`
}

type initLayerConfig struct {
	Endpoint  string `toml:"endpoint"`
	APIKey    string `toml:"api_key"`
	Namespace string `toml:"namespace"`
}

type initPrompter struct {
	in  *bufio.Reader
	out io.Writer
}

func runInteractiveInit(in io.Reader, out io.Writer) error {
	p := initPrompter{in: bufio.NewReader(in), out: out}
	path := daemon.DefaultConfigPath()
	fmt.Fprintln(out, "hev init")
	fmt.Fprintln(out, "This writes the Layer target used by the daemon and search commands.")
	fmt.Fprintln(out)
	if _, err := os.Stat(path); err == nil {
		ok, err := p.confirm(fmt.Sprintf("Update existing config at %s?", path), true)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "No changes made.")
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	cfg, err := loadInitConfig(path)
	if err != nil {
		return err
	}
	if cfg.hasLocal {
		fmt.Fprintln(out, "Warning: this config contains [local]. Updating the Layer target preserves local and archive settings and does not stop the local stack. Review layer.store for the hosted target; use HEV_CONFIG for separate local and hosted configs.")
	}
	endpoint, err := p.ask("Layer endpoint", defaultString(cfg.Layer.Endpoint, "https://gcp-us-central1.turbopuffer.com"))
	if err != nil {
		return err
	}
	keyLabel := "Layer API key"
	if cfg.Layer.APIKey != "" {
		keyLabel += " (Enter to keep existing key)"
	}
	apiKey, err := p.ask(keyLabel, "")
	apiKey = defaultString(apiKey, cfg.Layer.APIKey)
	if err != nil {
		return err
	}
	namespace, err := p.ask("Layer namespace", defaultString(cfg.Layer.Namespace, "hev-traces"))
	if err != nil {
		return err
	}
	if strings.TrimSpace(endpoint) == "" || strings.TrimSpace(apiKey) == "" || strings.TrimSpace(namespace) == "" {
		return fmt.Errorf("Layer endpoint, API key, and namespace are required")
	}
	cfg.Layer = initLayerConfig{Endpoint: endpoint, APIKey: apiKey, Namespace: namespace}
	if cfg.Capture.ScanInterval == "" {
		cfg.Capture.ScanInterval = "5m"
	}
	if err := writeInitConfig(path, cfg); err != nil {
		return err
	}
	fmt.Fprintf(out, "\nWrote config: %s\n\nNext:\n  hev config show\n  hev d\n  hev find \"something you remember\"\n", path)
	return nil
}

func (p initPrompter) ask(label, def string) (string, error) {
	if def == "" {
		fmt.Fprintf(p.out, "%s: ", label)
	} else {
		fmt.Fprintf(p.out, "%s [%s]: ", label, def)
	}
	line, err := p.in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = def
	}
	return line, nil
}

func (p initPrompter) confirm(label string, def bool) (bool, error) {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	answer, err := p.ask(label+" ("+hint+")", "")
	if err != nil {
		return false, err
	}
	if answer == "" {
		return def, nil
	}
	switch strings.ToLower(answer) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	}
	return false, fmt.Errorf("expected yes or no, got %q", answer)
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
