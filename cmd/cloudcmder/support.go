package main

import (
	_ "embed"

	"github.com/spf13/cobra"
)

//go:embed docs/support.md
var supportMD string

func newSupportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "support",
		Short: "Get help, report bugs, and find required IAM roles",
		RunE: func(_ *cobra.Command, _ []string) error {
			return renderMD(supportMD)
		},
	}
}
