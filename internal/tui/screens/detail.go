package screens

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/core"
	"cloudcmder.com/internal/tui/style"
)

type edgesLoadedMsg struct {
	edges []store.Edge
	err   error
}

// Detail is the two-pane drill-down screen for a single Resource.
type Detail struct {
	ctx    context.Context
	st     *store.Store
	run    store.RunSummary
	res    inventory.Resource
	detail any

	width  int
	height int

	loaded  bool
	loadErr error
	edges   []store.Edge

	graphKey key.Binding
}

// NewDetail constructs the Detail screen for the supplied resource. The
// caller passes the already-decoded kind-specific Detail (avoids re-decoding
// the json.RawMessage that LoadResources hands back).
func NewDetail(ctx context.Context, st *store.Store, run store.RunSummary, res inventory.Resource, detail any) *Detail {
	return &Detail{
		ctx: ctx, st: st, run: run, res: res, detail: detail,
		graphKey: key.NewBinding(key.WithKeys("g")),
	}
}

// Title satisfies core.Screen.
func (d *Detail) Title() string { return d.res.Name }

// Init loads edges from the store; the resource and its detail are already in
// hand from the parent ResourceList.
func (d *Detail) Init() tea.Cmd {
	return func() tea.Msg {
		es, err := d.st.LoadEdges(d.ctx, d.run.ID)
		return edgesLoadedMsg{edges: es, err: err}
	}
}

// Update handles load completion, resize, and the `g` (GraphView) push.
func (d *Detail) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case edgesLoadedMsg:
		d.loaded = true
		d.loadErr = m.err
		d.edges = m.edges
		return d, nil
	case tea.WindowSizeMsg:
		d.width = m.Width
		d.height = m.Height
		return d, nil
	case tea.KeyMsg:
		if key.Matches(m, d.graphKey) {
			return d, core.PushScreenCmd(NewGraphView(d.res, d.edges))
		}
	}
	return d, nil
}

// CurrentRun lets the App's :alias palette discover the run this Detail
// belongs to.
func (d *Detail) CurrentRun() *store.RunSummary { return &d.run }

// View renders Detail content as a single vertical stream: kind-specific
// fields, a separator, then the connections list. Frame controls the
// surrounding border and layout.
func (d *Detail) View() string {
	if !d.loaded {
		return style.Dim.Render("loading detail…")
	}
	if d.loadErr != nil {
		return lipgloss.NewStyle().Foreground(style.ColorError).
			Render("error loading edges: " + d.loadErr.Error())
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		d.detailPane(),
		"",
		style.Separator(40),
		"",
		d.connectionsPane(),
	)
	return body
}

func (d *Detail) detailPane() string {
	header := style.Accent.Render("DETAIL — ") + style.Dim.Render(string(d.res.Kind))
	rows := []string{header, style.Separator(40)}

	switch d.res.Kind {
	case inventory.KindVM:
		rows = append(rows, vmDetailRows(d.res, d.detail)...)
	case inventory.KindDisk:
		rows = append(rows, diskDetailRows(d.res, d.detail)...)
	case inventory.KindNetwork:
		rows = append(rows, networkDetailRows(d.res, d.detail)...)
	case inventory.KindSubnet:
		rows = append(rows, subnetDetailRows(d.res, d.detail)...)
	case inventory.KindFirewall:
		rows = append(rows, firewallDetailRows(d.res, d.detail)...)
	case inventory.KindLoadBalancer:
		rows = append(rows, lbDetailRows(d.res, d.detail)...)
	case inventory.KindDatabase:
		rows = append(rows, databaseDetailRows(d.res, d.detail)...)
	case inventory.KindCluster:
		rows = append(rows, clusterDetailRows(d.res, d.detail)...)
	case inventory.KindBucket:
		rows = append(rows, bucketDetailRows(d.res, d.detail)...)
	case inventory.KindFunction:
		rows = append(rows, functionDetailRows(d.res, d.detail)...)
	default:
		rows = append(rows,
			kvLine("Name", d.res.Name),
			kvLine("Region", d.res.Region),
			kvLine("Status", style.Status(d.res.Status).Render(d.res.Status)),
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func diskDetailRows(res inventory.Resource, detail any) []string {
	dd, _ := detail.(*inventory.DiskDetail)
	if dd == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	out := []string{
		kvLine("Size", fmt.Sprintf("%d GB", dd.SizeGB)),
		kvLine("Type", dd.Type),
		kvLine("Zone", dd.Zone),
		kvLine("Status", style.Status(res.Status).Render(res.Status)),
	}
	if len(dd.InUseBy) > 0 {
		out = append(out, "", style.Accent.Render("Attached to"))
		for _, ref := range dd.InUseBy {
			out = append(out, "  "+ref.ID)
		}
	}
	return out
}

func networkDetailRows(_ inventory.Resource, detail any) []string {
	nd, _ := detail.(*inventory.NetworkDetail)
	if nd == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	return []string{
		kvLine("Auto subnet", boolStr(nd.AutoSubnet)),
		kvLine("IPv4 range", nd.IPv4Range),
		kvLine("Subnets", fmt.Sprintf("%d", nd.SubnetCount)),
	}
}

func subnetDetailRows(_ inventory.Resource, detail any) []string {
	sd, _ := detail.(*inventory.SubnetDetail)
	if sd == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	return []string{
		kvLine("CIDR", sd.CIDR),
		kvLine("Region", sd.Region),
		kvLine("Network", sd.Network),
		kvLine("Private GA", boolStr(sd.Private)),
	}
}

func firewallDetailRows(_ inventory.Resource, detail any) []string {
	fd, _ := detail.(*inventory.FirewallDetail)
	if fd == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	out := []string{
		kvLine("Direction", fd.Direction),
		kvLine("Priority", fmt.Sprintf("%d", fd.Priority)),
		kvLine("Sources", strings.Join(fd.SourceRanges, ", ")),
		kvLine("Tags", strings.Join(fd.TargetTags, ", ")),
	}
	if len(fd.Allowed) > 0 {
		out = append(out, "", style.Accent.Render("Allowed"))
		for _, a := range fd.Allowed {
			out = append(out, "  "+a.Protocol+":"+strings.Join(a.Ports, ","))
		}
	}
	return out
}

func lbDetailRows(_ inventory.Resource, detail any) []string {
	lb, _ := detail.(*inventory.LoadBalancerDetail)
	if lb == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	return []string{
		kvLine("Scheme", lb.Scheme),
		kvLine("Protocol", lb.Protocol),
		kvLine("IP", lb.IPAddress),
		kvLine("Ports", strings.Join(lb.Ports, ", ")),
		kvLine("Backends", fmt.Sprintf("%d", lb.BackendCount)),
	}
}

func databaseDetailRows(res inventory.Resource, detail any) []string {
	dd, _ := detail.(*inventory.DatabaseDetail)
	if dd == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	vcpus := "—"
	if dd.VCPUs > 0 {
		vcpus = fmt.Sprintf("%d", dd.VCPUs)
	}
	mem := "—"
	if dd.MemoryMiB > 0 {
		mem = fmt.Sprintf("%.1f GiB", float64(dd.MemoryMiB)/1024.0)
	}
	return []string{
		kvLine("Engine", dd.Engine),
		kvLine("Tier", dd.Tier),
		kvLine("vCPUs", vcpus),
		kvLine("Memory", mem),
		kvLine("Storage", fmt.Sprintf("%d GB %s", dd.StorageGB, dd.StorageType)),
		kvLine("HA", boolStr(dd.HighAvailability)),
		kvLine("Maintenance", dd.MaintenanceWindow),
		kvLine("Status", style.Status(res.Status).Render(res.Status)),
	}
}

func clusterDetailRows(res inventory.Resource, detail any) []string {
	cd, _ := detail.(*inventory.ClusterDetail)
	if cd == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	return []string{
		kvLine("Version", cd.Version),
		kvLine("Location", cd.Location),
		kvLine("Nodes", fmt.Sprintf("%d", cd.NodeCount)),
		kvLine("Node MT", cd.NodeMachine),
		kvLine("Disk GB", fmt.Sprintf("%d", cd.NodeDiskGB)),
		kvLine("Autopilot", boolStr(cd.Autopilot)),
		kvLine("Status", style.Status(res.Status).Render(res.Status)),
	}
}

func bucketDetailRows(_ inventory.Resource, detail any) []string {
	bd, _ := detail.(*inventory.BucketDetail)
	if bd == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	return []string{
		kvLine("Location", bd.Location),
		kvLine("Class", bd.StorageClass),
		kvLine("Public", boolStr(bd.PublicAccess)),
		kvLine("Versioning", boolStr(bd.Versioning)),
	}
}

func functionDetailRows(res inventory.Resource, detail any) []string {
	fd, _ := detail.(*inventory.FunctionDetail)
	if fd == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	mem := "—"
	if fd.MemoryMiB > 0 {
		mem = fmt.Sprintf("%d MiB", fd.MemoryMiB)
	}
	cpu := "—"
	if fd.CPUs > 0 {
		cpu = fmt.Sprintf("%g", fd.CPUs)
	}
	return []string{
		kvLine("Runtime", fd.Runtime),
		kvLine("Trigger", fd.Trigger),
		kvLine("Memory", mem),
		kvLine("CPUs", cpu),
		kvLine("Max inst", fmt.Sprintf("%d", fd.MaxInst)),
		kvLine("Region", fd.Region),
		kvLine("Status", style.Status(res.Status).Render(res.Status)),
	}
}

func vmDetailRows(res inventory.Resource, detail any) []string {
	vm, _ := detail.(*inventory.VMDetail)
	if vm == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	out := []string{
		kvLine("Machine", vm.MachineType),
		kvLine("vCPUs", fmt.Sprintf("%d", vm.VCPUs)),
		kvLine("Memory", fmt.Sprintf("%.1f GiB", float64(vm.MemoryMiB)/1024.0)),
		kvLine("OS", vm.OSFamily),
		kvLine("Status", style.Status(res.Status).Render(res.Status)),
		kvLine("Zone", vm.Zone),
		kvLine("CPU Plat", vm.CPUPlatform),
	}
	if vm.Preemptible || vm.Spot {
		mods := []string{}
		if vm.Preemptible {
			mods = append(mods, "preemptible")
		}
		if vm.Spot {
			mods = append(mods, "spot")
		}
		out = append(out, kvLine("Mode", strings.Join(mods, ", ")))
	}
	if vm.BootDisk.Name != "" {
		out = append(out, "", style.Accent.Render("Boot disk"))
		out = append(out, "  "+vm.BootDisk.Name+"  "+vm.BootDisk.Type+"  "+
			fmt.Sprintf("%dG", vm.BootDisk.SizeGB))
	}
	if len(vm.AttachedDisks) > 0 {
		out = append(out, "", style.Accent.Render("Attached"))
		for _, ad := range vm.AttachedDisks {
			out = append(out, "  "+ad.Name+"  "+
				fmt.Sprintf("%dG", ad.SizeGB))
		}
	}
	if len(vm.NICs) > 0 {
		out = append(out, "", style.Accent.Render("NICs"))
		for _, n := range vm.NICs {
			line := "  " + n.Subnetwork + "  " + n.InternalIP
			if n.ExternalIP != "" {
				line += "  → " + n.ExternalIP
			}
			out = append(out, line)
		}
	}
	return out
}

func (d *Detail) connectionsPane() string {
	myRef := d.res.Ref.String()
	header := style.Accent.Render("CONNECTIONS")
	rows := []string{header, style.Separator(40)}

	outgoing := groupBy(filterEdges(d.edges, myRef, true))
	incoming := groupBy(filterEdges(d.edges, myRef, false))

	if len(outgoing) == 0 && len(incoming) == 0 {
		rows = append(rows, style.Dim.Render("(no connections recorded)"))
		return lipgloss.JoinVertical(lipgloss.Left, rows...)
	}

	for _, kind := range sortedRefKinds(outgoing) {
		rows = append(rows, style.Dim.Render(d.res.Name+" → "+string(kind)+":"))
		for _, e := range outgoing[kind] {
			rows = append(rows, "  "+humanRef(e.ToRef))
		}
		rows = append(rows, "")
	}
	for _, kind := range sortedRefKinds(incoming) {
		rows = append(rows, style.Dim.Render(string(kind)+" → "+d.res.Name+":"))
		for _, e := range incoming[kind] {
			rows = append(rows, "  "+humanRef(e.FromRef))
		}
		rows = append(rows, "")
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func filterEdges(edges []store.Edge, myRef string, outgoing bool) []store.Edge {
	out := make([]store.Edge, 0, len(edges))
	for _, e := range edges {
		match := e.FromRef == myRef
		if !outgoing {
			match = e.ToRef == myRef
		}
		if match {
			out = append(out, e)
		}
	}
	return out
}

func groupBy(edges []store.Edge) map[inventory.RefKind][]store.Edge {
	out := map[inventory.RefKind][]store.Edge{}
	for _, e := range edges {
		out[e.RefKind] = append(out[e.RefKind], e)
	}
	return out
}

func sortedRefKinds(m map[inventory.RefKind][]store.Edge) []inventory.RefKind {
	out := make([]inventory.RefKind, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// stable order without bringing in sort.Slice every render: bubble sort by
	// string for the small RefKind set (≤6 entries).
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && string(out[j]) < string(out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// humanRef strips the "provider:scope:Kind:" prefix, leaving "Kind/id" so the
// connections list reads cleanly without the boilerplate.
func humanRef(ref string) string {
	parts := strings.SplitN(ref, ":", 4)
	if len(parts) != 4 {
		return ref
	}
	return parts[2] + "/" + parts[3]
}

func kvLine(k, v string) string {
	return style.Dim.Render(fmt.Sprintf("%-10s ", k+":")) + v
}
