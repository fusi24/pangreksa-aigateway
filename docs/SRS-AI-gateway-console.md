# Software Requirements Specification
# Pangreksa AI Gateway Console

---

| Field         | Value                                                             |
|---------------|-------------------------------------------------------------------|
| Document ID   | SRS-AI-GATEWAY-CONSOLE-001                                        |
| Version       | 1.0                                                               |
| Status        | Draft                                                             |
| Prepared by   | AI Software Architect (iSAQB CPSA-A Aligned)                    |
| Date          | 2026-06-04                                                        |
| Standard      | iSAQB CPSA-A / ISO/IEC 29148:2018                                |
| Parent SRS    | SRS-AI-GATEWAY-ENGINE-001 v1.0                                    |
| Technology    | Next.js 16 (App Router), TypeScript, IBM Carbon Design System, Apache ECharts, JointJS, Mermaid, Highlight.js |

---

## Table of Contents

1. Introduction
2. System Overview
3. Architecture Overview
4. Functional Requirements
5. New Central Server API Requirements
6. Message Contracts & WebSocket Protocol
7. Data Architecture
8. Non-Functional Requirements
9. API Contracts (Console ↔ Central Server)
10. Security Architecture
11. Deployment Architecture
12. Risks & Mitigation
13. Appendix

---

## 1. Introduction

### 1.1 Purpose

This Software Requirements Specification defines the complete functional and non-functional
requirements for the **Pangreksa AI Gateway Console** — a web-based administration and live
monitoring interface for the Pangreksa AI Gateway Engine. This document targets: frontend
engineers, TypeScript developers, DevOps engineers, and QA engineers.

The console is the visual control plane for everything the Gateway Engine manages: configuration,
entitlements, registry, policies, and observability. It also serves as a real-time monitoring
dashboard — surfacing pod health, telemetry, logs, metrics, and distributed traces.

This SRS complements and extends **SRS-AI-GATEWAY-ENGINE-001** (the parent SRS). It specifies:
- The Next.js 16 frontend application
- New admin API endpoints that the Central Server must implement
- A WebSocket/SSE protocol for real-time telemetry push
- Report generation contracts (DOCX, XLSX, PDF)

### 1.2 Scope

**System Name:** Pangreksa AI Gateway Console

**In Scope:**
- Next.js 16 App Router web application (TypeScript)
- Six primary modules: Observability, Live Monitoring, Telemetry Viewer,
  Config Manager, Reporting, Auth & RBAC
- New Central Server admin API surface (implemented in Go on the existing Central Server)
- WebSocket/SSE real-time telemetry channel
- Report generation: DOCX, XLSX (client-side), PDF (server-side)
- Integration with Prometheus metrics, Loki/structured logs, Jaeger traces via proxy

**Out of Scope:**
- LLM model training or fine-tuning
- Native mobile applications
- Billing and invoicing (separate system)
- Modifications to the Gateway Daemon hot path
- Kafka consumer modifications (no new Kafka topics for the console)

### 1.3 Definitions & Acronyms

| Term           | Definition                                                                  |
|----------------|-----------------------------------------------------------------------------|
| Console        | This web application — the admin and monitoring UI                          |
| Central Server | The Go control-plane service defined in SRS-AI-GATEWAY-ENGINE-001          |
| Daemon / Pod   | A running Gateway Daemon instance (Docker container / Kubernetes pod)       |
| App Router     | Next.js 13+ routing paradigm using the `/app` directory with RSC            |
| RSC            | React Server Component — rendered on the server, no client JS bundle        |
| SSE            | Server-Sent Events — unidirectional server-to-client streaming over HTTP    |
| OTEL           | OpenTelemetry — distributed tracing and metrics standard                    |
| PAT            | Personal Access Token — user-bound API key                                  |
| VAT            | Virtual Account Token — service-identity API key                            |
| RBAC           | Role-Based Access Control                                                   |
| Carbon         | IBM Carbon Design System — component library, design tokens, accessibility  |
| ECharts        | Apache ECharts — data visualization charting library                        |
| JointJS        | Interactive graph/diagram engine for topology and workflow rendering        |
| Mermaid        | Text-to-diagram renderer for flowcharts, ERD, Gantt, sequence diagrams      |
| Highlight.js   | Syntax-highlighting library for 190+ languages (code/JSON/YAML display)     |
| SRS            | Software Requirements Specification                                         |
| TZ             | Timezone — all timestamps rendered in user's local timezone                 |
| FQN            | Fully Qualified Name — prompt version identifier                             |

### 1.4 References

| #    | Reference                                                                    |
|------|------------------------------------------------------------------------------|
| R-01 | SRS-AI-GATEWAY-ENGINE-001 v1.0 — Parent SRS                                 |
| R-02 | iSAQB CPSA-A Curriculum — https://www.isaqb.org                             |
| R-03 | ISO/IEC 29148:2018 — Requirements Engineering                                |
| R-04 | Next.js 16 Documentation — https://nextjs.org/docs                          |
| R-05 | OpenTelemetry — https://opentelemetry.io                                     |
| R-06 | Jaeger — https://www.jaegertracing.io                                        |
| R-07 | Prometheus HTTP API — https://prometheus.io/docs/prometheus/latest/querying/api |
| R-08 | IBM Carbon Design System — https://carbondesignsystem.com                    |
| R-09 | Apache ECharts — https://echarts.apache.org                                  |
| R-10 | JointJS — https://www.jointjs.com/opensource                                 |
| R-11 | Mermaid — https://mermaid.js.org                                             |
| R-12 | Highlight.js — https://highlightjs.org                                       |
| R-13 | docx npm — https://docx.js.org                                               |
| R-14 | ExcelJS — https://github.com/exceljs/exceljs                                 |

---

## 2. System Overview

### 2.1 System Context

```
╔══════════════════════════════════════════════════════════════════════════════════╗
║                         PANGREKSA CONSOLE — SYSTEM BOUNDARY                     ║
║                                                                                  ║
║  ┌──────────────────────────────────────────────────────────────────────────┐   ║
║  │                  NEXT.JS 16 WEB CONSOLE (Browser)                        │   ║
║  │                                                                          │   ║
║  │  Observability │ Live Monitor │ Telemetry │ Config │ Reports │ Auth      │   ║
║  └───────────────────────────────────────┬──────────────────────────────────┘   ║
║                                          │                                       ║
║                        REST + WebSocket/SSE over HTTPS                          ║
║                                          │                                       ║
║  ┌───────────────────────────────────────▼──────────────────────────────────┐   ║
║  │                     CENTRAL SERVER (Go)  :9000                           │   ║
║  │                                                                          │   ║
║  │  Existing:  /health  /config  /entitlement  /admin/entitlement           │   ║
║  │  NEW:       /admin/metrics  /admin/pods  /admin/logs  /admin/traces      │   ║
║  │             /admin/registry/*  /admin/reports/*  /ws/telemetry           │   ║
║  └───────┬───────────────────────────┬──────────────────────────────────────┘   ║
║          │                           │                                           ║
║          ▼                           ▼                                           ║
║  ┌───────────────┐         ┌─────────────────┐                                  ║
║  │  PostgreSQL   │         │  Redis Cluster  │                                  ║
║  │  (primary DB) │         │  (cache+pubsub) │                                  ║
║  └───────────────┘         └─────────────────┘                                  ║
║                                                                                  ║
║  OBSERVABILITY BACKENDS (proxied through Central Server or Next.js API routes): ║
║  ┌──────────────────┐  ┌──────────────┐  ┌───────────────┐                     ║
║  │   Prometheus     │  │   Loki       │  │   Jaeger      │                     ║
║  │   (metrics)      │  │   (logs)     │  │   (traces)    │                     ║
║  └──────────────────┘  └──────────────┘  └───────────────┘                     ║
╚══════════════════════════════════════════════════════════════════════════════════╝

EXTERNAL DATA SOURCES:
  Gateway Daemons ──OTEL──► OTEL Collector ──► Prometheus / Loki / Jaeger
  Gateway Daemons ──Kafka──► Central Server ──► PostgreSQL (request_logs)
```

### 2.2 System Goals

| ID       | Goal                                                                              |
|----------|-----------------------------------------------------------------------------------|
| GOAL-001 | Provide a unified admin UI to manage all configuration the Gateway Engine exposes |
| GOAL-002 | Visualize throughput, latency, token consumption, budget, and component usage     |
| GOAL-003 | Show real-time pod topology, health state, and live telemetry per daemon          |
| GOAL-004 | Surface OTEL metrics, structured logs, and distributed traces in a single pane   |
| GOAL-005 | Enable on-demand and scheduled report export in DOCX, XLSX, and PDF              |
| GOAL-006 | Support org-level RBAC — not all console users should see all modules            |
| GOAL-007 | Deliver a fast, accessible UI that handles real-time updates without full-page reload |

### 2.3 Constraints

| Type       | Constraint                                                                    |
|------------|-------------------------------------------------------------------------------|
| Framework  | Next.js 16.x (App Router, TypeScript strict mode)                             |
| UI System  | IBM Carbon Design System v11 (@carbon/react) — all layout, components, tokens |
| Charts     | Apache ECharts 5.x via `echarts-for-react` wrapper                            |
| Diagrams   | JointJS 4.x (interactive topology); Mermaid 11.x (static/text-driven diagrams) |
| Code View  | Highlight.js 11.x — syntax highlighting for JSON, YAML, Go, SQL, Markdown     |
| Node       | Node.js ≥ 22 LTS (required by Next.js 16)                                    |
| Auth       | Reuses existing PAT / JWT from Central Server; no separate auth service       |
| Deployment | Docker image; Nginx reverse proxy in front of Next.js                         |
| Browser    | Evergreen browsers only (Chrome 120+, Firefox 120+, Edge 120+, Safari 17+)   |
| API        | All Central Server communication over HTTPS; no direct DB access from browser |
| Realtime   | SSE preferred over WebSocket for live monitoring (firewall-friendly)          |
| Reports    | DOCX + XLSX: client-side JS; PDF: server-side Go endpoint on Central Server   |

---

## 3. Architecture Overview

### 3.1 Architectural Style

**Frontend: Server-Component-First SPA Hybrid with IBM Carbon Shell**

Next.js 16 App Router with React Server Components (RSC) for data-fetching pages, and
Client Components (`"use client"`) only for interactive elements (charts, real-time panels,
forms). This hybrid approach:
- Keeps initial page load fast (RSC renders on server, no JS hydration cost for static content)
- Enables streaming SSR (`<Suspense>`) for progressive data loading
- Keeps WebSocket / SSE clients isolated to client components

The entire visual shell is built on **IBM Carbon Design System v11** (`@carbon/react`).
Carbon provides the layout grid, navigation (`SideNav`, `Header`), data display
(`DataTable`, `Tag`, `Tile`, `Modal`), form controls, and design tokens
(color ramps, spacing scale, typography). No custom CSS framework is used — all styling
derives from Carbon tokens, overridden only where necessary via Carbon's theming API.

**Visual Library Role Assignment:**

| Library | Role in Console | Primary Screens |
|---|---|---|
| IBM Carbon | Layout shell, all UI components, design tokens | Every screen |
| Apache ECharts | Time-series, bar, pie, heatmap, gauge charts | Observability, Reporting |
| JointJS | Interactive pod topology graph, drag-and-drop | Live Monitor topology |
| Mermaid | Static architecture diagrams, ERD, flow docs | Config audit, documentation panels |
| Highlight.js | Code/JSON/YAML syntax display | Prompt editor, policy YAML, log viewer, audit diff |

**Backend-for-Frontend (BFF) via Next.js API Routes:**
All requests to Central Server, Prometheus, Loki, and Jaeger are proxied through
Next.js API routes (`/api/*`). This avoids CORS issues, centralizes auth header injection,
and allows response transformation before reaching the browser.

**State Management:**
- Server state: TanStack Query v5 (react-query) with cache invalidation
- UI state: Zustand (lightweight, no boilerplate)
- Real-time state: SSE via `EventSource` managed in a React context

### 3.2 High-Level Architecture

```
Browser
  │
  ├─ Next.js App Router Pages (RSC — server rendered)
  │    ├─ /dashboard          → Observability overview
  │    ├─ /monitor            → Live pod monitoring
  │    ├─ /telemetry          → Metrics / Logs / Traces viewer
  │    ├─ /config             → Registry & Policy manager
  │    ├─ /reports            → Report generation
  │    └─ /settings           → Auth, RBAC, org settings
  │
  ├─ Client Components (interactive islands)
  │    ├─ <MetricsChart>      → ECharts time-series / bar / gauge
  │    ├─ <PodTopology>       → JointJS interactive graph
  │    ├─ <LiveFeed>          → SSE EventSource consumer (Carbon DataTable)
  │    ├─ <TraceViewer>       → Jaeger trace waterfall (Carbon Tile + Mermaid)
  │    ├─ <LogExplorer>       → Virtual-scroll log table + Highlight.js
  │    └─ <DiagramPanel>      → Mermaid static diagram renderer
  │
  └─ Next.js API Routes (BFF layer)
       ├─ /api/metrics        → Proxy → Prometheus
       ├─ /api/logs           → Proxy → Loki / Central Server
       ├─ /api/traces         → Proxy → Jaeger
       ├─ /api/pods           → Proxy → Central Server /admin/pods
       ├─ /api/config/*       → Proxy → Central Server /admin/config/*
       ├─ /api/reports/*      → Proxy → Central Server /admin/reports/*
       └─ /api/sse/telemetry  → Proxy → Central Server SSE /sse/telemetry
```

### 3.3 Component Breakdown

| Component                 | Technology                    | Responsibility                                              |
|---------------------------|-------------------------------|-------------------------------------------------------------|
| Next.js App               | Next.js 16, TS                | Routing, RSC rendering, BFF API routes                      |
| UI Shell & Components     | IBM Carbon Design System v11  | Layout grid, SideNav, Header, DataTable, Modal, Tag, Tile   |
| Design Tokens             | `@carbon/themes`              | Color ramps, spacing scale, type scale — no custom CSS      |
| Charts & Dashboards       | Apache ECharts 5.x (`echarts-for-react`) | Line, bar, pie, gauge, heatmap, scatter    |
| Pod Topology Graph        | JointJS 4.x                   | Interactive directed graph — drag, zoom, click pod nodes    |
| Static Diagrams           | Mermaid 11.x                  | Architecture docs, ERD previews, config flow diagrams       |
| Code / Config Display     | Highlight.js 11.x             | JSON, YAML, Go, SQL, Markdown syntax highlighting           |
| Data Fetching             | TanStack Query v5             | Cache, polling, mutation, optimistic updates                |
| Global UI State           | Zustand 5.x                   | Time-range picker, filter state, sidebar collapse           |
| Real-time                 | SSE (native EventSource)      | Live telemetry push from Central Server SSE                 |
| Report: DOCX              | docx 9.x (npm)                | Client-side Word document generation                        |
| Report: XLSX              | ExcelJS 4.x (npm)             | Client-side Excel workbook generation                       |
| Report: PDF               | Central Server (Go)           | Server-side PDF rendered by Go; streamed as blob            |
| Tables (large datasets)   | Carbon `DataTable` + TanStack Table v8 | Virtualized, sortable, filterable data tables      |
| Log virtualisation        | react-virtuoso                | Virtual scroll for 10k+ log line rendering                  |
| Form validation           | react-hook-form + zod         | Schema-validated admin forms bound to Carbon form controls  |
| Date/time                 | date-fns 4.x                  | Timezone-aware formatting                                   |
| HTTP client               | native fetch (Next.js 16 extends) | Built-in, with cache tags and revalidation              |

### 3.4 Architecture Decision Records

#### ADR-001: Next.js 16 App Router over Vite + React SPA

- **Status:** Accepted
- **Context:** Two viable options: Next.js 16 App Router (SSR/RSC hybrid) vs. Vite + React (SPA).
  This is an internal admin console — SEO is irrelevant. However, the console fetches
  large configuration datasets and audit logs on page load.
- **Decision:** Next.js 16 App Router.
- **Rationale:** RSC allows the server to pre-fetch config data from the Central Server
  before sending HTML — no client-side loading spinners for initial renders.
  Next.js API routes (`/api/*`) act as a BFF proxy, eliminating CORS configuration on the
  Central Server and centralising token injection. `<Suspense>` streaming lets the page
  render progressively as data arrives.
- **Consequences:** Slightly higher complexity than a pure SPA. Node.js 22 LTS required.
  Developers must distinguish RSC from Client Components (`"use client"` boundary).

#### ADR-001b: IBM Carbon Design System as Sole UI Foundation

- **Status:** Accepted
- **Context:** Options: (A) TailwindCSS + shadcn/ui (utility-first, highly customizable),
  (B) IBM Carbon Design System (opinionated enterprise component library with full
  accessibility, WCAG 2.1 AA out of the box).
- **Decision:** IBM Carbon Design System v11 (`@carbon/react`) as the sole UI foundation.
  No TailwindCSS. Carbon tokens are the only design token source.
- **Rationale:** The Pangreksa console is an enterprise admin tool — consistency,
  accessibility, and information density matter more than visual novelty. Carbon ships
  production-ready `DataTable` (sortable, filterable, pagination, row select),
  `SideNav`, `Header`, `Modal`, `Notification`, `Tag`, `Tile`, `DatePicker`, and
  `ComboBox` — all WCAG 2.1 AA compliant with keyboard navigation and screen reader
  support built in. This eliminates the need to build accessible patterns from scratch.
  Carbon's White / Gray 10 / Gray 90 / Gray 100 themes cover light and dark mode without
  custom CSS. External chart libraries (ECharts, JointJS) consume Carbon design tokens
  (`$blue-60`, `$gray-90`, etc.) to maintain visual coherence.
- **Consequences:** Team must learn Carbon's component API and theming model. Customisation
  is constrained to Carbon's token system — intentionally, to enforce consistency.
  Bundle size is higher than a tree-shaken Tailwind build; mitigated with Next.js RSC
  (Carbon shell components render on server, reducing client hydration cost).

#### ADR-001c: Apache ECharts for All Dashboard Charts

- **Status:** Accepted
- **Context:** Options: Recharts (React-native), Apache ECharts (imperative, feature-rich),
  Grafana embedded (zero frontend work but extra deployment dependency).
- **Decision:** Apache ECharts 5.x via `echarts-for-react` wrapper for all dashboard
  charts (time-series, bar, pie, heatmap, gauge, scatter).
- **Rationale:** ECharts provides native support for large datasets (WebGL renderer for
  millions of points), built-in tooltips, data zoom, legend, and theme registration.
  Its `registerTheme` API accepts Carbon token values directly, enabling seamless
  Carbon White / Carbon Gray 100 dark-mode switching. ECharts is significantly more
  feature-complete than Recharts for the monitoring use case (gauge charts for budget
  %, heatmaps for hourly request density, scatter for latency distribution).
- **Consequences:** `echarts-for-react` must be lazy-loaded (dynamic import) to keep
  the initial bundle under 300 KB. The imperative ECharts API is less idiomatic React
  than Recharts — developers must use `useRef` + `getEchartsInstance()` for dynamic
  updates, which the team must learn.

#### ADR-001d: JointJS for Live Pod Topology

- **Status:** Accepted
- **Context:** The Live Monitor requires an interactive, zoomable graph showing daemon
  pods, the Central Server, and infrastructure nodes. Options: React Flow (React-native),
  JointJS (imperative, full-featured graph library), D3-force (low-level).
- **Decision:** JointJS 4.x (open-source Rappid core) for the pod topology graph.
- **Rationale:** JointJS provides built-in pan/zoom, auto-layout algorithms, custom node
  shapes, and click/hover event handling — all needed for the topology view. It mounts
  into a plain DOM `<div>` inside a Carbon `Tile`, and node styles are driven by Carbon
  tokens for visual coherence. JointJS supports live graph updates (add/remove/update
  cells) without full re-render, which is critical for the SSE-driven health state updates.
- **Consequences:** JointJS is imperative, not declarative — the graph is mutated via
  `graph.addCell()` / `cell.attr()` rather than React state. A thin React wrapper
  component (`<PodTopology>`) encapsulates the JointJS instance and bridges SSE events
  to graph mutations. JointJS requires explicit container dimensions — use
  `ResizeObserver` to call `paper.setDimensions()` on container resize.

#### ADR-001e: Mermaid for Static Diagrams and Documentation Panels

- **Status:** Accepted
- **Context:** Several console screens display static diagrams: the config audit page
  shows entitlement flow, the prompt editor shows prompt resolution flow, the
  documentation panel shows the gateway hot path.
- **Decision:** Mermaid 11.x for all static, text-driven diagram rendering.
- **Rationale:** Mermaid renders from human-readable text syntax — diagram definitions
  can be stored in the database alongside config entries or in documentation strings,
  and rendered on demand. This means engineers can update diagrams as text (no
  drag-and-drop tooling needed). Mermaid's Carbon-compatible dark theme
  (`mermaid.initialize({ theme: 'dark' })`) switches automatically with Carbon's
  theme toggle. Supported diagram types in scope: flowchart, sequenceDiagram, erDiagram,
  gantt (for sprint planning panels if added later).
- **Consequences:** Mermaid is not interactive — users cannot edit nodes. For interactive
  diagrams (pod topology, workflow builder) JointJS is used instead. Mermaid rendering
  is async (`mermaid.render()`) — diagrams must be wrapped in a `<Suspense>` or
  loading-state component.

#### ADR-001f: Highlight.js for All Code and Config Display

- **Status:** Accepted
- **Context:** The console displays user-supplied content in several places: prompt
  templates (Markdown + template syntax), policy YAML, audit log diffs (JSON), log
  viewer fields (JSON), and MCP tool definitions (JSON). This content needs
  syntax highlighting for readability.
- **Decision:** Highlight.js 11.x for all code and config rendering surfaces.
- **Rationale:** Highlight.js supports 190+ languages including JSON, YAML, Go, SQL,
  Markdown, and Bash. It is lightweight (core bundle ~10 KB), supports
  language auto-detection, and ships themes compatible with Carbon's gray palette
  (`github-dark-dimmed` matches Carbon Gray 90 theme). It integrates cleanly with
  Carbon's `CodeSnippet` component — Highlight.js provides the syntax tokens,
  Carbon provides the copy-button, expand/collapse, and container styling.
- **Consequences:** Highlight.js must be imported only in Client Components
  (`"use client"`) since it operates on the DOM. Language packs are imported
  individually to minimise bundle size (only `json`, `yaml`, `go`, `sql`,
  `markdown`, `bash` are registered).

#### ADR-002: SSE over WebSocket for Live Telemetry

- **Status:** Accepted
- **Context:** Live monitoring requires the server to push telemetry updates to the browser
  every few seconds. Options: WebSocket (bidirectional), SSE (unidirectional server-push).
- **Decision:** SSE (`text/event-stream`) via `/sse/telemetry` on the Central Server,
  proxied through a Next.js API route.
- **Rationale:** The console only needs server-to-client push (read-only monitoring).
  SSE works over standard HTTP/1.1 and HTTP/2, passes through corporate proxies and load
  balancers without special configuration, and is natively supported in all target browsers.
  WebSocket requires upgrade handshake and firewall allowlisting.
- **Consequences:** If bidirectional control (e.g. "restart pod from console") is added
  later, SSE must be supplemented with REST POST calls — acceptable design.

#### ADR-003: Client-Side DOCX + XLSX, Server-Side PDF

- **Status:** Accepted
- **Context:** Reports can be generated in the browser (JS libs) or on the server (Go libs).
- **Decision:** DOCX and XLSX are generated client-side (docx npm, ExcelJS). PDF is
  generated server-side by the Central Server Go process.
- **Rationale:** `docx` and ExcelJS are mature, zero-server-cost JS libraries. The console
  fetches aggregated JSON from the admin API and transforms it locally — no server round-trip
  for the file itself. PDF requires precise layout (tables, charts, page breaks) that is
  more reliable in Go (`go-pdf` / `wkhtmltopdf`) than in client-side canvas rendering.
  Large reports (>10k rows) benefit from server-side PDF to avoid browser memory pressure.
- **Consequences:** Large DOCX/XLSX files (>50k rows) may be slow in browser — warn users
  and suggest date-range filtering. PDF adds a Central Server dependency but is server-side
  and can be streamed progressively.

#### ADR-004: ECharts over Grafana Embed for Metrics

- **Status:** Accepted
- **Context:** Options: embed Grafana iframes (zero frontend work, but Grafana deployment
  required) vs. build native charts (Apache ECharts) against Prometheus HTTP API.
- **Decision:** Native ECharts charts for all metrics dashboards. Jaeger UI is accessed
  via deep-link redirect for full trace waterfall views.
- **Rationale:** Grafana requires a separate deployment, its own authentication, and
  iframe CSP configuration. Native ECharts gives full control over theming, layout, and
  Carbon token integration. ECharts' `registerTheme` API accepts Carbon token hex values
  directly, so the dashboard automatically switches between Carbon White and Gray 100
  when the user toggles dark mode. Jaeger trace detail views are complex enough that
  the Jaeger UI adds real value — deep-link to Jaeger rather than re-implementing
  the span waterfall.
- **Consequences:** Prometheus query API must be proxied through the Next.js BFF. ECharts
  must be lazy-loaded to keep initial bundle size under 300 KB. Jaeger traces require
  Jaeger to be network-accessible from the user's browser for deep-links to work.

#### ADR-005: TanStack Query for Server State

- **Status:** Accepted
- **Context:** The console fetches many independent data sources (metrics, pods, logs,
  config) with different staleness requirements.
- **Decision:** TanStack Query v5 for all server state (fetching, caching, invalidation,
  refetch-on-interval for live panels).
- **Rationale:** Built-in cache deduplication, background refetching, and `refetchInterval`
  make periodic polling (for pods health, budget counters) trivial. `invalidateQueries`
  after mutations (e.g. updating a guardrail rule) keeps the UI consistent.
- **Consequences:** Developers must understand query keys. Over-fetching risk if intervals
  are set too aggressively — default to 30s for most panels, configurable in settings.

#### ADR-006: BFF Proxy through Next.js API Routes

- **Status:** Accepted
- **Context:** Browser cannot call `http://central-server:9000` directly (CORS, internal
  network, auth header management).
- **Decision:** All external API calls go through Next.js API route handlers (`/api/*`).
  These routes forward requests to the Central Server, Prometheus, Loki, or Jaeger,
  injecting the `Authorization: Bearer` token from the session cookie.
- **Rationale:** Single CORS policy at the Next.js edge. Admin API keys never exposed to
  the browser. Centralised error handling and response normalisation.
- **Consequences:** One extra network hop (browser → Next.js → Central Server). Acceptable
  for admin console latency budgets (not hot path).

---

## 4. Functional Requirements

---

### 4.1 Module: Observability Dashboard

**SRS-FR-C-001: Time-Series Metrics Overview**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | Critical                                                               |
| Component          | `/app/dashboard/`                                                      |
| Description        | Display aggregated time-series charts for the following metrics,
                       scoped to the authenticated user's org, with a configurable
                       time-range selector (1h, 6h, 24h, 7d, 30d, custom): |

- **Latency:** P50, P95, P99 request latency (ms) — ECharts line chart with data zoom
- **Throughput:** Requests per minute (RPM) — ECharts area chart
- **Token consumption:** Input tokens, output tokens, total tokens — ECharts stacked bar chart
- **Cost / Budget:** Spend rate (USD/hour), cumulative spend, % of budget consumed — ECharts line chart with threshold marker line (`markLine`)
- **Component usage:** Requests through Prompt Registry, Skill Registry, MCP Registry, Guardrail Policy, Budget Policy, Rate Limit Policy — ECharts stacked bar (by component)
- **Error rate:** Percentage of requests with `status = error` — ECharts line chart with visual map threshold coloring

| Acceptance Criteria | |
|-|-|
| Given an authenticated admin, when the dashboard loads, then all six chart panels render within 3 seconds with data for the default 24h window. |
| Given any time-range change, when applied, then all charts refetch and re-render within 2 seconds. |
| Given an org with zero requests in the window, then charts render with empty-state messaging, not errors. |

**API Sketch:** `GET /admin/metrics?from=ISO8601&to=ISO8601&interval=1m|5m|1h&org_id=uuid`

---

**SRS-FR-C-002: Per-Entity Breakdown**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | High                                                                   |
| Component          | `/app/dashboard/breakdown`                                             |
| Description        | Allow filtering of all metrics by: `user_id`, `gateway_id`, `provider`,
                       `model`, `prompt_fqn`, `skill_name`. Renders the same chart set
                       as SRS-FR-C-001 but filtered to the selected entity. |

**API Sketch:** `GET /admin/metrics?...&user_id=uuid&provider=openai&model=gpt-4o`

---

**SRS-FR-C-003: Budget Consumption Panel**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | Critical                                                               |
| Component          | `/app/dashboard/budget`                                                |
| Description        | Display a list of all active budget rules with current period spend
                       vs. limit. Show: rule name, entity (user/org/team), period,
                       spend_usd, limit_usd, % consumed, status (OK / WARNING / CRITICAL).
                       Alert thresholds: 75% = yellow, 90% = orange, 100% = red. |

**API Sketch:** `GET /admin/budget/summary?org_id=uuid`

---

### 4.2 Module: Live Monitoring

**SRS-FR-C-004: Pod Topology View**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | Critical                                                               |
| Component          | `/app/monitor/topology`                                                |
| Description        | Render an interactive JointJS graph showing all registered daemon pods
                       connected to the Central Server. Each pod node is a custom
                       JointJS shape (Carbon-styled rounded rectangle) showing:
                       `gateway_id`, status badge (healthy / degraded / dead),
                       config version hash, uptime, last-seen timestamp.
                       The Central Server node shows: version, uptime, connected pod count.
                       Infrastructure nodes (Redis, PostgreSQL, Kafka) shown as leaf
                       nodes with Carbon iconography. Graph supports pan, zoom, and
                       click-to-detail interactions. Auto-layout via JointJS
                       `DirectedGraph` layout algorithm on initial render. |

Status rules (derived from `last_seen_at`):
- `healthy` — last heartbeat < 60s ago
- `degraded` — last heartbeat 60–300s ago
- `dead` — last heartbeat > 300s ago OR no record

| Acceptance Criteria | |
|-|-|
| Given any pod's status changes, when the SSE feed delivers an update, then the pod node badge updates within 5 seconds without full page reload. |

**API Sketch:** `GET /admin/pods` — returns daemon registry snapshot

---

**SRS-FR-C-005: Pod Detail Drawer**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | High                                                                   |
| Component          | `/app/monitor/topology` — slide-over panel                             |
| Description        | Clicking a pod node opens a side drawer showing: gateway_id, status,
                       config version, liveness history (last 10 health checks),
                       current config hash, provider ports, OTEL endpoint,
                       recent error count (last 5 min), and a link to that pod's
                       traces in Jaeger. |

---

**SRS-FR-C-006: Real-Time Telemetry Feed**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | High                                                                   |
| Component          | `/app/monitor/live`                                                    |
| Description        | A live event feed showing the last N transactions as they are processed
                       by any daemon. Displayed as a virtual-scroll table with columns:
                       timestamp, gateway_id, user_id (truncated), provider, model,
                       latency_ms, input_tokens, output_tokens, cost_usd, status,
                       guardrails_hit count. New rows animate in at the top.
                       User can pause/resume the feed, and filter by gateway_id or status. |
| Data source        | SSE `/sse/telemetry` — Central Server publishes one event per
                       committed Kafka transaction                                         |

---

### 4.3 Module: Telemetry Viewer

**SRS-FR-C-007: Metrics Explorer (Prometheus)**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | High                                                                   |
| Component          | `/app/telemetry/metrics`                                               |
| Description        | A Prometheus query UI proxied through the Next.js BFF. Provides:
                       preset metric cards rendered as ECharts gauges and line charts
                       (daemon goroutine count, Redis hit rate, Kafka consumer lag,
                       GC pause duration) and a free-form PromQL input for advanced
                       queries. Results rendered as ECharts time-series or Carbon
                       `DataTable`, user-selectable. |

**API Sketch:** `GET /api/metrics?query=PromQL&start=ISO8601&end=ISO8601&step=60`
(Next.js BFF proxies to Prometheus `GET /api/v1/query_range`)

---

**SRS-FR-C-008: Log Explorer**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | High                                                                   |
| Component          | `/app/telemetry/logs`                                                  |
| Description        | Paginated, searchable structured log viewer using Carbon `DataTable`
                       with react-virtuoso for virtual scroll. Supports:
                       - Free-text search across log fields
                       - Filter by: level (debug/info/warn/error), gateway_id, trace_id,
                         request_id, time range (Carbon `DatePicker`)
                       - Log lines rendered as expandable rows — expanded JSON payload
                         rendered with Highlight.js (`json` language pack) inside a
                         Carbon `CodeSnippet` with copy-button
                       - Click `trace_id` to deep-link to the corresponding Jaeger trace
                       - Infinite scroll or cursor-based pagination |

**API Sketch:** `GET /admin/logs?q=text&level=error&gateway_id=gw-001&from=ISO8601&to=ISO8601&limit=100&cursor=opaque`

---

**SRS-FR-C-009: Trace Viewer**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | Medium                                                                 |
| Component          | `/app/telemetry/traces`                                                |
| Description        | List recent distributed traces searchable by: trace_id, request_id,
                       user_id, operation name, duration range, status (success/error).
                       Clicking a trace row navigates to the Jaeger UI deep-link for
                       the full waterfall view. The console renders a summary card
                       (span count, root latency, top 3 spans) before the deep-link. |
| Note               | Full waterfall rendered by Jaeger UI (external); console shows summary
                       + provides the link. Full embedding is out of scope for v1.0. |

**API Sketch:** `GET /admin/traces?request_id=uuid&from=ISO8601&to=ISO8601&limit=50`

---

**SRS-FR-C-010: Request Drill-Down**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | High                                                                   |
| Component          | `/app/telemetry/request/[request_id]`                                  |
| Description        | Given a `request_id` from any log, trace, or live feed, render a
                       unified detail page showing: the full `request_logs` record,
                       the `cost_records` entry, guardrails fired, skills and MCPs used,
                       prompt FQN resolved, and a link to the Jaeger trace.
                       This is the primary correlation point across all observability surfaces. |

**API Sketch:** `GET /admin/request/:request_id`

---

### 4.4 Module: Configuration Manager

**SRS-FR-C-011: Prompt Registry CRUD**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | High                                                                   |
| Component          | `/app/config/prompts`                                                  |
| Description        | List all prompt repositories and versions for the org using Carbon
                       `DataTable`. Support: create new prompt version (immutable content,
                       auto-increment version), view prompt content in a Carbon
                       `CodeSnippet` with Highlight.js Markdown highlighting, view version
                       history in a Mermaid `gitGraph` or timeline, set "active" version
                       alias (`latest`), and preview template variable substitution inline.
                       Entitlement check: RBAC permission `gateway.prompt_registry.write`
                       required to mutate. |

**API Sketch:**
```
GET    /admin/config/prompts?org_id=uuid
GET    /admin/config/prompts/:repo/:name/versions
POST   /admin/config/prompts/:repo/:name          — create new version
GET    /admin/config/prompts/:fqn                 — resolve FQN to content
```

---

**SRS-FR-C-012: Skill Registry CRUD**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | High                                                                   |
| Component          | `/app/config/skills`                                                   |
| Description        | List all skills for the org. Support: create/update skill (name,
                       description ≤200 chars, SKILL.md content editor, preload toggle),
                       view skill usage stats (how many requests used this skill in the
                       last 7d), and delete skill (with confirmation). |

**API Sketch:**
```
GET    /admin/config/skills
POST   /admin/config/skills
PUT    /admin/config/skills/:id
DELETE /admin/config/skills/:id
```

---

**SRS-FR-C-013: MCP Server Registry CRUD**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | High                                                                   |
| Component          | `/app/config/mcp`                                                      |
| Description        | List all MCP servers. Support: add server (name, URL, auth type,
                       credentials — encrypted at rest), view tool definitions per server,
                       toggle HITL flag per tool, test connectivity (ping MCP server and
                       return available tools), and remove server. |

**API Sketch:**
```
GET    /admin/config/mcp
POST   /admin/config/mcp
PUT    /admin/config/mcp/:id
DELETE /admin/config/mcp/:id
POST   /admin/config/mcp/:id/test     — connectivity + tool discovery
```

---

**SRS-FR-C-014: Guardrail Policy Manager**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | High                                                                   |
| Component          | `/app/config/guardrails`                                               |
| Description        | List all guardrail rules for the org. Support: create rule (type,
                       hook, mode, enforcement, config JSON), reorder rules (drag-and-drop
                       priority), enable/disable individual rules, view guardrail hit stats
                       (how often each rule fired in the last 7d), and delete rule. |

---

**SRS-FR-C-015: Budget & Rate Limit Rule Manager**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | High                                                                   |
| Component          | `/app/config/policies`                                                 |
| Description        | Separate Carbon `Tabs` for Budget Rules and Rate Limit Rules. Each tab
                       shows ordered rules (priority order matches evaluation order).
                       Support: add/edit/delete rules via a YAML editor (Carbon
                       `TextArea` with Highlight.js YAML syntax highlighting),
                       reorder via drag-and-drop (Carbon `OrderedList`),
                       and preview effective limits for a given user/org entity rendered
                       as an ECharts gauge showing % of budget consumed. |

---

**SRS-FR-C-016: Entitlement Manager**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | Critical                                                               |
| Component          | `/app/config/entitlements`                                             |
| Description        | Per-user entitlement editor. Search users by email or user_id.
                       Edit: allowed_prompts (FQN patterns), allowed_skills,
                       allowed_mcps, budget_limit_usd, rate_limit_rpm/tpm, data_scope.
                       Save triggers `/admin/entitlement` update AND Redis invalidation
                       (daemon cache cleared within one request cycle). |

**API Sketch:**
```
GET  /admin/entitlement/:user_id
POST /admin/entitlement                — update + invalidate
```
(Existing endpoints from SRS-AI-GATEWAY-ENGINE-001 § SRS-FR-S-004 — reused as-is)

---

**SRS-FR-C-017: Config Change Audit Trail**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | High                                                                   |
| Component          | `/app/config/audit`                                                    |
| Description        | Read-only view of the `audit_log` table filtered to config-type events.
                       Columns: timestamp, user, event_type, resource_type, resource_id,
                       old_value (diff), new_value (diff). Supports text search and
                       date-range filtering. |

---

### 4.5 Module: Reporting

**SRS-FR-C-018: Report Builder**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | Medium                                                                 |
| Component          | `/app/reports`                                                         |
| Description        | A wizard-style report builder where the user selects:
                       - Report type: Usage Summary, Budget Report, Guardrail Audit,
                         Request Detail Export, Cost Breakdown
                       - Date range
                       - Scope: org-wide, by user, by provider, by model
                       - Output format: DOCX, XLSX, PDF
                       After clicking Generate, the console fetches the required aggregated
                       data from the admin API and produces the file. |

---

**SRS-FR-C-019: DOCX Report Generation (Client-Side)**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | Medium                                                                 |
| Component          | `lib/reports/docx-generator.ts`                                        |
| Description        | Uses the `docx` npm package to generate structured Word documents.
                       Each report type has a pre-defined template: title page with
                       Pangreksa branding, table of contents, data tables with alternating
                       row shading, summary paragraph generated from aggregated data,
                       and a footer with generation timestamp and user. |

---

**SRS-FR-C-020: XLSX Report Generation (Client-Side)**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | Medium                                                                 |
| Component          | `lib/reports/xlsx-generator.ts`                                        |
| Description        | Uses ExcelJS to produce multi-sheet Excel workbooks. Sheet 1: summary
                       metrics. Sheet 2: raw request log rows. Sheet 3: cost breakdown by
                       model. Applies header row formatting, column autofit, and
                       number formatting (USD for cost, integer for tokens). |

---

**SRS-FR-C-021: PDF Report Generation (Server-Side)**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | Medium                                                                 |
| Component          | Central Server `POST /admin/reports/pdf`                               |
| Description        | The console sends report parameters (type, date range, scope, format=pdf)
                       to the Central Server. The Central Server queries PostgreSQL,
                       generates a PDF using a Go PDF library, and streams the binary
                       response. The browser triggers a file download via `Content-Disposition`.
                       Max report rows: 100,000. Requests exceeding this are rejected with 400. |

**API Sketch:**
```
POST /admin/reports/pdf
Authorization: Bearer {ADMIN_API_KEY}
Content-Type: application/json
Body: {
  "type": "usage_summary | budget_report | guardrail_audit | request_detail | cost_breakdown",
  "from": "ISO8601",
  "to": "ISO8601",
  "scope": "org | user | provider | model",
  "scope_id": "uuid | string | null"
}
Response: 200 application/pdf (streamed)
Headers: Content-Disposition: attachment; filename="pangreksa-report-{type}-{date}.pdf"
```

---

### 4.6 Module: Auth & RBAC

**SRS-FR-C-022: Console Authentication**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | Critical                                                               |
| Component          | `/app/auth/`                                                           |
| Description        | Console login via username/password (local) or SSO (Entra/AD) using
                       the existing Central Server user model. On successful auth,
                       the Central Server issues a short-lived JWT (15 min) and a
                       refresh token (7 days). The JWT is stored as an httpOnly
                       Secure SameSite=Strict cookie — never in localStorage.
                       MFA enforced for admin-role users (TOTP via existing Central Server). |

---

**SRS-FR-C-023: RBAC-Gated Module Access**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | Critical                                                               |
| Component          | `middleware.ts` (Next.js middleware)                                   |
| Description        | Each module is gated by a RBAC permission token from
                       `user_entitlements.permissions`. Required permissions:
                       |

| Module              | Read Permission                        | Write Permission                     |
|---------------------|----------------------------------------|--------------------------------------|
| Observability       | `console.observability.read`           | N/A                                  |
| Live Monitoring     | `console.monitor.read`                 | N/A                                  |
| Telemetry Viewer    | `console.telemetry.read`               | N/A                                  |
| Config Manager      | `gateway.prompt_registry.read`         | `gateway.prompt_registry.write`      |
| Reports             | `console.reports.read`                 | `console.reports.generate`           |
| Auth & RBAC         | `console.admin.read`                   | `console.admin.write`                |

Users without read permission for a module see a 403 page — the route is never rendered.
Next.js middleware checks the JWT claims before the page component executes.

---

**SRS-FR-C-024: API Key Management**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | High                                                                   |
| Component          | `/app/settings/api-keys`                                               |
| Description        | Users can create, list, and revoke their own PATs. Org admins
                       can manage VATs (Virtual Account Tokens). The token value is
                       shown once (at creation), then only the hash prefix is displayed.
                       Revocation is immediate via the Central Server's existing
                       invalidation mechanism. |

---

**SRS-FR-C-025: Daemon Invalidation Trigger**

| Field              | Value                                                                  |
|--------------------|------------------------------------------------------------------------|
| Priority           | High                                                                   |
| Component          | Config Manager pages (any mutation)                                    |
| Description        | After any successful config mutation (registry, policy, entitlement),
                       the console calls `POST /admin/invalidate` on the Central Server
                       to trigger immediate Redis pub/sub invalidation. UI shows a toast
                       confirming: "Config change applied — daemons will refresh within
                       one poll cycle (~30s) or immediately if invalidated." |

---

## 5. New Central Server API Requirements

The following API endpoints do not exist in SRS-AI-GATEWAY-ENGINE-001 and must be
implemented as part of this project. All new endpoints:
- Are served under the same Central Server process on port 9000
- Require `Authorization: Bearer {ADMIN_API_KEY}` (different from DAEMON_API_KEY)
- Return JSON unless specified otherwise
- Emit to `audit_log` on any mutating operation

### 5.1 New Endpoint Summary

| Method | Path                           | Purpose                                               |
|--------|--------------------------------|-------------------------------------------------------|
| GET    | /admin/metrics                 | Aggregated request metrics over time                  |
| GET    | /admin/budget/summary          | Current spend vs. limits per budget rule              |
| GET    | /admin/pods                    | Registered daemon list with health status             |
| GET    | /admin/logs                    | Paginated structured log search                       |
| GET    | /admin/traces                  | Trace listing (metadata only)                         |
| GET    | /admin/request/:id             | Single request drill-down (all sources joined)        |
| GET    | /admin/config/prompts          | List prompt repos and versions                        |
| POST   | /admin/config/prompts/:r/:n    | Create new prompt version                             |
| GET/PUT/DELETE | /admin/config/skills/:id | Skill CRUD                                       |
| GET/POST/PUT/DELETE | /admin/config/mcp/:id | MCP server CRUD                              |
| POST   | /admin/config/mcp/:id/test     | Test MCP connectivity                                 |
| GET/POST/PUT/DELETE | /admin/config/guardrails/:id | Guardrail rule CRUD                     |
| GET/POST/PUT/DELETE | /admin/config/budget-rules/:id | Budget rule CRUD                       |
| GET/POST/PUT/DELETE | /admin/config/rate-rules/:id | Rate limit rule CRUD                    |
| POST   | /admin/reports/pdf             | Server-side PDF report generation                     |
| GET    | /admin/audit                   | Audit log query                                       |
| GET    | /sse/telemetry                 | SSE stream of live transaction events                 |

### 5.2 Daemon Registry Requirement

The Central Server must maintain a daemon heartbeat registry. The existing `POST /health`
endpoint (called by daemons) must be extended:

```
POST /health
Body (extended): {
  "gateway_id": "gw-prod-001",
  "version": "1.0.0",
  "config_version": "sha256hex",
  "uptime_seconds": 3600,
  "provider_ports": { "openai": 8080, "claude": 8081 },
  "otel_endpoint": "http://otel-collector:4317"
}
```

Central Server stores this in a new `daemon_registrations` table (see §7). Health status
is derived at query time from `last_seen_at`.

---

## 6. Message Contracts & SSE Protocol

### 6.1 SSE Telemetry Channel

```
ENDPOINT:  GET /sse/telemetry
AUTH:      Authorization: Bearer {ADMIN_API_KEY}
CONTENT-TYPE: text/event-stream
KEEP-ALIVE: comment ":" every 15s

EVENT TYPES:
══════════════════════════════════════════════════════

event: transaction
data: {
  "request_id": "uuid",
  "gateway_id": "gw-prod-001",
  "user_id": "uuid | null",
  "provider": "openai",
  "model": "gpt-4o",
  "status": "success | error",
  "latency_ms": 842,
  "input_tokens": 1024,
  "output_tokens": 512,
  "cost_usd": 0.00384,
  "guardrails_hit": ["pii-v1"],
  "skills_used": ["summarize"],
  "created_at": "ISO8601"
}

event: pod_heartbeat
data: {
  "gateway_id": "gw-prod-001",
  "status": "healthy | degraded | dead",
  "last_seen_at": "ISO8601",
  "config_version": "sha256hex"
}

event: budget_alert
data: {
  "rule_id": "uuid",
  "rule_name": "org-monthly-budget",
  "entity": "org:uuid",
  "threshold_pct": 90,
  "spend_usd": 900.00,
  "limit_usd": 1000.00,
  "period": "2026-06"
}
```

### 6.2 Admin Metrics Response Schema

```json
GET /admin/metrics?from=2026-06-04T00:00:00Z&to=2026-06-04T23:59:59Z&interval=1h

Response 200:
{
  "series": [
    {
      "metric": "latency_p95_ms | latency_p99_ms | rpm | input_tokens |
                  output_tokens | cost_usd | error_rate",
      "points": [
        { "ts": "ISO8601", "value": 123.4 },
        ...
      ]
    }
  ],
  "summary": {
    "total_requests": 15420,
    "total_cost_usd": 142.83,
    "total_input_tokens": 8200000,
    "total_output_tokens": 3100000,
    "avg_latency_p95_ms": 892,
    "error_rate_pct": 0.8
  }
}
```

### 6.3 Pod List Response Schema

```json
GET /admin/pods

Response 200:
{
  "pods": [
    {
      "gateway_id": "gw-prod-001",
      "status": "healthy",
      "version": "1.0.0",
      "config_version": "abc123def456",
      "uptime_seconds": 86400,
      "last_seen_at": "ISO8601",
      "provider_ports": { "openai": 8080, "claude": 8081 },
      "otel_endpoint": "http://otel-collector:4317",
      "recent_error_count": 3,
      "registered_at": "ISO8601"
    }
  ],
  "total": 4,
  "healthy": 3,
  "degraded": 1,
  "dead": 0
}
```

---

## 7. Data Architecture

### 7.1 New Tables (Central Server PostgreSQL)

The console requires two new tables in the existing PostgreSQL database.

#### 7.1.1 daemon_registrations

```sql
CREATE TABLE daemon_registrations (
    gateway_id      VARCHAR(255)  PRIMARY KEY,
    org_id          UUID          NOT NULL REFERENCES organizations(id),
    version         VARCHAR(50),
    config_version  VARCHAR(64),
    uptime_seconds  BIGINT        NOT NULL DEFAULT 0,
    provider_ports  JSONB         NOT NULL DEFAULT '{}',
    otel_endpoint   VARCHAR(512),
    last_seen_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    registered_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_daemon_reg_org ON daemon_registrations(org_id, last_seen_at DESC);
```

Status is derived at query time:
```sql
CASE
  WHEN last_seen_at > NOW() - INTERVAL '60 seconds'  THEN 'healthy'
  WHEN last_seen_at > NOW() - INTERVAL '300 seconds' THEN 'degraded'
  ELSE 'dead'
END AS status
```

#### 7.1.2 console_sessions (for Next.js httpOnly cookie refresh tokens)

```sql
CREATE TABLE console_sessions (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash VARCHAR(255) NOT NULL UNIQUE,
    ip_address      INET,
    user_agent      TEXT,
    expires_at      TIMESTAMPTZ  NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_console_sessions_user ON console_sessions(user_id, expires_at);
-- Auto-clean expired sessions
CREATE INDEX idx_console_sessions_expires ON console_sessions(expires_at);
```

### 7.2 ERD — Console-Specific Additions

```
Existing tables (from SRS-AI-GATEWAY-ENGINE-001):
  organizations ──(1:many)──► users
  users ──(1:1)──► user_entitlements
  organizations ──(1:many)──► request_logs
  organizations ──(1:many)──► audit_log

New tables added by Console:

  organizations (1) ──(many)──► daemon_registrations
                                    ├── gateway_id VARCHAR PK
                                    ├── org_id FK
                                    ├── version
                                    ├── config_version
                                    ├── uptime_seconds
                                    ├── provider_ports JSONB
                                    ├── otel_endpoint
                                    ├── last_seen_at TIMESTAMPTZ
                                    └── registered_at TIMESTAMPTZ

  users (1) ──(many)──► console_sessions
                             ├── id UUID PK
                             ├── user_id FK
                             ├── refresh_token_hash UNIQUE
                             ├── ip_address INET
                             ├── user_agent TEXT
                             ├── expires_at TIMESTAMPTZ
                             └── created_at TIMESTAMPTZ
```

### 7.3 Caching Strategy (Console-Specific)

| Data                      | Cache Layer        | TTL       | Invalidation                          |
|---------------------------|--------------------|-----------|---------------------------------------|
| Metrics aggregations      | TanStack Query     | 30s       | Auto-refetch on interval              |
| Pod list                  | TanStack Query     | 15s       | SSE `pod_heartbeat` triggers invalidate |
| Config (prompts/skills)   | TanStack Query     | 60s       | Invalidated on mutation               |
| Budget summary            | TanStack Query     | 30s       | SSE `budget_alert` triggers invalidate |
| JWT session               | httpOnly cookie    | 15 min    | Refresh token (7 days)                |
| Report data (for export)  | In-memory (one-time) | session | Discarded after download              |

---

## 8. Non-Functional Requirements

| ID           | Quality Attribute | Requirement                                | Metric / Target                                              |
|--------------|-------------------|--------------------------------------------|--------------------------------------------------------------|
| SRS-NFR-C-001 | Performance      | Initial page load (Dashboard)              | Time to Interactive < 3s on 100 Mbps connection             |
| SRS-NFR-C-002 | Performance      | Chart re-render on time-range change       | < 2s from user action to updated charts                     |
| SRS-NFR-C-003 | Performance      | Admin API response time (metrics)          | P95 < 500ms for 24h window queries                          |
| SRS-NFR-C-004 | Performance      | Config CRUD operations                     | P95 < 300ms                                                 |
| SRS-NFR-C-005 | Performance      | SSE telemetry delivery lag                 | < 5s from Kafka commit to browser event                     |
| SRS-NFR-C-006 | Scalability      | Concurrent console users per org           | Support 50 concurrent admins without degradation            |
| SRS-NFR-C-007 | Availability     | Console uptime SLA                         | 99.5% (follows Central Server SLA)                          |
| SRS-NFR-C-008 | Security         | Session storage                            | JWT in httpOnly Secure SameSite=Strict cookie — not localStorage |
| SRS-NFR-C-009 | Security         | CSRF protection                            | Next.js CSRF tokens on all mutating API routes              |
| SRS-NFR-C-010 | Security         | Content Security Policy                    | Strict CSP header; no inline scripts; no unsafe-eval        |
| SRS-NFR-C-011 | Security         | Admin API key exposure                     | ADMIN_API_KEY never sent to browser; injected server-side in BFF |
| SRS-NFR-C-012 | Accessibility    | WCAG compliance                            | WCAG 2.1 AA — all interactive elements keyboard-navigable   |
| SRS-NFR-C-013 | Accessibility    | Screen reader support                      | All charts have aria-label and data-table fallback          |
| SRS-NFR-C-014 | Maintainability  | TypeScript strict mode                     | `"strict": true` in tsconfig.json; no `any` in production code |
| SRS-NFR-C-015 | Maintainability  | Test coverage                              | ≥ 80% unit coverage (Vitest); ≥ 5 E2E flows (Playwright)   |
| SRS-NFR-C-016 | Observability    | Frontend error tracking                    | Sentry SDK integrated; uncaught errors reported with context |
| SRS-NFR-C-017 | Portability      | Browser support                            | Evergreen: Chrome 120+, Firefox 120+, Edge 120+, Safari 17+ |
| SRS-NFR-C-018 | Portability      | Responsive layout                          | Functional at 1280px minimum width (desktop-first admin tool) |
| SRS-NFR-C-019 | Performance      | Bundle size                                | Initial JS bundle < 300 KB (gzipped); chart lib lazy-loaded |
| SRS-NFR-C-020 | Reliability      | SSE reconnection                           | Auto-reconnect with exponential backoff (2s, 4s, 8s, max 60s) |

---

## 9. API Contracts (Console ↔ Central Server)

### 9.1 Authentication Flow

```
POST /auth/login
Content-Type: application/json
Body: { "email": "admin@org.com", "password": "..." }

Response 200:
{
  "access_token": "<JWT — 15 min>",
  "token_type": "Bearer",
  "expires_in": 900
}
Set-Cookie: pangreksa_refresh=<refresh_token>; HttpOnly; Secure; SameSite=Strict; Path=/auth/refresh; Max-Age=604800

────────────────────────────────────────────────────

POST /auth/refresh
Cookie: pangreksa_refresh=<refresh_token>

Response 200:
{
  "access_token": "<new JWT>",
  "expires_in": 900
}

────────────────────────────────────────────────────

POST /auth/logout
Authorization: Bearer <JWT>
Cookie: pangreksa_refresh=...

Response 204 (refresh token revoked in DB)
```

### 9.2 Metrics API

```
GET /admin/metrics
Authorization: Bearer {ADMIN_API_KEY}

Query params:
  from        ISO8601     required
  to          ISO8601     required
  interval    string      1m | 5m | 15m | 1h | 1d     default: 1h
  org_id      UUID        required (injected server-side from JWT claim)
  user_id     UUID        optional filter
  gateway_id  string      optional filter
  provider    string      optional filter
  model       string      optional filter

Response 200: MetricsResponse (see §6.2)

Error Responses:
  400 — invalid from/to range (max 90 days)
  401 — missing or expired token
  500 — database error
```

### 9.3 Pods API

```
GET /admin/pods
Authorization: Bearer {ADMIN_API_KEY}
Query: org_id=uuid (injected from JWT)

Response 200: PodsResponse (see §6.3)
```

### 9.4 Logs API

```
GET /admin/logs
Authorization: Bearer {ADMIN_API_KEY}

Query params:
  q           string      free-text search
  level       string      debug | info | warn | error
  gateway_id  string      filter
  trace_id    string      filter
  request_id  UUID        filter
  from        ISO8601     required
  to          ISO8601     required
  limit       int         default: 100, max: 1000
  cursor      string      opaque pagination cursor

Response 200:
{
  "logs": [
    {
      "ts": "ISO8601",
      "level": "info",
      "gateway_id": "gw-prod-001",
      "request_id": "uuid",
      "trace_id": "hex64",
      "message": "string",
      "fields": { "key": "value" }
    }
  ],
  "next_cursor": "opaque | null",
  "total_matched": 4821
}
```

### 9.5 Trace Summary API

```
GET /admin/traces
Authorization: Bearer {ADMIN_API_KEY}

Query params:
  request_id  UUID        filter
  trace_id    string      filter
  from        ISO8601     required
  to          ISO8601     required
  limit       int         default: 50

Response 200:
{
  "traces": [
    {
      "trace_id": "hex64",
      "request_id": "uuid",
      "root_operation": "gateway.proxy",
      "root_latency_ms": 842,
      "span_count": 12,
      "status": "success | error",
      "jaeger_url": "https://jaeger.internal/trace/{trace_id}",
      "created_at": "ISO8601"
    }
  ]
}
```

### 9.6 Request Drill-Down API

```
GET /admin/request/:request_id
Authorization: Bearer {ADMIN_API_KEY}

Response 200:
{
  "request": { ...full request_logs row... },
  "cost": { ...cost_records row... },
  "trace": {
    "trace_id": "hex64",
    "jaeger_url": "https://jaeger.internal/trace/{trace_id}"
  }
}

Error:
  404 — request_id not found
```

### 9.7 Config Registry APIs

```
═══════════════════ PROMPTS ═══════════════════════════════

GET /admin/config/prompts?org_id=uuid
Response 200: { "repos": [ { "name": "...", "prompts": [ ... ] } ] }

POST /admin/config/prompts/:repo/:name
Body: { "content": "text", "config": { "variables": ["var1"], "guardrails": [...] } }
Response 201: { "fqn": "chat_prompt:repo/name:version", "version": 3 }

═══════════════════ SKILLS ════════════════════════════════

GET    /admin/config/skills                → 200 [SkillRecord]
POST   /admin/config/skills               → 201 { id, name, version }
PUT    /admin/config/skills/:id           → 200 SkillRecord
DELETE /admin/config/skills/:id           → 204

═══════════════════ MCP SERVERS ═══════════════════════════

GET    /admin/config/mcp                  → 200 [McpServerRecord]
POST   /admin/config/mcp                  → 201 { id }
PUT    /admin/config/mcp/:id              → 200 McpServerRecord
DELETE /admin/config/mcp/:id             → 204
POST   /admin/config/mcp/:id/test        → 200 { "reachable": true, "tools": [...] }

═══════════════════ GUARDRAILS ════════════════════════════

GET    /admin/config/guardrails           → 200 [GuardrailRecord]
POST   /admin/config/guardrails           → 201 { id }
PUT    /admin/config/guardrails/:id       → 200 GuardrailRecord
DELETE /admin/config/guardrails/:id      → 204

═══════════════════ BUDGET & RATE RULES ═══════════════════

GET    /admin/config/budget-rules         → 200 [BudgetRuleRecord]
POST   /admin/config/budget-rules         → 201 { id }
PUT    /admin/config/budget-rules/:id     → 200 BudgetRuleRecord
DELETE /admin/config/budget-rules/:id    → 204

GET    /admin/config/rate-rules           → 200 [RateLimitRuleRecord]
POST   /admin/config/rate-rules           → 201 { id }
PUT    /admin/config/rate-rules/:id       → 200 RateLimitRuleRecord
DELETE /admin/config/rate-rules/:id      → 204
```

---

## 10. Security Architecture

| ID       | Category          | Requirement                                             | Implementation                                                  |
|----------|-------------------|---------------------------------------------------------|-----------------------------------------------------------------|
| SEC-C-001 | Transport        | TLS 1.3 for all console traffic                         | Nginx terminates TLS; Next.js served over HTTP internally       |
| SEC-C-002 | Authentication   | httpOnly Secure cookie for refresh token                | `Set-Cookie` with `HttpOnly; Secure; SameSite=Strict`          |
| SEC-C-003 | Authentication   | Access token (JWT) stored in memory only                | Never in localStorage / sessionStorage                          |
| SEC-C-004 | Authorization    | RBAC enforced in Next.js middleware                     | JWT claims checked on every protected route before render       |
| SEC-C-005 | Authorization    | Admin API key never sent to browser                     | Injected server-side in Next.js API routes only                 |
| SEC-C-006 | CSRF             | CSRF token on all POST/PUT/DELETE                       | Next.js built-in CSRF (App Router) + `SameSite=Strict` cookie  |
| SEC-C-007 | CSP              | Strict Content Security Policy                          | `default-src 'self'; script-src 'self'; object-src 'none'`     |
| SEC-C-008 | Input Validation | All form inputs validated with Zod                      | Client-side (immediate feedback) + server-side (in API route)   |
| SEC-C-009 | Audit            | All config mutations logged                             | Central Server writes to `audit_log` on every mutating endpoint |
| SEC-C-010 | Secrets          | No secrets in Next.js client bundle                     | All `ADMIN_API_KEY` usage in server-only files (`server-only` pkg) |
| SEC-C-011 | Dependency       | CVE scanning in CI                                      | `npm audit --audit-level=high` in GitHub Actions pipeline       |
| SEC-C-012 | Rate Limiting    | Console login rate-limited                              | Max 10 login attempts per IP per 10 min (Nginx rate limit)      |

---

## 11. Deployment Architecture

### 11.1 Next.js Project Structure

```
pangreksa-console/
├── app/
│   ├── (auth)/
│   │   ├── login/page.tsx
│   │   └── layout.tsx
│   ├── (console)/
│   │   ├── layout.tsx                    ← Carbon Shell (Header + SideNav) — RSC
│   │   ├── dashboard/
│   │   │   ├── page.tsx                  ← Observability overview (RSC + ECharts)
│   │   │   └── breakdown/page.tsx
│   │   ├── monitor/
│   │   │   ├── topology/page.tsx         ← JointJS topology — Client Component
│   │   │   └── live/page.tsx             ← SSE live feed + Carbon DataTable
│   │   ├── telemetry/
│   │   │   ├── metrics/page.tsx          ← ECharts + PromQL
│   │   │   ├── logs/page.tsx             ← Highlight.js log viewer
│   │   │   ├── traces/page.tsx
│   │   │   └── request/[id]/page.tsx
│   │   ├── config/
│   │   │   ├── prompts/page.tsx          ← Highlight.js + Mermaid version history
│   │   │   ├── skills/page.tsx
│   │   │   ├── mcp/page.tsx
│   │   │   ├── guardrails/page.tsx
│   │   │   ├── policies/page.tsx         ← Highlight.js YAML editor
│   │   │   ├── entitlements/page.tsx
│   │   │   └── audit/page.tsx            ← Highlight.js JSON diff
│   │   ├── reports/page.tsx
│   │   └── settings/
│   │       ├── api-keys/page.tsx
│   │       └── org/page.tsx
│   └── api/                              ← BFF API routes (server-only)
│       ├── auth/
│       │   ├── login/route.ts
│       │   ├── refresh/route.ts
│       │   └── logout/route.ts
│       ├── metrics/route.ts              ← Proxy → Central Server / Prometheus
│       ├── pods/route.ts
│       ├── logs/route.ts
│       ├── traces/route.ts
│       ├── request/[id]/route.ts
│       ├── config/
│       │   ├── prompts/[[...slug]]/route.ts
│       │   ├── skills/[[...slug]]/route.ts
│       │   ├── mcp/[[...slug]]/route.ts
│       │   └── guardrails/[[...slug]]/route.ts
│       ├── reports/
│       │   └── pdf/route.ts              ← Proxy → Central Server PDF
│       └── sse/
│           └── telemetry/route.ts        ← SSE proxy → Central Server
├── components/
│   ├── charts/
│   │   ├── MetricsChart.tsx              ← "use client" ECharts line/area wrapper
│   │   ├── BudgetGauge.tsx               ← "use client" ECharts gauge
│   │   ├── ComponentUsageBar.tsx         ← "use client" ECharts stacked bar
│   │   └── HeatmapChart.tsx              ← "use client" ECharts heatmap
│   ├── monitor/
│   │   ├── PodTopology.tsx               ← "use client" JointJS graph wrapper
│   │   ├── PodDetailDrawer.tsx           ← Carbon SidePanel
│   │   └── LiveFeed.tsx                  ← "use client" SSE + Carbon DataTable
│   ├── telemetry/
│   │   ├── LogExplorer.tsx               ← "use client" react-virtuoso + Highlight.js
│   │   ├── CodeBlock.tsx                 ← Highlight.js inside Carbon CodeSnippet
│   │   ├── MermaidDiagram.tsx            ← "use client" Mermaid renderer
│   │   ├── TraceList.tsx
│   │   └── RequestDetail.tsx
│   ├── config/
│   │   ├── PromptEditor.tsx              ← Highlight.js Markdown preview
│   │   ├── SkillForm.tsx
│   │   ├── GuardrailForm.tsx
│   │   └── PolicyYamlEditor.tsx          ← Highlight.js YAML
│   └── shell/                            ← Carbon Shell wrappers
│       ├── AppHeader.tsx
│       ├── AppSideNav.tsx
│       └── ThemeProvider.tsx             ← Carbon Theme (White ↔ Gray100)
├── lib/
│   ├── api/
│   │   ├── central-server.ts             ← Server-side typed fetch helpers
│   │   ├── prometheus.ts
│   │   └── jaeger.ts
│   ├── reports/
│   │   ├── docx-generator.ts
│   │   └── xlsx-generator.ts
│   ├── auth/
│   │   ├── session.ts                    ← JWT parse/validate (server-only)
│   │   └── rbac.ts
│   ├── charts/
│   │   ├── echarts-theme.ts              ← Carbon token → ECharts theme registration
│   │   └── jointjs-styles.ts             ← Carbon token → JointJS cell attributes
│   └── utils/
│       ├── format.ts                     ← Number, date, token formatting
│       └── sse.ts                        ← SSE client wrapper with backoff
├── hooks/
│   ├── useMetrics.ts                     ← TanStack Query hooks
│   ├── usePods.ts
│   ├── useLiveFeed.ts                    ← SSE EventSource hook
│   └── useReports.ts
├── store/
│   └── ui.ts                             ← Zustand: time-range, theme, sidebar state
├── middleware.ts                          ← RBAC route protection
├── next.config.ts
├── tsconfig.json                          ← strict: true
├── vitest.config.ts
├── playwright.config.ts
└── package.json
```

### 11.2 Key Dependencies

```json
{
  "dependencies": {
    "next": "16.2.7",
    "react": "19.1.0",
    "react-dom": "19.1.0",
    "typescript": "5.8.3",
    "@carbon/react": "1.71.0",
    "@carbon/themes": "11.30.0",
    "@carbon/icons-react": "11.56.0",
    "echarts": "5.6.0",
    "echarts-for-react": "3.0.2",
    "jointjs": "4.0.3",
    "mermaid": "11.4.1",
    "highlight.js": "11.11.1",
    "@tanstack/react-query": "5.80.0",
    "@tanstack/react-table": "8.21.3",
    "zustand": "5.0.4",
    "react-virtuoso": "4.12.3",
    "react-hook-form": "7.56.4",
    "zod": "3.25.23",
    "date-fns": "4.1.0",
    "docx": "9.5.0",
    "exceljs": "4.4.0",
    "server-only": "0.0.1"
  },
  "devDependencies": {
    "vitest": "3.2.2",
    "@playwright/test": "1.52.0",
    "@testing-library/react": "16.3.0",
    "eslint": "9.x",
    "@typescript-eslint/eslint-plugin": "8.x"
  }
}
```

> **Library version notes:**
> `@carbon/react` v1.71 is the current stable release for Carbon v11 compatible with React 19.
> `echarts-for-react` v3 wraps ECharts 5.x; import ECharts dynamically
> (`const EChartsReact = dynamic(() => import('echarts-for-react'), { ssr: false })`)
> to prevent SSR hydration errors. `jointjs` v4 (open-source Rappid core) is the latest
> stable; mount JointJS inside `useEffect` after the DOM is available.
> `mermaid` v11 requires async initialization — use `mermaid.initialize()` once
> at app startup and `mermaid.render()` per diagram in a `useEffect`.
> `highlight.js` v11 — import only required language packs to keep bundle lean.

### 11.3 Environment Variables

```
═══ NEXT.JS CONSOLE (server-side only — never exposed to browser) ══════════
  CENTRAL_SERVER_URL=http://central-server:9000
  ADMIN_API_KEY=<secret>            ← Central Server admin key (never in client bundle)
  PROMETHEUS_URL=http://prometheus:9090
  LOKI_URL=http://loki:3100
  JAEGER_URL=http://jaeger:16686
  NEXTAUTH_SECRET=<32-byte-hex>     ← JWT signing key for console sessions
  SESSION_COOKIE_NAME=pangreksa_session

═══ NEXT.JS PUBLIC (available in browser) ══════════════════════════════════
  NEXT_PUBLIC_APP_NAME=Pangreksa Console
  NEXT_PUBLIC_JAEGER_EXTERNAL_URL=https://jaeger.your-domain.com
  NEXT_PUBLIC_SENTRY_DSN=https://...@sentry.io/...
```

### 11.4 Dockerfile

```dockerfile
# Dockerfile.console
FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --frozen-lockfile

FROM node:22-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

FROM node:22-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
RUN addgroup -S pangreksa && adduser -S nextjs -G pangreksa
COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:pangreksa /app/.next/standalone ./
COPY --from=builder --chown=nextjs:pangreksa /app/.next/static ./.next/static
USER nextjs
EXPOSE 3000
CMD ["node", "server.js"]
```

### 11.5 CI/CD Pipeline

```
[Git Push to main]
    │
    ▼
[GitHub Actions]
    ├── tsc --noEmit (type check)
    ├── eslint --max-warnings 0
    ├── vitest run --coverage (≥ 80% check)
    ├── npm audit --audit-level=high
    ├── docker build -f docker/Dockerfile.console
    ├── docker push to registry (tagged with git SHA)
    ├── deploy to staging (docker compose pull && up -d)
    ├── playwright test (E2E against staging)
    └── (manual approval) → deploy to production
```

### 11.6 Environment Strategy

| Environment | Infrastructure                         | Scale                        |
|-------------|----------------------------------------|------------------------------|
| Development | `npm run dev` with `.env.local`        | Single process               |
| Staging     | Docker Compose (console + central-server + infra) | Single replica each |
| Production  | Docker Swarm or Kubernetes; Nginx LB in front | 2+ console replicas    |

---

## 12. Risks & Mitigation

| ID       | Risk                                                         | Likelihood | Impact | Mitigation                                                                          |
|----------|--------------------------------------------------------------|------------|--------|-------------------------------------------------------------------------------------|
| RSK-C-001 | SSE connection drops in corporate proxy environments        | Medium     | Medium | Auto-reconnect with backoff; fall back to 30s polling if SSE fails after 3 attempts |
| RSK-C-002 | Prometheus / Loki / Jaeger not deployed → telemetry empty   | Medium     | Medium | Graceful empty state per panel; console still functional without O11y backends      |
| RSK-C-003 | Large DOCX/XLSX generation (>50k rows) freezes browser      | Medium     | Low    | Row limit warning at 10k; suggest date range narrowing; Web Worker for generation  |
| RSK-C-004 | Admin API key rotation breaks console sessions              | Low        | High   | Key rotation runbook; rolling restart of console with new env var                   |
| RSK-C-005 | Carbon v11 + ECharts + JointJS SSR conflicts         | Medium     | Medium | ECharts and JointJS require `dynamic()` with `{ ssr: false }`; Mermaid requires `useEffect` — all three must be Client Components only |
| RSK-C-006 | Metrics query slow for large orgs (90-day range)            | Medium     | Medium | Enforce max 90-day range; use PostgreSQL read replica for metrics queries           |
| RSK-C-007 | SSE telemetry event rate too high for browser rendering     | Low        | Medium | Server-side rate limit: max 10 events/sec per SSE connection; client-side throttle |
| RSK-C-008 | JointJS pod graph unmanageable at 100+ daemons          | Low        | Medium | Auto-group pods by org/region; JointJS mini-map for navigation; max 50 nodes rendered at once; virtual pagination for large fleets |
| RSK-C-009 | New Central Server admin endpoints break daemon hot path    | Low        | High   | Admin endpoints served on separate router (/admin/*); no shared middleware with daemon path |
| RSK-C-010 | RBAC gaps — console user accesses forbidden config          | Low        | High   | Middleware enforces permissions before every route; Central Server validates JWT on every admin endpoint |

---

## 13. Appendix

### 13.1 Glossary

| Term                | Definition                                                                    |
|---------------------|-------------------------------------------------------------------------------|
| RSC                 | React Server Component — executes on the server; zero client JS               |
| Client Component    | React component marked `"use client"` — hydrated in the browser               |
| BFF                 | Backend for Frontend — Next.js API routes acting as a proxy layer             |
| SSE                 | Server-Sent Events — HTTP streaming for server-to-client push                 |
| Carbon              | IBM Carbon Design System — enterprise React component library (WCAG 2.1 AA)   |
| Carbon Token        | CSS/SCSS design variable from `@carbon/themes` (e.g. `$blue-60`, `$gray-90`) |
| ECharts             | Apache ECharts — imperative JS charting library; themed via `registerTheme`   |
| JointJS             | Imperative graph/diagram engine; mounted in DOM via `useEffect` + `useRef`    |
| Mermaid             | Text-to-diagram renderer; initialized once, rendered per diagram via async API |
| Highlight.js        | Syntax highlighter; language packs imported individually to control bundle size|
| TanStack Query      | Data-fetching library with cache, invalidation, polling (formerly react-query) |
| Zustand             | Lightweight React state manager for UI-only state                             |
| httpOnly cookie     | Browser cookie inaccessible to JS — used to store refresh tokens securely     |
| PromQL              | Prometheus Query Language for metrics time-series queries                     |

### 13.2 Pending Decisions (Pre-Implementation)

| #  | Decision                                                                            | Owner              | Deadline         |
|----|-------------------------------------------------------------------------------------|--------------------|------------------|
| D-01 | Confirm Loki vs. OpenSearch as the structured log backend                         | DevOps             | Before Sprint 1  |
| D-02 | Confirm Jaeger vs. Grafana Tempo as the trace backend                             | DevOps             | Before Sprint 1  |
| D-03 | Decide PDF Go library: `go-pdf`, `gofpdf`, or `wkhtmltopdf` (CGO constraint)     | Backend team       | Before Sprint 2  |
| D-04 | Decide if Sentry or a self-hosted alternative (GlitchTip) for error tracking      | Infra team         | Before Sprint 1  |
| D-05 | Define org-level plan limits (max daemons, max users) for the console to display  | Product            | Before Sprint 3  |

### 13.3 References

| #    | Reference                                                                          |
|------|------------------------------------------------------------------------------------|
| R-01 | SRS-AI-GATEWAY-ENGINE-001 v1.0                                                    |
| R-02 | Next.js 16 App Router Docs — https://nextjs.org/docs/app                         |
| R-03 | TanStack Query v5 — https://tanstack.com/query/v5                                 |
| R-04 | IBM Carbon Design System v11 — https://carbondesignsystem.com                     |
| R-05 | Apache ECharts 5.x — https://echarts.apache.org/en/api.html                       |
| R-06 | JointJS 4.x — https://www.jointjs.com/opensource                                  |
| R-07 | Mermaid 11.x — https://mermaid.js.org/config/setup/getting-started.html           |
| R-08 | Highlight.js 11.x — https://highlightjs.org                                       |
| R-09 | docx npm — https://docx.js.org                                                    |
| R-10 | ExcelJS — https://github.com/exceljs/exceljs                                      |
| R-11 | Prometheus HTTP API — https://prometheus.io/docs/prometheus/latest/querying/api   |
| R-12 | Jaeger Query API — https://www.jaegertracing.io/docs/apis                         |
| R-13 | WCAG 2.1 AA — https://www.w3.org/TR/WCAG21                                        |

---

> | Version | Date       | Author                        | Change                                                             |
> |---------|------------|-------------------------------|--------------------------------------------------------------------|
> | 1.0     | 2026-06-04 | AI Software Architect (iSAQB) | Initial draft                                                      |
> | 1.1     | 2026-06-04 | AI Software Architect (iSAQB) | UI stack updated: IBM Carbon + ECharts + JointJS + Mermaid + Highlight.js |

---

*End of SRS — Pangreksa AI Gateway Console v1.0*
