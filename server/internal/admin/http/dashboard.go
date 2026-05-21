package adminhttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	apps "xmdm/server/internal/apps"
	"xmdm/server/internal/artifacts"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/certificates"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/commands"
	"xmdm/server/internal/device"
	"xmdm/server/internal/deviceinfo"
	"xmdm/server/internal/enrollment"
	enrollmenthttp "xmdm/server/internal/enrollment/http"
	"xmdm/server/internal/files"
	"xmdm/server/internal/group"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/identity"
	"xmdm/server/internal/logs"
	managedfiles "xmdm/server/internal/managedfiles"
	"xmdm/server/internal/policy"

	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"
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
    .audit-table .audit-created { width: 10.5rem; }
    .audit-table .audit-actor { width: 7rem; max-width: 7rem; }
    .audit-table .audit-action { width: 7.5rem; }
    .audit-table .audit-resource { width: 13rem; }
    .audit-table .audit-details { width: auto; }
    .audit-table td.audit-actor,
    .audit-table td.audit-action,
    .audit-table td.audit-resource {
      overflow-wrap: anywhere;
    }
    .audit-table td.audit-details {
      min-width: 22rem;
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
	mux.HandleFunc("/admin/logs", d.logs)
	mux.HandleFunc("/admin/audit", d.audit)
}

func (d *dashboard) login(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		token := issueCSRFCookie(w, r)
		d.render(w, pageData{
			Title:     "Login",
			CSRFToken: token,
			Forms: []formData{{
				Title:  "Enter the control plane",
				Action: "/admin/login",
				Fields: []fieldData{
					{Name: "username", Label: "Username", Type: "text", Required: true},
					{Name: "password", Label: "Password", Type: "password", Required: true},
				},
				Submit: "Login",
			}},
		})
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			d.renderError(w, http.StatusBadRequest, "Login", "invalid form")
			return
		}
		if hasCSRFCookie(r) && !csrfTokenMatches(r) {
			d.renderError(w, http.StatusForbidden, "Login", "forbidden")
			return
		}
		session, err := d.svc.Login(r.FormValue("username"), r.FormValue("password"))
		if err == nil {
			http.SetCookie(w, &http.Cookie{Name: auth.SessionCookieName, Value: session.ID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: session.ExpiresAt})
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		if !errors.Is(err, auth.ErrInvalidCredentials) || d.deps.Identity == nil {
			d.renderError(w, http.StatusUnauthorized, "Login", "invalid credentials")
			return
		}
		user, role, userErr := d.deps.Identity.AuthenticateUser(r.Context(), d.deps.TenantID, strings.TrimSpace(r.FormValue("username")), r.FormValue("password"))
		if userErr != nil {
			d.renderError(w, http.StatusUnauthorized, "Login", "invalid credentials")
			return
		}
		session = d.svc.IssueSession(user.Email, dashboardPermissions(role.Permissions))
		http.SetCookie(w, &http.Cookie{Name: auth.SessionCookieName, Value: session.ID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: session.ExpiresAt})
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (d *dashboard) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if hasCSRFCookie(r) && !csrfTokenMatches(r) {
		d.renderError(w, http.StatusForbidden, "Logout", "forbidden")
		return
	}
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil {
		d.svc.Logout(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: auth.SessionCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
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

	allDevices := []device.Device{}
	totalDevices := 0
	activeDevices := 0
	pendingDevices := 0
	inactiveDevices := 0
	retiredOrWipedDevices := 0
	staleActiveDevices := 0
	assignedPolicyDevices := 0

	if d.deps.Devices != nil {
		items, err := d.deps.Devices.ListDevices(ctx, d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		allDevices = items
		totalDevices = len(items)
		for _, item := range items {
			if item.Status == device.StatusActive {
				activeDevices++
			} else {
				inactiveDevices++
			}
			if item.Status == device.StatusPending {
				pendingDevices++
			}
			status := strings.ToLower(strings.TrimSpace(item.Status))
			if status == "retired" || status == "wiped" {
				retiredOrWipedDevices++
			}
			if firstPolicyID(item.PolicyID) != "" {
				assignedPolicyDevices++
			}
		}
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
		items, err := d.deps.Policies.ListPolicies(ctx, d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		totalPolicies = len(items)
		for _, item := range items {
			switch item.Status {
			case policy.StatusActive:
				activePolicies++
			case policy.StatusRetired:
				retiredPolicies++
			}
		}
		tone := "neutral"
		if activePolicies > 0 {
			tone = "good"
		} else if totalPolicies > 0 {
			tone = "warn"
		}
		appendSignal("Policy library", strconv.Itoa(activePolicies), fmt.Sprintf("%d active, %d retired", activePolicies, retiredPolicies), tone, "/admin/policies")
	}

	if d.deps.Apps != nil {
		items, err := d.deps.Apps.ListApps(ctx, d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		for _, item := range items {
			if item.Status == apps.StatusActive {
				ov.ContentStats.ActiveApps++
			}
		}
	}

	if d.deps.ManagedFiles != nil {
		items, err := d.deps.ManagedFiles.ListManagedFiles(ctx, d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		for _, item := range items {
			if item.Status == managedfiles.StatusActive {
				ov.ContentStats.ActiveFiles++
			}
		}
	}

	if d.deps.Certificates != nil {
		items, err := d.deps.Certificates.ListCertificates(ctx, d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		for _, item := range items {
			if item.Status == certificates.StatusActive {
				ov.ContentStats.ActiveCerts++
			}
		}
	}

	totalContent := ov.ContentStats.ActiveApps + ov.ContentStats.ActiveFiles + ov.ContentStats.ActiveCerts
	contentTone := "neutral"
	if totalContent > 0 {
		contentTone = "good"
	}
	appendSignal("Content readiness", strconv.Itoa(totalContent), fmt.Sprintf("%d apps, %d files, %d certs", ov.ContentStats.ActiveApps, ov.ContentStats.ActiveFiles, ov.ContentStats.ActiveCerts), contentTone, "/admin/apps")

	if d.deps.Commands != nil {
		items, err := d.deps.Commands.ListRecent(ctx, d.deps.TenantID, 50)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		for _, item := range items {
			ov.CommandStats.Total++
			switch item.Status {
			case commands.StatusSent:
				ov.CommandStats.Sent++
			case commands.StatusAcked:
				ov.CommandStats.Acked++
			case commands.StatusFailed:
				ov.CommandStats.Failed++
			}
		}
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
		items, err := d.deps.DeviceInfo.Search(ctx, d.deps.TenantID, deviceinfo.SearchFilter{Limit: 200})
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		ov.DeviceActivityChart = buildUniqueDeviceActivityChart(items, now, 7)
		ov.DeviceTelemetryChart = buildDeviceTelemetryFreshnessChart(allDevices, items, now)
		staleActiveDevices = countStaleActiveDevices(allDevices, items, now, 72*time.Hour)
		ov.DeviceModelChart = buildDeviceModelChart(allDevices, items, 6)
	}

	var auditEvents []audit.Event
	auditLast24h := 0
	if d.deps.Audit != nil {
		items, err := d.deps.Audit.List(ctx, d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Overview", err)
			return
		}
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		})
		auditEvents = items
		for _, item := range items {
			if item.CreatedAt.After(now.Add(-24 * time.Hour)) {
				auditLast24h++
			}
		}
		if len(items) > 5 {
			ov.RecentActivity = items[:5]
		} else {
			ov.RecentActivity = items
		}
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
	ov.DeviceStatusChart = buildDeviceStatusChart(allDevices)
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
	b.WriteString(`<div class="overview-freshness">` + esc(data.Freshness) + `</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="overview-actions">`)
	b.WriteString(`<a class="button btn-primary" href="/admin/devices">Manage devices</a>`)
	b.WriteString(`<a class="button" href="/admin/policies">Review policies</a>`)
	b.WriteString(`<a class="button" href="/admin/audit">View audit log</a>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	statusTone := signalToneClass(data.SummaryTone)
	b.WriteString(`<div class="overview-status ` + statusTone + `">`)
	b.WriteString(`<span class="overview-status-dot"></span>`)
	b.WriteString(`<div><div class="overview-status-title">` + esc(data.SummaryTitle) + `</div>`)
	b.WriteString(`<div class="overview-status-detail">` + esc(data.SummaryDetail) + `</div></div>`)
	b.WriteString(`</div>`)

	if len(data.Signals) > 0 {
		b.WriteString(`<div class="health-strip">`)
		for _, signal := range data.Signals {
			tone := signalToneClass(signal.Tone)
			if signal.Href != "" {
				b.WriteString(`<a class="health-item-wrap ` + tone + `" href="` + escAttr(signal.Href) + `">`)
			} else {
				b.WriteString(`<div class="health-item-wrap ` + tone + `">`)
			}
			b.WriteString(`<div class="health-item">`)
			b.WriteString(`<div class="health-row"><span class="health-dot"></span><div class="health-label">` + esc(signal.Label) + `</div>`)
			if signal.Href != "" {
				b.WriteString(`<span class="health-nav-arrow">&rarr;</span>`)
			}
			b.WriteString(`</div>`)
			b.WriteString(`<div class="health-value">` + esc(signal.Value) + `</div>`)
			b.WriteString(`<div class="health-detail">` + esc(signal.Detail) + `</div>`)
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
			b.WriteString(`<div class="overview-metric-label">` + esc(metric.Label) + `</div>`)
			b.WriteString(`<div class="overview-metric-value">` + esc(metric.Value) + `</div>`)
			b.WriteString(`<div class="overview-metric-detail">` + esc(metric.Detail) + `</div>`)
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
	b.WriteString(`<span class="overview-panel-meta">` + strconv.Itoa(totalContent) + ` active items</span>`)
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
	b.WriteString(`<div class="overview-panel-header"><span class="overview-panel-title">` + esc(chart.Title) + `</span>`)
	if href != "" {
		b.WriteString(`<a class="overview-panel-link" href="` + escAttr(href) + `">` + esc(linkLabel) + `</a>`)
	} else if chart.Total > 0 {
		b.WriteString(`<span class="chart-total-badge">` + strconv.Itoa(chart.Total) + ` total</span>`)
	} else {
		b.WriteString(`<span class="overview-panel-meta">` + esc(meta) + `</span>`)
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

func segmentWidthPercent(count, total int) int {
	if total <= 0 {
		return 0
	}
	return count * 100 / total
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
			b.WriteString(`<a class="attention-item ` + tone + `" href="` + escAttr(item.Href) + `">`)
		} else {
			b.WriteString(`<div class="attention-item ` + tone + `">`)
		}
		b.WriteString(`<span class="attention-dot"></span>`)
		b.WriteString(`<div><div class="attention-title">` + esc(item.Title) + `</div><div class="attention-detail">` + esc(item.Detail) + `</div></div>`)
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
		b.WriteString(`<div class="cmd-row-label">` + esc(entry.label) + `</div>`)
		b.WriteString(`<div class="cmd-track"><div class="cmd-fill ` + entry.cls + `" style="width:` + strconv.Itoa(pct) + `%"></div></div>`)
		b.WriteString(`<div class="cmd-row-count">` + strconv.Itoa(entry.count) + `</div>`)
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
	b.WriteString(`<div class="cmd-stat"><div class="cmd-stat-value ` + rateCls + `">` + strconv.Itoa(successRate) + `%</div><div class="cmd-stat-label">ack rate</div></div>`)
	b.WriteString(`<div class="cmd-stat"><div class="cmd-stat-value">` + strconv.Itoa(stats.Total) + `</div><div class="cmd-stat-label">total</div></div>`)
	b.WriteString(`<div class="cmd-stat"><div class="cmd-stat-value cmd-rate-danger">` + strconv.Itoa(stats.Failed) + `</div><div class="cmd-stat-label">failed</div></div>`)
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
		b.WriteString(`<div class="comp-segment ` + seg.cls + `" style="width:` + strconv.Itoa(segmentWidthPercent(seg.count, total)) + `%" title="` + esc(seg.label) + `: ` + strconv.Itoa(seg.count) + `"></div>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<div class="comp-legend">`)
	for _, seg := range segs {
		b.WriteString(`<a class="comp-legend-item" href="` + escAttr(seg.href) + `">`)
		b.WriteString(`<span class="comp-dot ` + seg.cls + `"></span>`)
		b.WriteString(`<span class="comp-name">` + esc(seg.label) + `</span>`)
		b.WriteString(`<span class="comp-count">` + strconv.Itoa(seg.count) + `</span>`)
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
		b.WriteString(`<span class="comp-pct">` + esc(detail) + `</span>`)
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
		b.WriteString(`<div class="activity-time">` + esc(formatDashboardTime(item.CreatedAt)) + `</div>`)
		b.WriteString(`<div class="activity-main"><div class="activity-action">` + esc(item.Action) + `</div>`)
		b.WriteString(`<div class="activity-meta">` + esc(actor) + ` on ` + esc(resource) + `</div></div>`)
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
	b.WriteString(`<svg viewBox="0 0 720 220" role="img" aria-label="` + esc(chart.Title) + `">`)
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

func buildDeviceStatusChart(items []device.Device) overviewChart {
	chart := overviewChart{Title: "Device status distribution", EmptyNote: "No devices enrolled yet."}
	if len(items) == 0 {
		return chart
	}
	counts := map[string]int{}
	for _, item := range items {
		label := strings.TrimSpace(string(item.Status))
		if label == "" {
			label = "unknown"
		}
		counts[label]++
	}
	return chartFromCounts(chart.Title, counts, 8, chart.EmptyNote)
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

func (d *dashboard) users(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Users")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	items, err := d.deps.Identity.ListUsers(r.Context(), d.deps.TenantID)
	if err != nil {
		d.renderPageError(w, r, session, "Users", err)
		return
	}
	roles, err := d.deps.Identity.ListRoles(r.Context(), d.deps.TenantID)
	if err != nil {
		d.renderPageError(w, r, session, "Users", err)
		return
	}
	data := pageData{
		Title:    "Users",
		Subtitle: "Manage operator accounts and role bindings.",
		Items:    usersTable(items, roleNameByID(roles), d.csrfToken(r), d.canWrite(session)),
	}
	if d.canWrite(session) {
		data.Forms = []formData{{
			Title:  "Create user",
			Action: "/admin/users/create",
			Fields: []fieldData{
				{Name: "email", Label: "Email", Type: "email", Placeholder: "operator@example.com", Required: true},
				{Name: "password", Label: "Password", Type: "password", Placeholder: "password", Required: true},
				{Name: "roleId", Label: "Role", Type: "select", Placeholder: "Select a role", Options: allRoleOptions(roles), Required: true},
			},
			Submit: "Create user",
		}}
	}
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) userDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "User Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	items, err := d.deps.Identity.ListUsers(r.Context(), d.deps.TenantID)
	if err != nil {
		d.renderPageError(w, r, session, "User Detail", err)
		return
	}
	id := r.PathValue("id")
	var found *identity.User
	for i := range items {
		if items[i].ID == id {
			found = &items[i]
			break
		}
	}
	if found == nil {
		d.renderPageError(w, r, session, "User Detail", httpx.ErrNotFound)
		return
	}
	roles, err := d.deps.Identity.ListRoles(r.Context(), d.deps.TenantID)
	if err != nil {
		d.renderPageError(w, r, session, "User Detail", err)
		return
	}
	data := pageData{
		Title:    "User Detail",
		Subtitle: "Edit the operator account or retire it from the active roster.",
		Items:    template.HTML("<section class=\"panel\"><h2>Current user</h2>" + pre(found) + "</section>"),
	}
	if d.canWrite(session) {
		if found.Status == "active" {
			data.Forms = []formData{{
				Title:  "Update user",
				Action: "/admin/users/" + id + "/update",
				Fields: []fieldData{
					{Name: "email", Label: "Email", Type: "email", Value: found.Email, Placeholder: "operator@example.com", Required: true},
					{Name: "password", Label: "Password", Type: "password", Placeholder: "leave blank to keep the current password"},
					{Name: "roleId", Label: "Role", Type: "select", Value: found.RoleID, Placeholder: "Select a role", Options: allRoleOptions(roles), Required: true},
				},
				Submit: "Update user",
			}, {
				Title:  "Retire user",
				Action: "/admin/users/" + id + "/retire",
				Submit: "Retire user",
				Danger: true,
			}}
		}
	}
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) createUser(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Users") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/users", "invalid form")
		return
	}
	passwordHash, err := identity.HashPassword(r.FormValue("password"))
	if err != nil {
		d.redirectError(w, r, "/admin/users", err.Error())
		return
	}
	rec, err := d.deps.Identity.CreateUser(r.Context(), d.deps.TenantID, identity.UserUpsert{Email: strings.TrimSpace(r.FormValue("email")), PasswordHash: passwordHash, RoleID: strings.TrimSpace(r.FormValue("roleId"))})
	if err != nil {
		d.redirectError(w, r, "/admin/users", err.Error())
		return
	}
	d.recordAudit(r, "create", "users", rec.ID, map[string]any{"email": rec.Email, "roleId": rec.RoleID})
	d.redirectOK(w, r, "/admin/users", "user created")
}

func (d *dashboard) updateUser(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Users") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/users/"+r.PathValue("id"), "invalid form")
		return
	}
	passwordHash, err := identity.HashPassword(r.FormValue("password"))
	if err != nil {
		d.redirectError(w, r, "/admin/users/"+r.PathValue("id"), err.Error())
		return
	}
	rec, err := d.deps.Identity.UpdateUser(r.Context(), d.deps.TenantID, r.PathValue("id"), identity.UserUpsert{Email: strings.TrimSpace(r.FormValue("email")), PasswordHash: passwordHash, RoleID: strings.TrimSpace(r.FormValue("roleId"))})
	if err != nil {
		d.redirectError(w, r, "/admin/users/"+r.PathValue("id"), err.Error())
		return
	}
	if rec.Status != "active" {
		d.redirectError(w, r, "/admin/users/"+r.PathValue("id"), "conflict")
		return
	}
	d.recordAudit(r, "update", "users", rec.ID, map[string]any{"email": rec.Email, "roleId": rec.RoleID})
	d.redirectOK(w, r, "/admin/users/"+rec.ID, "user updated")
}

func (d *dashboard) retireUser(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Users") {
		return
	}
	rec, err := d.deps.Identity.RetireUser(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/users/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "retire", "users", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/users", "user retired")
}

func (d *dashboard) roles(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Roles")
	if !ok {
		return
	}
	items, err := d.deps.Identity.ListRoles(r.Context(), d.deps.TenantID)
	if err != nil {
		d.renderPageError(w, r, session, "Roles", err)
		return
	}
	data := pageData{
		Title:    "Roles",
		Subtitle: "Define the permission bundles available to operators.",
		Items:    rolesTable(items, d.csrfToken(r), d.canWrite(session)),
	}
	if d.canWrite(session) {
		data.Forms = []formData{{
			Title: "Create role", Action: "/admin/roles/create",
			Fields: []fieldData{{Name: "name", Label: "Name", Type: "text", Placeholder: "operators", Required: true}, {Name: "permissions", Label: "Permissions JSON array", Type: "textarea", Value: `["admin.read"]`, Placeholder: `["admin.read"]`, Required: true}},
			Help:   permissionsHelp(),
			Submit: "Create role",
		}}
	}
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) roleDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Role Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	items, err := d.deps.Identity.ListRoles(r.Context(), d.deps.TenantID)
	if err != nil {
		d.renderPageError(w, r, session, "Role Detail", err)
		return
	}
	id := r.PathValue("id")
	var found *identity.Role
	for i := range items {
		if items[i].ID == id {
			found = &items[i]
			break
		}
	}
	if found == nil {
		d.renderPageError(w, r, session, "Role Detail", httpx.ErrNotFound)
		return
	}
	perms, _ := json.Marshal(found.Permissions)
	data := pageData{
		Title:    "Role Detail",
		Subtitle: "Edit the permission bundle or retire it from active use.",
		Items:    template.HTML("<section class=\"panel\"><h2>Current role</h2>" + pre(found) + "</section>"),
	}
	if d.canWrite(session) {
		if found.Status == "active" {
			data.Forms = []formData{{
				Title:  "Update role",
				Action: "/admin/roles/" + id + "/update",
				Fields: []fieldData{
					{Name: "name", Label: "Name", Type: "text", Value: found.Name, Placeholder: "operators", Required: true},
					{Name: "permissions", Label: "Permissions JSON array", Type: "textarea", Value: string(perms), Placeholder: `["admin.read"]`, Required: true},
				},
				Help:   permissionsHelp(),
				Submit: "Update role",
			}, {
				Title:  "Retire role",
				Action: "/admin/roles/" + id + "/retire",
				Submit: "Retire role",
				Danger: true,
			}}
		}
	}
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) createRole(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Roles") {
		return
	}
	req, err := roleUpsertFromForm(r)
	if err != nil {
		d.redirectError(w, r, "/admin/roles", err.Error())
		return
	}
	rec, err := d.deps.Identity.CreateRole(r.Context(), d.deps.TenantID, req)
	if err != nil {
		d.redirectError(w, r, "/admin/roles", err.Error())
		return
	}
	d.recordAudit(r, "create", "roles", rec.ID, map[string]any{"name": rec.Name, "permissions": rec.Permissions})
	d.redirectOK(w, r, "/admin/roles", "role created")
}

func (d *dashboard) updateRole(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Roles") {
		return
	}
	req, err := roleUpsertFromForm(r)
	if err != nil {
		d.redirectError(w, r, "/admin/roles/"+r.PathValue("id"), err.Error())
		return
	}
	rec, err := d.deps.Identity.UpdateRole(r.Context(), d.deps.TenantID, r.PathValue("id"), req)
	if err != nil {
		d.redirectError(w, r, "/admin/roles/"+r.PathValue("id"), err.Error())
		return
	}
	if rec.Status != "active" {
		d.redirectError(w, r, "/admin/roles/"+r.PathValue("id"), "conflict")
		return
	}
	d.recordAudit(r, "update", "roles", rec.ID, map[string]any{"name": rec.Name, "permissions": rec.Permissions})
	d.redirectOK(w, r, "/admin/roles/"+rec.ID, "role updated")
}

func (d *dashboard) retireRole(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Roles") {
		return
	}
	rec, err := d.deps.Identity.RetireRole(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/roles/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "retire", "roles", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/roles", "role retired")
}

func (d *dashboard) groups(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Groups")
	if !ok {
		return
	}
	items, err := d.deps.Groups.ListGroups(r.Context(), d.deps.TenantID)
	if err != nil {
		d.renderPageError(w, r, session, "Groups", err)
		return
	}
	d.renderForSession(w, r, session, pageData{
		Title:    "Groups",
		Subtitle: "Define device cohorts and inspect the devices assigned to each cohort.",
		Forms:    []formData{{Title: "Create group", Action: "/admin/groups/create", Fields: []fieldData{{Name: "name", Label: "Name", Type: "text", Placeholder: "Field Devices", Required: true}}, Submit: "Create group"}},
		Items:    groupsTable(items, d.csrfToken(r), d.canWrite(session)),
	})
}

func (d *dashboard) createGroup(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Groups") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/groups", "invalid form")
		return
	}
	rec, err := d.deps.Groups.CreateGroup(r.Context(), d.deps.TenantID, group.GroupUpsert{Name: strings.TrimSpace(r.FormValue("name"))})
	if err != nil {
		d.redirectError(w, r, "/admin/groups", err.Error())
		return
	}
	d.recordAudit(r, "create", "groups", rec.ID, map[string]any{"name": rec.Name})
	d.redirectOK(w, r, "/admin/groups", "group created")
}

func (d *dashboard) groupDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Group Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	found, devices, policies, err := d.loadGroupDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		d.renderPageError(w, r, session, "Group Detail", err)
		return
	}
	data := d.groupDetailPageData(session, found, devices, policies)
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) updateGroup(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Groups") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/groups/"+r.PathValue("id"), "invalid form")
		return
	}
	rec, err := d.deps.Groups.UpdateGroup(r.Context(), d.deps.TenantID, r.PathValue("id"), group.GroupUpsert{Name: strings.TrimSpace(r.FormValue("name"))})
	if err != nil {
		d.redirectError(w, r, "/admin/groups/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "update", "groups", rec.ID, map[string]any{"name": rec.Name})
	d.redirectOK(w, r, "/admin/groups/"+rec.ID, "group updated")
}

func (d *dashboard) retireGroup(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Groups") {
		return
	}
	rec, err := d.deps.Groups.RetireGroup(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/groups", err.Error())
		return
	}
	d.recordAudit(r, "retire", "groups", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/groups", "group retired")
}

func (d *dashboard) loadGroupDetail(ctx context.Context, id string) (*group.Group, []device.Device, []policy.Policy, error) {
	groups, err := d.deps.Groups.ListGroups(ctx, d.deps.TenantID)
	if err != nil {
		return nil, nil, nil, err
	}
	var found *group.Group
	for i := range groups {
		if groups[i].ID == id {
			found = &groups[i]
			break
		}
	}
	if found == nil {
		return nil, nil, nil, httpx.ErrNotFound
	}
	devices := []device.Device{}
	if d.deps.Devices != nil {
		devices, err = d.deps.Devices.ListDevices(ctx, d.deps.TenantID)
		if err != nil {
			return nil, nil, nil, err
		}
		devices = groupDevicesFor(found.ID, devices)
	}
	policies := []policy.Policy{}
	if d.deps.Policies != nil {
		policies, err = d.deps.Policies.ListPolicies(ctx, d.deps.TenantID)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return found, devices, policies, nil
}

func (d *dashboard) groupDetailPageData(session *auth.Session, found *group.Group, devices []device.Device, policies []policy.Policy) pageData {
	body := "<section class=\"panel\"><h2>Current group</h2>" + pre(found) + "</section>"
	if len(devices) > 0 {
		body += "<section class=\"panel\"><h2>Member devices</h2>" + string(devicesTable(devices, policies, "", false)) + "</section>"
	} else {
		body += `<section class="panel"><h2>Member devices</h2><p class="muted">No devices are linked to this group.</p></section>`
	}
	data := pageData{
		Title:    "Group Detail",
		Subtitle: "Review the cohort, then update or retire it from this page.",
		Items:    template.HTML(body),
	}
	if d.canWrite(session) && found.Status != "retired" {
		data.Forms = []formData{{
			Title:  "Update group",
			Action: "/admin/groups/" + found.ID + "/update",
			Fields: []fieldData{{Name: "name", Label: "Name", Type: "text", Value: found.Name, Placeholder: "Field Devices", Required: true}},
			Submit: "Update group",
		}, {
			Title:  "Retire group",
			Action: "/admin/groups/" + found.ID + "/retire",
			Submit: "Retire group",
			Danger: true,
		}}
	}
	return data
}

func (d *dashboard) policies(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Policies")
	if !ok {
		return
	}
	items, err := d.deps.Policies.ListPolicies(r.Context(), d.deps.TenantID)
	if err != nil {
		d.renderPageError(w, r, session, "Policies", err)
		return
	}
	d.renderForSession(w, r, session, pageData{
		Title:    "Policies",
		Subtitle: "Define kiosk behavior and package rules before devices enroll.",
		Forms: []formData{{
			Title:  "Create policy",
			Action: "/admin/policies/create",
			Fields: policyFields(policy.Policy{}),
			Submit: "Create policy",
		}},
		Items: policiesTable(items, d.csrfToken(r), d.canWrite(session)),
	})
}

func (d *dashboard) policyDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Policy Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	found, err := d.deps.Policies.GetPolicy(r.Context(), d.deps.TenantID, id)
	if err != nil {
		d.renderPageError(w, r, session, "Policy Detail", err)
		return
	}
	apps, appAssignments, certificatesList, certificateAssignments, managedFiles, managedFileAssignments, err := d.loadPolicyDetailPolicyContent(r.Context(), found.ID)
	if err != nil {
		d.renderPageError(w, r, session, "Policy Detail", err)
		return
	}
	data := d.policyDetailPageData(session, d.csrfToken(r), found, apps, appAssignments, certificatesList, certificateAssignments, managedFiles, managedFileAssignments)
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) loadPolicyDetailPolicyContent(ctx context.Context, policyID string) ([]apps.App, []policy.PolicyApp, []certificates.Certificate, []policy.PolicyCertificate, []managedfiles.ManagedFile, []policy.PolicyManagedFile, error) {
	appsList := []apps.App{}
	if d.deps.Apps != nil {
		var err error
		appsList, err = d.deps.Apps.ListApps(ctx, d.deps.TenantID)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
	}
	assignments := []policy.PolicyApp{}
	if d.deps.Policies != nil {
		var err error
		assignments, err = d.deps.Policies.ListPolicyApps(ctx, d.deps.TenantID, policyID)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
	}
	certificatesList := []certificates.Certificate{}
	if d.deps.Certificates != nil {
		var err error
		certificatesList, err = d.deps.Certificates.ListCertificates(ctx, d.deps.TenantID)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
	}
	certificateAssignments := []policy.PolicyCertificate{}
	if d.deps.Policies != nil {
		var err error
		certificateAssignments, err = d.deps.Policies.ListPolicyCertificates(ctx, d.deps.TenantID, policyID)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
	}
	managedFilesList := []managedfiles.ManagedFile{}
	if d.deps.ManagedFiles != nil {
		var err error
		managedFilesList, err = d.deps.ManagedFiles.ListManagedFiles(ctx, d.deps.TenantID)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
	}
	managedFileAssignments := []policy.PolicyManagedFile{}
	if d.deps.Policies != nil {
		var err error
		managedFileAssignments, err = d.deps.Policies.ListPolicyManagedFiles(ctx, d.deps.TenantID, policyID)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
	}
	return appsList, assignments, certificatesList, certificateAssignments, managedFilesList, managedFileAssignments, nil
}

func (d *dashboard) togglePolicyApp(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Policy Detail") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/policies/"+r.PathValue("id"), "invalid form")
		return
	}
	policyID := r.PathValue("id")
	appID := r.PathValue("appId")
	items, err := d.deps.Policies.ListPolicyApps(r.Context(), d.deps.TenantID, policyID)
	if err != nil {
		d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
		return
	}
	active := false
	for _, item := range items {
		if item.AppID == appID && item.Status == policy.StatusActive {
			active = true
			break
		}
	}
	message := "app enabled"
	action := "update"
	if active {
		if err := d.deps.Policies.RemovePolicyApp(r.Context(), d.deps.TenantID, policyID, appID); err != nil {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
		message = "app disabled"
	} else {
		if _, err := d.deps.Policies.AddPolicyApp(r.Context(), d.deps.TenantID, policyID, appID); err != nil {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
	}
	d.recordAudit(r, action, "policy_apps", policyID+":"+appID, map[string]any{"policyId": policyID, "appId": appID, "enabled": !active})
	d.redirectOK(w, r, "/admin/policies/"+policyID, message)
}

func (d *dashboard) togglePolicyCertificate(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Policy Detail") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/policies/"+r.PathValue("id"), "invalid form")
		return
	}
	policyID := r.PathValue("id")
	certificateID := r.PathValue("certificateId")
	items, err := d.deps.Policies.ListPolicyCertificates(r.Context(), d.deps.TenantID, policyID)
	if err != nil {
		d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
		return
	}
	active := false
	for _, item := range items {
		if item.CertificateID == certificateID && item.Status == policy.StatusActive {
			active = true
			break
		}
	}
	message := "certificate enabled"
	action := "update"
	if active {
		if err := d.deps.Policies.RemovePolicyCertificate(r.Context(), d.deps.TenantID, policyID, certificateID); err != nil {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
		message = "certificate disabled"
	} else {
		if _, err := d.deps.Policies.AddPolicyCertificate(r.Context(), d.deps.TenantID, policyID, certificateID); err != nil {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
	}
	d.recordAudit(r, action, "policy_certificates", policyID+":"+certificateID, map[string]any{"policyId": policyID, "certificateId": certificateID, "enabled": !active})
	d.redirectOK(w, r, "/admin/policies/"+policyID, message)
}

func (d *dashboard) togglePolicyManagedFile(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Policy Detail") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/policies/"+r.PathValue("id"), "invalid form")
		return
	}
	policyID := r.PathValue("id")
	managedFileID := r.PathValue("managedFileId")
	items, err := d.deps.Policies.ListPolicyManagedFiles(r.Context(), d.deps.TenantID, policyID)
	if err != nil {
		d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
		return
	}
	active := false
	for _, item := range items {
		if item.ManagedFileID == managedFileID && item.Status == policy.StatusActive {
			active = true
			break
		}
	}
	message := "managed file enabled"
	action := "update"
	if active {
		if err := d.deps.Policies.RemovePolicyManagedFile(r.Context(), d.deps.TenantID, policyID, managedFileID); err != nil {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
		message = "managed file disabled"
	} else {
		if _, err := d.deps.Policies.AddPolicyManagedFile(r.Context(), d.deps.TenantID, policyID, managedFileID); err != nil {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
	}
	d.recordAudit(r, action, "policy_managed_files", policyID+":"+managedFileID, map[string]any{"policyId": policyID, "managedFileId": managedFileID, "enabled": !active})
	d.redirectOK(w, r, "/admin/policies/"+policyID, message)
}

func (d *dashboard) createPolicy(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Policies") {
		return
	}
	req, err := policyUpsertFromForm(r, nil)
	if err != nil {
		d.redirectError(w, r, "/admin/policies", err.Error())
		return
	}
	rec, err := d.deps.Policies.CreatePolicy(r.Context(), d.deps.TenantID, req)
	if err != nil {
		d.redirectError(w, r, "/admin/policies", err.Error())
		return
	}
	d.recordAudit(r, "create", "policies", rec.ID, map[string]any{"name": rec.Name, "version": rec.Version})
	d.redirectOK(w, r, "/admin/policies", "policy created")
}

func (d *dashboard) updatePolicy(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Policies") {
		return
	}
	existing, err := d.deps.Policies.GetPolicy(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/policies/"+r.PathValue("id"), err.Error())
		return
	}
	req, err := policyUpsertFromForm(r, &existing)
	if err != nil {
		d.redirectError(w, r, "/admin/policies/"+r.PathValue("id"), err.Error())
		return
	}
	rec, err := d.deps.Policies.UpdatePolicy(r.Context(), d.deps.TenantID, r.PathValue("id"), req)
	if err != nil {
		d.redirectError(w, r, "/admin/policies/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "update", "policies", rec.ID, map[string]any{"name": rec.Name, "version": rec.Version})
	d.redirectOK(w, r, "/admin/policies/"+rec.ID, "policy updated")
}

func (d *dashboard) retirePolicy(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Policies") {
		return
	}
	rec, err := d.deps.Policies.RetirePolicy(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/policies/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "retire", "policies", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/policies/"+rec.ID, "policy retired")
}

func (d *dashboard) devices(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Devices")
	if !ok {
		return
	}
	items, err := d.deps.Devices.ListDevices(r.Context(), d.deps.TenantID)
	if err != nil {
		d.renderPageError(w, r, session, "Devices", err)
		return
	}
	policies := []policy.Policy{}
	if d.deps.Policies != nil {
		policies, err = d.deps.Policies.ListPolicies(r.Context(), d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Devices", err)
			return
		}
	}
	groups := []group.Group{}
	if d.deps.Groups != nil {
		groups, err = d.deps.Groups.ListGroups(r.Context(), d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Devices", err)
			return
		}
	}
	d.renderForSession(w, r, session, pageData{
		Title: "Devices",
		Forms: []formData{{
			Title:  "Create device",
			Action: "/admin/devices/create",
			Fields: []fieldData{
				{Name: "name", Label: "Display name", Type: "text", Placeholder: "warehouse-tablet-001", Required: true},
				{Name: "policyId", Label: "Policy", Type: "select", Placeholder: "Select a policy", Options: allPolicyOptions(policies), Required: true},
				{Name: "groupIds", Label: "Groups", Type: "multiselect", Placeholder: "Select one or more groups", Options: activeGroupOptions(groups)},
			},
			Help:   "",
			Submit: "Create device",
		}},
		Items: devicesTable(items, policies, d.csrfToken(r), d.canWrite(session)),
	})
}

func (d *dashboard) deviceDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Device Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	devices, err := d.deps.Devices.ListDevices(r.Context(), d.deps.TenantID)
	if err != nil {
		d.renderPageError(w, r, session, "Device Detail", err)
		return
	}
	var found *device.Device
	for i := range devices {
		if devices[i].ID == id {
			found = &devices[i]
			break
		}
	}
	if found == nil {
		d.renderPageError(w, r, session, "Device Detail", httpx.ErrNotFound)
		return
	}
	policies := []policy.Policy{}
	if d.deps.Policies != nil {
		policies, err = d.deps.Policies.ListPolicies(r.Context(), d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Device Detail", err)
			return
		}
	}
	groups := []group.Group{}
	if d.deps.Groups != nil {
		groups, err = d.deps.Groups.ListGroups(r.Context(), d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Device Detail", err)
			return
		}
	}
	data := d.deviceDetailPageData(r, session, found, policies, groups, "")
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) deviceEnrollmentQR(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Device Detail") {
		return
	}
	session, ok := sessionFromRequest(r, d.svc)
	if !ok {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	id := r.PathValue("id")
	found, policies, groups, err := d.loadDeviceDetail(r.Context(), id)
	if err != nil {
		d.renderPageError(w, r, session, "Device Detail", err)
		return
	}
	if found.Status != device.StatusPending {
		d.renderPageError(w, r, session, "Device Detail", fmt.Errorf("device must be pending"))
		return
	}
	state, err := d.deviceEnrollmentQRState(r.Context(), firstNonEmpty(found.ID, found.Name))
	if err != nil {
		d.renderPageError(w, r, session, "Device Detail", err)
		return
	}
	result, err := d.generateEnrollmentQR(r.Context(), state)
	if err != nil {
		d.renderPageError(w, r, session, "Device Detail", err)
		return
	}
	d.recordAudit(r, "create", "enrollment_tokens", result.Issued.ID, map[string]any{"expiresAt": result.Issued.ExpiresAt, "deviceId": firstNonEmpty(found.ID, found.Name)})
	data := d.deviceDetailPageData(r, session, found, policies, groups, enrollmentQRCallout(result))
	data.Flash = "QR generated"
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) loadDeviceDetail(ctx context.Context, id string) (*device.Device, []policy.Policy, []group.Group, error) {
	devices, err := d.deps.Devices.ListDevices(ctx, d.deps.TenantID)
	if err != nil {
		return nil, nil, nil, err
	}
	var found *device.Device
	for i := range devices {
		if devices[i].ID == id {
			found = &devices[i]
			break
		}
	}
	if found == nil {
		return nil, nil, nil, httpx.ErrNotFound
	}
	policies := []policy.Policy{}
	if d.deps.Policies != nil {
		policies, err = d.deps.Policies.ListPolicies(ctx, d.deps.TenantID)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	groups := []group.Group{}
	if d.deps.Groups != nil {
		groups, err = d.deps.Groups.ListGroups(ctx, d.deps.TenantID)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return found, policies, groups, nil
}

func (d *dashboard) deviceDetailPageData(r *http.Request, session *auth.Session, found *device.Device, policies []policy.Policy, groups []group.Group, callout template.HTML) pageData {
	body := "<section class=\"panel\"><h2>Current device</h2>" + pre(found) + "</section>"
	policyMap := policyNameByID(policies)
	policyID := firstPolicyID(found.PolicyID)
	if policyID != "" {
		if rec, ok := policyMap[policyID]; ok {
			var b strings.Builder
			label := rec.Name
			if strings.TrimSpace(label) == "" {
				label = rec.ID
			}
			b.WriteString(`<section class="panel"><h2>Active policy</h2><div class="policy-summary">`)
			b.WriteString(summaryTextItem("Created", formatDashboardTime(rec.CreatedAt)))
			b.WriteString(summaryTextItem("ID", rec.ID))
			b.WriteString(`<div class="policy-summary-item"><div class="policy-summary-label">Name</div><div class="policy-summary-value"><a href="/admin/policies/` + escAttr(rec.ID) + `">` + esc(label) + `</a></div></div>`)
			b.WriteString(summaryTextItem("Version", strconv.Itoa(rec.Version)))
			b.WriteString(summaryHTMLItem("Kiosk mode", template.HTML(boolBadge(rec.KioskMode, "enabled", "disabled"))))
			b.WriteString(summaryHTMLItem("Status", template.HTML(statusBadge(rec.Status))))
			b.WriteString(`</div></section>`)
			body += b.String()
		} else {
			body += `<section class="panel"><h2>Active policy</h2><p class="muted">` + esc(policyID) + `</p></section>`
		}
	} else {
		body += `<section class="panel"><h2>Active policy</h2><p class="muted">No policy linked.</p></section>`
	}
	if len(found.GroupIDs) > 0 {
		groupMap := groupNameByID(groups)
		var b strings.Builder
		b.WriteString(`<section class="panel"><h2>Assigned groups</h2><div class="policy-summary">`)
		for _, groupID := range found.GroupIDs {
			label := groupID
			if rec, ok := groupMap[groupID]; ok && strings.TrimSpace(rec.Name) != "" {
				label = rec.Name
			}
			b.WriteString(summaryTextItem("Group", label))
		}
		b.WriteString(`</div></section>`)
		body += b.String()
	} else {
		body += `<section class="panel"><h2>Assigned groups</h2><p class="muted">No groups linked.</p></section>`
	}
	body += string(d.deviceConfigPreviewSection(r.Context(), found))
	deviceKey := firstNonEmpty(found.ID, found.Name)
	deviceRowID := found.RecordID()
	if d.deps.Logs != nil {
		rows, _ := d.deps.Logs.Search(r.Context(), d.deps.TenantID, logs.SearchFilter{DeviceID: deviceKey, Limit: 10})
		body += "<section class=\"panel\"><h2>Recent logs</h2>" + pre(rows) + "</section>"
	}
	if d.deps.DeviceInfo != nil {
		rows, _ := d.deps.DeviceInfo.Search(r.Context(), d.deps.TenantID, deviceinfo.SearchFilter{DeviceID: deviceKey, Limit: 10})
		body += "<section class=\"panel\"><h2>Recent device info</h2>" + pre(rows) + "</section>"
	}
	if d.deps.Commands != nil {
		rows, _ := d.deps.Commands.ListPending(r.Context(), d.deps.TenantID, deviceRowID)
		body += "<section class=\"panel\"><h2>Pending commands</h2>" + pre(rows) + "</section>"
	}
	data := pageData{
		Title:    "Device Detail",
		Subtitle: "Edit the device label or retire it from active use.",
		Items:    template.HTML(body),
	}
	if d.canWrite(session) && found.Status != device.StatusRetired && found.Status != device.StatusWiped {
		policyID := ""
		if found.PolicyID != nil {
			policyID = *found.PolicyID
		}
		options := allPolicyOptions(policies)
		if policyID != "" {
			foundPolicy := false
			for _, option := range options {
				if option.Value == policyID {
					foundPolicy = true
					break
				}
			}
			if !foundPolicy {
				options = append(options, optionData{Value: policyID, Label: policyID})
			}
		}
		groupOptions := activeGroupOptions(groups)
		if len(found.GroupIDs) > 0 {
			for _, groupID := range found.GroupIDs {
				foundGroup := false
				for _, option := range groupOptions {
					if option.Value == groupID {
						foundGroup = true
						break
					}
				}
				if !foundGroup {
					groupOptions = append(groupOptions, optionData{Value: groupID, Label: groupID})
				}
			}
		}
		data.Forms = []formData{{
			Title:  "Update device",
			Action: "/admin/devices/" + found.ID + "/update",
			Fields: []fieldData{
				{Name: "name", Label: "Display name", Type: "text", Value: found.Name, Placeholder: "warehouse-tablet-001", Required: true},
				{Name: "policyId", Label: "Policy", Type: "select", Value: policyID, Placeholder: "Select a policy", Options: options, Required: true},
				{Name: "groupIds", Label: "Groups", Type: "multiselect", Values: append([]string(nil), found.GroupIDs...), Placeholder: "Select one or more groups", Options: groupOptions},
			},
			Submit: "Update device",
		}, {
			Title:  "Retire device",
			Action: "/admin/devices/" + found.ID + "/retire",
			Submit: "Retire device",
			Danger: true,
		}}
	}
	if d.canWrite(session) && found.Status == device.StatusPending {
		form := deviceEnrollmentQRForm(found.ID)
		form.After = callout
		data.Forms = append(data.Forms, form)
	}
	return data
}

func (d *dashboard) deviceConfigPreviewSection(ctx context.Context, found *device.Device) template.HTML {
	if found == nil || found.PolicyID == nil || strings.TrimSpace(*found.PolicyID) == "" {
		return ""
	}
	if d.deps.Policies == nil {
		return `<section class="panel"><h2>Config preview</h2><p class="muted">Preview unavailable until policy data is configured.</p></section>`
	}
	deviceKey := firstNonEmpty(found.ID, found.Name)
	config, err := enrollmenthttp.BuildConfigSnapshot(ctx, d.deps.Policies, d.deps.Apps, d.deps.ManagedFiles, d.deps.Artifacts, d.deps.Certificates, d.deps.TenantID, deviceKey, found.PolicyID, found.BootstrapExtras, d.deps.Runtime)
	if err != nil {
		return template.HTML(`<section class="panel"><h2>Config preview</h2><p class="muted">Preview unavailable: ` + esc(err.Error()) + `</p></section>`)
	}
	preview := struct {
		Version      string                           `json:"version"`
		Runtime      enrollment.RuntimeSnapshot       `json:"runtime,omitempty"`
		Device       enrollment.DeviceSnapshot        `json:"device"`
		Policy       enrollment.PolicySnapshot        `json:"policy"`
		Apps         []enrollment.AppSnapshot         `json:"apps"`
		Files        []enrollment.ManagedFileSnapshot `json:"files"`
		Certificates []enrollment.CertificateSnapshot `json:"certificates"`
	}{
		Version:      config.Version,
		Runtime:      config.Runtime,
		Device:       config.Device,
		Policy:       config.Policy,
		Apps:         config.Apps,
		Files:        config.Files,
		Certificates: config.Certificates,
	}
	var b strings.Builder
	b.WriteString(`<section class="panel"><h2>Config preview</h2>`)
	b.WriteString(pre(preview))
	b.WriteString(`</section>`)
	return template.HTML(b.String())
}

func (d *dashboard) createDevice(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Devices") {
		return
	}
	req, err := deviceUpsertFromForm(r)
	if err != nil {
		d.redirectError(w, r, "/admin/devices", err.Error())
		return
	}
	if strings.TrimSpace(req.PolicyID) == "" {
		d.redirectError(w, r, "/admin/devices", "policy is required")
		return
	}
	if d.deps.Groups == nil && len(req.GroupIDs) > 0 {
		d.redirectError(w, r, "/admin/devices", "groups are unavailable")
		return
	}
	if strings.TrimSpace(req.SecretHash) == "" {
		secret, err := enrollment.NewTokenSecret()
		if err != nil {
			d.redirectError(w, r, "/admin/devices", err.Error())
			return
		}
		req.SecretHash = enrollment.HashToken(secret)
	}
	rec, err := d.deps.Devices.CreateDevice(r.Context(), d.deps.TenantID, req)
	if err != nil {
		d.redirectError(w, r, "/admin/devices", err.Error())
		return
	}
	d.recordAudit(r, "create", "devices", rec.ID, map[string]any{"name": rec.Name, "deviceId": rec.ID, "policyId": firstPolicyID(rec.PolicyID), "groupIds": req.GroupIDs})
	d.redirectOK(w, r, "/admin/devices", "device created")
}

func (d *dashboard) updateDevice(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Devices") {
		return
	}
	req, err := deviceUpsertFromForm(r)
	if err != nil {
		d.redirectError(w, r, "/admin/devices/"+r.PathValue("id"), err.Error())
		return
	}
	if strings.TrimSpace(req.PolicyID) == "" {
		d.redirectError(w, r, "/admin/devices/"+r.PathValue("id"), "policy is required")
		return
	}
	if strings.TrimSpace(req.SecretHash) != "" {
		d.redirectError(w, r, "/admin/devices/"+r.PathValue("id"), "device secret rotation is disabled")
		return
	}
	devices, err := d.deps.Devices.ListDevices(r.Context(), d.deps.TenantID)
	if err != nil {
		d.redirectError(w, r, "/admin/devices/"+r.PathValue("id"), err.Error())
		return
	}
	var found *device.Device
	for i := range devices {
		if devices[i].ID == r.PathValue("id") {
			found = &devices[i]
			break
		}
	}
	if found == nil {
		d.redirectError(w, r, "/admin/devices/"+r.PathValue("id"), "not found")
		return
	}
	if found.Status == device.StatusRetired || found.Status == device.StatusWiped {
		d.redirectError(w, r, "/admin/devices/"+r.PathValue("id"), "conflict")
		return
	}
	rec, err := d.deps.Devices.UpdateDevice(r.Context(), d.deps.TenantID, r.PathValue("id"), req)
	if err != nil {
		d.redirectError(w, r, "/admin/devices/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "update", "devices", rec.ID, map[string]any{"name": rec.Name, "deviceId": rec.ID, "policyId": firstPolicyID(rec.PolicyID)})
	d.redirectOK(w, r, "/admin/devices/"+rec.ID, "device updated")
}

func (d *dashboard) retireDevice(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Devices") {
		return
	}
	rec, err := d.deps.Devices.RetireDevice(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/devices", err.Error())
		return
	}
	d.recordAudit(r, "retire", "devices", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/devices", "device retired")
}

func (d *dashboard) apps(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Apps")
	if !ok {
		return
	}
	items, err := d.deps.Apps.ListApps(r.Context(), d.deps.TenantID)
	if err != nil {
		d.renderPageError(w, r, session, "Apps", err)
		return
	}
	versions := map[string][]apps.Version{}
	for _, item := range items {
		rows, _ := d.deps.Apps.ListVersions(r.Context(), d.deps.TenantID, item.ID)
		versions[item.ID] = rows
	}
	d.renderForSession(w, r, session, pageData{Title: "Apps", Forms: []formData{managedAppForm("/admin/apps/create")}, Items: appsTable(items, versions)})
}

func (d *dashboard) appDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "App Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	found, versions, err := d.loadAppDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		d.renderPageError(w, r, session, "App Detail", err)
		return
	}
	data := d.appDetailPageData(session, found, versions)
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) loadAppDetail(ctx context.Context, id string) (*apps.App, []apps.Version, error) {
	found, err := d.deps.Apps.GetApp(ctx, d.deps.TenantID, id)
	if err != nil {
		return nil, nil, err
	}
	versions, err := d.deps.Apps.ListVersions(ctx, d.deps.TenantID, found.ID)
	if err != nil {
		return nil, nil, err
	}
	return &found, versions, nil
}

func (d *dashboard) appDetailPageData(session *auth.Session, found *apps.App, versions []apps.Version) pageData {
	latest := appLatestPublishedVersion(versions)
	body := "<section class=\"panel\"><h2>Current app</h2>" + appSummary(*found, latest) + "</section>"
	if len(versions) > 0 {
		body += "<section class=\"panel\"><h2>Versions</h2>" + appVersionsTable(versions) + "</section>"
	}
	data := pageData{
		Title:    "App Detail",
		Subtitle: "Edit the app metadata or retire it from active use.",
		Items:    template.HTML(body),
	}
	if d.canWrite(session) && found.Status != apps.StatusRetired {
		data.Forms = []formData{{
			Title:  "Update app",
			Action: "/admin/apps/" + found.ID + "/update",
			Fields: []fieldData{
				{Name: "packageName", Label: "Package name", Type: "text", Value: found.PackageName, Placeholder: "com.example.app", Required: true},
				{Name: "name", Label: "App name", Type: "text", Value: found.Name, Placeholder: "Example App", Required: true},
			},
			Submit: "Update app",
		}, {
			Title:  "Retire app",
			Action: "/admin/apps/" + found.ID + "/retire",
			Submit: "Retire app",
			Danger: true,
		}}
	}
	return data
}

func (d *dashboard) downloadApp(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "App Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if d.deps.Artifacts == nil || d.deps.Apps == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	found, versions, err := d.loadAppDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		d.renderPageError(w, r, session, "App Detail", err)
		return
	}
	latest := appLatestPublishedVersion(versions)
	if latest == nil {
		http.NotFound(w, r)
		return
	}
	latestVersion, err := d.deps.Apps.GetVersion(r.Context(), d.deps.TenantID, found.ID, latest.ID)
	if err != nil {
		d.renderPageError(w, r, session, "App Detail", err)
		return
	}
	if latestVersion.Artifact == nil {
		http.NotFound(w, r)
		return
	}
	body, err := d.deps.Artifacts.Get(r.Context(), latestVersion.Artifact.StorageKey)
	if err != nil {
		d.renderPageError(w, r, session, "App Detail", err)
		return
	}
	defer body.Close()
	content, err := io.ReadAll(body)
	if err != nil {
		d.renderPageError(w, r, session, "App Detail", err)
		return
	}
	w.Header().Set("Content-Type", latestVersion.Artifact.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(content)), 10))
	w.Header().Set("X-XMDM-Artifact-Checksum", checksum.SHA256Base64URL(content))
	w.Header().Set("X-XMDM-Artifact-Size", strconv.FormatInt(int64(len(content)), 10))
	downloadName := found.PackageName
	if downloadName == "" {
		downloadName = found.Name
	}
	if downloadName == "" {
		downloadName = found.ID
	}
	if latestVersion.VersionName != "" {
		downloadName += "-" + latestVersion.VersionName
	}
	downloadName += ".apk"
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(content))
}

func (d *dashboard) createApp(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Apps") {
		return
	}
	req, content, err := managedAppUpsertFromMultipart(r)
	if err != nil {
		d.redirectError(w, r, "/admin/apps", err.Error())
		return
	}
	appRec, existingVersion, reusedExisting, err := d.findManagedAppForCreate(r.Context(), req.App, req.VersionCode, req.File.Checksum)
	if err != nil {
		d.redirectError(w, r, "/admin/apps", err.Error())
		return
	}
	if existingVersion != nil {
		d.recordAudit(r, "create", "apps", appRec.ID, map[string]any{
			"packageName": appRec.PackageName,
			"name":        appRec.Name,
			"versionId":   existingVersion.ID,
			"versionName": existingVersion.VersionName,
			"versionCode": existingVersion.VersionCode,
			"artifactId":  existingVersion.ArtifactID,
		})
		d.redirectOK(w, r, "/admin/apps/"+appRec.ID, "managed app already up to date")
		return
	}
	if len(content) > 0 {
		if err := d.deps.Artifacts.Put(r.Context(), req.StorageKey, bytes.NewReader(content), req.MimeType, req.SizeBytes); err != nil {
			d.redirectError(w, r, "/admin/apps", err.Error())
			return
		}
	}
	fileRec, err := d.deps.Files.CreateFile(r.Context(), d.deps.TenantID, req.File)
	if err != nil {
		_ = d.deps.Artifacts.Delete(r.Context(), req.StorageKey)
		d.redirectError(w, r, "/admin/apps", err.Error())
		return
	}
	artifactID := fileRec.ArtifactID
	versionRec, err := d.deps.Apps.CreateVersion(r.Context(), d.deps.TenantID, appRec.ID, apps.VersionUpsert{
		VersionName: req.VersionName,
		VersionCode: req.VersionCode,
		ArtifactID:  &artifactID,
		Checksum:    fileRec.Checksum,
		Publish:     req.Publish,
	})
	if err != nil {
		_, _ = d.deps.Files.RetireFile(r.Context(), d.deps.TenantID, fileRec.ID)
		_ = d.deps.Artifacts.Delete(r.Context(), req.StorageKey)
		if !reusedExisting {
			_, _ = d.deps.Apps.RetireApp(r.Context(), d.deps.TenantID, appRec.ID)
		}
		d.redirectError(w, r, "/admin/apps", err.Error())
		return
	}
	d.recordAudit(r, "create", "apps", appRec.ID, map[string]any{
		"packageName": appRec.PackageName,
		"name":        appRec.Name,
		"versionId":   versionRec.ID,
		"versionName": versionRec.VersionName,
		"versionCode": versionRec.VersionCode,
		"artifactId":  fileRec.ArtifactID,
		"fileName":    fileRec.Name,
	})
	d.redirectOK(w, r, "/admin/apps/"+appRec.ID, "managed app created")
}

func (d *dashboard) findManagedAppForCreate(ctx context.Context, appReq apps.AppUpsert, versionCode int64, checksumValue string) (apps.App, *apps.Version, bool, error) {
	items, err := d.deps.Apps.ListApps(ctx, d.deps.TenantID)
	if err != nil {
		return apps.App{}, nil, false, err
	}
	for _, item := range items {
		if item.PackageName != appReq.PackageName {
			continue
		}
		if item.Status != apps.StatusActive {
			return apps.App{}, nil, false, fmt.Errorf("app package already exists")
		}
		versions, err := d.deps.Apps.ListVersions(ctx, d.deps.TenantID, item.ID)
		if err != nil {
			return apps.App{}, nil, false, err
		}
		for _, version := range versions {
			if version.VersionCode == versionCode {
				if version.Checksum != checksumValue {
					return apps.App{}, nil, false, fmt.Errorf("app version already exists with different content")
				}
				return item, &version, true, nil
			}
		}
		return item, nil, true, nil
	}
	rec, err := d.deps.Apps.CreateApp(ctx, d.deps.TenantID, appReq)
	if err != nil {
		return apps.App{}, nil, false, err
	}
	return rec, nil, false, nil
}

func (d *dashboard) updateApp(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Apps") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/apps/"+r.PathValue("id"), "invalid form")
		return
	}
	rec, err := d.deps.Apps.UpdateApp(r.Context(), d.deps.TenantID, r.PathValue("id"), apps.AppUpsert{PackageName: strings.TrimSpace(r.FormValue("packageName")), Name: strings.TrimSpace(r.FormValue("name"))})
	if err != nil {
		d.redirectError(w, r, "/admin/apps/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "update", "apps", rec.ID, map[string]any{"packageName": rec.PackageName, "name": rec.Name})
	d.redirectOK(w, r, "/admin/apps/"+rec.ID, "app updated")
}

func (d *dashboard) retireApp(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Apps") {
		return
	}
	rec, err := d.deps.Apps.RetireApp(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/apps/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "retire", "apps", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/apps/"+rec.ID, "app retired")
}

func (d *dashboard) createAppVersion(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Apps") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/apps", "invalid form")
		return
	}
	versionCode, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("versionCode")), 10, 64)
	if err != nil {
		d.redirectError(w, r, "/admin/apps", "invalid version code")
		return
	}
	artifactID := strings.TrimSpace(r.FormValue("artifactId"))
	req := apps.VersionUpsert{VersionName: strings.TrimSpace(r.FormValue("versionName")), VersionCode: versionCode, Checksum: strings.TrimSpace(r.FormValue("checksum")), Publish: true}
	if artifactID != "" {
		req.ArtifactID = &artifactID
	}
	rec, err := d.deps.Apps.CreateVersion(r.Context(), d.deps.TenantID, r.PathValue("id"), req)
	if err != nil {
		d.redirectError(w, r, "/admin/apps/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "create", "app_versions", rec.ID, map[string]any{"appId": rec.AppID, "versionName": rec.VersionName, "versionCode": rec.VersionCode, "status": rec.Status})
	d.redirectOK(w, r, "/admin/apps/"+rec.AppID, "app version created")
}

func (d *dashboard) managedFiles(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Managed Files")
	if !ok {
		return
	}
	items, err := d.deps.ManagedFiles.ListManagedFiles(r.Context(), d.deps.TenantID)
	if err != nil {
		d.renderPageError(w, r, session, "Managed Files", err)
		return
	}
	d.renderForSession(w, r, session, pageData{Title: "Managed Files", Forms: []formData{{Title: "Upload managed file", Action: "/admin/managed-files/create", EncType: "multipart/form-data", Fields: []fieldData{{Name: "path", Label: "Device path", Type: "text", Required: true, Placeholder: "/sdcard/xmdm/device-config.txt"}, {Name: "replaceVariables", Label: "Replace variables", Type: "checkbox", Value: "on"}, {Name: "file", Label: "File", Type: "file", Required: true}}, Submit: "Upload managed file"}}, Items: managedFilesTable(items)})
}

func (d *dashboard) managedFileDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Managed File Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	found, err := d.loadManagedFileDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		d.renderPageError(w, r, session, "Managed File Detail", err)
		return
	}
	d.renderForSession(w, r, session, d.managedFileDetailPageData(session, found))
}

func (d *dashboard) loadManagedFileDetail(ctx context.Context, id string) (*managedfiles.ManagedFile, error) {
	found, err := d.deps.ManagedFiles.GetManagedFile(ctx, d.deps.TenantID, id)
	if err != nil {
		return nil, err
	}
	return &found, nil
}

func (d *dashboard) managedFileDetailPageData(session *auth.Session, found *managedfiles.ManagedFile) pageData {
	var body strings.Builder
	body.WriteString(`<section class="panel"><h2>Current managed file</h2><div class="policy-summary">`)
	body.WriteString(summaryTextItem("Created", formatDashboardTime(found.CreatedAt)))
	body.WriteString(summaryTextItem("Updated", formatDashboardTime(found.UpdatedAt)))
	body.WriteString(summaryTextItem("ID", found.ID))
	body.WriteString(summaryTextItem("Path", found.Path))
	body.WriteString(summaryTextItem("File ID", found.FileID))
	if found.File != nil {
		body.WriteString(summaryTextItem("Source file", found.File.Name))
		body.WriteString(summaryTextItem("Artifact", found.File.ArtifactID))
		body.WriteString(summaryTextItem("Checksum", found.File.Checksum))
		body.WriteString(summaryTextItem("MIME", found.File.MimeType))
		body.WriteString(summaryHTMLItem("Download", template.HTML(`<a class="button btn-primary" href="/admin/managed-files/`+escAttr(found.ID)+`/download">Download file</a>`)))
	}
	body.WriteString(summaryHTMLItem("Template", template.HTML(boolBadge(found.ReplaceVariables, "enabled", "disabled"))))
	body.WriteString(summaryHTMLItem("Status", template.HTML(statusBadge(found.Status))))
	body.WriteString(`</div>`)
	body.WriteString(string(rawDataDetails("Raw managed file data", found)))
	body.WriteString(`</section>`)
	data := pageData{
		Title:    "Managed File Detail",
		Subtitle: "Review the managed file binding or retire it from active use.",
		Items:    template.HTML(body.String()),
	}
	if d.canWrite(session) && found.Status != managedfiles.StatusRetired {
		data.Forms = []formData{{
			Title:  "Retire managed file",
			Action: "/admin/managed-files/" + found.ID + "/retire",
			Submit: "Retire managed file",
			Danger: true,
		}}
	}
	return data
}

func (d *dashboard) downloadManagedFile(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Managed File Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if d.deps.Artifacts == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	found, err := d.loadManagedFileDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		d.renderPageError(w, r, session, "Managed File Detail", err)
		return
	}
	if found.File == nil || found.File.Artifact == nil || found.Status != managedfiles.StatusActive {
		http.NotFound(w, r)
		return
	}
	body, err := d.deps.Artifacts.Get(r.Context(), found.File.Artifact.StorageKey)
	if err != nil {
		d.renderPageError(w, r, session, "Managed File Detail", err)
		return
	}
	defer body.Close()
	content, err := io.ReadAll(body)
	if err != nil {
		d.renderPageError(w, r, session, "Managed File Detail", err)
		return
	}
	w.Header().Set("Content-Type", found.File.Artifact.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(content)), 10))
	w.Header().Set("X-XMDM-Artifact-Checksum", checksum.SHA256Base64URL(content))
	w.Header().Set("X-XMDM-Artifact-Size", strconv.FormatInt(int64(len(content)), 10))
	downloadName := found.File.Name
	if downloadName == "" {
		downloadName = found.Path
	}
	if downloadName == "" {
		downloadName = found.ID
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(content))
}

func (d *dashboard) createManagedFile(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Managed Files") {
		return
	}
	fileReq, bindingReq, content, err := managedFileUpsertFromMultipart(r)
	if err != nil {
		d.redirectError(w, r, "/admin/managed-files", err.Error())
		return
	}
	if len(content) > 0 {
		if err := d.deps.Artifacts.Put(r.Context(), fileReq.StorageKey, bytes.NewReader(content), fileReq.MimeType, int64(len(content))); err != nil {
			d.redirectError(w, r, "/admin/managed-files", err.Error())
			return
		}
	}
	fileRec, err := d.deps.Files.CreateFile(r.Context(), d.deps.TenantID, fileReq)
	if err != nil {
		_ = d.deps.Artifacts.Delete(r.Context(), fileReq.StorageKey)
		d.redirectError(w, r, "/admin/managed-files", err.Error())
		return
	}
	bindingReq.FileID = fileRec.ID
	rec, err := d.deps.ManagedFiles.CreateManagedFile(r.Context(), d.deps.TenantID, bindingReq)
	if err != nil {
		_, _ = d.deps.Files.RetireFile(r.Context(), d.deps.TenantID, fileRec.ID)
		_ = d.deps.Artifacts.Delete(r.Context(), fileReq.StorageKey)
		d.redirectError(w, r, "/admin/managed-files", err.Error())
		return
	}
	d.recordAudit(r, "create", "managed_files", rec.ID, map[string]any{"fileId": rec.FileID, "path": rec.Path, "sourceName": fileRec.Name, "replaceVariables": rec.ReplaceVariables})
	d.redirectOK(w, r, "/admin/managed-files", "managed file uploaded")
}

func (d *dashboard) retireManagedFile(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Managed Files") {
		return
	}
	rec, err := d.deps.ManagedFiles.RetireManagedFile(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/managed-files", err.Error())
		return
	}
	d.recordAudit(r, "retire", "managed_files", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/managed-files", "managed file retired")
}

func (d *dashboard) certificates(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Certificates")
	if !ok {
		return
	}
	items, err := d.deps.Certificates.ListCertificates(r.Context(), d.deps.TenantID)
	if err != nil {
		d.renderPageError(w, r, session, "Certificates", err)
		return
	}
	d.renderForSession(w, r, session, pageData{Title: "Certificates", Forms: []formData{certificateUploadForm("/admin/certificates/create")}, Items: certificatesTable(items)})
}

func (d *dashboard) certificateDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Certificate Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	found, err := d.loadCertificateDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		d.renderPageError(w, r, session, "Certificate Detail", err)
		return
	}
	d.renderForSession(w, r, session, d.certificateDetailPageData(session, found))
}

func (d *dashboard) downloadCertificate(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Certificate Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if d.deps.Artifacts == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	found, err := d.loadCertificateDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		d.renderPageError(w, r, session, "Certificate Detail", err)
		return
	}
	if found.Artifact == nil {
		http.NotFound(w, r)
		return
	}
	body, err := d.deps.Artifacts.Get(r.Context(), found.Artifact.StorageKey)
	if err != nil {
		d.renderPageError(w, r, session, "Certificate Detail", err)
		return
	}
	defer body.Close()
	content, err := io.ReadAll(body)
	if err != nil {
		d.renderPageError(w, r, session, "Certificate Detail", err)
		return
	}
	w.Header().Set("Content-Type", found.Artifact.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(content)), 10))
	w.Header().Set("X-XMDM-Artifact-Checksum", checksum.SHA256Base64URL(content))
	w.Header().Set("X-XMDM-Artifact-Size", strconv.FormatInt(int64(len(content)), 10))
	downloadName := found.Name
	if downloadName == "" {
		downloadName = found.ID
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(content))
}

func (d *dashboard) loadCertificateDetail(ctx context.Context, id string) (*certificates.Certificate, error) {
	found, err := d.deps.Certificates.GetCertificate(ctx, d.deps.TenantID, id)
	if err != nil {
		return nil, err
	}
	return &found, nil
}

func (d *dashboard) certificateDetailPageData(session *auth.Session, found *certificates.Certificate) pageData {
	var body strings.Builder
	body.WriteString(`<section class="panel"><h2>Current certificate</h2><div class="policy-summary">`)
	body.WriteString(summaryTextItem("Created", formatDashboardTime(found.CreatedAt)))
	body.WriteString(summaryTextItem("Updated", formatDashboardTime(found.UpdatedAt)))
	body.WriteString(summaryTextItem("ID", found.ID))
	body.WriteString(summaryTextItem("Name", found.Name))
	body.WriteString(summaryTextItem("Artifact", found.ArtifactID))
	body.WriteString(summaryTextItem("Checksum", found.Checksum))
	if found.Artifact != nil {
		body.WriteString(summaryHTMLItem("Download", template.HTML(`<a class="button btn-primary" href="/admin/certificates/`+escAttr(found.ID)+`/download">Download certificate</a>`)))
	}
	if found.Artifact != nil {
		body.WriteString(summaryTextItem("Storage key", found.Artifact.StorageKey))
		body.WriteString(summaryTextItem("Size", strconv.FormatInt(found.Artifact.SizeBytes, 10)))
		body.WriteString(summaryTextItem("MIME", found.Artifact.MimeType))
	}
	body.WriteString(summaryHTMLItem("Status", template.HTML(statusBadge(found.Status))))
	body.WriteString(`</div>`)
	body.WriteString(string(rawDataDetails("Raw certificate data", found)))
	body.WriteString(`</section>`)
	data := pageData{
		Title:    "Certificate Detail",
		Subtitle: "Review the certificate artifact or retire it from active use.",
		Items:    template.HTML(body.String()),
	}
	if d.canWrite(session) && found.Status != certificates.StatusRetired {
		data.Forms = []formData{{
			Title:  "Retire certificate",
			Action: "/admin/certificates/" + found.ID + "/retire",
			Submit: "Retire certificate",
			Danger: true,
		}}
	}
	return data
}

func (d *dashboard) createCertificate(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Certificates") {
		return
	}
	req, content, err := certificateUpsertFromMultipart(r)
	if err != nil {
		d.redirectError(w, r, "/admin/certificates", err.Error())
		return
	}
	if len(content) > 0 {
		if actual := checksum.SHA256Base64URL(content); actual != req.Checksum {
			d.redirectError(w, r, "/admin/certificates", "checksum mismatch")
			return
		}
		if err := d.deps.Artifacts.Put(r.Context(), req.StorageKey, bytes.NewReader(content), req.MimeType, int64(len(content))); err != nil {
			d.redirectError(w, r, "/admin/certificates", err.Error())
			return
		}
	}
	rec, err := d.deps.Certificates.CreateCertificate(r.Context(), d.deps.TenantID, req)
	if err != nil {
		_ = d.deps.Artifacts.Delete(r.Context(), req.StorageKey)
		d.redirectError(w, r, "/admin/certificates", err.Error())
		return
	}
	d.recordAudit(r, "create", "certificates", rec.ID, map[string]any{"name": rec.Name, "checksum": rec.Checksum, "artifactId": rec.ArtifactID})
	d.redirectOK(w, r, "/admin/certificates", "certificate uploaded")
}

func (d *dashboard) retireCertificate(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Certificates") {
		return
	}
	rec, err := d.deps.Certificates.RetireCertificate(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/certificates", err.Error())
		return
	}
	d.recordAudit(r, "retire", "certificates", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/certificates", "certificate retired")
}

func (d *dashboard) commands(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Commands")
	if !ok {
		return
	}
	devices := []device.Device{}
	if d.deps.Devices != nil {
		var err error
		devices, err = d.deps.Devices.ListDevices(r.Context(), d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Commands", err)
			return
		}
	}
	deviceMap := make(map[string]device.Device, len(devices))
	for _, item := range devices {
		deviceMap[item.ID] = item
	}
	groups := []group.Group{}
	if d.deps.Groups != nil {
		var err error
		groups, err = d.deps.Groups.ListGroups(r.Context(), d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Commands", err)
			return
		}
	}
	items, err := d.deps.Commands.ListRecent(r.Context(), d.deps.TenantID, queryLimit(r, 25))
	if err != nil {
		d.renderPageError(w, r, session, "Commands", err)
		return
	}
	d.renderForSession(w, r, session, pageData{
		Title:    "Commands",
		Subtitle: "Send commands to individual devices or device groups. Broadcast is disabled from the dashboard.",
		Forms:    []formData{{Title: "Send command", Action: "/admin/commands/create", Fields: commandFields(devices, groups), Submit: "Send command"}},
		Items:    commandsTable(items, deviceMap),
	})
}

func (d *dashboard) commandDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Commands")
	if !ok {
		return
	}
	if d.deps.Commands == nil {
		d.renderError(w, http.StatusNotFound, "Commands", "not found")
		return
	}
	commandID := strings.TrimSpace(r.PathValue("id"))
	found, err := d.deps.Commands.Get(r.Context(), d.deps.TenantID, commandID)
	if err != nil {
		if err == httpx.ErrNotFound {
			d.renderError(w, http.StatusNotFound, "Commands", "not found")
			return
		}
		d.renderPageError(w, r, session, "Commands", err)
		return
	}
	devices := []device.Device{}
	if d.deps.Devices != nil {
		var err error
		devices, err = d.deps.Devices.ListDevices(r.Context(), d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Commands", err)
			return
		}
	}
	deviceMap := make(map[string]device.Device, len(devices))
	for _, item := range devices {
		deviceMap[item.ID] = item
	}
	d.renderForSession(w, r, session, pageData{
		Title:    "Command Detail",
		Subtitle: "Inspect the queued command row, payload, delivery state, and device acknowledgement result.",
		Items:    commandSummary(found, deviceMap[found.DeviceID]),
	})
}

func (d *dashboard) createCommand(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Commands") {
		return
	}
	req, err := commandUpsertFromForm(r)
	if err != nil {
		d.redirectError(w, r, "/admin/commands", err.Error())
		return
	}
	created, err := d.deps.Commands.Enqueue(r.Context(), d.deps.TenantID, req)
	if err != nil {
		d.redirectError(w, r, "/admin/commands", err.Error())
		return
	}
	for _, rec := range created {
		d.recordAudit(r, "create", "commands", rec.ID, map[string]any{"type": rec.Type, "status": rec.Status, "deviceId": rec.DeviceID})
	}
	d.redirectOK(w, r, "/admin/commands", fmt.Sprintf("%d command(s) queued", len(created)))
}

func (d *dashboard) logs(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Logs")
	if !ok {
		return
	}
	filter, err := logFilterFromQuery(r)
	if err != nil {
		d.renderPageError(w, r, session, "Logs", err)
		return
	}
	items, err := d.deps.Logs.Search(r.Context(), d.deps.TenantID, filter)
	if err != nil {
		d.renderPageError(w, r, session, "Logs", err)
		return
	}
	d.renderForSession(w, r, session, pageData{Title: "Logs", Items: searchAndTable("/admin/logs", []fieldData{{Name: "deviceId", Label: "Device ID", Type: "text", Value: r.URL.Query().Get("deviceId")}, {Name: "source", Label: "Source", Type: "text", Value: r.URL.Query().Get("source")}, {Name: "level", Label: "Level", Type: "text", Value: r.URL.Query().Get("level")}, {Name: "q", Label: "Query", Type: "text", Value: r.URL.Query().Get("q")}, {Name: "limit", Label: "Limit", Type: "number", Value: firstNonEmpty(r.URL.Query().Get("limit"), "25")}}, logsTable(items))})
}

func (d *dashboard) audit(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Audit")
	if !ok {
		return
	}
	items, err := d.deps.Audit.List(r.Context(), d.deps.TenantID)
	if err != nil {
		d.renderPageError(w, r, session, "Audit", err)
		return
	}
	d.renderForSession(w, r, session, pageData{Title: "Audit", Items: auditTable(items)})
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

func roleUpsertFromForm(r *http.Request) (identity.RoleUpsert, error) {
	if err := r.ParseForm(); err != nil {
		return identity.RoleUpsert{}, err
	}
	var permissions []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(r.FormValue("permissions"))), &permissions); err != nil {
		return identity.RoleUpsert{}, fmt.Errorf("invalid permissions json")
	}
	return identity.RoleUpsert{Name: strings.TrimSpace(r.FormValue("name")), Permissions: permissions}, nil
}

func policyUpsertFromForm(r *http.Request, existing *policy.Policy) (policy.PolicyUpsert, error) {
	if err := r.ParseForm(); err != nil {
		return policy.PolicyUpsert{}, err
	}
	restrictions := policyRestrictionPayload{
		AllowPackages:                splitLines(r.FormValue("allowPackages")),
		BlockPackages:                splitLines(r.FormValue("blockPackages")),
		SuspendPackages:              splitLines(r.FormValue("suspendPackages")),
		KioskKeepScreenOn:            hasFormField(r, "kioskKeepScreenOn"),
		KioskStayAwakeWhilePluggedIn: hasFormField(r, "kioskStayAwakeWhilePluggedIn"),
		KioskUnlockOnBoot:            hasFormField(r, "kioskUnlockOnBoot"),
		KioskExitPasscode:            strings.TrimSpace(r.FormValue("kioskExitPasscode")),
	}
	raw, err := json.Marshal(restrictions)
	if err != nil {
		return policy.PolicyUpsert{}, fmt.Errorf("invalid restrictions")
	}
	req := policy.PolicyUpsert{Name: strings.TrimSpace(r.FormValue("name")), KioskMode: hasFormField(r, "kioskMode"), KioskAppPackage: strings.TrimSpace(r.FormValue("kioskAppPackage")), Restrictions: json.RawMessage(raw)}
	if req.KioskMode && !kioskExitPasscodeConfigured(req.Restrictions) && (existing == nil || !existing.KioskMode) {
		return policy.PolicyUpsert{}, fmt.Errorf("kiosk policies require restrictions.kioskExitPasscode")
	}
	return req, nil
}

func splitLines(value string) []string {
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func kioskExitPasscodeConfigured(restrictions json.RawMessage) bool {
	if len(restrictions) == 0 || string(restrictions) == "null" {
		return false
	}
	var parsed policyRestrictionPayload
	if err := json.Unmarshal(restrictions, &parsed); err != nil {
		return false
	}
	return strings.TrimSpace(parsed.KioskExitPasscode) != ""
}

func deviceUpsertFromForm(r *http.Request) (device.DeviceUpsert, error) {
	if err := r.ParseForm(); err != nil {
		return device.DeviceUpsert{}, err
	}
	return device.DeviceUpsert{Name: strings.TrimSpace(r.FormValue("name")), SecretHash: hashPlainValue(r.FormValue("deviceSecret")), PolicyID: strings.TrimSpace(r.FormValue("policyId")), GroupIDs: splitFormValues(r.Form["groupIds"])}, nil
}

func hashPlainValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return enrollment.HashToken(value)
}

func splitFormValues(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func commandUpsertFromForm(r *http.Request) (commands.Upsert, error) {
	if err := r.ParseForm(); err != nil {
		return commands.Upsert{}, err
	}
	var payload map[string]any
	rawPayload := strings.TrimSpace(r.FormValue("payload"))
	if rawPayload != "" {
		if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
			return commands.Upsert{}, fmt.Errorf("invalid payload json")
		}
	}
	var expiresAt *time.Time
	if raw := strings.TrimSpace(r.FormValue("expiresAt")); raw != "" {
		parsed, err := time.ParseInLocation("2006-01-02T15:04", raw, time.Local)
		if err != nil {
			parsed, err = time.ParseInLocation("2006-01-02T15:04:05", raw, time.Local)
		}
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, raw)
		}
		if err != nil {
			return commands.Upsert{}, fmt.Errorf("invalid expiry")
		}
		expiresAt = &parsed
	}
	targetType := strings.TrimSpace(r.FormValue("targetType"))
	if targetType == "" {
		targetType = commands.TargetDevice
	}
	switch targetType {
	case commands.TargetDevice, commands.TargetGroup:
	default:
		return commands.Upsert{}, fmt.Errorf("broadcast commands are disabled")
	}
	return commands.Upsert{Type: strings.TrimSpace(r.FormValue("type")), Payload: payload, ExpiresAt: expiresAt, Target: commands.Target{Type: targetType, DeviceID: strings.TrimSpace(r.FormValue("targetDeviceId")), GroupID: strings.TrimSpace(r.FormValue("targetGroupId"))}}, nil
}

func managedFileUpsertFromMultipart(r *http.Request) (files.FileUpsert, managedfiles.ManagedFileUpsert, []byte, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return files.FileUpsert{}, managedfiles.ManagedFileUpsert{}, nil, err
	}
	content, err := uploadedContent(r)
	if err != nil {
		return files.FileUpsert{}, managedfiles.ManagedFileUpsert{}, nil, err
	}
	path := strings.TrimSpace(r.FormValue("path"))
	if path == "" {
		return files.FileUpsert{}, managedfiles.ManagedFileUpsert{}, nil, fmt.Errorf("path is required")
	}
	filename := uploadedFileName(r)
	return files.FileUpsert{
			Name:       managedFileName(path, filename),
			StorageKey: managedFileStorageKey(path, filename),
			Checksum:   checksum.SHA256Base64URL(content),
			SizeBytes:  int64(len(content)),
			MimeType:   managedFileMimeType(filename, content),
		},
		managedfiles.ManagedFileUpsert{
			Path:             path,
			ReplaceVariables: hasFormField(r, "replaceVariables"),
		},
		content,
		nil
}

func certificateUpsertFromMultipart(r *http.Request) (certificates.CertificateUpsert, []byte, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return certificates.CertificateUpsert{}, nil, err
	}
	content, err := uploadedContent(r)
	if err != nil {
		return certificates.CertificateUpsert{}, nil, err
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return certificates.CertificateUpsert{}, nil, fmt.Errorf("name is required")
	}
	filename := uploadedFileName(r)
	return certificates.CertificateUpsert{
		Name:       name,
		StorageKey: certificateStorageKey(name, filename),
		Checksum:   checksum.SHA256Base64URL(content),
		SizeBytes:  int64(len(content)),
		MimeType:   certificateMimeType(filename, content),
	}, content, nil
}

func uploadedContent(r *http.Request) ([]byte, error) {
	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("file is required")
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("file is empty")
	}
	return content, nil
}

func uploadedFileName(r *http.Request) string {
	if r.MultipartForm == nil {
		return ""
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 || files[0] == nil {
		return ""
	}
	return strings.TrimSpace(files[0].Filename)
}

func certificateStorageKey(name, filename string) string {
	base := strings.TrimSpace(filepath.Base(filename))
	if base == "" || base == "." {
		base = strings.TrimSpace(name)
	}
	if base == "" {
		base = "certificate"
	}
	base = strings.NewReplacer("/", "_", "\\", "_").Replace(base)
	return "artifacts/certificates/" + uuid.NewString() + "/" + base
}

func managedFileName(path, filename string) string {
	base := strings.TrimSpace(filepath.Base(filename))
	if base == "" || base == "." {
		base = strings.TrimSpace(filepath.Base(path))
	}
	if base == "" || base == "." {
		base = "managed-file"
	}
	base = strings.NewReplacer("/", "_", "\\", "_").Replace(base)
	pathSuffix := checksum.SHA256Base64URL([]byte(path))
	if len(pathSuffix) > 8 {
		pathSuffix = pathSuffix[:8]
	}
	uploadSuffix := uuid.NewString()
	if len(uploadSuffix) > 8 {
		uploadSuffix = uploadSuffix[:8]
	}
	return "managed-" + base + "-" + pathSuffix + "-" + uploadSuffix
}

func managedFileStorageKey(path, filename string) string {
	base := strings.TrimSpace(filepath.Base(filename))
	if base == "" || base == "." {
		base = strings.TrimSpace(filepath.Base(path))
	}
	if base == "" || base == "." {
		base = "managed-file"
	}
	base = strings.NewReplacer("/", "_", "\\", "_").Replace(base)
	return "artifacts/managed-files/" + uuid.NewString() + "/" + base
}

func managedFileMimeType(filename string, content []byte) string {
	if ext := strings.ToLower(filepath.Ext(filename)); ext != "" {
		if found := mime.TypeByExtension(ext); found != "" {
			return found
		}
	}
	if detected := http.DetectContentType(content); detected != "" {
		return detected
	}
	return "application/octet-stream"
}

func certificateMimeType(filename string, content []byte) string {
	if ext := strings.ToLower(filepath.Ext(filename)); ext != "" {
		if ext == ".pem" {
			return "application/x-pem-file"
		}
		if ext == ".cer" || ext == ".crt" || ext == ".der" {
			return "application/x-x509-ca-cert"
		}
		if ext == ".p12" || ext == ".pfx" {
			return "application/x-pkcs12"
		}
		if found := mime.TypeByExtension(ext); found != "" {
			return found
		}
	}
	if detected := http.DetectContentType(content); detected != "" {
		return detected
	}
	return "application/octet-stream"
}

func logFilterFromQuery(r *http.Request) (logs.SearchFilter, error) {
	return logs.SearchFilter{DeviceID: r.URL.Query().Get("deviceId"), Source: r.URL.Query().Get("source"), Level: r.URL.Query().Get("level"), Query: r.URL.Query().Get("q"), Limit: queryLimit(r, 25)}, nil
}

func deviceInfoFilterFromQuery(r *http.Request) (deviceinfo.SearchFilter, error) {
	return deviceinfo.SearchFilter{DeviceID: r.URL.Query().Get("deviceId"), Query: r.URL.Query().Get("q"), Limit: queryLimit(r, 25)}, nil
}

func queryLimit(r *http.Request, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return fallback
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return fallback
	}
	return limit
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
		b.WriteString(`<tr><td>` + strconv.Itoa(i+1) + `</td><td>` + string(renderStructuredValue(v.Index(i), depth+1)) + `</td></tr>`)
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

func usersTable(items []identity.User, roles map[string]identity.Role, csrf string, canWrite bool) template.HTML {
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Created</th><th>ID</th><th>Email</th><th>Role</th><th>Status</th></tr></thead><tbody>`)
	for _, item := range items {
		roleLabel := item.RoleID
		roleLink := ""
		if role, ok := roles[item.RoleID]; ok {
			roleLabel = role.Name
			roleLink = "/admin/roles/" + role.ID
		}
		roleHTML := esc(roleLabel)
		if roleLink != "" {
			roleHTML = "<a href=\"" + escAttr(roleLink) + "\">" + roleHTML + "</a>"
		}
		b.WriteString("<tr><td>" + esc(formatDashboardTime(item.CreatedAt)) + "</td><td>" + esc(item.ID) + "</td><td><a href=\"/admin/users/" + escAttr(item.ID) + "\">" + esc(item.Email) + "</a></td><td>" + roleHTML + "</td><td>" + statusBadge(item.Status) + "</td></tr>")
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func rolesTable(items []identity.Role, csrf string, canWrite bool) template.HTML {
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Created</th><th>ID</th><th>Name</th><th>Permissions</th><th>Status</th></tr></thead><tbody>`)
	for _, item := range items {
		perms, _ := json.Marshal(item.Permissions)
		b.WriteString("<tr><td>" + esc(formatDashboardTime(item.CreatedAt)) + "</td><td>" + esc(item.ID) + "</td><td><a href=\"/admin/roles/" + escAttr(item.ID) + "\">" + esc(item.Name) + "</a></td><td><code>" + esc(string(perms)) + "</code></td><td>" + statusBadge(item.Status) + "</td></tr>")
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func formatDashboardTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("Jan 2, 2006 15:04 MST")
}

func permissionsHelp() template.HTML {
	perms := auth.AllPermissions()
	var b strings.Builder
	b.WriteString(`<div class="field-help">Available permissions:<div class="permission-catalog">`)
	for _, perm := range perms {
		b.WriteString(`<code>` + esc(string(perm)) + `</code>`)
	}
	b.WriteString(`</div><div class="muted">Use a JSON array such as <code>["admin.read","admin.write"]</code>.</div></div>`)
	return template.HTML(b.String())
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

func allRoleOptions(items []identity.Role) []optionData {
	options := make([]optionData, 0, len(items))
	for _, item := range items {
		label := item.Name
		if strings.TrimSpace(label) == "" {
			label = item.ID
		}
		if item.Status != "active" {
			label += " (" + item.Status + ")"
		}
		options = append(options, optionData{Value: item.ID, Label: label})
	}
	return options
}

func allPolicyOptions(items []policy.Policy) []optionData {
	options := make([]optionData, 0, len(items))
	for _, item := range items {
		label := item.Name
		if strings.TrimSpace(label) == "" {
			label = item.ID
		}
		if item.Status != "active" {
			label += " (" + item.Status + ")"
		}
		options = append(options, optionData{Value: item.ID, Label: label})
	}
	return options
}

func groupNameByID(items []group.Group) map[string]group.Group {
	groups := make(map[string]group.Group, len(items))
	for _, item := range items {
		groups[item.ID] = item
	}
	return groups
}

func groupDevicesFor(groupID string, items []device.Device) []device.Device {
	devices := make([]device.Device, 0)
	for _, item := range items {
		for _, assigned := range item.GroupIDs {
			if strings.TrimSpace(assigned) == groupID {
				devices = append(devices, item)
				break
			}
		}
	}
	return devices
}

func policyNameByID(items []policy.Policy) map[string]policy.Policy {
	policies := make(map[string]policy.Policy, len(items))
	for _, item := range items {
		policies[item.ID] = item
	}
	return policies
}

func containsString(items []string, value string) bool {
	value = strings.TrimSpace(value)
	for _, item := range items {
		if strings.TrimSpace(item) == value {
			return true
		}
	}
	return false
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

func firstPolicyID(policyID *string) string {
	if policyID == nil {
		return ""
	}
	return strings.TrimSpace(*policyID)
}

func roleNameByID(items []identity.Role) map[string]identity.Role {
	roles := make(map[string]identity.Role, len(items))
	for _, item := range items {
		roles[item.ID] = item
	}
	return roles
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

func groupsTable(items []group.Group, csrf string, canWrite bool) template.HTML {
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Created</th><th>ID</th><th>Name</th><th>Status</th></tr></thead><tbody>`)
	for _, item := range items {
		label := item.Name
		if strings.TrimSpace(label) == "" {
			label = item.ID
		}
		b.WriteString("<tr><td>" + esc(formatDashboardTime(item.CreatedAt)) + "</td><td>" + esc(item.ID) + "</td><td><a href=\"/admin/groups/" + escAttr(item.ID) + "\">" + esc(label) + "</a></td><td>" + statusBadge(item.Status) + "</td></tr>")
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func policiesTable(items []policy.Policy, csrf string, canWrite bool) template.HTML {
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Created</th><th>ID</th><th>Name</th><th>Kiosk</th><th>Status</th></tr></thead><tbody>`)
	for _, item := range items {
		b.WriteString("<tr><td>" + esc(formatDashboardTime(item.CreatedAt)) + "</td><td>" + esc(item.ID) + "</td><td><a href=\"/admin/policies/" + escAttr(item.ID) + "\">" + esc(item.Name) + "</a></td><td>" + boolBadge(item.KioskMode, "enabled", "disabled") + "</td><td>" + statusBadge(item.Status) + "</td></tr>")
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func devicesTable(items []device.Device, policies []policy.Policy, csrf string, canWrite bool) template.HTML {
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Created</th><th>ID</th><th>Name</th><th>Policy</th><th>Status</th></tr></thead><tbody>`)
	policyMap := policyNameByID(policies)
	for _, item := range items {
		policyID := ""
		if item.PolicyID != nil {
			policyID = *item.PolicyID
		}
		policyLabel := policyID
		if rec, ok := policyMap[policyID]; ok {
			policyLabel = rec.Name
			if rec.Status != "active" {
				policyLabel += " (" + rec.Status + ")"
			}
		}
		b.WriteString("<tr><td>" + esc(formatDashboardTime(item.CreatedAt)) + "</td><td>" + esc(item.ID) + "</td><td><a href=\"/admin/devices/" + escAttr(item.ID) + "\">" + esc(item.Name) + "</a></td><td>")
		if policyID != "" {
			if rec, ok := policyMap[policyID]; ok {
				b.WriteString(`<a href="/admin/policies/` + escAttr(rec.ID) + `">` + esc(policyLabel) + `</a>`)
			} else {
				b.WriteString(esc(policyLabel))
			}
		} else {
			b.WriteString("—")
		}
		b.WriteString("</td><td>" + statusBadge(item.Status) + "</td></tr>")
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func appsTable(items []apps.App, versions map[string][]apps.Version) template.HTML {
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Created</th><th>ID</th><th>Name</th><th>Package</th><th>Latest published</th><th>Status</th></tr></thead><tbody>`)
	for _, item := range items {
		latest := appLatestPublishedVersion(versions[item.ID])
		b.WriteString("<tr><td>" + esc(formatDashboardTime(item.CreatedAt)) + "</td><td>" + esc(item.ID) + "</td><td><a href=\"/admin/apps/" + escAttr(item.ID) + "\">" + esc(item.Name) + "</a></td><td>" + esc(item.PackageName) + "</td><td>" + esc(appVersionLabel(latest)) + "</td><td>" + statusBadge(item.Status) + "</td></tr>")
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func appLatestPublishedVersion(items []apps.Version) *apps.Version {
	var latest *apps.Version
	for i := range items {
		if items[i].Status != apps.VersionStatusPublished {
			continue
		}
		if latest == nil {
			latest = &items[i]
			continue
		}
		if items[i].PublishedAt != nil && latest.PublishedAt != nil {
			if items[i].PublishedAt.After(*latest.PublishedAt) {
				latest = &items[i]
			}
			continue
		}
		if items[i].PublishedAt != nil && latest.PublishedAt == nil {
			latest = &items[i]
			continue
		}
		if items[i].PublishedAt == nil && latest.PublishedAt == nil && items[i].CreatedAt.After(latest.CreatedAt) {
			latest = &items[i]
		}
	}
	return latest
}

func appVersionLabel(item *apps.Version) string {
	if item == nil {
		return "—"
	}
	return item.VersionName + " (#" + strconv.FormatInt(item.VersionCode, 10) + ")"
}

func appSummary(found apps.App, latest *apps.Version) template.HTML {
	var b strings.Builder
	b.WriteString(`<div class="policy-summary">`)
	b.WriteString(summaryTextItem("Created", formatDashboardTime(found.CreatedAt)))
	b.WriteString(summaryTextItem("Updated", formatDashboardTime(found.UpdatedAt)))
	b.WriteString(summaryTextItem("ID", found.ID))
	b.WriteString(summaryTextItem("Name", found.Name))
	b.WriteString(summaryTextItem("Package", found.PackageName))
	if latest != nil && (latest.ArtifactID != nil || latest.Artifact != nil) {
		b.WriteString(summaryHTMLItem("Download", template.HTML(`<a class="button btn-primary" href="/admin/apps/`+escAttr(found.ID)+`/download">Download latest APK</a>`)))
	}
	b.WriteString(summaryHTMLItem("Status", template.HTML(statusBadge(found.Status))))
	b.WriteString(`</div>`)
	b.WriteString(string(rawDataDetails("Raw app data", found)))
	if latest != nil {
		b.WriteString(string(rawDataDetails("Raw latest version data", latest)))
	}
	return template.HTML(b.String())
}

func appVersionsTable(items []apps.Version) template.HTML {
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Created</th><th>ID</th><th>Name</th><th>Code</th><th>Status</th><th>Published</th><th>Artifact</th></tr></thead><tbody>`)
	for _, item := range items {
		published := "—"
		if item.PublishedAt != nil {
			published = formatDashboardTime(*item.PublishedAt)
		}
		artifact := item.Checksum
		if item.ArtifactID != nil && strings.TrimSpace(*item.ArtifactID) != "" {
			artifact = *item.ArtifactID
		}
		b.WriteString("<tr><td>" + esc(formatDashboardTime(item.CreatedAt)) + "</td><td>" + esc(item.ID) + "</td><td>" + esc(item.VersionName) + "</td><td>" + esc(strconv.FormatInt(item.VersionCode, 10)) + "</td><td>" + statusBadge(item.Status) + "</td><td>" + esc(published) + "</td><td><code>" + esc(artifact) + "</code></td></tr>")
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func managedFilesTable(items []managedfiles.ManagedFile) template.HTML {
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Created</th><th>ID</th><th>Path</th><th>File</th><th>Template</th><th>Status</th></tr></thead><tbody>`)
	for _, item := range items {
		path := item.Path
		if strings.TrimSpace(path) == "" {
			path = item.ID
		}
		fileLabel := item.FileID
		if item.File != nil && strings.TrimSpace(item.File.Name) != "" {
			fileLabel = item.File.Name
		}
		b.WriteString("<tr><td>" + esc(formatDashboardTime(item.CreatedAt)) + "</td><td>" + esc(item.ID) + "</td><td><a href=\"/admin/managed-files/" + escAttr(item.ID) + "\">" + esc(path) + "</a></td><td>" + esc(fileLabel) + "</td><td>" + boolBadge(item.ReplaceVariables, "enabled", "disabled") + "</td><td>" + statusBadge(item.Status) + "</td></tr>")
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func certificatesTable(items []certificates.Certificate) template.HTML {
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Created</th><th>ID</th><th>Name</th><th>Artifact</th><th>Status</th></tr></thead><tbody>`)
	for _, item := range items {
		artifact := item.ArtifactID
		if item.Artifact != nil && strings.TrimSpace(item.Artifact.StorageKey) != "" {
			artifact = item.Artifact.StorageKey
		}
		b.WriteString("<tr><td>" + esc(formatDashboardTime(item.CreatedAt)) + "</td><td>" + esc(item.ID) + "</td><td><a href=\"/admin/certificates/" + escAttr(item.ID) + "\">" + esc(item.Name) + "</a></td><td>" + esc(artifact) + "</td><td>" + statusBadge(item.Status) + "</td></tr>")
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func commandsTable(items []commands.Command, devices map[string]device.Device) template.HTML {
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Created</th><th>ID</th><th>Type</th><th>Device</th><th>Status</th><th>Expires</th></tr></thead><tbody>`)
	for _, item := range items {
		deviceLabel := item.DeviceID
		if rec, ok := devices[item.DeviceID]; ok {
			deviceLabel = rec.Name
			if strings.TrimSpace(deviceLabel) == "" {
				deviceLabel = rec.ID
			}
			deviceLabel = `<a href="/admin/devices/` + escAttr(rec.ID) + `">` + esc(deviceLabel) + `</a>`
		} else {
			deviceLabel = esc(deviceLabel)
		}
		commandLink := `<a href="/admin/commands/` + escAttr(item.ID) + `">` + esc(item.ID) + `</a>`
		expires := "—"
		if item.ExpiresAt != nil {
			expires = formatDashboardTime(*item.ExpiresAt)
		}
		b.WriteString("<tr><td>" + esc(formatDashboardTime(item.CreatedAt)) + "</td><td>" + commandLink + "</td><td>" + esc(item.Type) + "</td><td>" + deviceLabel + "</td><td>" + statusBadge(item.Status) + "</td><td>" + esc(expires) + "</td></tr>")
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func commandFields(devices []device.Device, groups []group.Group) []fieldData {
	return []fieldData{
		{Name: "type", Label: "Command", Type: "select", Value: "ping", Required: true, Options: commandTypeOptions()},
		{Name: "targetType", Label: "Target type", Type: "select", Value: commands.TargetDevice, Required: true, Options: []optionData{{Value: commands.TargetDevice, Label: "Device"}, {Value: commands.TargetGroup, Label: "Group"}}},
		{Name: "targetDeviceId", Label: "Device", Type: "select", Placeholder: "Select device", Options: commandDeviceOptions(devices)},
		{Name: "targetGroupId", Label: "Group", Type: "select", Placeholder: "Select group", Options: commandGroupOptions(groups)},
		{Name: "payload", Label: "Payload JSON", Type: "textarea", Value: "{}"},
		{Name: "expiresAt", Label: "Expires at", Type: "datetime-local"},
	}
}

func commandTypeOptions() []optionData {
	return []optionData{
		{Value: "ping", Label: "ping"},
		{Value: "reboot", Label: "reboot"},
		{Value: "sync_config", Label: "sync_config"},
		{Value: "exit_kiosk", Label: "exit_kiosk"},
	}
}

func commandDeviceOptions(items []device.Device) []optionData {
	options := make([]optionData, 0, len(items))
	for _, item := range items {
		if item.Status != device.StatusActive && item.Status != device.StatusEnrolled {
			continue
		}
		label := item.Name
		if strings.TrimSpace(label) == "" {
			label = item.ID
		}
		options = append(options, optionData{Value: item.ID, Label: label})
	}
	return options
}

func commandGroupOptions(items []group.Group) []optionData {
	options := make([]optionData, 0, len(items))
	for _, item := range items {
		if item.Status != "active" {
			continue
		}
		label := item.Name
		if strings.TrimSpace(label) == "" {
			label = item.ID
		}
		options = append(options, optionData{Value: item.ID, Label: label})
	}
	return options
}

func activeGroupOptions(items []group.Group) []optionData {
	options := make([]optionData, 0, len(items))
	for _, item := range items {
		if item.Status != "active" {
			continue
		}
		label := item.Name
		if strings.TrimSpace(label) == "" {
			label = item.ID
		}
		options = append(options, optionData{Value: item.ID, Label: label})
	}
	return options
}

func commandSummary(found commands.Command, deviceRec device.Device) template.HTML {
	deviceLabel := found.DeviceID
	if strings.TrimSpace(deviceRec.ID) != "" {
		deviceLabel = deviceRec.Name
		if strings.TrimSpace(deviceLabel) == "" {
			deviceLabel = deviceRec.ID
		}
		deviceLabel = `<a href="/admin/devices/` + escAttr(deviceRec.ID) + `">` + esc(deviceLabel) + `</a>`
	} else {
		deviceLabel = esc(deviceLabel)
	}
	expires := "—"
	if found.ExpiresAt != nil {
		expires = formatDashboardTime(*found.ExpiresAt)
	}
	acked := "—"
	if found.AckedAt != nil {
		acked = formatDashboardTime(*found.AckedAt)
	}
	var b strings.Builder
	b.WriteString(`<h2>Current command</h2><div class="policy-summary">`)
	b.WriteString(summaryTextItem("Created", formatDashboardTime(found.CreatedAt)))
	b.WriteString(summaryTextItem("Updated", formatDashboardTime(found.UpdatedAt)))
	b.WriteString(summaryTextItem("ID", found.ID))
	b.WriteString(summaryTextItem("Type", found.Type))
	b.WriteString(summaryHTMLItem("Device", template.HTML(deviceLabel)))
	b.WriteString(summaryHTMLItem("Status", template.HTML(statusBadge(found.Status))))
	b.WriteString(summaryTextItem("Expires", expires))
	b.WriteString(summaryTextItem("Acked", acked))
	b.WriteString(`<div class="policy-summary-item policy-summary-wide"><div class="policy-summary-label">Payload</div><div class="policy-summary-value">` + preStructuredOnly(found.Payload) + `</div></div>`)
	if len(found.Result) > 0 {
		b.WriteString(`<div class="policy-summary-item policy-summary-wide"><div class="policy-summary-label">Result</div><div class="policy-summary-value">` + preStructuredOnly(found.Result) + `</div></div>`)
	} else {
		b.WriteString(summaryTextItem("Result", "—"))
	}
	b.WriteString(`</div>`)
	b.WriteString(string(rawDataDetails("Raw command data", found)))
	return template.HTML(b.String())
}

func logsTable(items []logs.Record) template.HTML {
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Observed</th><th>Device</th><th>Level</th><th>Source</th><th>Message</th></tr></thead><tbody>`)
	for _, item := range items {
		b.WriteString("<tr><td>" + esc(item.ObservedAt.Format(time.RFC3339)) + "</td><td>" + esc(item.DeviceID) + "</td><td>" + esc(item.Level) + "</td><td>" + esc(item.Source) + "</td><td>" + esc(item.Message) + "</td></tr>")
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func deviceInfoTable(items []deviceinfo.Record) template.HTML {
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Observed</th><th>Device</th><th>Payload</th></tr></thead><tbody>`)
	for _, item := range items {
		b.WriteString("<tr><td>" + esc(item.ObservedAt.Format(time.RFC3339)) + "</td><td>" + esc(item.DeviceID) + "</td><td>" + pre(item.Payload) + "</td></tr>")
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func auditTable(items []audit.Event) template.HTML {
	var b strings.Builder
	b.WriteString(`<table class="audit-table"><colgroup><col class="audit-created"><col class="audit-actor"><col class="audit-action"><col class="audit-resource"><col class="audit-details"></colgroup><thead><tr><th class="audit-created">Created</th><th class="audit-actor">Actor</th><th class="audit-action">Action</th><th class="audit-resource">Resource</th><th class="audit-details">Details</th></tr></thead><tbody>`)
	for _, item := range items {
		b.WriteString(`<tr>`)
		b.WriteString(`<td class="audit-created">` + esc(item.CreatedAt.Format(time.RFC3339)) + `</td>`)
		b.WriteString(`<td class="audit-actor">` + esc(item.Actor) + `</td>`)
		b.WriteString(`<td class="audit-action">` + esc(item.Action) + `</td>`)
		b.WriteString(`<td class="audit-resource">` + esc(item.ResourceType+"/"+item.ResourceID) + `</td>`)
		b.WriteString(`<td class="audit-details">` + pre(item.Details) + `</td>`)
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</tbody></table>`)
	return template.HTML(b.String())
}

func searchAndTable(action string, fields []fieldData, table template.HTML) template.HTML {
	var b strings.Builder
	b.WriteString(`<form method="get" action="` + escAttr(action) + `" class="panel">`)
	for _, field := range fields {
		b.WriteString(`<label for="` + escAttr(field.Name) + `">` + esc(field.Label) + `</label><input id="` + escAttr(field.Name) + `" name="` + escAttr(field.Name) + `" type="` + escAttr(field.Type) + `" value="` + escAttr(field.Value) + `">`)
	}
	b.WriteString(`<p><button type="submit">Search</button></p></form>`)
	b.WriteString(string(table))
	return template.HTML(b.String())
}

func policyFields(item policy.Policy) []fieldData {
	restrictions := policyRestrictionFormData(item.Restrictions)
	kioskModeValue := ""
	if item.KioskMode {
		kioskModeValue = "on"
	}
	return []fieldData{
		{Name: "name", Label: "Name", Type: "text", Value: item.Name, Placeholder: "default-policy", Required: true},
		{Name: "kioskAppPackage", Label: "Kiosk app package", Type: "text", Value: item.KioskAppPackage, Placeholder: "com.android.chrome"},
		{Name: "kioskMode", Label: "Enable kiosk mode", Type: "checkbox", Value: kioskModeValue},
		{Name: "kioskExitPasscode", Label: "Kiosk exit passcode", Type: "text", Value: restrictions.KioskExitPasscode, Placeholder: "1234"},
		{Name: "allowPackages", Label: "Allow packages", Type: "textarea", Value: restrictions.AllowPackages, Placeholder: "One package per line\ncom.android.chrome\ncom.android.settings"},
		{Name: "blockPackages", Label: "Block packages", Type: "textarea", Value: restrictions.BlockPackages, Placeholder: "One package per line\ncom.android.camera\ncom.android.phone"},
		{Name: "suspendPackages", Label: "Suspend packages", Type: "textarea", Value: restrictions.SuspendPackages, Placeholder: "One package per line\ncom.android.music\ncom.example.social"},
		{Name: "kioskKeepScreenOn", Label: "Keep screen on", Type: "checkbox", Value: boolValue(restrictions.KioskKeepScreenOn)},
		{Name: "kioskStayAwakeWhilePluggedIn", Label: "Stay awake while plugged in", Type: "checkbox", Value: boolValue(restrictions.KioskStayAwakeWhilePluggedIn)},
		{Name: "kioskUnlockOnBoot", Label: "Unlock on boot", Type: "checkbox", Value: boolValue(restrictions.KioskUnlockOnBoot)},
	}
}

func (d *dashboard) policyDetailPageData(session *auth.Session, csrf string, found policy.Policy, items []apps.App, assignments []policy.PolicyApp, certificatesList []certificates.Certificate, certificateAssignments []policy.PolicyCertificate, managedFiles []managedfiles.ManagedFile, managedFileAssignments []policy.PolicyManagedFile) pageData {
	managedApps := policyManagedAppsSummary(found.ID, items, assignments, csrf, d.canWrite(session) && found.Status != "retired")
	managedCertificates := policyManagedCertificatesSummary(found.ID, certificatesList, certificateAssignments, csrf, d.canWrite(session) && found.Status != "retired")
	managedFileBindings := policyManagedFilesSummary(found.ID, managedFiles, managedFileAssignments, csrf, d.canWrite(session) && found.Status != "retired")
	data := pageData{
		Title:    "Policy Detail",
		Subtitle: "Edit the policy definition, enable managed apps, managed files, and certificates, or retire it from active use.",
		Items:    template.HTML(string(policySummary(found)) + string(managedApps) + string(managedFileBindings) + string(managedCertificates)),
	}
	if d.canWrite(session) && found.Status != "retired" {
		data.Forms = []formData{{
			Title:  "Update policy",
			Action: "/admin/policies/" + found.ID + "/update",
			Fields: policyFields(found),
			Submit: "Update policy",
		}, {
			Title:  "Retire policy",
			Action: "/admin/policies/" + found.ID + "/retire",
			Submit: "Retire policy",
			Danger: true,
		}}
	}
	return data
}

func policyManagedAppsSummary(policyID string, items []apps.App, assignments []policy.PolicyApp, csrf string, canWrite bool) template.HTML {
	activeCount := 0
	for _, app := range items {
		if app.Status == apps.StatusActive {
			activeCount++
		}
	}
	if activeCount == 0 {
		return template.HTML(`<section class="panel policy-detail-section"><h2 class="section-heading"><span>Managed apps</span><span class="section-heading-meta">0 available</span></h2><p class="muted">No managed apps are available yet.</p></section>`)
	}
	assignmentMap := appAssignmentStatusByID(assignments)
	enabledCount := 0
	for _, app := range items {
		if app.Status == apps.StatusActive && assignmentMap[app.ID] {
			enabledCount++
		}
	}
	var b strings.Builder
	b.WriteString(`<section class="panel policy-detail-section"><h2 class="section-heading"><span>Managed apps</span><span class="section-heading-meta">` + strconv.Itoa(enabledCount) + ` enabled / ` + strconv.Itoa(activeCount) + ` available</span></h2><table><thead><tr><th>Name</th><th>Package</th><th>Status</th><th>Policy state</th><th>Action</th></tr></thead><tbody>`)
	for _, app := range items {
		if app.Status != apps.StatusActive {
			continue
		}
		enabled := assignmentMap[app.ID]
		label := app.Name
		if strings.TrimSpace(label) == "" {
			label = app.PackageName
		}
		b.WriteString(`<tr><td><a href="/admin/apps/` + escAttr(app.ID) + `">` + esc(label) + `</a></td><td>` + esc(app.PackageName) + `</td><td>` + statusBadge(app.Status) + `</td><td>` + boolBadge(enabled, "enabled", "disabled") + `</td><td>`)
		if canWrite {
			b.WriteString(policyAppToggleForm("/admin/policies/"+policyID+"/apps/"+app.ID+"/toggle", csrf, enabled))
		}
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</tbody></table></section>`)
	return template.HTML(b.String())
}

func policyManagedCertificatesSummary(policyID string, items []certificates.Certificate, assignments []policy.PolicyCertificate, csrf string, canWrite bool) template.HTML {
	activeCount := 0
	for _, cert := range items {
		if cert.Status == certificates.StatusActive {
			activeCount++
		}
	}
	if activeCount == 0 {
		return template.HTML(`<section class="panel policy-detail-section"><h2 class="section-heading"><span>Managed certificates</span><span class="section-heading-meta">0 available</span></h2><p class="muted">No managed certificates are available yet.</p></section>`)
	}
	assignmentMap := certificateAssignmentStatusByID(assignments)
	enabledCount := 0
	for _, cert := range items {
		if cert.Status == certificates.StatusActive && assignmentMap[cert.ID] {
			enabledCount++
		}
	}
	var b strings.Builder
	b.WriteString(`<section class="panel policy-detail-section"><h2 class="section-heading"><span>Managed certificates</span><span class="section-heading-meta">` + strconv.Itoa(enabledCount) + ` enabled / ` + strconv.Itoa(activeCount) + ` available</span></h2><table><thead><tr><th>Name</th><th>Artifact</th><th>Status</th><th>Policy state</th><th>Action</th></tr></thead><tbody>`)
	for _, cert := range items {
		if cert.Status != certificates.StatusActive {
			continue
		}
		enabled := assignmentMap[cert.ID]
		label := cert.Name
		if strings.TrimSpace(label) == "" {
			label = cert.ID
		}
		artifact := cert.ArtifactID
		if cert.Artifact != nil && strings.TrimSpace(cert.Artifact.StorageKey) != "" {
			artifact = cert.Artifact.StorageKey
		}
		b.WriteString(`<tr><td><a href="/admin/certificates/` + escAttr(cert.ID) + `">` + esc(label) + `</a></td><td>` + esc(artifact) + `</td><td>` + statusBadge(cert.Status) + `</td><td>` + boolBadge(enabled, "enabled", "disabled") + `</td><td>`)
		if canWrite {
			b.WriteString(policyCertificateToggleForm("/admin/policies/"+policyID+"/certificates/"+cert.ID+"/toggle", csrf, enabled))
		}
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</tbody></table></section>`)
	return template.HTML(b.String())
}

func policyManagedFilesSummary(policyID string, items []managedfiles.ManagedFile, assignments []policy.PolicyManagedFile, csrf string, canWrite bool) template.HTML {
	activeCount := 0
	for _, item := range items {
		if item.Status == managedfiles.StatusActive {
			activeCount++
		}
	}
	if activeCount == 0 {
		return template.HTML(`<section class="panel policy-detail-section"><h2 class="section-heading"><span>Managed files</span><span class="section-heading-meta">0 available</span></h2><p class="muted">No managed files are available yet.</p></section>`)
	}
	assignmentMap := managedFileAssignmentStatusByID(assignments)
	enabledCount := 0
	for _, item := range items {
		if item.Status == managedfiles.StatusActive && assignmentMap[item.ID] {
			enabledCount++
		}
	}
	var b strings.Builder
	b.WriteString(`<section class="panel policy-detail-section"><h2 class="section-heading"><span>Managed files</span><span class="section-heading-meta">` + strconv.Itoa(enabledCount) + ` enabled / ` + strconv.Itoa(activeCount) + ` available</span></h2><table><thead><tr><th>Path</th><th>File</th><th>Template</th><th>Status</th><th>Policy state</th><th>Action</th></tr></thead><tbody>`)
	for _, item := range items {
		if item.Status != managedfiles.StatusActive {
			continue
		}
		enabled := assignmentMap[item.ID]
		path := item.Path
		if strings.TrimSpace(path) == "" {
			path = item.ID
		}
		fileLabel := item.FileID
		if item.File != nil && strings.TrimSpace(item.File.Name) != "" {
			fileLabel = item.File.Name
		}
		b.WriteString(`<tr><td><a href="/admin/managed-files/` + escAttr(item.ID) + `">` + esc(path) + `</a></td><td>` + esc(fileLabel) + `</td><td>` + boolBadge(item.ReplaceVariables, "enabled", "disabled") + `</td><td>` + statusBadge(item.Status) + `</td><td>` + boolBadge(enabled, "enabled", "disabled") + `</td><td>`)
		if canWrite {
			b.WriteString(policyManagedFileToggleForm("/admin/policies/"+policyID+"/managed-files/"+item.ID+"/toggle", csrf, enabled))
		}
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</tbody></table></section>`)
	return template.HTML(b.String())
}

func policySummary(found policy.Policy) template.HTML {
	kioskAppPackage := found.KioskAppPackage
	if strings.TrimSpace(kioskAppPackage) == "" {
		kioskAppPackage = "—"
	}
	var b strings.Builder
	b.WriteString(`<section class="panel"><h2 class="section-heading"><span>Current policy</span><span class="section-heading-meta">` + esc(found.Status) + ` · v` + strconv.Itoa(found.Version) + `</span></h2><div class="policy-summary">`)
	b.WriteString(summaryTextItem("Created", formatDashboardTime(found.CreatedAt)))
	b.WriteString(summaryTextItem("Updated", formatDashboardTime(found.UpdatedAt)))
	b.WriteString(summaryTextItem("ID", found.ID))
	b.WriteString(summaryTextItem("Name", found.Name))
	b.WriteString(summaryTextItem("Version", strconv.Itoa(found.Version)))
	b.WriteString(summaryHTMLItem("Kiosk mode", template.HTML(boolBadge(found.KioskMode, "enabled", "disabled"))))
	b.WriteString(summaryTextItem("Kiosk app package", kioskAppPackage))
	b.WriteString(summaryHTMLItem("Status", template.HTML(statusBadge(found.Status))))
	b.WriteString(`<div class="policy-summary-item policy-summary-wide"><div class="policy-summary-label">Stored restrictions</div><div class="policy-summary-value policy-restrictions-value">` + string(renderPolicyRestrictions(found.Restrictions)) + `</div></div>`)
	b.WriteString(`</div>`)
	b.WriteString(string(rawDataDetails("Raw policy data", found)))
	b.WriteString(`</section>`)
	return template.HTML(b.String())
}

func renderPolicyRestrictions(raw json.RawMessage) template.HTML {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return template.HTML(`<span class="structured-muted">No restrictions stored.</span>`)
	}

	var parsed policyRestrictionPayload
	if err := json.Unmarshal(trimmed, &parsed); err != nil {
		return renderStructuredJSON(raw)
	}

	var b strings.Builder
	b.WriteString(`<div class="policy-summary">`)
	b.WriteString(summaryHTMLItem("Allow packages", restrictionPackageList(parsed.AllowPackages)))
	b.WriteString(summaryHTMLItem("Block packages", restrictionPackageList(parsed.BlockPackages)))
	b.WriteString(summaryHTMLItem("Suspend packages", restrictionPackageList(parsed.SuspendPackages)))
	b.WriteString(summaryHTMLItem("Keep screen on", template.HTML(boolBadge(parsed.KioskKeepScreenOn, "enabled", "disabled"))))
	b.WriteString(summaryHTMLItem("Stay awake while plugged in", template.HTML(boolBadge(parsed.KioskStayAwakeWhilePluggedIn, "enabled", "disabled"))))
	b.WriteString(summaryHTMLItem("Unlock on boot", template.HTML(boolBadge(parsed.KioskUnlockOnBoot, "enabled", "disabled"))))
	b.WriteString(summaryHTMLItem("Exit passcode configured", template.HTML(boolBadge(strings.TrimSpace(parsed.KioskExitPasscode) != "", "yes", "no"))))
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

func restrictionPackageList(items []string) template.HTML {
	if len(items) == 0 {
		return template.HTML(`<span class="structured-muted">None</span>`)
	}
	var b strings.Builder
	b.WriteString(`<div class="structured-list">`)
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		b.WriteString(`<span class="structured-pill">` + esc(item) + `</span>`)
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

func summaryTextItem(label, value string) string {
	return `<div class="policy-summary-item"><div class="policy-summary-label">` + esc(label) + `</div><div class="policy-summary-value">` + esc(value) + `</div></div>`
}

func summaryHTMLItem(label string, value template.HTML) string {
	return `<div class="policy-summary-item"><div class="policy-summary-label">` + esc(label) + `</div><div class="policy-summary-value">` + string(value) + `</div></div>`
}

func appAssignmentStatusByID(assignments []policy.PolicyApp) map[string]bool {
	out := make(map[string]bool, len(assignments))
	for _, assignment := range assignments {
		out[assignment.AppID] = assignment.Status == policy.StatusActive
	}
	return out
}

func certificateAssignmentStatusByID(assignments []policy.PolicyCertificate) map[string]bool {
	out := make(map[string]bool, len(assignments))
	for _, assignment := range assignments {
		out[assignment.CertificateID] = assignment.Status == policy.StatusActive
	}
	return out
}

func managedFileAssignmentStatusByID(assignments []policy.PolicyManagedFile) map[string]bool {
	out := make(map[string]bool, len(assignments))
	for _, assignment := range assignments {
		out[assignment.ManagedFileID] = assignment.Status == policy.StatusActive
	}
	return out
}

func policyRestrictionFormData(raw json.RawMessage) policyRestrictionFormState {
	var parsed policyRestrictionPayload
	if len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &parsed)
	}
	return policyRestrictionFormState{
		AllowPackages:                strings.Join(parsed.AllowPackages, "\n"),
		BlockPackages:                strings.Join(parsed.BlockPackages, "\n"),
		SuspendPackages:              strings.Join(parsed.SuspendPackages, "\n"),
		KioskKeepScreenOn:            parsed.KioskKeepScreenOn,
		KioskStayAwakeWhilePluggedIn: parsed.KioskStayAwakeWhilePluggedIn,
		KioskUnlockOnBoot:            parsed.KioskUnlockOnBoot,
		KioskExitPasscode:            strings.TrimSpace(parsed.KioskExitPasscode),
	}
}

type policyRestrictionFormState struct {
	AllowPackages                string
	BlockPackages                string
	SuspendPackages              string
	KioskKeepScreenOn            bool
	KioskStayAwakeWhilePluggedIn bool
	KioskUnlockOnBoot            bool
	KioskExitPasscode            string
}

type policyRestrictionPayload struct {
	AllowPackages                []string `json:"allowPackages,omitempty"`
	BlockPackages                []string `json:"blockPackages,omitempty"`
	SuspendPackages              []string `json:"suspendPackages,omitempty"`
	KioskKeepScreenOn            bool     `json:"kioskKeepScreenOn,omitempty"`
	KioskStayAwakeWhilePluggedIn bool     `json:"kioskStayAwakeWhilePluggedIn,omitempty"`
	KioskUnlockOnBoot            bool     `json:"kioskUnlockOnBoot,omitempty"`
	KioskExitPasscode            string   `json:"kioskExitPasscode,omitempty"`
}

func boolValue(v bool) string {
	if v {
		return "on"
	}
	return ""
}

func certificateUploadForm(action string) formData {
	return formData{
		Title:   "Upload certificate",
		Action:  action,
		EncType: "multipart/form-data",
		Fields: []fieldData{
			{Name: "name", Label: "Name", Type: "text", Placeholder: "Root CA", Required: true},
			{Name: "file", Label: "File", Type: "file", Required: true},
		},
		Help:   "",
		Submit: "Upload certificate",
	}
}

type managedAppCreateRequest struct {
	App         apps.AppUpsert
	VersionName string
	VersionCode int64
	Publish     bool
	File        files.FileUpsert
	StorageKey  string
	MimeType    string
	SizeBytes   int64
}

func managedAppForm(action string) formData {
	return formData{
		Title:   "Create managed app",
		Action:  action,
		EncType: "multipart/form-data",
		Fields: []fieldData{
			{Name: "packageName", Label: "Package name", Type: "text", Placeholder: "com.example.app", Required: true},
			{Name: "name", Label: "App name", Type: "text", Placeholder: "Example App", Required: true},
			{Name: "versionCode", Label: "Version code", Type: "number", Placeholder: "100", Required: true},
			{Name: "file", Label: "APK file", Type: "file", Required: true},
		},
		Help:   template.HTML(`Upload the APK once. The dashboard derives the artifact storage key, checksum, and file record on the server, then creates the app and publishes its first version in one step.`),
		Submit: "Create managed app",
	}
}

func managedAppUpsertFromMultipart(r *http.Request) (managedAppCreateRequest, []byte, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return managedAppCreateRequest{}, nil, err
	}
	versionCode, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("versionCode")), 10, 64)
	if err != nil {
		return managedAppCreateRequest{}, nil, fmt.Errorf("invalid version code")
	}
	app := apps.AppUpsert{PackageName: strings.TrimSpace(r.FormValue("packageName")), Name: strings.TrimSpace(r.FormValue("name"))}
	if app.PackageName == "" || app.Name == "" {
		return managedAppCreateRequest{}, nil, fmt.Errorf("invalid form")
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		return managedAppCreateRequest{}, nil, fmt.Errorf("file is required")
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return managedAppCreateRequest{}, nil, err
	}
	if len(content) == 0 {
		return managedAppCreateRequest{}, nil, fmt.Errorf("file is empty")
	}
	checksumValue := checksum.SHA256Base64URL(content)
	versionName := strings.TrimSpace(r.FormValue("versionName"))
	if versionName == "" {
		versionName = fmt.Sprintf("v%d", versionCode)
	}
	fileName := sanitizeArtifactName(fmt.Sprintf("%s-%d.apk", app.PackageName, versionCode))
	storageKey := "artifacts/apps/" + uuid.NewString() + "/" + fileName
	return managedAppCreateRequest{
		App:         app,
		VersionName: versionName,
		VersionCode: versionCode,
		Publish:     true,
		File: files.FileUpsert{
			Name:       fileName,
			StorageKey: storageKey,
			Checksum:   checksumValue,
			SizeBytes:  int64(len(content)),
			MimeType:   "application/vnd.android.package-archive",
		},
		StorageKey: storageKey,
		MimeType:   "application/vnd.android.package-archive",
		SizeBytes:  int64(len(content)),
	}, content, nil
}

func sanitizeArtifactName(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	value = strings.NewReplacer("\\", "_", "/", "_", " ", "_").Replace(value)
	if value == "." || value == "" {
		return "artifact.apk"
	}
	return value
}

type enrollmentQRFormState struct {
	ServerURL       string
	TTL             string
	ComponentName   string
	PackageURL      string
	PackageChecksum string
	DeviceID        string
}

type enrollmentQRResult struct {
	Issued      enrollment.IssuedToken
	PayloadJSON string
	PNGDataURL  string
}

const (
	deviceEnrollmentTTL        = "2h"
	defaultAgentAppPackage     = "com.xmdm.launcher"
	agentEnrollmentPackagePath = "/api/v1/enrollment/agent.apk"
)

func (d *dashboard) deviceEnrollmentQRState(ctx context.Context, deviceID string) (enrollmentQRFormState, error) {
	serverURL, err := normalizedPublicServerURL(d.deps.ServerPublicURL)
	if err != nil {
		return enrollmentQRFormState{}, err
	}
	appPackage := firstNonEmpty(strings.TrimSpace(d.deps.AgentAppPackage), defaultAgentAppPackage)
	appRecord, latest, err := latestPublishedAgentAppVersion(ctx, d.deps.Apps, d.deps.TenantID, appPackage)
	if err != nil {
		if err == httpx.ErrNotFound {
			return enrollmentQRFormState{}, fmt.Errorf("agent managed app %q must have a latest published APK version", appPackage)
		}
		return enrollmentQRFormState{}, err
	}
	packageURL := strings.TrimRight(serverURL, "/") + agentEnrollmentPackagePath
	return enrollmentQRFormState{
		ServerURL:       serverURL,
		TTL:             deviceEnrollmentTTL,
		ComponentName:   appRecord.PackageName + "/.AdminReceiver",
		PackageURL:      packageURL,
		PackageChecksum: latest.Checksum,
		DeviceID:        deviceID,
	}, nil
}

func normalizedPublicServerURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("server public url is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid server public url")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func latestPublishedAgentAppVersion(ctx context.Context, store apps.Repository, tenantID, agentAppPackage string) (apps.App, apps.Version, error) {
	if store == nil {
		return apps.App{}, apps.Version{}, httpx.ErrNotFound
	}
	packageName := firstNonEmpty(strings.TrimSpace(agentAppPackage), defaultAgentAppPackage)
	items, err := store.ListApps(ctx, tenantID)
	if err != nil {
		return apps.App{}, apps.Version{}, err
	}
	for _, item := range items {
		if item.Status != apps.StatusActive || item.PackageName != packageName {
			continue
		}
		versions, err := store.ListVersions(ctx, tenantID, item.ID)
		if err != nil {
			return apps.App{}, apps.Version{}, err
		}
		latest := appLatestPublishedVersion(versions)
		if latest == nil || latest.ArtifactID == nil || strings.TrimSpace(latest.Checksum) == "" {
			return apps.App{}, apps.Version{}, httpx.ErrNotFound
		}
		return item, *latest, nil
	}
	return apps.App{}, apps.Version{}, httpx.ErrNotFound
}

func deviceEnrollmentQRForm(deviceID string) formData {
	return formData{
		Title:  "Generate enrollment QR",
		Action: "/admin/devices/" + deviceID + "/enrollment/qr",
		Help:   "",
		Submit: "Generate QR",
	}
}

func (d *dashboard) generateEnrollmentQR(ctx context.Context, state enrollmentQRFormState) (enrollmentQRResult, error) {
	ttl, err := time.ParseDuration(strings.TrimSpace(state.TTL))
	if err != nil || ttl <= 0 {
		return enrollmentQRResult{}, fmt.Errorf("invalid ttl")
	}
	issued, err := d.deps.Enrollment.IssueToken(ctx, d.deps.TenantID, time.Now().UTC().Add(ttl))
	if err != nil {
		return enrollmentQRResult{}, err
	}
	payload, err := buildEnrollmentQRPayload(state, issued.Secret)
	if err != nil {
		return enrollmentQRResult{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return enrollmentQRResult{}, err
	}
	png, err := qrcode.Encode(string(raw), qrcode.Medium, 512)
	if err != nil {
		return enrollmentQRResult{}, err
	}
	result := enrollmentQRResult{
		Issued:      issued,
		PayloadJSON: string(raw),
		PNGDataURL:  "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
	}
	return result, nil
}

func buildEnrollmentQRPayload(state enrollmentQRFormState, token string) (map[string]any, error) {
	serverURL := strings.TrimSpace(state.ServerURL)
	packageURL := strings.TrimSpace(state.PackageURL)
	packageChecksum := strings.TrimSpace(state.PackageChecksum)
	deviceID := strings.TrimSpace(state.DeviceID)
	if serverURL == "" || packageURL == "" || packageChecksum == "" || deviceID == "" {
		return nil, fmt.Errorf("server url, package url, package checksum, and device id are required")
	}
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid server url")
	}
	extras := map[string]any{
		"com.xmdm.BASE_URL":         parsed.String(),
		"com.xmdm.ENROLLMENT_TOKEN": token,
		"com.xmdm.DEVICE_ID":        deviceID,
		"com.xmdm.DEVICE_ID_USE":    "serial",
	}
	return map[string]any{
		"android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME":            firstNonEmpty(strings.TrimSpace(state.ComponentName), "com.xmdm.launcher/.AdminReceiver"),
		"android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION": packageURL,
		"android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM":          packageChecksum,
		"android.app.extra.PROVISIONING_LEAVE_ALL_SYSTEM_APPS_ENABLED":          true,
		"android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE":                    extras,
	}, nil
}

func enrollmentQRCallout(result enrollmentQRResult) template.HTML {
	var b strings.Builder
	b.WriteString(`<div style="display:grid;gap:1rem">`)
	b.WriteString(`<section><h3>QR JSON</h3>`)
	b.WriteString(`<pre class="qr-json">` + template.HTMLEscapeString(result.PayloadJSON) + `</pre>`)
	b.WriteString(`</section>`)
	b.WriteString(`<section><h3>QR preview</h3><img alt="Enrollment QR preview" style="max-width:320px;width:100%;height:auto;border:1px solid var(--border);border-radius:.5rem;background:#fff;padding:.5rem" src="` + escAttr(result.PNGDataURL) + `"></section>`)
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

func policyAppToggleForm(action, csrf string, enabled bool) string {
	label := "Enable"
	buttonClass := ""
	if enabled {
		label = "Disable"
		buttonClass = ` class="danger"`
	}
	return `<form class="inline" method="post" action="` + escAttr(action) + `"><input type="hidden" name="csrfToken" value="` + escAttr(csrf) + `"><button type="submit"` + buttonClass + `>` + esc(label) + `</button></form>`
}

func policyCertificateToggleForm(action, csrf string, enabled bool) string {
	return policyAppToggleForm(action, csrf, enabled)
}

func policyManagedFileToggleForm(action, csrf string, enabled bool) string {
	return policyAppToggleForm(action, csrf, enabled)
}

func esc(v string) string {
	return template.HTMLEscapeString(v)
}

func escAttr(v string) string {
	return template.HTMLEscapeString(v)
}
