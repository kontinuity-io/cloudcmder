package screens

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/core"
	"cloudcmder.com/internal/tui/style"
)

// DetailMode controls what the right pane renders for a Resource. Cycles
// via the `m` key. Full is the default; ConnectionsOnly and RawJSON are
// data-focused alternates; InlineGraph reuses the GraphView ASCII tree
// renderer embedded in the right pane.
type DetailMode int

const (
	DetailModeFull DetailMode = iota
	DetailModeConnectionsOnly
	DetailModeRawJSON
	DetailModeInlineGraph
)

// detailModeCount is the number of distinct DetailMode values; lets
// cycleMode wrap without hardcoding the upper bound.
const detailModeCount = 4

// tabStripHeight is the fixed vertical cost of the tab strip (top border +
// label row + bottom border). Used to shrink the viewport budget so the strip
// stays pinned above scrollable content.
const tabStripHeight = 3

// detailTabs is the ordered display labels for the four DetailModes. Index
// matches the DetailMode iota so detailTabs[d.mode] is always the active tab.
var detailTabs = []string{"Overview", "Connections", "JSON", "Graph"}

type edgesLoadedMsg struct {
	edges []store.Edge
	err   error
}

// Detail is the two-pane drill-down screen for a single Resource. Content
// renders through a bubbles/viewport so resources whose details exceed the
// pane height (VMs with many disks/NICs, Detail panes in narrow terminals)
// stay navigable instead of clipping. Scrollable via ↑/↓ · PgUp/PgDn ·
// Ctrl-u/Ctrl-d (the viewport's default pager bindings).
type Detail struct {
	ctx    context.Context
	st     *store.Store
	run    store.RunSummary
	res    inventory.Resource
	detail any

	spin   spinner.Model
	width  int
	height int

	loaded  bool
	loadErr error
	edges   []store.Edge

	mode       DetailMode
	modeKey    key.Binding
	prevTabKey key.Binding
	nextTabKey key.Binding
	jumpTabKey key.Binding

	graphKey key.Binding

	vp           viewport.Model
	contentDirty bool // when true, View() re-renders and pushes into vp before drawing
}

// NewDetail constructs the Detail screen for the supplied resource. The
// caller passes the already-decoded kind-specific Detail (avoids re-decoding
// the json.RawMessage that LoadResources hands back).
func NewDetail(ctx context.Context, st *store.Store, run store.RunSummary, res inventory.Resource, detail any) *Detail {
	s := spinner.New()
	s.Spinner = spinner.Dot
	vp := viewport.New()
	// Horizontal scroll is meaningless for kvLine output (each line is a
	// short key:value pair); strip the bindings so Left/Right/h/l don't
	// behave inconsistently when the user lands on Detail.
	vp.KeyMap.Left = key.NewBinding(key.WithDisabled())
	vp.KeyMap.Right = key.NewBinding(key.WithDisabled())
	return &Detail{
		ctx: ctx, st: st, run: run, res: res, detail: detail, spin: s,
		modeKey:      key.NewBinding(key.WithKeys("m")),
		prevTabKey:   key.NewBinding(key.WithKeys("shift+left")),
		nextTabKey:   key.NewBinding(key.WithKeys("shift+right")),
		jumpTabKey:   key.NewBinding(key.WithKeys("1", "2", "3", "4")),
		graphKey:     key.NewBinding(key.WithKeys("g")),
		vp:           vp,
		contentDirty: true,
	}
}

// CycleMode advances the right-pane mode to the next DetailMode, wrapping
// back to Full after the last one. Exported so Frame can drive the cycle
// (the user holds Tab+m vs. just m depending on focus).
func (d *Detail) CycleMode() {
	d.mode = (d.mode + 1) % detailModeCount
	d.contentDirty = true
	d.vp.GotoTop()
}

// Title satisfies core.Screen.
func (d *Detail) Title() string { return d.res.Name }

// Init loads edges from the store and kicks the spinner so the right pane
// shows visible feedback during the (~10ms) query.
func (d *Detail) Init() tea.Cmd {
	load := func() tea.Msg {
		es, err := d.st.LoadEdges(d.ctx, d.run.ID)
		return edgesLoadedMsg{edges: es, err: err}
	}
	return tea.Batch(load, d.spin.Tick)
}

// Update handles load completion, resize, mode/graph keys, and forwards
// remaining keys to the viewport so the content scrolls.
func (d *Detail) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case edgesLoadedMsg:
		d.loaded = true
		d.loadErr = m.err
		d.edges = m.edges
		d.contentDirty = true
		return d, nil
	case tea.WindowSizeMsg:
		d.width = m.Width
		d.height = m.Height
		d.vp.SetWidth(m.Width)
		d.vp.SetHeight(m.Height - tabStripHeight)
		d.contentDirty = true
		return d, nil
	case spinner.TickMsg:
		if !d.loaded {
			var cmd tea.Cmd
			d.spin, cmd = d.spin.Update(msg)
			return d, cmd
		}
		return d, nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(m, d.prevTabKey):
			d.mode = (d.mode + detailModeCount - 1) % detailModeCount
			d.contentDirty = true
			d.vp.GotoTop()
			return d, nil
		case key.Matches(m, d.nextTabKey), key.Matches(m, d.modeKey):
			d.CycleMode()
			return d, nil
		case key.Matches(m, d.jumpTabKey):
			if len(m.Text) == 1 && m.Text[0] >= '1' && m.Text[0] <= '4' {
				d.mode = DetailMode(int(m.Text[0] - '1'))
				d.contentDirty = true
				d.vp.GotoTop()
			}
			return d, nil
		case key.Matches(m, d.graphKey):
			return d, core.PushScreenCmd(NewGraphView(d.res, d.edges))
		}
		// Everything else (↑/↓, PgUp/PgDn, Ctrl-u/Ctrl-d, j/k, etc.) →
		// scroll the viewport.
		var cmd tea.Cmd
		d.vp, cmd = d.vp.Update(msg)
		return d, cmd
	}
	return d, nil
}

// CurrentRun lets the App's :alias palette discover the run this Detail
// belongs to.
func (d *Detail) CurrentRun() *store.RunSummary { return &d.run }

// View renders the tab strip (pinned) above a scrollable viewport. The
// viewport is fed by renderBody() — recomputed lazily when content changes
// (mode cycle, edges loaded, resize). Frame / SingleView draws the outer border.
func (d *Detail) View() string {
	if !d.loaded {
		return d.spin.View() + style.Dim.Render(" loading detail…")
	}
	if d.loadErr != nil {
		return lipgloss.NewStyle().Foreground(style.ColorError).
			Render("error loading edges: " + d.loadErr.Error())
	}
	if d.contentDirty || d.vp.Width() == 0 {
		d.vp.SetContent(d.renderBody())
		d.contentDirty = false
	}
	if d.vp.Width() == 0 || d.vp.Height() == 0 {
		// Pre-size fallback before WindowSizeMsg — render combined output
		// without the viewport (lipgloss will clip downstream).
		return lipgloss.JoinVertical(lipgloss.Left, d.tabStrip(), d.renderBody())
	}
	return lipgloss.JoinVertical(lipgloss.Left, d.tabStrip(), d.vp.View())
}

// tabStrip renders the four labelled tabs aligned at the bottom. The active
// tab uses ActiveTabBorder (bottom = " ") so it visually opens into the
// content below. A TabGap extends the bottom rule to d.width.
func (d *Detail) tabStrip() string {
	cells := make([]string, 0, len(detailTabs)+1)
	for i, label := range detailTabs {
		if DetailMode(i) == d.mode {
			cells = append(cells, style.TabActive.Render(label))
		} else {
			cells = append(cells, style.TabInactive.Render(label))
		}
	}
	row := lipgloss.JoinHorizontal(lipgloss.Bottom, cells...)
	if gap := d.width - lipgloss.Width(row); gap > 0 {
		cells = append(cells, style.TabGap.Render(strings.Repeat(" ", gap)))
		row = lipgloss.JoinHorizontal(lipgloss.Bottom, cells...)
	}
	return row
}

// renderBody produces the scrollable content for the current mode. Called by
// View() on demand; result is fed into the viewport.
func (d *Detail) renderBody() string {
	switch d.mode {
	case DetailModeConnectionsOnly:
		return d.connectionsPane()
	case DetailModeRawJSON:
		return d.rawJSONPane()
	case DetailModeInlineGraph:
		return buildTree(d.res.Name, d.edges, d.res.Ref.String())
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		d.detailPane(),
		"",
		style.Separator(40),
		"",
		d.connectionsPane(),
	)
}

// rawJSONPane pretty-prints the decoded Detail struct (or, when the
// resource was scanned with --dump-native, the provider's raw payload).
// Useful when an audit needs to confirm a field that the kind-specific
// rows don't surface.
func (d *Detail) rawJSONPane() string {
	header := style.Accent.Render("RAW JSON — ") + style.Dim.Render(string(d.res.Kind))
	target := d.res.Native
	label := "native"
	if target == nil {
		target = d.detail
		label = "detail"
	}
	if target == nil {
		return lipgloss.JoinVertical(lipgloss.Left,
			header,
			style.Separator(40),
			style.Dim.Render("(no "+label+" data — scan with --dump-native to capture)"),
		)
	}
	body, err := json.MarshalIndent(target, "", "  ")
	if err != nil {
		return lipgloss.JoinVertical(lipgloss.Left,
			header,
			style.Separator(40),
			lipgloss.NewStyle().Foreground(style.ColorError).
				Render("marshal error: "+err.Error()),
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		style.Dim.Render("("+label+")"),
		style.Separator(40),
		string(body),
	)
}

func (d *Detail) detailPane() string {
	rows := []string{style.Dim.Render(string(d.res.Kind)), style.Separator(40)}

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
	case inventory.KindGCPBigQuery:
		rows = append(rows, bigQueryDetailRows(d.res, d.detail)...)
	case inventory.KindGCPPubSub:
		rows = append(rows, pubSubDetailRows(d.res, d.detail)...)
	case inventory.KindGCPMemorystore:
		rows = append(rows, memorystoreDetailRows(d.res, d.detail)...)
	case inventory.KindGCPArtifactRegistry:
		rows = append(rows, artifactRegistryDetailRows(d.res, d.detail)...)
	case inventory.KindGCPSecretManager:
		rows = append(rows, secretManagerDetailRows(d.res, d.detail)...)
	case inventory.KindGCPAppEngine:
		rows = append(rows, appEngineDetailRows(d.res, d.detail)...)
	case inventory.KindGCPVertexAI,
		inventory.KindGCPApigee,
		inventory.KindGCPFirebase,
		inventory.KindGCPDNS,
		inventory.KindGCPCloudScheduler,
		inventory.KindGCPSpanner,
		inventory.KindGCPBigtable,
		inventory.KindGCPKMS,
		inventory.KindGCPDataflow,
		inventory.KindGCPDataproc,
		inventory.KindGCPComposer,
		inventory.KindGCPCloudTasks,
		inventory.KindGCPMonitoring,
		inventory.KindGCPLogging,
		inventory.KindGCPOSConfig,
		inventory.KindGCPVPN,
		inventory.KindGCPRouter,
		inventory.KindGCPCloudBuild:
		rows = append(rows, stubDetailRows(d.res, d.detail)...)
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
	if dd.MarketplaceClass != "" {
		out = append(out, kvLine("Marketplace", dd.MarketplaceClass))
		out = append(out, kvLine("Mkt project", dd.MarketplaceProject))
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
	out := []string{
		kvLine("Version", cd.Version),
		kvLine("Location", cd.Location),
		kvLine("Nodes", fmt.Sprintf("%d", cd.NodeCount)),
		kvLine("Node MT", cd.NodeMachine),
		kvLine("Disk GB", fmt.Sprintf("%d", cd.NodeDiskGB)),
		kvLine("Serverless", boolStr(cd.Serverless)),
		kvLine("Status", style.Status(res.Status).Render(res.Status)),
	}
	if s := inventory.AcceleratorSummary(cd.Accelerators); s != "" {
		out = append(out, kvLine("GPUs", s))
	}
	return out
}

func bucketDetailRows(_ inventory.Resource, detail any) []string {
	bd, _ := detail.(*inventory.BucketDetail)
	if bd == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	rows := []string{
		kvLine("Location", bd.Location),
		kvLine("Class", bd.StorageClass),
		kvLine("Public", boolStr(bd.PublicAccess)),
		kvLine("Versioning", boolStr(bd.Versioning)),
		kvLine("Size", formatBytes(bd.SizeBytes)),
		kvLine("Objects", formatCount(bd.ObjectCount)),
	}
	if bd.SizeBytes == 0 && bd.ObjectCount == 0 {
		rows = append(rows, style.Dim.Render("(Cloud Monitoring updates daily; buckets <~24h old show 0)"))
	}
	return rows
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

func bigQueryDetailRows(res inventory.Resource, detail any) []string {
	bd, _ := detail.(*inventory.BigQueryDetail)
	if bd == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	out := []string{
		kvLine("Subtype", bd.Subtype),
		kvLine("Region", bd.Region),
		kvLine("Loc type", bd.LocationType),
		kvLine("Storage", formatBytes(bd.StorageBytes)),
		kvLine("Tables", fmt.Sprintf("%d", bd.TableCount)),
		kvLine("Status", style.Status(res.Status).Render(res.Status)),
	}
	edition := bd.Edition
	if edition == "" {
		edition = "on-demand"
	}
	out = append(out, kvLine("Edition", edition))
	if bd.Slots > 0 {
		out = append(out, kvLine("Slots", fmt.Sprintf("%d", bd.Slots)))
	}
	return out
}

func pubSubDetailRows(res inventory.Resource, detail any) []string {
	pd, _ := detail.(*inventory.PubSubDetail)
	if pd == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	out := []string{
		kvLine("Subtype", pd.Subtype),
		kvLine("Region", pd.Region),
		kvLine("Status", style.Status(res.Status).Render(res.Status)),
	}
	retention := pd.MessageRetention
	if retention == "" {
		retention = "—"
	}
	out = append(out, kvLine("Retention", retention))
	switch pd.Subtype {
	case "Subscription":
		out = append(out, kvLine("Delivery", pd.DeliveryType))
	case "Topic":
		out = append(out,
			kvLine("Subs", fmt.Sprintf("%d", pd.SubscriptionCount)),
			kvLine("Published", formatBytes(pd.PublishedBytes)),
		)
	}
	return out
}

func memorystoreDetailRows(res inventory.Resource, detail any) []string {
	md, _ := detail.(*inventory.MemorystoreDetail)
	if md == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	out := []string{
		kvLine("Subtype", md.Subtype),
		kvLine("Service", md.ServiceType),
		kvLine("Region", md.Region),
		kvLine("Status", style.Status(res.Status).Render(res.Status)),
	}
	if md.MemorySizeGB > 0 {
		out = append(out, kvLine("Memory", fmt.Sprintf("%d GB", md.MemorySizeGB)))
	}
	if md.Version != "" {
		out = append(out, kvLine("Version", md.Version))
	}
	if md.Tier != "" {
		out = append(out, kvLine("Tier", md.Tier))
	}
	if md.NodeType != "" {
		out = append(out, kvLine("Node type", md.NodeType))
	}
	if md.ShardCount > 0 {
		out = append(out, kvLine("Shards", formatCount(int64(md.ShardCount))))
	}
	if md.ReplicaCount > 0 {
		out = append(out, kvLine("Replicas", formatCount(int64(md.ReplicaCount))))
	}
	return out
}

func artifactRegistryDetailRows(res inventory.Resource, detail any) []string {
	ad, _ := detail.(*inventory.ArtifactRegistryDetail)
	if ad == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	format := ad.Format
	if format == "" {
		format = "—"
	}
	mode := ad.Mode
	if mode == "" {
		mode = "—"
	}
	return []string{
		kvLine("Subtype", ad.Subtype),
		kvLine("Region", ad.Region),
		kvLine("Format", format),
		kvLine("Mode", mode),
		kvLine("Size", formatBytes(ad.SizeBytes)),
		kvLine("Status", style.Status(res.Status).Render(res.Status)),
	}
}

func appEngineDetailRows(res inventory.Resource, detail any) []string {
	ae, _ := detail.(*inventory.AppEngineDetail)
	if ae == nil {
		// Stub detail (Service or Version grain) — fall back to generic stub view.
		return stubDetailRows(res, detail)
	}
	hostname := ae.DefaultHostname
	if hostname == "" {
		hostname = "—"
	}
	authDomain := ae.AuthDomain
	if authDomain == "" {
		authDomain = "—"
	}
	dbType := ae.DatabaseType
	if dbType == "" {
		dbType = "—"
	}
	svcCount := "—"
	if ae.ServiceCount > 0 {
		svcCount = fmt.Sprintf("%d", ae.ServiceCount)
	}
	return []string{
		kvLine("Location", ae.LocationID),
		kvLine("Hostname", hostname),
		kvLine("Auth domain", authDomain),
		kvLine("DB type", dbType),
		kvLine("Services", svcCount),
		kvLine("Status", style.Status(res.Status).Render(res.Status)),
	}
}

func secretManagerDetailRows(res inventory.Resource, detail any) []string {
	sd, _ := detail.(*inventory.SecretManagerDetail)
	if sd == nil {
		return []string{style.Dim.Render("(no enriched detail — re-run --scan)")}
	}
	region := sd.Region
	if region == "" {
		region = "global"
	}
	out := []string{
		kvLine("Subtype", sd.Subtype),
		kvLine("Region", region),
		kvLine("Replication", sd.Replication),
		kvLine("Versions", fmt.Sprintf("%d", sd.ActiveVersions)),
		kvLine("Status", style.Status(res.Status).Render(res.Status)),
	}
	rotation := sd.RotationPeriod
	if rotation == "" {
		rotation = "—"
	}
	out = append(out, kvLine("Rotation", rotation))
	if sd.RotationTopic != "" {
		out = append(out, kvLine("Topic", sd.RotationTopic))
	}
	out = append(out, kvLine("Access ops", formatCount(sd.AccessOperations)))
	return out
}
func stubDetailRows(res inventory.Resource, detail any) []string {
	sd, _ := detail.(*inventory.StubDetail)
	if sd == nil {
		return []string{
			kvLine("Name", res.Name),
			kvLine("Region", res.Region),
			kvLine("Status", style.Status(res.Status).Render(res.Status)),
		}
	}
	return []string{
		kvLine("Subtype", sd.Subtype),
		kvLine("Region", sd.Region),
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
	if s := inventory.AcceleratorSummary(vm.Accelerators); s != "" {
		out = append(out, kvLine("GPUs", s))
	}
	if vm.MarketplaceClass != "" {
		out = append(out, kvLine("Marketplace", vm.MarketplaceClass))
		out = append(out, kvLine("Mkt project", vm.MarketplaceProject))
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
	rows := []string{style.Separator(40)}

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
