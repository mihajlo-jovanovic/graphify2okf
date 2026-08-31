package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

const (
	defaultInput   = "./graphify-out/graph.json"
	defaultOutput  = "./okf-bundle"
	defaultGroupBy = "directory"
)

type generateOptions struct {
	input   string
	output  string
	groupBy string
}

func newRootCommand(stdout, stderr io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:          "graphify2okf",
		Short:        "Convert a Graphify graph to an OKF bundle",
		SilenceUsage: true,
	}
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.AddCommand(newGenerateCommand())
	return command
}

func newGenerateCommand() *cobra.Command {
	options := generateOptions{}
	command := &cobra.Command{
		Use:   "generate",
		Short: "Parse a Graphify graph and generate an OKF bundle",
		Args:  cobra.NoArgs,
		PreRunE: func(command *cobra.Command, _ []string) error {
			if options.groupBy != "directory" && options.groupBy != "community" {
				return fmt.Errorf("invalid --group-by %q: must be directory or community", options.groupBy)
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			file, err := os.Open(options.input)
			if err != nil {
				return fmt.Errorf("open input %q: %w", options.input, err)
			}
			defer file.Close()

			if _, err := parseGraph(file); err != nil {
				return fmt.Errorf("parse input %q: %w", options.input, err)
			}
			// Bundle planning and writing are implemented in subsequent stages.
			return nil
		},
	}

	flags := command.Flags()
	flags.StringVarP(&options.input, "input", "i", defaultInput, "path to Graphify graph.json")
	flags.StringVarP(&options.output, "output", "o", defaultOutput, "path to the OKF bundle")
	flags.StringVarP(&options.groupBy, "group-by", "g", defaultGroupBy, "group nodes by directory or community")
	return command
}
