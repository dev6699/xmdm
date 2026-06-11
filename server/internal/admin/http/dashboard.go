package adminhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"xmdm/server/internal/apps"
	"xmdm/server/internal/artifacts"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/certificates"
	"xmdm/server/internal/commands"
	"xmdm/server/internal/device"
	"xmdm/server/internal/deviceinfo"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/files"
	"xmdm/server/internal/group"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/identity"
	"xmdm/server/internal/logs"
	"xmdm/server/internal/managedfiles"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/plugins"
	"xmdm/server/internal/policy"
)

type DashboardDependencies struct {
	Identity        identity.Repository
	Apps            apps.Repository
	Files           files.Repository
	ManagedFiles    managedfiles.Repository
	Logs            logsRepository
	Commands        commands.Repository
	DeviceInfo      deviceInfoRepository
	Certificates    certificates.Repository
	Artifacts       artifacts.Store
	Groups          group.Repository
	Policies        policy.Repository
	Devices         device.Repository
	Enrollment      enrollment.Repository
	Runtime         enrollment.RuntimeSnapshot
	ServerPublicURL string
	AgentAppPackage string
	PluginManager   *plugins.Manager
	Audit           audit.Store
	TenantID        string
}

type logsRepository interface {
	Search(ctx context.Context, tenantID string, filter logs.SearchFilter) ([]logs.Record, error)
}

type deviceInfoRepository interface {
	Search(ctx context.Context, tenantID string, filter deviceinfo.SearchFilter) ([]deviceinfo.Record, error)
}

type dashboard struct {
	svc  *auth.Service
	deps DashboardDependencies
	tmpl *template.Template
}

type pageData struct {
	Title           string
	Subtitle        string
	User            string
	CSRFToken       string
	CanWrite        bool
	Flash           string
	Error           string
	Callout         template.HTML
	Overview        template.HTML
	Sections        []sectionData
	Forms           []formData
	Items           any
	SearchQuery     string
	FormsAfterItems bool
	ItemsRaw        bool
}

type sectionData struct {
	Title string
	Count int
	Body  template.HTML
}

type formData struct {
	Title   string
	Action  string
	EncType string
	Fields  []fieldData
	Help    template.HTML
	After   template.HTML
	Submit  string
	Danger  bool
}

type fieldData struct {
	Name        string
	Label       string
	Type        string
	Value       string
	Values      []string
	Placeholder string
	Required    bool
	Options     []optionData
}

type optionData struct {
	Value string
	Label string
}

const dashboardTemplate = `<!doctype html>
<html lang="en" data-theme="dark">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}} - XMDM</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link href="https://fonts.googleapis.com/css2?family=Space+Mono:wght@400;700&family=DM+Sans:ital,wght@0,300;0,400;0,500;0,600;1,400&display=swap" rel="stylesheet">
  <style>
    /* ═══════════════════════════════════════════
       TOKENS
    ═══════════════════════════════════════════ */
    :root, [data-theme="dark"] {
      --bg:            #0b0e11;
      --surface:       #111519;
      --surface2:      #161c23;
      --surface3:      #1c2430;
      --border:        rgba(255,255,255,.07);
      --border-hi:     rgba(74,222,158,.3);
      --ink:           #e2e8f0;
      --ink-2:         #94a3b8;
      --ink-3:         #5a6a7e;
      --accent:        #4ade9e;
      --accent-solid:  #2bb87a;
      --accent-dim:    rgba(74,222,158,.1);
      --accent-glow:   rgba(74,222,158,.2);
      --danger:        #f87171;
      --danger-solid:  #dc2626;
      --danger-dim:    rgba(248,113,113,.1);
      --warn:          #fbbf24;
      --flash-bg:      rgba(74,222,158,.07);
      --flash-border:  rgba(74,222,158,.3);
      --flash-ink:     #4ade9e;
      --pre-bg:        #0d1117;
      --pre-ink:       #7d9ab5;
      --header-bg:     rgba(11,14,17,.95);
      --nav-bg:        #0e1218;
      --nav-width:     15rem;
      --radius-sm:     .4rem;
      --radius:        .6rem;
      --radius-lg:     .9rem;
      --shadow:        0 4px 24px rgba(0,0,0,.4);
      --shadow-sm:     0 2px 8px rgba(0,0,0,.25);
      color-scheme: dark;
    }
    [data-theme="light"] {
      --bg:            #f1f5f9;
      --surface:       #ffffff;
      --surface2:      #f8fafc;
      --surface3:      #f1f5f9;
      --border:        rgba(0,0,0,.08);
      --border-hi:     rgba(15,120,95,.35);
      --ink:           #0f172a;
      --ink-2:         #475569;
      --ink-3:         #94a3b8;
      --accent:        #0a7c5c;
      --accent-solid:  #086648;
      --accent-dim:    rgba(10,124,92,.08);
      --accent-glow:   rgba(10,124,92,.15);
      --danger:        #dc2626;
      --danger-solid:  #b91c1c;
      --danger-dim:    rgba(220,38,38,.08);
      --warn:          #d97706;
      --flash-bg:      #ecfdf5;
      --flash-border:  #6ee7b7;
      --flash-ink:     #065f46;
      --pre-bg:        #f8fafc;
      --pre-ink:       #475569;
      --header-bg:     #0f2922;
      --nav-bg:        #ffffff;
      --shadow:        0 4px 20px rgba(0,0,0,.08);
      --shadow-sm:     0 1px 4px rgba(0,0,0,.06);
      color-scheme: light;
    }

    /* ═══════════════════════════════════════════
       RESET & BASE
    ═══════════════════════════════════════════ */
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

    body {
      font-family: "DM Sans", system-ui, sans-serif;
      font-size: .9375rem;
      line-height: 1.6;
      color: var(--ink);
      background: var(--bg);
      min-height: 100vh;
      transition: background .2s, color .2s;
    }
    [data-theme="dark"] body {
      background-image: radial-gradient(ellipse 70% 40% at 50% -5%, rgba(74,222,158,.055) 0%, transparent 65%);
    }

    /* ═══════════════════════════════════════════
       HEADER
    ═══════════════════════════════════════════ */
    header {
      position: sticky; top: 0; z-index: 200;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 1rem;
      height: 3.25rem;
      padding: 0 1.5rem;
      background: var(--header-bg);
      border-bottom: 1px solid rgba(255,255,255,.06);
      backdrop-filter: blur(16px);
    }
    .brand {
      display: flex; align-items: center; gap: .5rem;
      font-family: "Space Mono", monospace;
      font-size: .8rem; font-weight: 700;
      letter-spacing: .06em;
      color: #4ade9e;
      text-decoration: none;
    }
    .brand-dot {
      width: 7px; height: 7px; border-radius: 50%;
      background: #4ade9e;
      box-shadow: 0 0 8px #4ade9e88;
      animation: pulse-dot 2.8s ease-in-out infinite;
    }
    @keyframes pulse-dot {
      0%,100% { opacity:1; transform:scale(1); }
      50%      { opacity:.35; transform:scale(.65); }
    }
    .header-right { display: flex; align-items: center; gap: .6rem; }
    .header-user {
      font-family: "Space Mono", monospace;
      font-size: .72rem;
      color: rgba(255,255,255,.86);
      padding: .25rem .6rem;
      background: rgba(255,255,255,.08);
      border-radius: var(--radius-sm);
    }

    /* theme toggle */
    .btn-icon {
      display: flex; align-items: center; justify-content: center;
      width: 2rem; height: 2rem;
      border-radius: var(--radius-sm);
      border: 1px solid rgba(255,255,255,.12);
      background: transparent;
      color: rgba(255,255,255,.55);
      cursor: pointer; font-size: .95rem; padding: 0;
      transition: background .15s, border-color .15s, color .15s;
    }
    .btn-icon:hover { background: rgba(255,255,255,.1); color: #fff; border-color: rgba(255,255,255,.25); box-shadow: none; }
    .icon-sun  { display: none; }
    .icon-moon { display: block; }
    [data-theme="light"] .icon-sun  { display: block; }
    [data-theme="light"] .icon-moon { display: none; }

    /* logout in header */
    .btn-logout {
      font-family: "DM Sans", sans-serif;
      font-size: .8rem;
      padding: .3rem .75rem;
      height: 2rem;
      color: rgba(255,255,255,.92);
      border-color: rgba(255,255,255,.2);
      background: transparent;
    }
    .btn-logout:hover { background: rgba(255,255,255,.12); color: #fff; border-color: rgba(255,255,255,.32); box-shadow: none; }
    [data-theme="light"] .btn-logout {
      color: var(--ink);
      border-color: var(--border);
      background: rgba(255,255,255,.82);
    }
    [data-theme="light"] .btn-logout:hover {
      background: var(--surface2);
      color: var(--ink);
      border-color: var(--border);
      box-shadow: none;
    }
    [data-theme="light"] .header-user {
      color: var(--ink);
      background: rgba(255,255,255,.9);
    }

    /* ═══════════════════════════════════════════
       LAYOUT SHELL
    ═══════════════════════════════════════════ */
    .shell {
      display: grid;
      grid-template-columns: var(--nav-width) 1fr;
      min-height: calc(100vh - 3.25rem);
    }

    /* ═══════════════════════════════════════════
       SIDEBAR NAV
    ═══════════════════════════════════════════ */
    .sidebar {
      position: sticky;
      top: 3.25rem;
      height: calc(100vh - 3.25rem);
      overflow-y: auto;
      background: var(--nav-bg);
      border-right: 1px solid var(--border);
      padding: 1rem .75rem 2rem;
      display: flex;
      flex-direction: column;
      gap: 2px;
      scrollbar-width: thin;
      scrollbar-color: var(--border) transparent;
      transition: background .2s;
    }
    .sidebar::-webkit-scrollbar { width: 4px; }
    .sidebar::-webkit-scrollbar-thumb { background: var(--border); border-radius: 2px; }

    .nav-group-label {
      font-family: "Space Mono", monospace;
      font-size: .6rem;
      font-weight: 700;
      letter-spacing: .14em;
      text-transform: uppercase;
      color: var(--ink-3);
      padding: .9rem .6rem .3rem;
      user-select: none;
    }
    .nav-group-label:first-child { padding-top: .2rem; }

    .sidebar a {
      display: flex; align-items: center; gap: .6rem;
      padding: .5rem .7rem;
      border-radius: var(--radius-sm);
      font-size: .875rem;
      font-weight: 400;
      color: var(--ink-2);
      text-decoration: none;
      transition: background .12s, color .12s;
      position: relative;
    }
    .sidebar a .nav-icon {
      width: 1.05rem;
      height: 1.05rem;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      flex-shrink: 0;
      opacity: .72;
      color: currentColor;
    }
    .sidebar a .nav-icon svg {
      width: 1rem;
      height: 1rem;
      display: block;
      stroke: currentColor;
      fill: none;
      stroke-width: 1.6;
      stroke-linecap: round;
      stroke-linejoin: round;
    }
    .sidebar a:hover {
      background: var(--accent-dim);
      color: var(--ink);
    }
    .sidebar a:hover .nav-icon { opacity: 1; }
    .sidebar a.active {
      background: var(--accent-dim);
      color: var(--accent);
      font-weight: 600;
    }
    .sidebar a.active .nav-icon { opacity: 1; }
    .sidebar a.active::before {
      content: '';
      position: absolute; left: 0; top: 20%; bottom: 20%;
      width: 3px;
      background: var(--accent);
      border-radius: 0 2px 2px 0;
    }

    /* ═══════════════════════════════════════════
       MAIN CONTENT
    ═══════════════════════════════════════════ */
    .content {
      min-width: 0;
      padding: 1.75rem 2.25rem 3rem;
    }

    /* page header row */
    .page-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 1rem;
      margin-bottom: 1.75rem;
      flex-wrap: wrap;
    }
    h1 {
      font-size: 1.35rem;
      font-weight: 600;
      letter-spacing: -.02em;
      color: var(--ink);
      line-height: 1.2;
    }
    .page-subtitle {
      font-size: .825rem;
      color: var(--ink-2);
      margin-top: .2rem;
      font-weight: 400;
    }

    /* section headings inside panels */
    h2 {
      font-family: "Space Mono", monospace;
      font-size: .72rem;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: .1em;
      color: var(--ink-3);
      margin-bottom: 1.1rem;
      padding-bottom: .65rem;
      border-bottom: 1px solid var(--border);
    }

    /* ═══════════════════════════════════════════
       LOGIN PAGE — full-page centred layout
    ═══════════════════════════════════════════ */
    body.page-login {
      display: flex;
      flex-direction: column;
      min-height: 100vh;
    }
    .login-wrap {
      flex: 1;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      padding: 2rem 1rem;
    }
    .login-card {
      width: 100%;
      max-width: 28rem;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--radius-lg);
      padding: 2.5rem 2.75rem;
      box-shadow: var(--shadow);
    }
    .login-logo {
      display: flex; align-items: center; justify-content: center; gap: .5rem;
      margin-bottom: 2rem;
    }
    .login-logo-dot {
      width: 10px; height: 10px; border-radius: 50%;
      background: var(--accent);
      box-shadow: 0 0 12px var(--accent-glow);
      animation: pulse-dot 2.8s ease-in-out infinite;
    }
    .login-logo-text {
      font-family: "Space Mono", monospace;
      font-size: 1rem; font-weight: 700;
      letter-spacing: .06em;
      color: var(--accent);
    }
    .login-title {
      text-align: center;
      font-size: 1.25rem;
      font-weight: 600;
      color: var(--ink);
      margin-bottom: .4rem;
    }
    .login-sub {
      text-align: center;
      font-size: .85rem;
      color: var(--ink-2);
      margin-bottom: 2rem;
    }
    .login-card label { margin-top: 1.1rem; }
    .login-card label:first-of-type { margin-top: 0; }
    .login-card .btn-primary {
      width: 100%;
      justify-content: center;
      margin-top: 1.5rem;
      padding: .65rem 1rem;
      font-size: .9rem;
    }
    .login-footer {
      margin-top: 1.5rem;
      text-align: center;
      font-size: .75rem;
      color: var(--ink-3);
    }

    /* ═══════════════════════════════════════════
       PANELS & CARDS
    ═══════════════════════════════════════════ */
    .panel {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--radius-lg);
      padding: 1.5rem;
      margin-bottom: 1.5rem;
      box-shadow: var(--shadow-sm);
      transition: border-color .18s, background .2s;
    }
    .panel:hover { border-color: var(--border-hi); }
    details.panel {
      padding: 0;
    }
    details.panel > summary {
      list-style: none;
      cursor: pointer;
      padding: 1.5rem;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 1rem;
    }
    details.panel > summary::-webkit-details-marker {
      display: none;
    }
    details.panel > summary::after {
      content: '▾';
      color: var(--ink-3);
      font-size: .8rem;
      flex: 0 0 auto;
      transition: transform .15s ease;
    }
    details.panel:not([open]) > summary::after {
      transform: rotate(-90deg);
    }
    details.panel > .panel-body {
      padding: 0 1.5rem 1.5rem;
    }

    /* metric grid */
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(12rem, 1fr));
      gap: 1rem;
      margin-bottom: 1.5rem;
    }
    .metric-card {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--radius-lg);
      padding: 1.25rem 1.35rem 1.1rem;
      box-shadow: var(--shadow-sm);
      transition: border-color .18s, transform .18s, box-shadow .18s;
    }
    .metric-card:hover {
      border-color: var(--border-hi);
      transform: translateY(-1px);
      box-shadow: 0 6px 20px rgba(74,222,158,.06);
    }
    [data-theme="light"] .metric-card:hover { box-shadow: 0 6px 20px rgba(0,0,0,.08); }
    .metric-label {
      font-family: "Space Mono", monospace;
      font-size: .65rem;
      text-transform: uppercase;
      letter-spacing: .1em;
      color: var(--ink-3);
      margin-bottom: .6rem;
    }
    .metric-value {
      font-family: "Space Mono", monospace;
      font-size: 2.1rem;
      font-weight: 700;
      color: var(--ink);
      line-height: 1;
    }
    .metric-note {
      font-size: .75rem;
      color: var(--ink-2);
      margin-top: .5rem;
    }
    /* --------------------------------------------------
       OVERVIEW - executive dashboard
    -------------------------------------------------- */
    .overview-stack {
      display: grid;
      gap: 1.25rem;
      margin-bottom: 1.25rem;
    }
    .overview-hero {
      padding: 1.75rem 2rem;
      background: linear-gradient(135deg, var(--surface) 0%, var(--surface2) 100%);
      border: 1px solid var(--border);
      border-radius: var(--radius-lg);
      position: relative;
      overflow: hidden;
    }
    .overview-hero::before {
      content: '';
      position: absolute;
      top: 0; right: 0;
      width: 340px; height: 210px;
      background: radial-gradient(ellipse at top right, var(--accent-dim) 0%, transparent 72%);
      pointer-events: none;
    }
    .overview-top {
      display: flex;
      justify-content: space-between;
      align-items: flex-start;
      gap: 1.25rem;
      flex-wrap: wrap;
      position: relative;
    }
    .overview-kicker {
      display: inline-flex;
      align-items: center;
      gap: .4rem;
      font-family: "Space Mono", monospace;
      font-size: .6rem;
      text-transform: uppercase;
      letter-spacing: .15em;
      color: var(--accent);
      margin-bottom: .45rem;
    }
    .overview-kicker::before {
      content: '';
      display: inline-block;
      width: 6px; height: 6px;
      border-radius: 50%;
      background: var(--accent);
    }
    .overview-title-block h1 {
      font-size: 1.6rem;
      font-weight: 700;
      letter-spacing: -.03em;
      color: var(--ink);
      line-height: 1.15;
    }
    .overview-subtitle {
      max-width: 46rem;
      color: var(--ink-2);
      font-size: .9rem;
      margin-top: .45rem;
    }
    .overview-freshness {
      color: var(--ink-3);
      font-size: .76rem;
      margin-top: .55rem;
      font-family: "Space Mono", monospace;
      letter-spacing: .02em;
    }
    .overview-actions {
      display: flex;
      align-items: center;
      justify-content: flex-end;
      gap: .55rem;
      flex-wrap: wrap;
      position: relative;
    }
    .overview-status {
      display: grid;
      grid-template-columns: auto 1fr;
      gap: .75rem;
      align-items: start;
      margin-top: 1.35rem;
      padding: .95rem 1rem;
      background: var(--surface3);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      position: relative;
    }
    .overview-status-dot {
      width: .62rem;
      height: .62rem;
      border-radius: 999px;
      background: var(--ink-3);
      margin-top: .35rem;
    }
    .overview-status-title {
      font-weight: 700;
      color: var(--ink);
      line-height: 1.3;
    }
    .overview-status-detail {
      color: var(--ink-2);
      font-size: .82rem;
      margin-top: .15rem;
    }
    .tone-good .overview-status-dot { background: var(--accent); }
    .tone-warn .overview-status-dot { background: var(--warn); }
    .tone-danger .overview-status-dot { background: var(--danger); }
    .tone-good .overview-status-title { color: var(--accent); }
    .tone-warn .overview-status-title { color: var(--warn); }
    .tone-danger .overview-status-title { color: var(--danger); }

    /* overview signal strip */
    .health-strip {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(11.5rem, 1fr));
      gap: .75rem;
      margin-top: 1.35rem;
      padding-top: 1.35rem;
      border-top: 1px solid var(--border);
      position: relative;
    }
    .health-item-wrap {
      display: block;
      height: 100%;
      text-decoration: none;
      border-radius: var(--radius);
    }
    a.health-item-wrap { cursor: pointer; }
    a.health-item-wrap .health-item { transition: border-color .16s, background .16s, box-shadow .16s; }
    a.health-item-wrap:hover .health-item {
      border-color: var(--border-hi);
      box-shadow: var(--shadow-sm);
    }
    .health-item {
      display: grid;
      grid-template-rows: auto auto 1fr;
      gap: .3rem;
      height: 100%;
      min-height: 8.1rem;
      padding: 1rem 1.1rem;
      background: var(--surface3);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      min-width: 0;
      position: relative;
      overflow: hidden;
    }
    .health-item::after {
      content: '';
      position: absolute;
      bottom: 0; left: 0; right: 0;
      height: 2px;
      background: var(--ink-3);
    }
    .tone-good .health-item::after  { background: var(--accent); }
    .tone-warn .health-item::after  { background: var(--warn); }
    .tone-danger .health-item::after { background: var(--danger); }
    .health-row {
      display: flex;
      align-items: center;
      gap: .45rem;
    }
    .health-dot {
      width: .45rem;
      height: .45rem;
      border-radius: 999px;
      background: var(--ink-3);
      flex: 0 0 auto;
    }
    .tone-good  .health-dot { background: var(--accent); }
    .tone-warn  .health-dot { background: var(--warn); }
    .tone-danger .health-dot { background: var(--danger); }
    .health-label {
      font-family: "DM Sans", system-ui, sans-serif;
      font-size: .74rem;
      font-weight: 700;
      color: var(--ink-2);
      flex: 1;
    }
    .health-nav-arrow {
      font-size: .8rem;
      color: var(--ink-3);
      opacity: 0;
      transition: opacity .15s, transform .15s;
    }
    a.health-item-wrap:hover .health-nav-arrow { opacity: 1; transform: translateX(2px); }
    .health-value {
      font-family: "Space Mono", monospace;
      font-size: 1.5rem;
      font-weight: 700;
      color: var(--ink);
      line-height: 1.1;
      padding-top: .1rem;
    }
    .tone-good  .health-value { color: var(--accent); }
    .tone-warn  .health-value { color: var(--warn); }
    .tone-danger .health-value { color: var(--danger); }
    .health-detail {
      color: var(--ink-3);
      font-size: .74rem;
      line-height: 1.4;
      min-height: 2.1rem;
      padding-bottom: .35rem;
      align-self: end;
    }

    /* overview metrics and attention */
    .overview-metrics-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr));
      gap: .85rem;
    }
    .overview-metric-card {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--radius-lg);
      padding: 1.1rem 1.2rem;
      box-shadow: var(--shadow-sm);
      display: flex;
      flex-direction: column;
      min-height: 8.25rem;
    }
    .overview-metric-label {
      font-size: .72rem;
      font-weight: 700;
      color: var(--ink-2);
      margin-bottom: .45rem;
    }
    .overview-metric-value {
      font-family: "Space Mono", monospace;
      font-size: 1.65rem;
      font-weight: 700;
      color: var(--ink);
      line-height: 1;
    }
    .overview-metric-detail {
      color: var(--ink-3);
      font-size: .76rem;
      margin-top: .45rem;
      margin-bottom: .75rem;
    }
    .overview-metric-spark {
      margin-top: auto;
      height: 4px;
      border-radius: 999px;
      background: var(--surface3);
      overflow: hidden;
    }
    .overview-metric-spark span {
      display: block;
      height: 100%;
      border-radius: 999px;
      background: var(--accent-solid);
      min-width: 4px;
    }
    .attention-list {
      display: grid;
      gap: .65rem;
    }
    .attention-item {
      display: grid;
      grid-template-columns: auto 1fr auto;
      align-items: start;
      gap: .75rem;
      padding: .8rem .9rem;
      background: var(--surface2);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      text-decoration: none;
    }
    .attention-item:hover { border-color: var(--border-hi); background: var(--surface3); }
    .attention-dot {
      width: .5rem;
      height: .5rem;
      border-radius: 999px;
      background: var(--ink-3);
      margin-top: .45rem;
    }
    .tone-good .attention-dot { background: var(--accent); }
    .tone-warn .attention-dot { background: var(--warn); }
    .tone-danger .attention-dot { background: var(--danger); }
    .attention-title {
      color: var(--ink);
      font-weight: 700;
      line-height: 1.3;
    }
    .attention-detail {
      color: var(--ink-2);
      font-size: .8rem;
      margin-top: .12rem;
    }
    .attention-arrow {
      color: var(--ink-3);
      font-size: .8rem;
      padding-top: .2rem;
    }

    /* overview panels */
    .overview-charts-row,
    .overview-bottom-row {
      display: grid;
      grid-template-columns: minmax(0, 1.55fr) minmax(0, 1fr);
      gap: 1.25rem;
    }
    .overview-bottom-row { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
    .overview-device-grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 1.25rem;
      align-items: stretch;
    }
    .overview-device-grid .overview-panel {
      min-height: 23.5rem;
      display: flex;
      flex-direction: column;
    }
    .overview-device-grid .overview-chart {
      flex: 1;
      min-height: 17rem;
      display: flex;
      align-items: stretch;
    }
    .overview-device-grid .overview-chart svg {
      width: 100%;
      height: 100%;
      min-height: 17rem;
    }
    .overview-device-grid .chart-empty {
      flex: 1;
      min-height: 17rem;
    }
    .overview-panel {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--radius-lg);
      padding: 1.35rem 1.5rem;
      margin-bottom: 0;
      box-shadow: var(--shadow-sm);
    }
    .overview-panel-header {
      display: flex;
      align-items: baseline;
      justify-content: space-between;
      gap: .75rem;
      margin-bottom: 1.1rem;
      padding-bottom: .75rem;
      border-bottom: 1px solid var(--border);
    }
    .overview-panel-title {
      font-family: "Space Mono", monospace;
      font-size: .62rem;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: .13em;
      color: var(--ink-3);
    }
    .overview-panel-meta {
      font-size: .72rem;
      color: var(--ink-3);
    }
    .overview-panel-link {
      color: var(--accent);
      text-decoration: none;
      font-size: .76rem;
      font-weight: 600;
    }
    .overview-panel-link:hover { text-decoration: underline; }

    /* audit bar chart */
    .overview-chart { display: grid; gap: .5rem; }
    .overview-chart svg { width: 100%; height: auto; display: block; }
    .chart-axis {
      fill: var(--ink-3);
      font-family: "Space Mono", monospace;
      font-size: 10px;
    }
    .chart-gridline { stroke: var(--border); stroke-width: 1; stroke-dasharray: 3 3; }
    .chart-bar-bg  { fill: var(--surface3); }
    .chart-bar     { fill: var(--accent-solid); opacity: .82; }
    .chart-empty {
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      gap: .25rem;
      min-height: 120px;
      color: var(--ink-2);
      font-size: .82rem;
      text-align: center;
      border: 1px dashed var(--border);
      border-radius: var(--radius);
      padding: 1rem;
    }
    .chart-empty small {
      color: var(--ink-3);
      font-size: .74rem;
      line-height: 1.4;
    }
    .chart-total-badge {
      display: inline-flex;
      align-items: center;
      gap: .3rem;
      font-family: "Space Mono", monospace;
      font-size: .62rem;
      color: var(--accent);
      background: var(--accent-dim);
      border: 1px solid var(--border-hi);
      border-radius: 999px;
      padding: .12rem .55rem;
    }

    /* command breakdown */
    .cmd-breakdown {
      display: grid;
      gap: .65rem;
      margin-bottom: 1rem;
    }
    .cmd-row {
      display: grid;
      grid-template-columns: 4.5rem 1fr 2.5rem;
      align-items: center;
      gap: .6rem;
    }
    .cmd-row-label {
      font-family: "Space Mono", monospace;
      font-size: .62rem;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: .08em;
      color: var(--ink-3);
    }
    .cmd-track {
      height: 8px;
      background: var(--surface3);
      border-radius: 999px;
      overflow: hidden;
    }
    .cmd-fill {
      height: 100%;
      border-radius: 999px;
      transition: width .4s ease;
      min-width: 2px;
    }
    .cmd-bar-sent   { background: var(--warn); }
    .cmd-bar-acked  { background: var(--accent-solid); }
    .cmd-bar-failed { background: var(--danger); }
    .cmd-row-count {
      font-family: "Space Mono", monospace;
      font-size: .72rem;
      font-weight: 700;
      color: var(--ink-2);
      text-align: right;
    }
    .cmd-summary {
      display: flex;
      gap: 1rem;
      padding-top: .85rem;
      border-top: 1px solid var(--border);
    }
    .cmd-stat { text-align: center; flex: 1; }
    .cmd-stat-value {
      font-family: "Space Mono", monospace;
      font-size: 1.3rem;
      font-weight: 700;
      color: var(--ink);
      line-height: 1.1;
    }
    .cmd-rate-good   { color: var(--accent); }
    .cmd-rate-warn   { color: var(--warn); }
    .cmd-rate-danger { color: var(--danger); }
    .cmd-stat-label {
      font-family: "Space Mono", monospace;
      font-size: .58rem;
      text-transform: uppercase;
      letter-spacing: .1em;
      color: var(--ink-3);
      margin-top: .2rem;
    }

    /* content composition */
    .content-comp { display: grid; gap: 1rem; }
    .comp-bar {
      display: flex;
      height: 10px;
      border-radius: 999px;
      overflow: hidden;
      gap: 2px;
      background: var(--surface3);
    }
    .comp-segment { height: 100%; transition: width .4s ease; min-width: 2px; }
    .comp-apps  { background: var(--accent-solid); }
    .comp-files { background: var(--warn); }
    .comp-certs { background: #60a5fa; }
    .comp-legend {
      display: grid;
      grid-template-columns: repeat(3, 1fr);
      gap: .5rem;
    }
    .comp-legend-item {
      display: flex;
      flex-direction: column;
      gap: .2rem;
      padding: .75rem .85rem;
      background: var(--surface2);
      border: 1px solid var(--border);
      border-radius: var(--radius-sm);
      text-decoration: none;
      transition: border-color .15s, background .15s;
    }
    .comp-legend-item:hover { border-color: var(--border-hi); background: var(--surface3); }
    .comp-dot {
      display: block;
      width: 8px; height: 8px;
      border-radius: 50%;
      margin-bottom: .2rem;
    }
    .comp-dot.comp-apps  { background: var(--accent-solid); }
    .comp-dot.comp-files { background: var(--warn); }
    .comp-dot.comp-certs { background: #60a5fa; }
    .comp-name {
      font-family: "Space Mono", monospace;
      font-size: .58rem;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: .1em;
      color: var(--ink-3);
    }
    .comp-count {
      font-family: "Space Mono", monospace;
      font-size: 1.1rem;
      font-weight: 700;
      color: var(--ink);
      line-height: 1;
    }
    .comp-pct {
      font-size: .72rem;
      color: var(--ink-3);
    }

    /* recent activity */
    .activity-list {
      display: grid;
      gap: .55rem;
    }
    .activity-item {
      display: grid;
      grid-template-columns: 5.8rem 1fr;
      gap: .8rem;
      padding: .7rem .75rem;
      background: var(--surface2);
      border: 1px solid var(--border);
      border-radius: var(--radius-sm);
    }
    .activity-time {
      font-family: "Space Mono", monospace;
      color: var(--ink-3);
      font-size: .68rem;
      line-height: 1.45;
    }
    .activity-main {
      min-width: 0;
    }
    .activity-action {
      color: var(--ink);
      font-weight: 700;
      line-height: 1.3;
      overflow-wrap: anywhere;
    }
    .activity-meta {
      color: var(--ink-2);
      font-size: .78rem;
      margin-top: .12rem;
      overflow-wrap: anywhere;
    }

    @media (max-width: 1080px) {
      .overview-charts-row,
      .overview-bottom-row { grid-template-columns: 1fr; }
      .comp-legend { grid-template-columns: repeat(3, 1fr); }
    }
    @media (max-width: 860px) {
      .overview-device-grid { grid-template-columns: 1fr; }
    }
    @media (max-width: 640px) {
      .overview-hero { padding: 1.35rem; }
      .overview-actions { justify-content: flex-start; }
      .comp-legend { grid-template-columns: 1fr; }
      .activity-item { grid-template-columns: 1fr; }
    }

    /* ═══════════════════════════════════════════
       ALERTS
    ═══════════════════════════════════════════ */
    .alert {
      display: flex; align-items: flex-start; gap: .6rem;
      padding: .75rem 1rem;
      border-radius: var(--radius);
      margin-bottom: 1.25rem;
      font-size: .875rem;
      line-height: 1.5;
    }
    .alert-icon { font-size: 1rem; flex-shrink: 0; margin-top: .05rem; }
    .alert-success {
      background: var(--flash-bg);
      border: 1px solid var(--flash-border);
      color: var(--flash-ink);
    }
    .alert-error {
      background: var(--danger-dim);
      border: 1px solid rgba(248,113,113,.3);
      color: var(--danger);
    }
    /* keep old class names working */
    .flash { display:flex; align-items:flex-start; gap:.6rem; padding:.75rem 1rem; border-radius:var(--radius); margin-bottom:1.25rem; font-size:.875rem; background:var(--flash-bg); border:1px solid var(--flash-border); color:var(--flash-ink); }
    .error { display:flex; align-items:flex-start; gap:.6rem; padding:.75rem 1rem; border-radius:var(--radius); margin-bottom:1.25rem; font-size:.875rem; background:var(--danger-dim); border:1px solid rgba(248,113,113,.3); color:var(--danger); }

    /* ═══════════════════════════════════════════
       TABLES
    ═══════════════════════════════════════════ */
    .table-wrap { overflow-x: auto; margin: 0 -1.5rem; padding: 0 1.5rem; }
    .pager {
      display: flex;
      justify-content: space-between;
      align-items: center;
      flex-wrap: wrap;
      gap: 1rem;
      margin: 0;
      padding: 1rem 0 0;
      border-top: 1px solid var(--border);
      color: var(--ink-2);
      width: 100%;
    }
    .pager a,
    .pager span {
      white-space: nowrap;
    }
    .pager a {
      color: var(--accent);
      text-decoration: none;
      font-weight: 600;
    }
    .pager a:hover {
      text-decoration: underline;
    }
    .pager-disabled {
      opacity: .45;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: .875rem;
      min-width: 500px;
    }
    thead tr { border-bottom: 1px solid var(--border); }
    th {
      text-align: left;
      padding: .55rem .75rem;
      font-family: "Space Mono", monospace;
      font-size: .635rem;
      text-transform: uppercase;
      letter-spacing: .1em;
      color: var(--ink-3);
      font-weight: 700;
      white-space: nowrap;
      background: var(--surface2);
    }
    th:first-child { border-radius: var(--radius-sm) 0 0 var(--radius-sm); }
    th:last-child  { border-radius: 0 var(--radius-sm) var(--radius-sm) 0; }
    td {
      padding: .65rem .75rem;
      border-bottom: 1px solid var(--border);
      vertical-align: middle;
      color: var(--ink);
      font-size: .875rem;
    }
    tbody tr { transition: background .1s; }
    tbody tr:hover { background: var(--surface2); }
    tbody tr:last-child td { border-bottom: none; }

    /* inline table forms: compact inputs */
    td input, td textarea, td select {
      padding: .3rem .55rem;
      font-size: .8rem;
      min-width: 7rem;
    }
    td textarea { min-height: 3.5rem; }

    /* ═══════════════════════════════════════════
       FORMS
    ═══════════════════════════════════════════ */
    .form-grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 0 1.5rem;
    }
    .form-grid .field-full { grid-column: 1 / -1; }

    label, .form-label {
      display: block;
      font-family: "Space Mono", monospace;
      font-size: .67rem;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: .08em;
      color: var(--ink-3);
      margin: 1rem 0 .35rem;
    }
    input, textarea, select {
      width: 100%;
      padding: .6rem .85rem;
      background: var(--surface2);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      color: var(--ink);
      font-family: "DM Sans", sans-serif;
      font-size: .9rem;
      line-height: 1.5;
      transition: border-color .15s, box-shadow .15s, background .2s;
      outline: none;
      appearance: none;
    }
    input::placeholder, textarea::placeholder { color: var(--ink-3); }
    input:hover, textarea:hover, select:hover { border-color: var(--ink-3); }
    input:focus, textarea:focus, select:focus {
      border-color: var(--accent-solid);
      box-shadow: 0 0 0 3px var(--accent-dim);
      background: var(--surface);
    }
    input[type="checkbox"] {
      width: auto; height: 1rem; width: 1rem;
      accent-color: var(--accent-solid);
      appearance: auto;
      cursor: pointer;
      margin-right: .4rem;
      flex: none;
    }
    textarea { min-height: 7rem; resize: vertical; font-family: "Space Mono", monospace; font-size: .8rem; }
    select {
      background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%236b7a8a' stroke-width='2'%3E%3Cpath d='M6 9l6 6 6-6'/%3E%3C/svg%3E");
      background-repeat: no-repeat;
      background-position: right .75rem center;
      padding-right: 2.25rem;
    }
    select option { background: var(--surface2); }

    /* ═══════════════════════════════════════════
       BUTTONS
    ═══════════════════════════════════════════ */
    button, .button {
      display: inline-flex; align-items: center; gap: .35rem;
      padding: .5rem 1rem;
      border: 1px solid var(--border);
      border-radius: var(--radius);
      background: var(--surface2);
      color: var(--ink-2);
      font-family: "DM Sans", sans-serif;
      font-size: .85rem;
      font-weight: 500;
      cursor: pointer;
      text-decoration: none;
      white-space: nowrap;
      transition: background .14s, color .14s, border-color .14s, box-shadow .14s;
    }
    button:hover, .button:hover {
      background: var(--surface3);
      color: var(--ink);
      border-color: var(--ink-3);
      box-shadow: var(--shadow-sm);
    }
    .btn-primary, button[type="submit"]:not(.danger):not(.btn-icon):not(.btn-logout) {
      background: var(--accent-dim);
      color: var(--accent);
      border-color: var(--accent-solid);
      font-weight: 600;
    }
    .btn-primary:hover, button[type="submit"]:not(.danger):not(.btn-icon):not(.btn-logout):hover {
      background: var(--accent-solid);
      color: #fff;
      border-color: var(--accent-solid);
      box-shadow: 0 0 16px var(--accent-glow);
    }
    .danger, button.danger {
      background: var(--danger-dim);
      color: var(--danger);
      border-color: rgba(248,113,113,.35);
    }
    .danger:hover, button.danger:hover {
      background: var(--danger-solid);
      color: #fff;
      border-color: var(--danger-solid);
      box-shadow: 0 0 12px rgba(220,38,38,.25);
    }
    form.inline { display: inline; }
    .actions { display: flex; gap: .4rem; flex-wrap: wrap; align-items: center; }
    p { margin-top: 1.25rem; }

    /* ═══════════════════════════════════════════
       CODE / PRE
    ═══════════════════════════════════════════ */
    code, pre {
      font-family: "Space Mono", monospace;
      font-size: .78rem;
    }
    code {
      background: var(--surface3);
      padding: .1rem .35rem;
      border-radius: var(--radius-sm);
      color: var(--accent);
    }
    pre {
      white-space: pre-wrap;
      overflow-x: auto;
      background: var(--pre-bg);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      padding: .85rem 1rem;
      color: var(--pre-ink);
      line-height: 1.6;
      transition: background .2s;
    }
    pre.qr-json {
      white-space: pre-wrap;
      word-break: break-word;
      overflow-wrap: anywhere;
      max-width: 100%;
    }

    /* ═══════════════════════════════════════════
       STRUCTURED DETAIL VIEWS
    ═══════════════════════════════════════════ */
    .structured-data {
      display: grid;
      gap: .75rem;
      min-width: 0;
    }
    .structured-table {
      width: 100%;
      min-width: 0;
      border-collapse: separate;
      border-spacing: 0;
      border: 1px solid var(--border);
      border-radius: var(--radius);
      overflow: hidden;
      background: var(--surface2);
    }
    .structured-table > thead > tr > th,
    .structured-table > thead > tr > td,
    .structured-table > tbody > tr > th,
    .structured-table > tbody > tr > td {
      vertical-align: top;
      border-right: 1px solid var(--border);
      border-bottom: 1px solid var(--border);
      overflow-wrap: anywhere;
    }
    .structured-table > thead > tr > th:last-child,
    .structured-table > thead > tr > td:last-child,
    .structured-table > tbody > tr > th:last-child,
    .structured-table > tbody > tr > td:last-child { border-right: none; }
    .structured-table > tbody > tr:last-child > th,
    .structured-table > tbody > tr:last-child > td { border-bottom: none; }
    .structured-table > thead > tr:last-child > th,
    .structured-table > thead > tr:last-child > td { border-bottom: 1px solid var(--border); }
    .structured-table .structured-table {
      background: var(--surface);
      border: 1px solid var(--border);
      border-collapse: separate;
      border-spacing: 0;
      box-shadow: inset 0 0 0 1px var(--border);
    }
    .structured-table td > .structured-data,
    .structured-table td > .structured-table {
      margin-top: .15rem;
    }
    .structured-data .structured-table {
      border: 1px solid var(--border);
    }
    .structured-key {
      width: 13rem;
      color: var(--ink-3);
      background: var(--surface3);
    }
    .structured-empty {
      padding: .85rem 1rem;
      color: var(--ink-2);
      border: 1px dashed var(--border);
      border-radius: var(--radius);
      background: var(--surface2);
    }
    .structured-scalar {
      color: var(--ink);
    }
    .structured-muted {
      color: var(--ink-3);
    }
    .structured-list {
      display: flex;
      flex-wrap: wrap;
      gap: .35rem;
    }
    .structured-pill {
      display: inline-flex;
      align-items: center;
      padding: .16rem .5rem;
      border-radius: 999px;
      background: var(--surface3);
      border: 1px solid var(--border);
      color: var(--ink-2);
      font-family: "Space Mono", monospace;
      font-size: .68rem;
    }

    /* audit table layout */
    .audit-table {
      table-layout: fixed;
      min-width: 760px;
    }
    .audit-table .audit-created { width: 12.5rem; }
    .audit-table .audit-actor { width: 6rem; max-width: 6rem; }
    .audit-table .audit-action { width: 7rem; }
    .audit-table .audit-resource { width: 11rem; }
    .audit-table .audit-details { width: auto; }
    .audit-table td.audit-actor,
    .audit-table td.audit-action,
    .audit-table td.audit-resource {
      overflow-wrap: anywhere;
    }
    .audit-table td.audit-details {
      min-width: 30rem;
      overflow-wrap: anywhere;
    }
    .audit-table td.audit-details pre,
    .audit-table td.audit-details .raw-data,
    .audit-table td.audit-details .structured-data {
      max-width: 100%;
    }

    /* ═══════════════════════════════════════════
       MISC HELPERS
    ═══════════════════════════════════════════ */
    .muted { color: var(--ink-2); }
    .divider {
      height: 1px;
      background: var(--border);
      margin: 1.5rem 0;
    }
    .status-pill {
      display: inline-flex;
      align-items: center;
      padding: .16rem .5rem;
      border-radius: 999px;
      font-family: "Space Mono", monospace;
      font-size: .62rem;
      font-weight: 700;
      letter-spacing: .1em;
      text-transform: uppercase;
      border: 1px solid transparent;
      white-space: nowrap;
    }
    .status-active {
      color: var(--accent);
      background: var(--accent-dim);
      border-color: var(--border-hi);
    }
    .status-enabled {
      color: var(--accent);
      background: var(--accent-dim);
      border-color: var(--border-hi);
    }
    .status-disabled {
      color: var(--ink-2);
      background: rgba(148,163,184,.08);
      border-color: rgba(148,163,184,.24);
    }
    .status-pending {
      color: var(--warn);
      background: rgba(251,191,36,.08);
      border-color: rgba(251,191,36,.28);
    }
    .status-enrolled {
      color: var(--warn);
      background: rgba(251,191,36,.08);
      border-color: rgba(251,191,36,.28);
    }
    .status-issued {
      color: var(--accent);
      background: var(--accent-dim);
      border-color: var(--border-hi);
    }
    .status-consumed {
      color: var(--ink);
      background: rgba(148,163,184,.08);
      border-color: rgba(148,163,184,.24);
    }
    .status-expired {
      color: var(--ink-2);
      background: rgba(148,163,184,.06);
      border-color: rgba(148,163,184,.2);
    }
    .status-retired {
      color: var(--danger);
      background: var(--danger-dim);
      border-color: rgba(248,113,113,.28);
    }
    .field-help {
      margin-top: .5rem;
      color: var(--ink-2);
      font-size: .82rem;
      line-height: 1.5;
    }
    .field-help p + p,
    .field-help ul + p,
    .field-help p + ul,
    .field-help ul + ul {
      margin-top: .6rem;
    }
    .field-help ul {
      margin: .55rem 0 0 1.15rem;
      padding: 0;
      display: grid;
      gap: .25rem;
    }
    .policy-summary {
      display: block;
      border: 1px solid var(--border);
      border-radius: var(--radius);
      overflow: hidden;
      background: var(--surface2);
    }
    .policy-summary-item {
      display: grid;
      grid-template-columns: 13rem minmax(0, 1fr);
      min-width: 0;
      border-bottom: 1px solid var(--border);
    }
    .policy-summary-item:last-child {
      border-bottom: none;
    }
    .policy-summary-label {
      font-family: "Space Mono", monospace;
      font-size: .635rem;
      font-weight: 700;
      letter-spacing: .1em;
      text-transform: uppercase;
      color: var(--ink-3);
      background: var(--surface3);
      padding: .65rem .75rem;
      overflow-wrap: anywhere;
    }
    .policy-summary-value {
      color: var(--ink);
      padding: .65rem .75rem;
      overflow-wrap: anywhere;
      min-width: 0;
    }
    .policy-summary-wide {
      grid-template-columns: 13rem minmax(0, 1fr);
    }
    .policy-detail-section {
      margin-top: 1.5rem;
    }
    .section-heading {
      display: flex;
      align-items: baseline;
      justify-content: space-between;
      gap: .75rem;
    }
    .section-heading-meta {
      font-family: "DM Sans", system-ui, sans-serif;
      font-size: .72rem;
      font-weight: 600;
      letter-spacing: 0;
      text-transform: none;
      color: var(--ink-3);
      white-space: nowrap;
    }
    details.raw-data {
      margin-top: .85rem;
      border: 1px solid var(--border);
      border-radius: var(--radius);
      background: var(--surface2);
      overflow: hidden;
      position: relative;
    }
    details.raw-data summary {
      cursor: pointer;
      padding: .7rem 4.8rem .7rem .85rem;
      color: var(--ink-2);
      font-size: .78rem;
      font-weight: 700;
      user-select: none;
      list-style: none;
      display: flex;
      align-items: center;
      gap: .55rem;
      min-height: 2.55rem;
    }
    details.raw-data summary::-webkit-details-marker { display: none; }
    details.raw-data summary::before {
      content: '▸';
      color: var(--ink-3);
      font-size: .78rem;
      line-height: 1;
      flex: 0 0 auto;
    }
    details.raw-data[open] summary {
      border-bottom: 1px solid var(--border);
    }
    details.raw-data[open] summary::before { content: '▾'; }
    .raw-data-header {
      display: inline-flex;
      align-items: center;
      gap: .55rem;
      min-width: 0;
    }
    .raw-copy {
      position: absolute;
      top: .45rem;
      right: .65rem;
      z-index: 2;
      padding: .25rem .55rem;
      font-size: .72rem;
      border-radius: var(--radius-sm);
    }
    details.raw-data pre {
      margin: 0;
      border: none;
      border-radius: 0;
      background: transparent;
    }
    details.raw-data .structured-data {
      padding: .85rem;
    }
    .policy-summary + .policy-detail-section {
      margin-top: 1.75rem;
    }
    .policy-restrictions-value {
      margin-top: .55rem;
    }
    .policy-restrictions-value .structured-table {
      background: var(--surface);
    }
    .policy-restrictions-value .structured-key {
      width: 14rem;
    }
    .policy-summary-value .structured-data {
      margin-top: .35rem;
    }
    .summary-detail-note {
      color: var(--ink-3);
      font-size: .76rem;
      margin-top: .55rem;
    }
    @media (max-width: 860px) {
      .policy-summary-item,
      .policy-summary-wide {
        grid-template-columns: 1fr;
      }
      .policy-summary-label {
        border-bottom: 1px solid var(--border);
      }
    }
    .permission-catalog {
      display: flex;
      flex-wrap: wrap;
      gap: .35rem;
      margin-top: .45rem;
    }
    .permission-catalog code {
      color: var(--ink);
      background: var(--surface3);
      border: 1px solid var(--border);
    }
    .toggle-group {
      display: flex;
      flex-wrap: wrap;
      gap: .5rem;
      margin-top: .35rem;
    }
    .toggle-option {
      display: inline-flex;
      align-items: center;
      gap: .4rem;
      padding: .4rem .65rem;
      border: 1px solid var(--border);
      border-radius: var(--radius-sm);
      background: var(--surface2);
      color: var(--ink-2);
      font-size: .85rem;
      text-transform: none;
      letter-spacing: 0;
      cursor: pointer;
    }
    .toggle-option input {
      margin: 0;
    }
            .checkbox-group {
              display: grid;
              gap: .28rem;
              margin-top: .25rem;
              max-height: 11rem;
              overflow-y: auto;
              padding-right: .25rem;
            }
            .checkbox-option {
              display: inline-flex;
              align-items: center;
              gap: .35rem;
              min-width: 0;
              color: var(--ink-2);
              font-size: .76rem;
              line-height: 1.35;
            }
    .checkbox-option input {
      margin: 0;
      flex: 0 0 auto;
    }

    /* ═══════════════════════════════════════════
       RESPONSIVE
    ═══════════════════════════════════════════ */
    @media (max-width: 800px) {
      .shell { grid-template-columns: 1fr; }
      .sidebar {
        position: static;
        height: auto;
        flex-direction: row;
        flex-wrap: wrap;
        padding: .5rem .75rem;
        border-right: none;
        border-bottom: 1px solid var(--border);
        gap: 2px;
      }
      .nav-group-label { display: none; }
      .sidebar a { font-size: .8rem; padding: .35rem .6rem; }
      .sidebar a::before { display: none; }
      .content { padding: 1.25rem 1rem 2rem; }
      .form-grid { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body{{if not .User}} class="page-login"{{end}}>

  <!-- ── HEADER ───────────────────────────────────── -->
  <header>
    <a href="/admin" class="brand">
      <span class="brand-dot"></span>
      XMDM
    </a>
    <div class="header-right">
      {{if .User}}<span class="header-user">{{.User}}</span>{{end}}
      <button class="btn-icon" onclick="toggleTheme()" title="Toggle theme" type="button">
        <span class="icon-moon">🌙</span>
        <span class="icon-sun">☀️</span>
      </button>
      {{if .User}}<form method="post" action="/admin/logout" class="inline"><input type="hidden" name="csrfToken" value="{{.CSRFToken}}"><button type="submit" class="btn-logout">Sign out</button></form>{{end}}
    </div>
  </header>

  <!-- ══════════════════════════════════════════════
       LOGIN PAGE — no sidebar, card centred
  ══════════════════════════════════════════════ -->
  {{if not .User}}
  <div class="login-wrap">
    {{if .Flash}}<div class="flash">✓ {{.Flash}}</div>{{end}}
    {{if .Error}}<div class="error">⚠ {{.Error}}</div>{{end}}
    {{range .Forms}}
    <div class="login-card">
      <div class="login-logo">
        <span class="login-logo-dot"></span>
        <span class="login-logo-text">XMDM Admin</span>
      </div>
      <div class="login-title">{{.Title}}</div>
      <div class="login-sub">Sign in to manage your device fleet</div>
      <form method="post" action="{{.Action}}">
        <input type="hidden" name="csrfToken" value="{{$.CSRFToken}}">
        {{range .Fields}}
          <label for="{{.Name}}">{{.Label}}</label>
          <input id="{{.Name}}" name="{{.Name}}" type="{{.Type}}" value="{{.Value}}" placeholder="{{.Placeholder}}" {{if .Required}}required{{end}} autocomplete="{{if eq .Type "password"}}current-password{{else}}username{{end}}">
        {{end}}
        <button type="submit" class="btn-primary">{{.Submit}}</button>
      </form>
    </div>
    <div class="login-footer">XMDM · Mobile Device Management</div>
    {{end}}
  </div>

  <!-- ══════════════════════════════════════════════
       AUTHENTICATED LAYOUT — sidebar + content
  ══════════════════════════════════════════════ -->
  {{else}}
  <div class="shell">
    <nav class="sidebar" id="sidebar">
      <a href="/admin"              data-path="/admin"><span class="nav-icon">{{navIcon "overview"}}</span> Overview</a>
      <span class="nav-group-label">Fleet</span>
      <a href="/admin/devices"      data-path="/admin/devices"><span class="nav-icon">{{navIcon "devices"}}</span> Devices</a>
      <a href="/admin/policies"     data-path="/admin/policies"><span class="nav-icon">{{navIcon "policies"}}</span> Policies</a>
      <a href="/admin/groups"       data-path="/admin/groups"><span class="nav-icon">{{navIcon "groups"}}</span> Groups</a>
      <a href="/admin/commands"     data-path="/admin/commands"><span class="nav-icon">{{navIcon "commands"}}</span> Commands</a>
      <span class="nav-group-label">Content</span>
      <a href="/admin/apps"         data-path="/admin/apps"><span class="nav-icon">{{navIcon "apps"}}</span> Apps</a>
      <a href="/admin/managed-files" data-path="/admin/managed-files"><span class="nav-icon">{{navIcon "managed-files"}}</span> Managed Files</a>
      <a href="/admin/certificates" data-path="/admin/certificates"><span class="nav-icon">{{navIcon "certificates"}}</span> Certificates</a>
      <span class="nav-group-label">Identity</span>
      <a href="/admin/users"        data-path="/admin/users"><span class="nav-icon">{{navIcon "users"}}</span> Users</a>
      <a href="/admin/roles"        data-path="/admin/roles"><span class="nav-icon">{{navIcon "roles"}}</span> Roles</a>
      <span class="nav-group-label">Operations</span>
      <a href="/admin/audit"        data-path="/admin/audit"><span class="nav-icon">{{navIcon "audit"}}</span> Audit</a>
    </nav>

    <main class="content">
      {{if not .Overview}}
      <div class="page-header">
        <div>
          <h1>{{.Title}}</h1>
          {{if .Subtitle}}<p class="page-subtitle">{{.Subtitle}}</p>{{end}}
        </div>
      </div>
      {{end}}
      {{if .Flash}}<div class="flash">✓ {{.Flash}}</div>{{end}}
      {{if .Error}}<div class="error">⚠ {{.Error}}</div>{{end}}
      {{if .Callout}}<section class="panel">{{.Callout}}</section>{{end}}
      {{if .Overview}}<div class="overview-stack">{{.Overview}}</div>{{end}}

      {{if not .FormsAfterItems}}{{range .Forms}}<section class="panel"><h2>{{.Title}}</h2><form method="post" action="{{.Action}}" {{if .EncType}}enctype="{{.EncType}}"{{end}}><input type="hidden" name="csrfToken" value="{{$.CSRFToken}}">{{range .Fields}}{{$field := .}}{{if eq $field.Type "checkbox"}}<label id="{{$field.Name}}-label" for="{{$field.Name}}">{{$field.Label}}</label><input id="{{$field.Name}}" name="{{$field.Name}}" type="checkbox" value="on" {{if $field.Value}}checked{{end}}>{{else if eq $field.Type "multiselect"}}<div class="form-label" id="{{$field.Name}}-label">{{$field.Label}}</div><div class="checkbox-group" role="group" aria-labelledby="{{$field.Name}}-label">{{range $field.Options}}<label class="checkbox-option"><input id="{{$field.Name}}-{{.Value}}" name="{{$field.Name}}" type="checkbox" value="{{.Value}}" {{if containsString $field.Values .Value}}checked{{end}}><span>{{.Label}}</span></label>{{end}}</div>{{else if eq $field.Type "radio"}}<div class="form-label" id="{{$field.Name}}-label">{{$field.Label}}</div><div class="toggle-group" role="radiogroup" aria-labelledby="{{$field.Name}}-label">{{range $field.Options}}<label class="toggle-option"><input id="{{$field.Name}}-{{.Value}}" name="{{$field.Name}}" type="radio" value="{{.Value}}" {{if eq .Value $field.Value}}checked{{end}}><span>{{.Label}}</span></label>{{end}}</div>{{else}}<label for="{{$field.Name}}" id="{{$field.Name}}-label">{{$field.Label}}</label>{{if eq $field.Type "textarea"}}<textarea id="{{$field.Name}}" name="{{$field.Name}}" placeholder="{{$field.Placeholder}}" {{if $field.Required}}required{{end}}>{{$field.Value}}</textarea>{{else if eq $field.Type "select"}}<select id="{{$field.Name}}" name="{{$field.Name}}" {{if $field.Required}}required{{end}}>{{if $field.Placeholder}}<option value="" disabled {{if not $field.Value}}selected{{end}}>{{$field.Placeholder}}</option>{{end}}{{range $field.Options}}<option value="{{.Value}}" {{if eq .Value $field.Value}}selected{{end}}>{{.Label}}</option>{{end}}</select>{{else}}<input id="{{$field.Name}}" name="{{$field.Name}}" type="{{$field.Type}}" value="{{$field.Value}}" placeholder="{{$field.Placeholder}}" {{if $field.Required}}required{{end}}>{{end}}{{end}}{{end}}{{if .Help}}<div class="field-help">{{.Help}}</div>{{end}}<p><button type="submit" {{if .Danger}}class="danger"{{end}}>{{.Submit}}</button></p>{{if .After}}{{.After}}{{end}}</form></section>{{end}}{{end}}

      {{if .Items}}{{if .ItemsRaw}}{{.Items}}{{else}}<section class="panel"><div class="table-wrap">{{.Items}}</div></section>{{end}}{{end}}

      {{if .FormsAfterItems}}{{range .Forms}}<section class="panel"><h2>{{.Title}}</h2><form method="post" action="{{.Action}}" {{if .EncType}}enctype="{{.EncType}}"{{end}}><input type="hidden" name="csrfToken" value="{{$.CSRFToken}}">{{range .Fields}}{{$field := .}}{{if eq $field.Type "checkbox"}}<label id="{{$field.Name}}-label" for="{{$field.Name}}">{{$field.Label}}</label><input id="{{$field.Name}}" name="{{$field.Name}}" type="checkbox" value="on" {{if $field.Value}}checked{{end}}>{{else if eq $field.Type "multiselect"}}<div class="form-label" id="{{$field.Name}}-label">{{$field.Label}}</div><div class="checkbox-group" role="group" aria-labelledby="{{$field.Name}}-label">{{range $field.Options}}<label class="checkbox-option"><input id="{{$field.Name}}-{{.Value}}" name="{{$field.Name}}" type="checkbox" value="{{.Value}}" {{if containsString $field.Values .Value}}checked{{end}}><span>{{.Label}}</span></label>{{end}}</div>{{else if eq $field.Type "radio"}}<div class="form-label" id="{{$field.Name}}-label">{{$field.Label}}</div><div class="toggle-group" role="radiogroup" aria-labelledby="{{$field.Name}}-label">{{range $field.Options}}<label class="toggle-option"><input id="{{$field.Name}}-{{.Value}}" name="{{$field.Name}}" type="radio" value="{{.Value}}" {{if eq .Value $field.Value}}checked{{end}}><span>{{.Label}}</span></label>{{end}}</div>{{else}}<label for="{{$field.Name}}" id="{{$field.Name}}-label">{{$field.Label}}</label>{{if eq $field.Type "textarea"}}<textarea id="{{$field.Name}}" name="{{$field.Name}}" placeholder="{{$field.Placeholder}}" {{if $field.Required}}required{{end}}>{{$field.Value}}</textarea>{{else if eq $field.Type "select"}}<select id="{{$field.Name}}" name="{{$field.Name}}" {{if $field.Required}}required{{end}}>{{if $field.Placeholder}}<option value="" disabled {{if not $field.Value}}selected{{end}}>{{$field.Placeholder}}</option>{{end}}{{range $field.Options}}<option value="{{.Value}}" {{if eq .Value $field.Value}}selected{{end}}>{{.Label}}</option>{{end}}</select>{{else}}<input id="{{$field.Name}}" name="{{$field.Name}}" type="{{$field.Type}}" value="{{$field.Value}}" placeholder="{{$field.Placeholder}}" {{if $field.Required}}required{{end}}>{{end}}{{end}}{{end}}{{if .Help}}<div class="field-help">{{.Help}}</div>{{end}}<p><button type="submit" {{if .Danger}}class="danger"{{end}}>{{.Submit}}</button></p>{{if .After}}{{.After}}{{end}}</form></section>{{end}}{{end}}
    </main>
  </div>
  {{end}}

  <script>
    /* ── theme ── */
    (function(){
      var t = localStorage.getItem('xmdm-theme') || 'dark';
      document.documentElement.setAttribute('data-theme', t);
    })();
    function toggleTheme(){
      var t = document.documentElement.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
      document.documentElement.setAttribute('data-theme', t);
      localStorage.setItem('xmdm-theme', t);
    }

    function copyRawData(button){
      var details = button.closest('details.raw-data');
      var pre = details ? details.querySelector('pre') : null;
      if(!pre) return;
      var text = pre.innerText || pre.textContent || '';
      var done = function(){
        var original = button.textContent;
        button.textContent = 'Copied';
        button.disabled = true;
        setTimeout(function(){ button.textContent = original; button.disabled = false; }, 1200);
      };
      if(navigator.clipboard && navigator.clipboard.writeText){
        navigator.clipboard.writeText(text).then(done).catch(function(){ fallbackCopy(text); done(); });
      } else {
        fallbackCopy(text); done();
      }
    }
    function fallbackCopy(text){
      var ta = document.createElement('textarea');
      ta.value = text;
      ta.setAttribute('readonly', '');
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      try { document.execCommand('copy'); } catch(e) {}
      document.body.removeChild(ta);
    }

    /* ── active nav highlight ── */
    (function(){
      var path = window.location.pathname;
      var links = document.querySelectorAll('#sidebar a[data-path]');
      /* find the longest prefix match (most specific wins) */
      var best = null, bestLen = 0;
      links.forEach(function(a){
        var p = a.getAttribute('data-path');
        if(path === p || (path.startsWith(p) && (p === '/admin' ? path === '/admin' : true))){
          if(p.length > bestLen){ best = a; bestLen = p.length; }
        }
      });
      /* exact-match fallback for /admin root */
      if(!best){
        links.forEach(function(a){
          if(a.getAttribute('data-path') === '/admin' && path === '/admin'){ best = a; }
        });
      }
      if(best) best.classList.add('active');
    })();
  </script>
</body>
</html>`

func RegisterDashboard(mux httpx.Router, svc *auth.Service, deps DashboardDependencies) {
	d := &dashboard{svc: svc, deps: deps, tmpl: template.Must(template.New("dashboard").Funcs(template.FuncMap{"containsString": containsString, "navIcon": navIcon}).Parse(dashboardTemplate))}
	mux.HandleFunc("/admin", d.overview)
	mux.HandleFunc("/admin/login", d.login)
	mux.HandleFunc("/admin/me", d.me)
	mux.HandleFunc("/admin/logout", d.logout)
	mux.HandleFunc("/admin/users", d.users)
	mux.HandleFunc("/admin/users/{id}", d.userDetail)
	mux.HandleFunc("/admin/users/create", d.createUser)
	mux.HandleFunc("/admin/users/{id}/update", d.updateUser)
	mux.HandleFunc("/admin/users/{id}/retire", d.retireUser)
	mux.HandleFunc("/admin/roles", d.roles)
	mux.HandleFunc("/admin/roles/{id}", d.roleDetail)
	mux.HandleFunc("/admin/roles/create", d.createRole)
	mux.HandleFunc("/admin/roles/{id}/update", d.updateRole)
	mux.HandleFunc("/admin/roles/{id}/retire", d.retireRole)
	mux.HandleFunc("/admin/groups", d.groups)
	mux.HandleFunc("/admin/groups/{id}", d.groupDetail)
	mux.HandleFunc("/admin/groups/create", d.createGroup)
	mux.HandleFunc("/admin/groups/{id}/update", d.updateGroup)
	mux.HandleFunc("/admin/groups/{id}/retire", d.retireGroup)
	mux.HandleFunc("/admin/policies", d.policies)
	mux.HandleFunc("/admin/policies/{id}", d.policyDetail)
	mux.HandleFunc("/admin/policies/create", d.createPolicy)
	mux.HandleFunc("/admin/policies/{id}/update", d.updatePolicy)
	mux.HandleFunc("/admin/policies/{id}/retire", d.retirePolicy)
	mux.HandleFunc("/admin/policies/{id}/apps/{appId}/toggle", d.togglePolicyApp)
	mux.HandleFunc("/admin/policies/{id}/certificates/{certificateId}/toggle", d.togglePolicyCertificate)
	mux.HandleFunc("/admin/policies/{id}/managed-files/{managedFileId}/toggle", d.togglePolicyManagedFile)
	mux.HandleFunc("/admin/devices", d.devices)
	mux.HandleFunc("/admin/devices/{id}", d.deviceDetail)
	mux.HandleFunc("/admin/devices/create", d.createDevice)
	mux.HandleFunc("/admin/devices/{id}/update", d.updateDevice)
	mux.HandleFunc("/admin/devices/{id}/retire", d.retireDevice)
	mux.HandleFunc("/admin/devices/{id}/enrollment/qr", d.deviceEnrollmentQR)
	mux.HandleFunc("/admin/apps", d.apps)
	mux.HandleFunc("/admin/apps/create", d.createApp)
	mux.HandleFunc("/admin/apps/{id}", d.appDetail)
	mux.HandleFunc("/admin/apps/{id}/download", d.downloadApp)
	mux.HandleFunc("/admin/apps/{id}/update", d.updateApp)
	mux.HandleFunc("/admin/apps/{id}/retire", d.retireApp)
	mux.HandleFunc("/admin/apps/{id}/versions/create", d.createAppVersion)
	mux.HandleFunc("/admin/managed-files", d.managedFiles)
	mux.HandleFunc("/admin/managed-files/{id}", d.managedFileDetail)
	mux.HandleFunc("/admin/managed-files/{id}/download", d.downloadManagedFile)
	mux.HandleFunc("/admin/managed-files/create", d.createManagedFile)
	mux.HandleFunc("/admin/managed-files/{id}/retire", d.retireManagedFile)
	mux.HandleFunc("/admin/certificates", d.certificates)
	mux.HandleFunc("/admin/certificates/{id}", d.certificateDetail)
	mux.HandleFunc("/admin/certificates/{id}/download", d.downloadCertificate)
	mux.HandleFunc("/admin/certificates/create", d.createCertificate)
	mux.HandleFunc("/admin/certificates/{id}/retire", d.retireCertificate)
	mux.HandleFunc("/admin/commands", d.commands)
	mux.HandleFunc("/admin/commands/{id}", d.commandDetail)
	mux.HandleFunc("/admin/commands/create", d.createCommand)
	mux.HandleFunc("/admin/audit", d.audit)
	if d.deps.PluginManager != nil {
		d.deps.PluginManager.Register(httpx.WithPrefix(mux, "/admin"), d.svc)
	}
}

func (d *dashboard) requireRead(w http.ResponseWriter, r *http.Request, title string) (*auth.Session, bool) {
	session, ok := sessionFromRequest(r, d.svc)
	if !ok {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return nil, false
	}
	if !auth.HasPermission(session.Permissions, auth.PermissionAdminRead) {
		d.renderForSession(w, r, session, pageData{Title: title, Error: "forbidden"})
		return nil, false
	}
	return session, true
}

func (d *dashboard) requirePostWrite(w http.ResponseWriter, r *http.Request, title string) bool {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return false
	}
	session, ok := sessionFromRequest(r, d.svc)
	if !ok {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return false
	}
	if !auth.HasPermission(session.Permissions, auth.PermissionAdminWrite) {
		d.renderForSession(w, r, session, pageData{Title: title, Error: "forbidden"})
		return false
	}
	if !csrfTokenMatches(r) {
		d.renderForSession(w, r, session, pageData{Title: title, Error: "forbidden"})
		return false
	}
	return true
}

func (d *dashboard) canWrite(session *auth.Session) bool {
	return auth.HasPermission(session.Permissions, auth.PermissionAdminWrite)
}

func (d *dashboard) csrfToken(r *http.Request) string {
	if cookie, err := r.Cookie(csrfCookieName); err == nil {
		return cookie.Value
	}
	return ""
}

func (d *dashboard) renderForSession(w http.ResponseWriter, r *http.Request, session *auth.Session, data pageData) {
	data.User = session.Username
	data.CSRFToken = issueCSRFCookie(w, r)
	data.CanWrite = d.canWrite(session)
	data.Flash = firstNonEmpty(data.Flash, r.URL.Query().Get("ok"))
	data.Error = firstNonEmpty(data.Error, r.URL.Query().Get("error"))
	if strings.Contains(data.Title, "Detail") {
		data.FormsAfterItems = true
		data.ItemsRaw = true
	}
	d.render(w, data)
}

func (d *dashboard) render(w http.ResponseWriter, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := d.tmpl.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (d *dashboard) renderError(w http.ResponseWriter, status int, title, msg string) {
	w.WriteHeader(status)
	d.render(w, pageData{Title: title, Error: msg})
}

func (d *dashboard) renderPageError(w http.ResponseWriter, r *http.Request, session *auth.Session, title string, err error) {
	d.renderForSession(w, r, session, pageData{Title: title, Error: safeError(err)})
}

func (d *dashboard) redirectOK(w http.ResponseWriter, r *http.Request, path, msg string) {
	http.Redirect(w, r, path+"?ok="+urlQuery(msg), http.StatusSeeOther)
}

func (d *dashboard) redirectError(w http.ResponseWriter, r *http.Request, path, msg string) {
	http.Redirect(w, r, path+"?error="+urlQuery(safeErrorText(msg)), http.StatusSeeOther)
}

func (d *dashboard) recordAudit(r *http.Request, action, resourceType, resourceID string, details map[string]any) {
	if d.deps.Audit == nil {
		return
	}
	session, ok := sessionFromRequest(r, d.svc)
	if !ok {
		return
	}
	_, _ = d.deps.Audit.Record(r.Context(), d.deps.TenantID, session.Username, action, resourceType, resourceID, details)
}

func panelSectionHTML(class, title string, body template.HTML) template.HTML {
	var b strings.Builder
	b.WriteString(`<section class="panel`)
	if strings.TrimSpace(class) != "" {
		b.WriteByte(' ')
		b.WriteString(escAttr(class))
	}
	b.WriteString(`">`)
	if strings.TrimSpace(title) != "" {
		b.WriteString(`<h2>`)
		b.WriteString(esc(title))
		b.WriteString(`</h2>`)
	}
	b.WriteString(string(body))
	b.WriteString(`</section>`)
	return template.HTML(b.String())
}

func panelMessageHTML(title, message string) template.HTML {
	return panelSectionHTML("", title, template.HTML(`<p class="muted">`+esc(message)+`</p>`))
}

func detailsPanelHTML(title string, body template.HTML) template.HTML {
	return panelSectionHTML("", title, body)
}

func tableHTML(class string, headers []string, rows string) template.HTML {
	var b strings.Builder
	b.WriteString(`<table`)
	if strings.TrimSpace(class) != "" {
		b.WriteString(` class="`)
		b.WriteString(escAttr(class))
		b.WriteString(`"`)
	}
	b.WriteString(`><thead><tr>`)
	for _, header := range headers {
		b.WriteString(`<th>`)
		b.WriteString(esc(header))
		b.WriteString(`</th>`)
	}
	b.WriteString(`</tr></thead><tbody>`)
	b.WriteString(rows)
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func tableRowHTML(cells ...template.HTML) string {
	var b strings.Builder
	b.WriteString(`<tr>`)
	for _, cell := range cells {
		b.WriteString(`<td>`)
		b.WriteString(string(cell))
		b.WriteString(`</td>`)
	}
	b.WriteString(`</tr>`)
	return b.String()
}

func queryLimitForKey(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return fallback
	}
	return limit
}

func queryPageForKey(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	page, err := strconv.Atoi(raw)
	if err != nil || page <= 0 {
		return fallback
	}
	return page
}

func listPaginationParams(r *http.Request, fallbackLimit int) (int, pagination.Params) {
	return listPaginationParamsForKeys(r, "page", "limit", fallbackLimit)
}

func listPaginationParamsForKeys(r *http.Request, pageKey, limitKey string, fallbackLimit int) (int, pagination.Params) {
	limit := queryLimitForKey(r, limitKey, fallbackLimit)
	page := queryPageForKey(r, pageKey, 1)
	limit = pagination.Normalize(pagination.Params{Limit: limit}, fallbackLimit, paginationLimitCap(limitKey)).Limit
	return page, pagination.Params{Limit: limit + 1, Offset: pagination.Offset(page, limit)}
}

func paginationLimitCap(limitKey string) int {
	switch limitKey {
	case "logsLimit", "deviceInfoLimit":
		return 500
	default:
		return 100
	}
}

func pagerHTML(r *http.Request, page, limit int, hasNext bool) template.HTML {
	return pagerHTMLForKeys(r, "page", "limit", page, limit, hasNext)
}

func pagerHTMLForKeys(r *http.Request, pageKey, limitKey string, page, limit int, hasNext bool) template.HTML {
	if page <= 1 && !hasNext {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<nav class="pager">`)
	if page > 1 {
		prevQuery := cloneQuery(r.URL.Query())
		if page-1 <= 1 {
			prevQuery.Del(pageKey)
		} else {
			prevQuery.Set(pageKey, strconv.Itoa(page-1))
		}
		if limit > 0 {
			prevQuery.Set(limitKey, strconv.Itoa(limit))
		}
		prevURL := *r.URL
		prevURL.RawQuery = prevQuery.Encode()
		fmt.Fprintf(&b, `<a href="%s">Previous</a>`, escAttr(prevURL.String()))
	} else {
		b.WriteString(`<span class="pager-disabled">Previous</span>`)
	}
	fmt.Fprintf(&b, `<span>Page %d</span>`, page)
	if hasNext {
		nextQuery := cloneQuery(r.URL.Query())
		nextQuery.Set(pageKey, strconv.Itoa(page+1))
		if limit > 0 {
			nextQuery.Set(limitKey, strconv.Itoa(limit))
		}
		nextURL := *r.URL
		nextURL.RawQuery = nextQuery.Encode()
		fmt.Fprintf(&b, `<a href="%s">Next</a>`, escAttr(nextURL.String()))
	} else {
		b.WriteString(`<span class="pager-disabled">Next</span>`)
	}
	b.WriteString(`</nav>`)
	return template.HTML(b.String())
}

func withPager(content template.HTML, pager template.HTML) template.HTML {
	if strings.TrimSpace(string(pager)) == "" {
		return content
	}
	return template.HTML(string(content) + string(pager))
}

func paginateItems[T any](items []T, limit int) ([]T, bool) {
	if limit <= 0 {
		return items, false
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	return items, hasNext
}

func cloneQuery(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for key, items := range values {
		out[key] = append([]string(nil), items...)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func hasFormField(r *http.Request, name string) bool {
	_, ok := r.Form[name]
	return ok
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	return safeErrorText(err.Error())
}

func safeErrorText(msg string) string {
	switch msg {
	case "invalid input", "invalid form", "invalid json", "not found", "conflict", "unauthorized", "forbidden":
		return msg
	default:
		if strings.TrimSpace(msg) == "" {
			return "operation failed"
		}
		return msg
	}
}

func urlQuery(value string) string {
	return url.QueryEscape(value)
}

func pre(value any) string {
	return string(structuredDataWithRaw(value))
}

func preStructuredOnly(value any) string {
	return string(structuredData(value))
}

func structuredDataWithRaw(value any) template.HTML {
	structured := string(structuredData(value))
	return template.HTML(structured + string(rawDataDetails("Raw data", value)))
}

func rawDataDetails(label string, value any) template.HTML {
	data, ok := rawDataJSON(value)
	if !ok {
		return ""
	}
	if strings.TrimSpace(label) == "" {
		label = "Raw data"
	}
	return template.HTML(`<details class="raw-data"><summary><span class="raw-data-header"><span>` + esc(label) + `</span></span></summary><button type="button" class="button raw-copy" onclick="copyRawData(this)">Copy</button><pre class="qr-json">` + template.HTMLEscapeString(data) + `</pre></details>`)
}

func rawDataJSON(value any) (string, bool) {
	if raw, ok := value.(json.RawMessage); ok {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			return "", false
		}
		var decoded any
		if err := json.Unmarshal(trimmed, &decoded); err == nil {
			data, err := json.MarshalIndent(decoded, "", "  ")
			if err == nil && len(data) > 0 && string(data) != "null" {
				return string(data), true
			}
		}
		return string(trimmed), true
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil || len(data) == 0 || string(data) == "null" {
		return "", false
	}
	return string(data), true
}

func structuredData(value any) template.HTML {
	var b strings.Builder
	b.WriteString(`<div class="structured-data">`)
	b.WriteString(string(renderStructuredValue(reflect.ValueOf(value), 0)))
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

func renderStructuredValue(v reflect.Value, depth int) template.HTML {
	if !v.IsValid() {
		return template.HTML(`<span class="structured-muted">—</span>`)
	}
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return template.HTML(`<span class="structured-muted">—</span>`)
		}
		v = v.Elem()
	}
	if t, ok := v.Interface().(time.Time); ok {
		return template.HTML(`<span class="structured-scalar">` + esc(formatDashboardTime(t)) + `</span>`)
	}
	if raw, ok := v.Interface().(json.RawMessage); ok {
		return renderStructuredJSON(raw)
	}
	switch v.Kind() {
	case reflect.Struct:
		return renderStructuredStruct(v, depth)
	case reflect.Map:
		return renderStructuredMap(v, depth)
	case reflect.Slice, reflect.Array:
		return renderStructuredSlice(v, depth)
	case reflect.Bool:
		return template.HTML(boolBadge(v.Bool(), "true", "false"))
	case reflect.String:
		text := v.String()
		if strings.TrimSpace(text) == "" {
			return template.HTML(`<span class="structured-muted">—</span>`)
		}
		return template.HTML(`<span class="structured-scalar">` + esc(text) + `</span>`)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return template.HTML(`<span class="structured-scalar">` + strconv.FormatInt(v.Int(), 10) + `</span>`)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return template.HTML(`<span class="structured-scalar">` + strconv.FormatUint(v.Uint(), 10) + `</span>`)
	case reflect.Float32, reflect.Float64:
		return template.HTML(`<span class="structured-scalar">` + strconv.FormatFloat(v.Float(), 'f', -1, 64) + `</span>`)
	default:
		data, _ := json.MarshalIndent(v.Interface(), "", "  ")
		if len(data) == 0 || string(data) == "null" {
			return template.HTML(`<span class="structured-muted">—</span>`)
		}
		return template.HTML(`<code>` + template.HTMLEscapeString(string(data)) + `</code>`)
	}
}

func renderStructuredJSON(raw json.RawMessage) template.HTML {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return template.HTML(`<span class="structured-muted">—</span>`)
	}
	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return template.HTML(`<code>` + template.HTMLEscapeString(string(trimmed)) + `</code>`)
	}
	return renderStructuredValue(reflect.ValueOf(decoded), 0)
}

func renderStructuredStruct(v reflect.Value, depth int) template.HTML {
	rows := structuredStructRows(v, depth, map[string]bool{})
	if len(rows) == 0 {
		return template.HTML(`<div class="structured-empty">No details available.</div>`)
	}
	return template.HTML(`<table class="structured-table"><tbody>` + strings.Join(rows, "") + `</tbody></table>`)
}

func structuredStructRows(v reflect.Value, depth int, seen map[string]bool) []string {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()
	rows := make([]string, 0, v.NumField())
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		fv := v.Field(i)
		if isZeroValue(fv) {
			continue
		}
		if shouldFlattenStructuredField(field, fv) {
			rows = append(rows, structuredStructRows(fv, depth, seen)...)
			continue
		}
		label := fieldLabel(field.Name)
		if seen[label] {
			continue
		}
		seen[label] = true
		rows = append(rows, `<tr><th class="structured-key">`+esc(label)+`</th><td>`+string(renderStructuredValue(fv, depth+1))+`</td></tr>`)
	}
	return rows
}

func shouldFlattenStructuredField(field reflect.StructField, v reflect.Value) bool {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(field.Name))
	return field.Anonymous || name == "recordbase" || name == "record" || name == "base" || name == "model"
}

func renderStructuredMap(v reflect.Value, depth int) template.HTML {
	if v.IsNil() || v.Len() == 0 {
		return template.HTML(`<div class="structured-empty">No values.</div>`)
	}
	keys := v.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface()) })
	rows := make([]string, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, `<tr><th class="structured-key">`+esc(fmt.Sprint(key.Interface()))+`</th><td>`+string(renderStructuredValue(v.MapIndex(key), depth+1))+`</td></tr>`)
	}
	return template.HTML(`<table class="structured-table"><tbody>` + strings.Join(rows, "") + `</tbody></table>`)
}

func renderStructuredSlice(v reflect.Value, depth int) template.HTML {
	if v.Len() == 0 {
		return template.HTML(`<div class="structured-empty">No records found.</div>`)
	}
	if isScalarKind(indirectKind(v.Index(0))) {
		items := make([]string, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			items = append(items, `<span class="structured-pill">`+strings.TrimPrefix(strings.TrimSuffix(string(renderStructuredValue(v.Index(i), depth+1)), `</span>`), `<span class="structured-scalar">`)+`</span>`)
		}
		return template.HTML(`<div class="structured-list">` + strings.Join(items, "") + `</div>`)
	}
	var b strings.Builder
	b.WriteString(`<table class="structured-table"><thead><tr><th class="structured-key">#</th><th>Record</th></tr></thead><tbody>`)
	for i := 0; i < v.Len(); i++ {
		fmt.Fprintf(&b, `<tr><td>%d</td><td>%s</td></tr>`, i+1, string(renderStructuredValue(v.Index(i), depth+1)))
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func isZeroValue(v reflect.Value) bool {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return true
		}
		v = v.Elem()
	}
	return v.IsZero()
}

func indirectValue(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

func indirectKind(v reflect.Value) reflect.Kind {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Invalid
		}
		v = v.Elem()
	}
	return v.Kind()
}

func isScalarKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func fieldLabel(name string) string {
	if strings.TrimSpace(name) == "" {
		return "Field"
	}
	var words []string
	start := 0
	runes := []rune(name)
	for i := 1; i < len(runes); i++ {
		if runes[i] >= 'A' && runes[i] <= 'Z' && (runes[i-1] < 'A' || runes[i-1] > 'Z') {
			words = append(words, string(runes[start:i]))
			start = i
		}
	}
	words = append(words, string(runes[start:]))
	return strings.Join(words, " ")
}

func formatDashboardTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("Jan 2, 2006 15:04 MST")
}

func boolBadge(enabled bool, enabledLabel, disabledLabel string) string {
	class := "status-pill status-disabled"
	label := disabledLabel
	if enabled {
		class = "status-pill status-enabled"
		label = enabledLabel
	}
	return `<span class="` + class + `">` + esc(label) + `</span>`
}

func dashboardPermissions(values []string) []auth.Permission {
	perms := make([]auth.Permission, 0, len(values))
	for _, value := range values {
		perms = append(perms, auth.Permission(value))
	}
	return perms
}

func allPermissionOptions(perms []auth.Permission) []optionData {
	options := make([]optionData, 0, len(perms))
	for _, perm := range perms {
		options = append(options, optionData{Value: string(perm), Label: string(perm)})
	}
	return options
}

func navIcon(name string) template.HTML {
	var svg string
	switch name {
	case "overview":
		svg = `<svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><path d="M2 7.5 8 2l6 5.5"/><path d="M4 7.5V14h8V7.5"/><path d="M6.5 14V9.5h3V14"/></svg>`
	case "devices":
		svg = `<svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><rect x="4" y="1.5" width="8" height="13" rx="1.5"/><path d="M6 4.5h4"/><path d="M7.25 12.5h1.5"/></svg>`
	case "policies":
		svg = `<svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><path d="M3.5 2.5h6.25l2.75 2.75V13.5a1 1 0 0 1-1 1h-8a1 1 0 0 1-1-1v-10a1 1 0 0 1 1-1z"/><path d="M9.75 2.5v3h3"/><path d="M5 8h6"/><path d="M5 10.5h6"/></svg>`
	case "groups":
		svg = `<svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><circle cx="5" cy="5" r="2"/><circle cx="11.5" cy="6" r="1.75"/><path d="M2.75 13a4 4 0 0 1 4-4h1.5a4 4 0 0 1 4 4"/><path d="M9.5 13a3.25 3.25 0 0 1 3.25-3.25H14"/><path d="M1.75 10.5A2.5 2.5 0 0 1 4.25 8h.5"/></svg>`
	case "commands":
		svg = `<svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><path d="M8.5 1.75 3.5 8h3.25L6 14.25 12.5 7H9.25z"/></svg>`
	case "apps":
		svg = `<svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><rect x="2" y="2" width="5" height="5" rx="1"/><rect x="9" y="2" width="5" height="5" rx="1"/><rect x="2" y="9" width="5" height="5" rx="1"/><rect x="9" y="9" width="5" height="5" rx="1"/></svg>`
	case "managed-files":
		svg = `<svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><path d="M4 1.75h5l3 3V14.25a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-11.5a1 1 0 0 1 1-1z"/><path d="M9 1.75v3h3"/><path d="M5.5 8h5"/><path d="M5.5 10.5h5"/></svg>`
	case "certificates":
		svg = `<svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><path d="M4 2.5h8v7.25a4 4 0 0 1-4 4 4 4 0 0 1-4-4z"/><path d="M6.5 13.75 5.25 15l-.5-2.5 1.75-1"/><path d="M7.25 7.25a.75.75 0 1 1 1.5 0v1h-1.5z"/><path d="M8 8.25v1"/></svg>`
	case "users":
		svg = `<svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><circle cx="8" cy="5" r="2.25"/><path d="M3.5 13a4.5 4.5 0 0 1 9 0"/></svg>`
	case "roles":
		svg = `<svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><path d="M8 1.75 13 4v4c0 3.25-2.1 5.7-5 6.5-2.9-.8-5-3.25-5-6.5V4z"/><path d="M8 5.5v3"/><path d="M8 10.25h.01"/></svg>`
	case "audit":
		svg = `<svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><rect x="2.75" y="2" width="10.5" height="12" rx="1.5"/><path d="M5 4.75h6"/><path d="M5 7.5h6"/><path d="M5 10.25h3.5"/></svg>`
	default:
		svg = `<svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><circle cx="8" cy="8" r="6"/></svg>`
	}
	return template.HTML(svg)
}

func statusBadge(status string) string {
	class := "status-pill"
	switch status {
	case "pending":
		class += " status-pending"
	case "active":
		class += " status-active"
	case "enrolled":
		class += " status-enrolled"
	case "issued":
		class += " status-issued"
	case "consumed":
		class += " status-consumed"
	case "expired":
		class += " status-expired"
	case "revoked":
		class += " status-retired"
	case "retired":
		class += " status-retired"
	}
	return `<span class="` + class + `">` + esc(status) + `</span>`
}

func esc(v string) string {
	return template.HTMLEscapeString(v)
}

func escAttr(v string) string {
	return template.HTMLEscapeString(v)
}
