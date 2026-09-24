package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
	"github.com/spf13/cobra"
)

func newEvalCmd() *cobra.Command {
	var namespace, file string
	root := &cobra.Command{Use: "eval", Short: "Store trace evaluations"}
	put := &cobra.Command{Use: "put [file]", Short: "Put generic evaluation JSON rows into Layer", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			if file != "" {
				return fmt.Errorf("use a file argument or --file, not both")
			}
			file = args[0]
		}
		var input io.Reader = cmd.InOrStdin()
		if file != "" && file != "-" {
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			defer f.Close()
			input = f
		}
		cl, err := client(namespace)
		if err != nil {
			return err
		}
		return putEvals(input, cmd.OutOrStdout(), cl.WriteEvals)
	}}
	put.Flags().StringVar(&namespace, "namespace", "", "Layer namespace (evals use the -evals suffix)")
	put.Flags().StringVar(&file, "file", "", "JSON rows file (default stdin)")
	root.AddCommand(put)
	return root
}
func putEvals(input io.Reader, output io.Writer, write func([]trace.Eval) (layer.WriteResult, error)) error {
	decoder := json.NewDecoder(input)
	batch := []trace.Eval{}
	total, tokens := 0, 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		result, err := write(batch)
		if err != nil {
			return err
		}
		total += len(batch)
		tokens += result.EmbeddingTokens
		batch = nil
		return nil
	}
	for row := 1; ; row++ {
		var e trace.Eval
		err := decoder.Decode(&e)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("eval row %d: %w", row, err)
		}
		batch = append(batch, e)
		if len(batch) == 30 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}
	fmt.Fprintf(output, "put %d eval rows; %d embedding tokens\n", total, tokens)
	return nil
}
func init() { rootCmd.AddCommand(newEvalCmd()) }
