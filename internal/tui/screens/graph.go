package screens

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/core"
	"cloudcmder.com/internal/tui/style"
)

// GraphView renders the resource's connection cluster as an ASCII tree.
// Pushed by Detail when the user presses `g`. Esc returns (handled by App).
type GraphView struct {
	res   inventory.Resource
	edges []store.Edge
}

// NewGraphView builds the screen from the (already-loaded) edge slice.
func NewGraphView(res inventory.Resource, edges []store.Edge) *GraphView {
	return &GraphView{res: res, edges: edges}
}

// Title satisfies core.Screen.
func (g *GraphView) Title() string { return "Graph: " + g.res.Name }

// Init is a no-op; everything we need was already loaded by Detail.
func (g *GraphView) Init() tea.Cmd { return nil }

// Update closes the modal on Esc and ignores everything else.
func (g *GraphView) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "esc" {
		return g, core.PopScreenCmd()
	}
	return g, nil
}

// View renders the tree inside the standard rounded border.
func (g *GraphView) View() string {
	myRef := g.res.Ref.String()
	out := buildTree(g.res.Name, g.edges, myRef)
	if out == "" {
		out = style.Dim.Render("  (no connections recorded for this resource)")
	}
	return style.BorderActive.Render(out)
}

func buildTree(name string, edges []store.Edge, myRef string) string {
	outgoing := groupBy(filterEdges(edges, myRef, true))
	incoming := groupBy(filterEdges(edges, myRef, false))

	var sb strings.Builder
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		style.Accent.Render(name),
		style.Dim.Render("  ("+string(refOnlyKind(myRef))+")"),
	))
	sb.WriteString("\n")

	if len(outgoing) > 0 {
		sb.WriteString(style.Dim.Render("│"))
		sb.WriteString("\n")
		sb.WriteString("├── " + style.Accent.Render("OUTGOING"))
		sb.WriteString("\n")
		writeRefKindBlocks(&sb, outgoing, "│   ", true)
	}
	if len(incoming) > 0 {
		sb.WriteString(style.Dim.Render("│"))
		sb.WriteString("\n")
		sb.WriteString("└── " + style.Accent.Render("INCOMING"))
		sb.WriteString("\n")
		writeRefKindBlocks(&sb, incoming, "    ", false)
	}
	return sb.String()
}

func writeRefKindBlocks(sb *strings.Builder, m map[inventory.RefKind][]store.Edge, prefix string, outgoing bool) {
	kinds := sortedRefKinds(m)
	for ki, k := range kinds {
		isLastKind := ki == len(kinds)-1
		branch := "├── "
		childPrefix := prefix + "│   "
		if isLastKind {
			branch = "└── "
			childPrefix = prefix + "    "
		}
		sb.WriteString(prefix + branch + style.Dim.Render(string(k)))
		sb.WriteString("\n")
		entries := m[k]
		for ei, e := range entries {
			isLastEntry := ei == len(entries)-1
			leafBranch := "├── "
			if isLastEntry {
				leafBranch = "└── "
			}
			ref := e.ToRef
			if !outgoing {
				ref = e.FromRef
			}
			sb.WriteString(childPrefix + leafBranch + humanRef(ref))
			sb.WriteString("\n")
		}
	}
}

// refOnlyKind extracts the Kind segment from a canonical ref string. Used by
// the tree header to label the focal node.
func refOnlyKind(ref string) inventory.Kind {
	parts := strings.SplitN(ref, ":", 4)
	if len(parts) != 4 {
		return ""
	}
	return inventory.Kind(parts[2])
}
