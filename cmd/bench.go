package cmd

import (
	_ "embed"

	"github.com/saltyorg/sb-go/host"

	"github.com/spf13/cobra"
)

//go:embed bench_script.sh
var benchmarkScript string

func newBenchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "bench",
		Short: "Runs bench.sh benchmark",
		Long:  `Runs bench.sh benchmark`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return host.RunBenchmark(cmd.Context(), benchmarkScript)
		},
	}
}

func addBenchCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newBenchCommand())
}
