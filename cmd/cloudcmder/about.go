package main

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
)

//go:embed docs/about.md
var aboutMD string

func newAboutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "about",
		Short: "What cloudcmder is and how it works",
		RunE: func(_ *cobra.Command, _ []string) error {
			return renderMD(aboutMD)
		},
	}
}

func renderMD(src string) error {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		return err
	}
	out, err := r.Render(src)
	if err != nil {
		return err
	}
	fmt.Fprint(os.Stdout, out)
	return nil
}
