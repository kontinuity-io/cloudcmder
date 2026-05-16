package main

import (
	"fmt"
	"io"
	"os"

	"github.com/arsham/figurine/figurine"
	"github.com/spf13/cobra"

	"charm.land/lipgloss/v2"
	"cloudcmder.com/internal/tui/style"
	"cloudcmder.com/internal/version"
)

func newVersionCmd() *cobra.Command {
	var short bool
	c := &cobra.Command{
		Use:   "version",
		Short: "Print cloudcmder version with build metadata",
		RunE: func(_ *cobra.Command, _ []string) error {
			if short {
				fmt.Println(version.String())
				return nil
			}
			renderBanner(os.Stdout)
			return nil
		},
	}
	c.Flags().BoolVar(&short, "short", false, "Print one-line version for scripting")
	return c
}

func renderBanner(w io.Writer) {
	info := version.Get()

	// Wordmark via figurine (ANSI Regular figlet font with per-glyph gradient).
	// Honour NO_COLOR so CI and pipes get clean output.
	if os.Getenv("NO_COLOR") == "" {
		_ = figurine.Write(w, "cloudcmder", "ANSI Regular.flf")
	} else {
		fmt.Fprintln(w, "cloudcmder")
	}

	// Metadata box.
	meta := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Foreground(style.ColorAccent).Render("☁  cloudcmder ")+
			lipgloss.NewStyle().Foreground(style.ColorPeach).Render(info.Version)+
			lipgloss.NewStyle().Foreground(style.ColorDim).Render(" on ")+
			lipgloss.NewStyle().Foreground(style.ColorTeal).Render(info.OS+"/"+info.Arch),
		lipgloss.NewStyle().Foreground(style.ColorDim).Render("commit ")+
			lipgloss.NewStyle().Foreground(style.ColorLavender).Render(info.Commit)+
			lipgloss.NewStyle().Foreground(style.ColorDim).Render("  built ")+
			lipgloss.NewStyle().Foreground(style.ColorDim).Render(info.Date)+
			lipgloss.NewStyle().Foreground(style.ColorDim).Render("  "+info.GoVer),
		"",
		lipgloss.NewStyle().Foreground(style.ColorDim).Render("Inventory · navigate · export your GCP estate"),
		lipgloss.NewStyle().Foreground(style.ColorDim).Render("Docs:  ")+
			lipgloss.NewStyle().Foreground(style.ColorSapphire).Render("https://cloudcmder.com/docs.html"),
		lipgloss.NewStyle().Foreground(style.ColorDim).Render("Web:   ")+
			lipgloss.NewStyle().Foreground(style.ColorSapphire).Render("https://cloudcmder.com"),
	)

	fmt.Fprintln(w, style.BorderBanner.Render(meta))
}
