package adminhttp

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/pagination"
)

func (d *dashboard) audit(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Audit")
	if !ok {
		return
	}
	page, params := listPaginationParams(r, pagination.DefaultLimit)
	limit := params.Limit - 1
	items, err := d.deps.Audit.List(r.Context(), d.deps.TenantID, params)
	if err != nil {
		d.renderPageError(w, r, session, "Audit", err)
		return
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	d.renderForSession(w, r, session, pageData{Title: "Audit", Items: withPager(auditTable(items), pagerHTML(r, page, limit, hasNext))})
}

func auditTable(items []audit.Event) template.HTML {
	var b strings.Builder
	b.WriteString(`<table class="audit-table"><colgroup><col class="audit-created"><col class="audit-actor"><col class="audit-action"><col class="audit-resource"><col class="audit-details"></colgroup><thead><tr><th class="audit-created">Created</th><th class="audit-actor">Actor</th><th class="audit-action">Action</th><th class="audit-resource">Resource</th><th class="audit-details">Details</th></tr></thead><tbody>`)
	for _, item := range items {
		b.WriteString(`<tr>`)
		fmt.Fprintf(&b, `<td class="audit-created">%s</td>`, esc(formatDashboardTime(item.CreatedAt)))
		fmt.Fprintf(&b, `<td class="audit-actor">%s</td>`, esc(item.Actor))
		fmt.Fprintf(&b, `<td class="audit-action">%s</td>`, esc(item.Action))
		fmt.Fprintf(&b, `<td class="audit-resource">%s</td>`, esc(item.ResourceType+"/"+item.ResourceID))
		fmt.Fprintf(&b, `<td class="audit-details">%s</td>`, pre(item.Details))
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}
