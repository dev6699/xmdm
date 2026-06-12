package adminhttp

import (
	"fmt"
	"html/template"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/device"
	"xmdm/server/internal/deviceinfo"
	"xmdm/server/internal/pagination"
)

const overviewRecentActivityLimit = 5

type overviewSignal struct {
	Label  string
	Value  string
	Detail string
	Tone   string
	Href   string // if set, the card navigates on click
}

type overviewMetric struct {
	Label  string
	Value  string
	Detail string
}

type overviewAttention struct {
	Title  string
	Detail string
	Tone   string
	Href   string
}

type overviewChart struct {
	Title     string
	Subtitle  string
	Labels    []string
	Values    []int
	Total     int
	EmptyNote string
}

type overviewCommandStats struct {
	Sent   int
	Acked  int
	Failed int
	Total  int
}

type overviewContentStats struct {
	ActiveApps  int
	ActiveFiles int
	ActiveCerts int
}

type overviewDashboardData struct {
	Freshness            string
	SummaryTitle         string
	SummaryDetail        string
	SummaryTone          string
	CanWrite             bool
	Signals              []overviewSignal
	Metrics              []overviewMetric
	Attention            []overviewAttention
	Chart                overviewChart
	DeviceStatusChart    overviewChart
	DeviceActivityChart  overviewChart
	DeviceTelemetryChart overviewChart
	CommandTrendChart    overviewChart
	DeviceModelChart     overviewChart
	CommandStats         overviewCommandStats
	ContentStats         overviewContentStats
	RecentActivity       []audit.Event
}

func (d *dashboard) overview(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Overview")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	now := time.Now().Local()
	ov := overviewDashboardData{
		Freshness: "Last updated " + formatDashboardTime(now),
		CanWrite:  d.canWrite(session),
	}
	appendSignal := func(label, value, detail, tone, href string) {
		ov.Signals = append(ov.Signals, overviewSignal{Label: label, Value: value, Detail: detail, Tone: tone, Href: href})
	}
	appendAttention := func(title, detail, tone, href string) {
		ov.Attention = append(ov.Attention, overviewAttention{Title: title, Detail: detail, Tone: tone, Href: href})
	}

	activeDeviceRecords := []device.Device{}
	totalDevices := 0
	activeDevices := 0
	pendingDevices := 0
	inactiveDevices := 0
	retiredOrWipedDevices := 0
	staleActiveDevices := 0

	if d.deps.Devices != nil {
		stats, err := d.deps.Devices.GetOverviewStats(ctx, d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		items, err := d.deps.Devices.ListActiveDevices(ctx, d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		activeDeviceRecords = items
		totalDevices = stats.Total
		activeDevices = stats.Active
		pendingDevices = stats.Pending
		retiredOrWipedDevices = stats.RetiredOrWiped
		inactiveDevices = totalDevices - activeDevices
		tone := "neutral"
		if totalDevices == 0 {
			tone = "warn"
		} else if inactiveDevices > 0 {
			tone = "warn"
		} else {
			tone = "good"
		}
		detail := fmt.Sprintf("%d of %d devices active", activeDevices, totalDevices)
		if inactiveDevices > 0 {
			detail = fmt.Sprintf("%d active, %d require review", activeDevices, inactiveDevices)
		}
		appendSignal("Device readiness", strconv.Itoa(activeDevices), detail, tone, "/admin/devices")
	}

	totalPolicies := 0
	activePolicies := 0
	retiredPolicies := 0
	if d.deps.Policies != nil {
		stats, err := d.deps.Policies.GetOverviewStats(ctx, d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		totalPolicies = stats.Total
		activePolicies = stats.Active
		retiredPolicies = stats.Retired
		tone := "neutral"
		if activePolicies > 0 {
			tone = "good"
		} else if totalPolicies > 0 {
			tone = "warn"
		}
		appendSignal("Policy library", strconv.Itoa(activePolicies), fmt.Sprintf("%d active, %d retired", activePolicies, retiredPolicies), tone, "/admin/policies")
	}

	if d.deps.Apps != nil {
		stats, err := d.deps.Apps.GetOverviewStats(ctx, d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		ov.ContentStats.ActiveApps = stats.Active
	}

	if d.deps.ManagedFiles != nil {
		stats, err := d.deps.ManagedFiles.GetOverviewStats(ctx, d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		ov.ContentStats.ActiveFiles = stats.Active
	}

	if d.deps.Certificates != nil {
		stats, err := d.deps.Certificates.GetOverviewStats(ctx, d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		ov.ContentStats.ActiveCerts = stats.Active
	}

	totalContent := ov.ContentStats.ActiveApps + ov.ContentStats.ActiveFiles + ov.ContentStats.ActiveCerts
	contentTone := "neutral"
	if totalContent > 0 {
		contentTone = "good"
	}
	appendSignal("Content readiness", strconv.Itoa(totalContent), fmt.Sprintf("%d apps, %d files, %d certs", ov.ContentStats.ActiveApps, ov.ContentStats.ActiveFiles, ov.ContentStats.ActiveCerts), contentTone, "/admin/apps")

	if d.deps.Commands != nil {
		stats, err := d.deps.Commands.GetOverviewStats(ctx, d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		items, err := d.deps.Commands.ListRecent(ctx, d.deps.TenantID, pagination.Params{Limit: 50})
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		ov.CommandStats = overviewCommandStats{Total: stats.Total, Sent: stats.Sent, Acked: stats.Acked, Failed: stats.Failed}
		ov.CommandTrendChart = buildCommandDeliveryTrendChart(items, now, 7)
		commandTone := "good"
		if ov.CommandStats.Failed > 0 {
			commandTone = "danger"
		} else if ov.CommandStats.Sent > 0 {
			commandTone = "warn"
		}
		detail := fmt.Sprintf("%d acknowledged, %d pending, %d failed", ov.CommandStats.Acked, ov.CommandStats.Sent, ov.CommandStats.Failed)
		appendSignal("Command health", strconv.Itoa(ov.CommandStats.Total), detail, commandTone, "/admin/commands")
	}

	if d.deps.DeviceInfo != nil {
		items, err := d.deps.DeviceInfo.Search(ctx, d.deps.TenantID, deviceinfo.SearchFilter{Pagination: pagination.Params{Limit: 200}})
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		ov.DeviceActivityChart = buildUniqueDeviceActivityChart(items, now, 7)
		ov.DeviceTelemetryChart = buildDeviceTelemetryFreshnessChart(activeDeviceRecords, items, now)
		staleActiveDevices = countStaleActiveDevices(activeDeviceRecords, items, now, 72*time.Hour)
		ov.DeviceModelChart = buildDeviceModelChart(activeDeviceRecords, items, 6)
	}

	var auditEvents []audit.Event
	auditLast24h := 0
	if d.deps.Audit != nil {
		count, err := d.deps.Audit.CountSince(ctx, d.deps.TenantID, now.Add(-24*time.Hour))
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		auditLast24h = count
		items, err := d.deps.Audit.List(ctx, d.deps.TenantID, pagination.Params{Limit: overviewRecentActivityLimit})
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		})
		auditEvents = items
		ov.RecentActivity = items
		auditTone := "neutral"
		if auditLast24h > 0 {
			auditTone = "good"
		}
		appendSignal("Audit activity", strconv.Itoa(auditLast24h), "events in the last 24 hours", auditTone, "/admin/audit")
	}

	reviewDevices := pendingDevices + staleActiveDevices
	if totalDevices > 0 && reviewDevices > 0 {
		detail := fmt.Sprintf("%d pending enrollment", pendingDevices)
		if staleActiveDevices > 0 {
			detail = fmt.Sprintf("%d pending enrollment, %d active device%s stale for over 72 hours", pendingDevices, staleActiveDevices, pluralSuffix(staleActiveDevices))
		}
		appendAttention("Devices require review", fmt.Sprintf("%d of %d devices need operator review: %s.", reviewDevices, totalDevices, detail), "warn", "/admin/devices")
	}
	if pendingDevices > 0 {
		appendAttention("Pending enrollment", fmt.Sprintf("%d device%s awaiting enrollment or activation.", pendingDevices, pluralSuffix(pendingDevices)), "warn", "/admin/devices")
	}
	if totalDevices == 0 && d.deps.Devices != nil {
		appendAttention("No devices enrolled", "Create or enroll the first device to activate fleet monitoring.", "warn", "/admin/devices")
	}
	if ov.CommandStats.Failed > 0 {
		appendAttention("Failed commands", fmt.Sprintf("%d recent command%s failed and should be investigated.", ov.CommandStats.Failed, pluralSuffix(ov.CommandStats.Failed)), "danger", "/admin/commands")
	}
	if ov.CommandStats.Sent > 0 {
		appendAttention("Commands pending acknowledgement", fmt.Sprintf("%d command%s still waiting for device acknowledgement.", ov.CommandStats.Sent, pluralSuffix(ov.CommandStats.Sent)), "warn", "/admin/commands")
	}
	if d.deps.Policies != nil && totalPolicies == 0 {
		appendAttention("No policies configured", "Create a policy before onboarding production devices.", "warn", "/admin/policies")
	} else if d.deps.Policies != nil && totalPolicies > 0 && activePolicies == 0 {
		appendAttention("No active policies", "All policies are retired or inactive; devices may not receive the expected configuration.", "warn", "/admin/policies")
	}
	if d.deps.Audit != nil && auditLast24h == 0 {
		appendAttention("No recent audit activity", "No operator or system events were recorded in the last 24 hours.", "neutral", "/admin/audit")
	}

	ackRate := 0
	if ov.CommandStats.Total > 0 {
		ackRate = ov.CommandStats.Acked * 100 / ov.CommandStats.Total
	}
	ov.Metrics = append(ov.Metrics,
		telemetryFreshnessMetric(ov.DeviceTelemetryChart, totalDevices),
		overviewMetric{Label: "Pending enrollment", Value: strconv.Itoa(pendingDevices), Detail: "Devices waiting to become active"},
		overviewMetric{Label: "Command ack rate", Value: strconv.Itoa(ackRate) + "%", Detail: fmt.Sprintf("%d of %d recent commands acknowledged", ov.CommandStats.Acked, ov.CommandStats.Total)},
		overviewMetric{Label: "Content items", Value: strconv.Itoa(totalContent), Detail: "Active apps, managed files, and certificates"},
	)

	ov.Chart = buildOverviewChart(auditEvents, now, 7)
	if d.deps.Devices != nil {
		counts, err := d.deps.Devices.GetStatusCounts(ctx, d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		ov.DeviceStatusChart = buildDeviceStatusChart(counts)
	}
	if len(ov.DeviceActivityChart.Values) == 0 {
		ov.DeviceActivityChart = buildOverviewChart(nil, now, 7)
		ov.DeviceActivityChart.Title = "Device activity timeline"
		ov.DeviceActivityChart.EmptyNote = "No device telemetry in the last 7 days."
	}
	if len(ov.DeviceTelemetryChart.Values) == 0 {
		ov.DeviceTelemetryChart = overviewChart{Title: "Device telemetry freshness", EmptyNote: "No device telemetry freshness data available."}
	}
	if len(ov.DeviceModelChart.Values) == 0 {
		ov.DeviceModelChart = overviewChart{Title: "Device model breakdown", EmptyNote: "No device model telemetry available."}
	}
	if len(ov.CommandTrendChart.Values) == 0 {
		ov.CommandTrendChart = buildCommandDeliveryTrendChart(nil, now, 7)
	}
	ov.SummaryTitle, ov.SummaryDetail, ov.SummaryTone = overviewStatusSummary(ov, activeDevices, pendingDevices, retiredOrWipedDevices, totalDevices, activePolicies, auditLast24h)

	if len(ov.Signals) == 0 {
		appendSignal("Snapshot", "ready", "No live repositories were attached", "neutral", "")
	}

	overviewHTML := renderOverviewDashboard(ov)
	d.renderForSession(w, r, session, pageData{Title: "Fleet Overview", Subtitle: "Monitor device readiness, policy coverage, content distribution, and recent activity.", Overview: overviewHTML})
}

func renderOverviewDashboard(data overviewDashboardData) template.HTML {
	var b strings.Builder

	b.WriteString(`<div class="overview-hero">`)
	b.WriteString(`<div class="overview-top">`)
	b.WriteString(`<div class="overview-title-block">`)
	b.WriteString(`<div class="overview-kicker">Control plane</div>`)
	b.WriteString(`<h1>Fleet Overview</h1>`)
	b.WriteString(`<div class="overview-subtitle">Monitor device readiness, policy coverage, content distribution, command delivery, and recent administrative activity.</div>`)
	fmt.Fprintf(&b, `<div class="overview-freshness">%s</div>`, esc(data.Freshness))
	b.WriteString(`</div>`)
	b.WriteString(`<div class="overview-actions">`)
	b.WriteString(`<a class="button btn-primary" href="/admin/devices">Manage devices</a>`)
	b.WriteString(`<a class="button" href="/admin/policies">Review policies</a>`)
	b.WriteString(`<a class="button" href="/admin/audit">View audit log</a>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	statusTone := signalToneClass(data.SummaryTone)
	fmt.Fprintf(&b, `<div class="overview-status %s">`, statusTone)
	b.WriteString(`<span class="overview-status-dot"></span>`)
	fmt.Fprintf(&b, `<div><div class="overview-status-title">%s</div>`, esc(data.SummaryTitle))
	fmt.Fprintf(&b, `<div class="overview-status-detail">%s</div></div>`, esc(data.SummaryDetail))
	b.WriteString(`</div>`)

	if len(data.Signals) > 0 {
		b.WriteString(`<div class="health-strip">`)
		for _, signal := range data.Signals {
			tone := signalToneClass(signal.Tone)
			if signal.Href != "" {
				fmt.Fprintf(&b, `<a class="health-item-wrap %s" href="%s">`, tone, escAttr(signal.Href))
			} else {
				fmt.Fprintf(&b, `<div class="health-item-wrap %s">`, tone)
			}
			b.WriteString(`<div class="health-item">`)
			fmt.Fprintf(&b, `<div class="health-row"><span class="health-dot"></span><div class="health-label">%s</div>`, esc(signal.Label))
			if signal.Href != "" {
				b.WriteString(`<span class="health-nav-arrow">&rarr;</span>`)
			}
			b.WriteString(`</div>`)
			fmt.Fprintf(&b, `<div class="health-value">%s</div>`, esc(signal.Value))
			fmt.Fprintf(&b, `<div class="health-detail">%s</div>`, esc(signal.Detail))
			b.WriteString(`</div>`)
			if signal.Href != "" {
				b.WriteString(`</a>`)
			} else {
				b.WriteString(`</div>`)
			}
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)

	if len(data.Metrics) > 0 {
		b.WriteString(`<div class="overview-metrics-grid">`)
		for _, metric := range data.Metrics {
			b.WriteString(`<div class="overview-metric-card">`)
			fmt.Fprintf(&b, `<div class="overview-metric-label">%s</div>`, esc(metric.Label))
			fmt.Fprintf(&b, `<div class="overview-metric-value">%s</div>`, esc(metric.Value))
			fmt.Fprintf(&b, `<div class="overview-metric-detail">%s</div>`, esc(metric.Detail))
			b.WriteString(renderMetricSparkline(metric.Value))
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`<div class="overview-panel">`)
	b.WriteString(`<div class="overview-panel-header"><span class="overview-panel-title">Needs attention</span><span class="overview-panel-meta">Operational priorities</span></div>`)
	b.WriteString(string(renderAttentionList(data.Attention)))
	b.WriteString(`</div>`)

	b.WriteString(`<div class="overview-device-grid">`)
	b.WriteString(string(renderOverviewChartPanel(data.DeviceStatusChart, "devices", "/admin/devices", "View devices")))
	b.WriteString(string(renderOverviewChartPanel(data.DeviceActivityChart, "Last 7 days", "/admin/devices", "View devices")))
	b.WriteString(string(renderOverviewChartPanel(data.DeviceTelemetryChart, "freshness", "/admin/devices", "View devices")))
	b.WriteString(string(renderOverviewChartPanel(data.CommandTrendChart, "Last 7 days", "/admin/commands", "View commands")))
	b.WriteString(string(renderOverviewChartPanel(data.DeviceModelChart, "top models", "/admin/devices", "View devices")))
	b.WriteString(string(renderOverviewCommandPanel(data.CommandStats)))
	b.WriteString(`</div>`)

	b.WriteString(`<div class="overview-bottom-row">`)
	b.WriteString(`<div class="overview-panel">`)
	b.WriteString(`<div class="overview-panel-header"><span class="overview-panel-title">Content library</span>`)
	totalContent := data.ContentStats.ActiveApps + data.ContentStats.ActiveFiles + data.ContentStats.ActiveCerts
	fmt.Fprintf(&b, `<span class="overview-panel-meta">%d active items</span>`, totalContent)
	b.WriteString(`</div>`)
	b.WriteString(string(renderContentComposition(data.ContentStats)))
	b.WriteString(`</div>`)

	b.WriteString(`<div class="overview-panel">`)
	b.WriteString(`<div class="overview-panel-header"><span class="overview-panel-title">Recent activity</span><a class="overview-panel-link" href="/admin/audit">View audit log</a></div>`)
	b.WriteString(string(renderRecentActivity(data.RecentActivity)))
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	return template.HTML(b.String())
}

func renderOverviewChartPanel(chart overviewChart, meta, href, linkLabel string) template.HTML {
	var b strings.Builder
	b.WriteString(`<div class="overview-panel">`)
	fmt.Fprintf(&b, `<div class="overview-panel-header"><span class="overview-panel-title">%s</span>`, esc(chart.Title))
	if href != "" {
		fmt.Fprintf(&b, `<a class="overview-panel-link" href="%s">%s</a>`, escAttr(href), esc(linkLabel))
	} else if chart.Total > 0 {
		fmt.Fprintf(&b, `<span class="chart-total-badge">%d total</span>`, chart.Total)
	} else {
		fmt.Fprintf(&b, `<span class="overview-panel-meta">%s</span>`, esc(meta))
	}
	b.WriteString(`</div><div class="overview-chart">`)
	b.WriteString(string(renderOverviewChart(chart)))
	b.WriteString(`</div></div>`)
	return template.HTML(b.String())
}

func renderOverviewCommandPanel(stats overviewCommandStats) template.HTML {
	var b strings.Builder
	b.WriteString(`<div class="overview-panel">`)
	b.WriteString(`<div class="overview-panel-header"><span class="overview-panel-title">Command delivery</span><a class="overview-panel-link" href="/admin/commands">View all</a></div>`)
	b.WriteString(string(renderCommandBreakdown(stats)))
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

func renderMetricSparkline(value string) string {
	cleaned := strings.TrimSpace(strings.TrimSuffix(value, "%"))
	n, err := strconv.Atoi(cleaned)
	if err != nil {
		return ""
	}

	width := n
	if strings.Contains(value, "%") {
		if width < 0 {
			width = 0
		}
		if width > 100 {
			width = 100
		}
	} else {
		switch {
		case n <= 0:
			width = 0
		case n <= 2:
			width = 18
		case n <= 5:
			width = 32
		case n <= 12:
			width = 58
		case n <= 25:
			width = 78
		default:
			width = 100
		}
	}
	return `<div class="overview-metric-spark" aria-hidden="true"><span style="width:` + strconv.Itoa(width) + `%"></span></div>`
}

func renderAttentionList(items []overviewAttention) template.HTML {
	if len(items) == 0 {
		return template.HTML(`<div class="attention-list"><div class="attention-item tone-good"><span class="attention-dot"></span><div><div class="attention-title">No immediate action required</div><div class="attention-detail">Fleet health, command delivery, and policy coverage are within the expected range.</div></div></div></div>`)
	}
	var b strings.Builder
	b.WriteString(`<div class="attention-list">`)
	for _, item := range items {
		tone := signalToneClass(item.Tone)
		if item.Href != "" {
			fmt.Fprintf(&b, `<a class="attention-item %s" href="%s">`, tone, escAttr(item.Href))
		} else {
			fmt.Fprintf(&b, `<div class="attention-item %s">`, tone)
		}
		b.WriteString(`<span class="attention-dot"></span>`)
		fmt.Fprintf(&b, `<div><div class="attention-title">%s</div><div class="attention-detail">%s</div></div>`, esc(item.Title), esc(item.Detail))
		if item.Href != "" {
			b.WriteString(`<span class="attention-arrow">&rarr;</span></a>`)
		} else {
			b.WriteString(`</div>`)
		}
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

func renderCommandBreakdown(stats overviewCommandStats) template.HTML {
	if stats.Total == 0 {
		return template.HTML(`<div class="chart-empty"><strong>No recent commands</strong><small>Commands sent to devices will appear here.</small></div>`)
	}
	var b strings.Builder
	b.WriteString(`<div class="cmd-breakdown">`)
	type bar struct {
		label string
		count int
		cls   string
	}
	bars := []bar{
		{"Sent", stats.Sent, "cmd-bar-sent"},
		{"Acked", stats.Acked, "cmd-bar-acked"},
		{"Failed", stats.Failed, "cmd-bar-failed"},
	}
	for _, entry := range bars {
		pct := 0
		if stats.Total > 0 {
			pct = entry.count * 100 / stats.Total
		}
		b.WriteString(`<div class="cmd-row">`)
		fmt.Fprintf(&b, `<div class="cmd-row-label">%s</div>`, esc(entry.label))
		fmt.Fprintf(&b, `<div class="cmd-track"><div class="cmd-fill %s" style="width:%d%%"></div></div>`, entry.cls, pct)
		fmt.Fprintf(&b, `<div class="cmd-row-count">%d</div>`, entry.count)
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)

	b.WriteString(`<div class="cmd-summary">`)
	successRate := 0
	if stats.Total > 0 {
		successRate = stats.Acked * 100 / stats.Total
	}
	rateCls := "cmd-rate-good"
	if successRate < 80 {
		rateCls = "cmd-rate-warn"
	}
	if successRate < 50 {
		rateCls = "cmd-rate-danger"
	}
	fmt.Fprintf(&b, `<div class="cmd-stat"><div class="cmd-stat-value %s">%d%%</div><div class="cmd-stat-label">ack rate</div></div>`, rateCls, successRate)
	fmt.Fprintf(&b, `<div class="cmd-stat"><div class="cmd-stat-value">%d</div><div class="cmd-stat-label">total</div></div>`, stats.Total)
	fmt.Fprintf(&b, `<div class="cmd-stat"><div class="cmd-stat-value cmd-rate-danger">%d</div><div class="cmd-stat-label">failed</div></div>`, stats.Failed)
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

func renderContentComposition(stats overviewContentStats) template.HTML {
	total := stats.ActiveApps + stats.ActiveFiles + stats.ActiveCerts
	var b strings.Builder
	b.WriteString(`<div class="content-comp">`)
	type segment struct {
		label string
		count int
		cls   string
		href  string
	}
	segs := []segment{
		{"Apps", stats.ActiveApps, "comp-apps", "/admin/apps"},
		{"Managed files", stats.ActiveFiles, "comp-files", "/admin/managed-files"},
		{"Certificates", stats.ActiveCerts, "comp-certs", "/admin/certificates"},
	}
	if total == 0 {
		b.WriteString(`<div class="chart-empty"><strong>No content configured</strong><small>Active apps, managed files, and certificates will appear here once added.</small></div>`)
		b.WriteString(`</div>`)
		return template.HTML(b.String())
	}
	b.WriteString(`<div class="comp-bar">`)
	for _, seg := range segs {
		if seg.count == 0 {
			continue
		}
		fmt.Fprintf(&b, `<div class="comp-segment %s" style="width:%d%%" title="%s: %d"></div>`, seg.cls, segmentWidthPercent(seg.count, total), esc(seg.label), seg.count)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<div class="comp-legend">`)
	for _, seg := range segs {
		fmt.Fprintf(&b, `<a class="comp-legend-item" href="%s">`, escAttr(seg.href))
		fmt.Fprintf(&b, `<span class="comp-dot %s"></span>`, seg.cls)
		fmt.Fprintf(&b, `<span class="comp-name">%s</span>`, esc(seg.label))
		fmt.Fprintf(&b, `<span class="comp-count">%d</span>`, seg.count)
		detail := ""
		switch seg.label {
		case "Apps":
			detail = strconv.Itoa(seg.count) + " app" + pluralSuffix(seg.count)
		case "Managed files":
			detail = strconv.Itoa(seg.count) + " file" + pluralSuffix(seg.count)
		case "Certificates":
			detail = strconv.Itoa(seg.count) + " cert" + pluralSuffix(seg.count)
		default:
			detail = strconv.Itoa(seg.count) + " item" + pluralSuffix(seg.count)
		}
		fmt.Fprintf(&b, `<span class="comp-pct">%s</span>`, esc(detail))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

func renderRecentActivity(items []audit.Event) template.HTML {
	if len(items) == 0 {
		return template.HTML(`<div class="chart-empty"><strong>No recent activity</strong><small>Operator actions and system events will appear here once recorded.</small></div>`)
	}
	var b strings.Builder
	b.WriteString(`<div class="activity-list">`)
	for _, item := range items {
		resource := item.ResourceType
		if strings.TrimSpace(item.ResourceID) != "" {
			resource += "/" + item.ResourceID
		}
		actor := firstNonEmpty(item.Actor, "system")
		b.WriteString(`<div class="activity-item">`)
		fmt.Fprintf(&b, `<div class="activity-time">%s</div>`, esc(formatDashboardTime(item.CreatedAt)))
		fmt.Fprintf(&b, `<div class="activity-main"><div class="activity-action">%s</div>`, esc(item.Action))
		fmt.Fprintf(&b, `<div class="activity-meta">%s on %s</div></div>`, esc(actor), esc(resource))
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

func renderOverviewChart(chart overviewChart) template.HTML {
	if len(chart.Values) == 0 || chart.Total == 0 {
		if chart.EmptyNote == "" {
			chart.EmptyNote = "No activity in the selected window."
		}
		return template.HTML(`<div class="chart-empty"><strong>` + esc(chart.EmptyNote) + `</strong><small>Audit events will appear here as operators and systems make changes.</small></div>`)
	}
	const width = 720
	const height = 220
	const left = 36
	const right = 14
	const top = 18
	const bottom = 42
	plotWidth := width - left - right
	plotHeight := height - top - bottom
	maxValue := 0
	for _, value := range chart.Values {
		if value > maxValue {
			maxValue = value
		}
	}
	if maxValue == 0 {
		maxValue = 1
	}
	cellWidth := float64(plotWidth) / float64(len(chart.Values))
	barWidth := cellWidth * 0.68
	barGap := (cellWidth - barWidth) / 2
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 720 220" role="img" aria-label="%s">`, esc(chart.Title))
	for i := 0; i < 4; i++ {
		y := float64(top+plotHeight) - (float64(plotHeight) * float64(i) / 3)
		b.WriteString(fmt.Sprintf(`<line class="chart-gridline" x1="%d" x2="%d" y1="%.1f" y2="%.1f"></line>`, left, width-right, y, y))
	}
	for i, value := range chart.Values {
		barHeight := float64(plotHeight) * float64(value) / float64(maxValue)
		x := float64(left) + float64(i)*cellWidth + barGap
		y := float64(top+plotHeight) - barHeight
		barLabel := chart.Labels[i]
		b.WriteString(fmt.Sprintf(`<rect class="chart-bar-bg" x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="6"></rect>`, x, float64(top), barWidth, float64(plotHeight)))
		b.WriteString(fmt.Sprintf(`<rect class="chart-bar" x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="6"><title>%s: %d</title></rect>`, x, y, barWidth, barHeight, esc(barLabel), value))
		if value > 0 {
			b.WriteString(fmt.Sprintf(`<text class="chart-axis" x="%.1f" y="%.1f" text-anchor="middle">%d</text>`, x+barWidth/2, y-4, value))
		}
		b.WriteString(fmt.Sprintf(`<text class="chart-axis" x="%.1f" y="%d" text-anchor="middle">%s</text>`, x+barWidth/2, height-18, esc(barLabel)))
	}
	b.WriteString(fmt.Sprintf(`<text class="chart-axis" x="%d" y="%d">%d</text>`, 4, top+11, maxValue))
	b.WriteString(fmt.Sprintf(`<text class="chart-axis" x="%d" y="%d">0</text>`, 10, top+plotHeight+4))
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

func buildDeviceStatusChart(counts device.StatusCounts) overviewChart {
	chart := overviewChart{Title: "Device status distribution", EmptyNote: "No devices enrolled yet."}
	total := counts.Pending + counts.Enrolled + counts.Active + counts.Locked + counts.Suspended + counts.Retired + counts.Wiped
	if total == 0 {
		return chart
	}
	values := map[string]int{
		"pending":   counts.Pending,
		"enrolled":  counts.Enrolled,
		"active":    counts.Active,
		"locked":    counts.Locked,
		"suspended": counts.Suspended,
		"retired":   counts.Retired,
		"wiped":     counts.Wiped,
	}
	return chartFromCounts(chart.Title, values, 8, chart.EmptyNote)
}

func buildDeviceTelemetryFreshnessChart(devices []device.Device, records any, now time.Time) overviewChart {
	chart := overviewChart{Title: "Device telemetry freshness", EmptyNote: "No device telemetry freshness data available."}
	if len(devices) == 0 {
		return chart
	}

	latestByDevice := latestTelemetryByDevice(records)
	counts := map[string]int{
		"Last 24h":     0,
		"2–7 days":     0,
		"Stale 8+d":    0,
		"No telemetry": 0,
	}

	for _, item := range devices {
		deviceKey := strings.TrimSpace(firstNonEmpty(item.RecordID(), item.ID, item.Name))
		if deviceKey == "" {
			counts["No telemetry"]++
			continue
		}

		latest, ok := latestByDevice[deviceKey]
		if !ok {
			counts["No telemetry"]++
			continue
		}

		age := now.Sub(latest)
		switch {
		case age <= 24*time.Hour:
			counts["Last 24h"]++
		case age <= 7*24*time.Hour:
			counts["2–7 days"]++
		default:
			counts["Stale 8+d"]++
		}
	}

	for _, label := range []string{"Last 24h", "2–7 days", "Stale 8+d", "No telemetry"} {
		value := counts[label]
		if value <= 0 {
			continue
		}
		chart.Labels = append(chart.Labels, label)
		chart.Values = append(chart.Values, value)
		chart.Total += value
	}
	return chart
}

func latestTelemetryByDevice(records any) map[string]time.Time {
	latest := map[string]time.Time{}
	values := reflect.ValueOf(records)
	if values.Kind() != reflect.Slice && values.Kind() != reflect.Array {
		return latest
	}
	for i := 0; i < values.Len(); i++ {
		item := values.Index(i)
		observed, ok := firstTimeField(item, "ObservedAt", "CreatedAt", "UpdatedAt")
		if !ok {
			continue
		}
		for _, key := range []string{
			firstStringField(item, "DeviceID", "DeviceId", "DeviceRecordID", "DeviceRecordId"),
			firstPayloadString(item, "deviceId", "deviceID", "deviceRecordId", "deviceRecordID", "serialNumber"),
		} {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if previous, exists := latest[key]; !exists || observed.After(previous) {
				latest[key] = observed
			}
		}
	}
	return latest
}

func buildStatusTimelineChart(items any, now time.Time, days int, title, emptyNote string) overviewChart {
	chart := overviewChart{Title: title, EmptyNote: emptyNote}
	if days <= 0 {
		days = 7
	}
	loc := now.Location()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(days - 1))
	chart.Labels = make([]string, days)
	chart.Values = make([]int, days)
	for i := 0; i < days; i++ {
		chart.Labels[i] = start.AddDate(0, 0, i).Format("Jan 2")
	}
	values := reflect.ValueOf(items)
	if values.Kind() != reflect.Slice && values.Kind() != reflect.Array {
		return chart
	}
	for i := 0; i < values.Len(); i++ {
		observed, ok := firstTimeField(values.Index(i), "ObservedAt", "CreatedAt", "UpdatedAt")
		if !ok {
			continue
		}
		observed = observed.In(loc)
		if observed.Before(start) || observed.After(now) {
			continue
		}
		idx := int(observed.Sub(start).Hours() / 24)
		if idx >= 0 && idx < len(chart.Values) {
			chart.Values[idx]++
			chart.Total++
		}
	}
	return chart
}

func countStaleActiveDevices(devices []device.Device, records any, now time.Time, threshold time.Duration) int {
	if len(devices) == 0 {
		return 0
	}
	latestByDevice := latestTelemetryByDevice(records)
	count := 0
	for _, item := range devices {
		if item.Status != device.StatusActive {
			continue
		}
		deviceKey := strings.TrimSpace(firstNonEmpty(item.RecordID(), item.ID, item.Name))
		latest, ok := latestByDevice[deviceKey]
		if !ok || now.Sub(latest) > threshold {
			count++
		}
	}
	return count
}

func buildUniqueDeviceActivityChart(records any, now time.Time, days int) overviewChart {
	chart := overviewChart{Title: "Device activity timeline", EmptyNote: "No device telemetry in the last 7 days."}
	if days <= 0 {
		days = 7
	}
	loc := now.Location()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(days - 1))
	chart.Labels = make([]string, days)
	chart.Values = make([]int, days)
	for i := 0; i < days; i++ {
		chart.Labels[i] = start.AddDate(0, 0, i).Format("Jan 2")
	}
	seen := make([]map[string]bool, days)
	for i := range seen {
		seen[i] = map[string]bool{}
	}
	values := reflect.ValueOf(records)
	if values.Kind() != reflect.Slice && values.Kind() != reflect.Array {
		return chart
	}
	for i := 0; i < values.Len(); i++ {
		item := values.Index(i)
		observed, ok := firstTimeField(item, "ObservedAt", "CreatedAt", "UpdatedAt")
		if !ok {
			continue
		}
		observed = observed.In(loc)
		if observed.Before(start) || observed.After(now) {
			continue
		}
		deviceKey := firstStringField(item, "DeviceID", "DeviceId", "DeviceRecordID", "DeviceRecordId")
		if deviceKey == "" {
			deviceKey = firstPayloadString(item, "deviceId", "deviceID", "deviceRecordId", "deviceRecordID")
		}
		deviceKey = strings.TrimSpace(deviceKey)
		if deviceKey == "" {
			continue
		}
		idx := int(observed.Sub(start).Hours() / 24)
		if idx >= 0 && idx < len(chart.Values) && !seen[idx][deviceKey] {
			seen[idx][deviceKey] = true
			chart.Values[idx]++
			chart.Total++
		}
	}
	return chart
}

func buildCommandDeliveryTrendChart(items any, now time.Time, days int) overviewChart {
	chart := overviewChart{Title: "Command delivery volume", EmptyNote: "No command activity in the last 7 days."}
	if days <= 0 {
		days = 7
	}
	loc := now.Location()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(days - 1))
	chart.Labels = make([]string, days)
	chart.Values = make([]int, days)
	for i := 0; i < days; i++ {
		chart.Labels[i] = start.AddDate(0, 0, i).Format("Jan 2")
	}
	values := reflect.ValueOf(items)
	if values.Kind() != reflect.Slice && values.Kind() != reflect.Array {
		return chart
	}
	for i := 0; i < values.Len(); i++ {
		item := values.Index(i)
		observed, ok := firstTimeField(item, "UpdatedAt", "CreatedAt")
		if !ok {
			continue
		}
		observed = observed.In(loc)
		if observed.Before(start) || observed.After(now) {
			continue
		}
		idx := int(observed.Sub(start).Hours() / 24)
		if idx >= 0 && idx < len(chart.Values) {
			chart.Values[idx]++
			chart.Total++
		}
	}
	return chart
}

func buildDeviceModelChart(devices []device.Device, records any, limit int) overviewChart {
	chart := overviewChart{Title: "Device model breakdown", EmptyNote: "No device model telemetry available."}
	if len(devices) == 0 {
		return chart
	}
	modelByDevice := latestTelemetryModelByDevice(records)
	counts := map[string]int{}
	for _, item := range devices {
		deviceKey := strings.TrimSpace(firstNonEmpty(item.RecordID(), item.ID, item.Name))
		if deviceKey == "" {
			continue
		}
		label := modelByDevice[deviceKey]
		if strings.TrimSpace(label) == "" {
			label = "Unknown"
		}
		counts[label]++
	}
	return chartFromCounts(chart.Title, counts, limit, chart.EmptyNote)
}

func latestTelemetryModelByDevice(records any) map[string]string {
	type observedModel struct {
		observed time.Time
		model    string
	}
	latest := map[string]observedModel{}
	values := reflect.ValueOf(records)
	if values.Kind() != reflect.Slice && values.Kind() != reflect.Array {
		return map[string]string{}
	}
	for i := 0; i < values.Len(); i++ {
		item := values.Index(i)
		observed, ok := firstTimeField(item, "ObservedAt", "CreatedAt", "UpdatedAt")
		if !ok {
			continue
		}
		model := firstPayloadString(item, "model", "Model", "deviceModel", "DeviceModel")
		deviceKey := firstStringField(item, "DeviceID", "DeviceId", "DeviceRecordID", "DeviceRecordId")
		if deviceKey == "" {
			deviceKey = firstPayloadString(item, "deviceId", "deviceID", "deviceRecordId", "deviceRecordID")
		}
		deviceKey = strings.TrimSpace(deviceKey)
		if deviceKey == "" {
			continue
		}
		if previous, exists := latest[deviceKey]; !exists || observed.After(previous.observed) {
			latest[deviceKey] = observedModel{observed: observed, model: model}
		}
	}
	result := map[string]string{}
	for deviceKey, item := range latest {
		result[deviceKey] = item.model
	}
	return result
}

func telemetryFreshnessMetric(chart overviewChart, totalDevices int) overviewMetric {
	seenRecent := 0
	for i, label := range chart.Labels {
		normalized := strings.ToLower(strings.TrimSpace(label))
		if (normalized == "last 24h" || normalized == "seen last 24h" || normalized == "seen today") && i < len(chart.Values) {
			seenRecent = chart.Values[i]
			break
		}
	}
	value := "0%"
	if totalDevices > 0 {
		value = strconv.Itoa(seenRecent*100/totalDevices) + "%"
	}
	detail := fmt.Sprintf("%d of %d devices seen in the last 24 hours", seenRecent, totalDevices)
	if seenRecent > 0 {
		detail += " · latest telemetry is current"
	}
	return overviewMetric{Label: "Telemetry freshness", Value: value, Detail: detail}
}

func chartFromCounts(title string, counts map[string]int, limit int, emptyNote string) overviewChart {
	chart := overviewChart{Title: title, EmptyNote: emptyNote}
	if len(counts) == 0 {
		return chart
	}
	type pair struct {
		label string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for label, count := range counts {
		if count <= 0 {
			continue
		}
		pairs = append(pairs, pair{label: titleCaseLabel(label), count: count})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].label < pairs[j].label
		}
		return pairs[i].count > pairs[j].count
	})
	if limit > 0 && len(pairs) > limit {
		other := 0
		for _, item := range pairs[limit-1:] {
			other += item.count
		}
		pairs = append(pairs[:limit-1], pair{label: "Other", count: other})
	}
	for _, item := range pairs {
		chart.Labels = append(chart.Labels, item.label)
		chart.Values = append(chart.Values, item.count)
		chart.Total += item.count
	}
	return chart
}

func segmentWidthPercent(count, total int) int {
	if total <= 0 {
		return 0
	}
	return count * 100 / total
}

func firstTimeField(value reflect.Value, names ...string) (time.Time, bool) {
	value = indirectValue(value)
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return time.Time{}, false
	}
	for _, name := range names {
		field := value.FieldByName(name)
		if field.IsValid() && field.CanInterface() {
			if t, ok := field.Interface().(time.Time); ok && !t.IsZero() {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

func firstStringField(value reflect.Value, names ...string) string {
	value = indirectValue(value)
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return ""
	}
	for _, name := range names {
		field := value.FieldByName(name)
		if field.IsValid() && field.CanInterface() {
			if s := stringFromReflectValue(field); s != "" {
				return s
			}
		}
	}
	return ""
}

func firstPayloadString(value reflect.Value, names ...string) string {
	value = indirectValue(value)
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return ""
	}
	for _, name := range names {
		field := value.FieldByName(name)
		if field.IsValid() && field.CanInterface() {
			if s := stringFromReflectValue(field); s != "" {
				return s
			}
		}
	}
	payload := value.FieldByName("Payload")
	if payload.IsValid() && payload.CanInterface() {
		payloadValue := payload.Interface()
		if m, ok := payloadValue.(map[string]any); ok {
			for _, name := range names {
				if s, ok := m[name].(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
		rv := indirectValue(payload)
		if rv.IsValid() && rv.Kind() == reflect.Map {
			for _, name := range names {
				key := reflect.ValueOf(name)
				if key.Type().AssignableTo(rv.Type().Key()) {
					v := rv.MapIndex(key)
					if v.IsValid() {
						if s := stringFromReflectValue(v); s != "" {
							return s
						}
					}
				}
			}
		}
	}
	return ""
}

func stringFromReflectValue(value reflect.Value) string {
	value = indirectValue(value)
	if !value.IsValid() || !value.CanInterface() {
		return ""
	}
	if value.Kind() == reflect.String {
		return strings.TrimSpace(value.String())
	}
	if s, ok := value.Interface().(fmt.Stringer); ok {
		return strings.TrimSpace(s.String())
	}
	return ""
}

func titleCaseLabel(label string) string {
	label = strings.TrimSpace(strings.ReplaceAll(label, "_", " "))
	if label == "" {
		return "Unknown"
	}
	parts := strings.Fields(label)
	for i, part := range parts {
		if len(part) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func buildOverviewChart(events []audit.Event, now time.Time, days int) overviewChart {
	chart := overviewChart{
		Title:     "Audit activity, last 7 days",
		EmptyNote: "No audit activity in the last 7 days.",
	}
	if days <= 0 {
		days = 7
	}
	loc := now.Location()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(days - 1))
	chart.Labels = make([]string, days)
	chart.Values = make([]int, days)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i)
		chart.Labels[i] = day.Format("Jan 2")
	}
	for _, event := range events {
		observed := event.CreatedAt.In(loc)
		if observed.Before(start) || observed.After(now) {
			continue
		}
		index := int(observed.Sub(start).Hours() / 24)
		if index >= 0 && index < len(chart.Values) {
			chart.Values[index]++
			chart.Total++
		}
	}
	return chart
}

func overviewStatusSummary(data overviewDashboardData, activeDevices, pendingDevices, retiredOrWipedDevices, totalDevices, activePolicies, auditLast24h int) (string, string, string) {
	tone := "good"
	for _, item := range data.Attention {
		switch strings.ToLower(strings.TrimSpace(item.Tone)) {
		case "danger":
			tone = "danger"
		case "warn":
			if tone != "danger" {
				tone = "warn"
			}
		}
	}

	title := "Fleet status: Healthy"
	switch tone {
	case "warn":
		title = "Fleet status: Review recommended"
	case "danger":
		title = "Fleet status: Attention required"
	}

	parts := make([]string, 0, 6)
	if totalDevices > 0 {
		parts = append(parts, fmt.Sprintf("%d active", activeDevices))
		parts = append(parts, fmt.Sprintf("%d pending", pendingDevices))
		parts = append(parts, fmt.Sprintf("%d retired/wiped", retiredOrWipedDevices))
	}
	if activePolicies > 0 {
		parts = append(parts, fmt.Sprintf("%d active policies", activePolicies))
	}
	if data.CommandStats.Total > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d commands acknowledged", data.CommandStats.Acked, data.CommandStats.Total))
	}
	parts = append(parts, fmt.Sprintf("%d audit events in the last 24 hours", auditLast24h))
	if len(parts) == 0 {
		parts = append(parts, "Repositories are ready, but no live fleet data is attached yet")
	}
	return title, strings.Join(parts, " - "), tone
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func signalToneClass(tone string) string {
	switch strings.ToLower(strings.TrimSpace(tone)) {
	case "good":
		return "tone-good"
	case "warn":
		return "tone-warn"
	case "danger":
		return "tone-danger"
	default:
		return "tone-neutral"
	}
}
