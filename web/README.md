# Pangreksa AI Gateway Console

Web-based administration and live monitoring interface for the Pangreksa AI Gateway Engine.
See [`../docs/SRS-AI-gateway-console.md`](../docs/SRS-AI-gateway-console.md) for the full specification.

## Stack

| Layer | Technology |
|---|---|
| Framework | Next.js 16.2 — App Router, React Server Components, TypeScript strict |
| UI | IBM Carbon Design System v11 (`@carbon/react`) |
| Charts | Apache ECharts 5 via `echarts-for-react` |
| Topology | `@joint/core` 4 (interactive pod graph) |
| Diagrams | Mermaid 11 (static, text-driven) |
| Code display | Highlight.js 11 |
| Server state | TanStack Query v5 |
| UI state | Zustand 5 |
| Forms | react-hook-form + zod |
| Real-time | SSE (native `EventSource`) |
| Reports | docx 9 (DOCX), ExcelJS 4 (XLSX), Central Server PDF endpoint |
| Auth | JWT httpOnly cookie via `jose` |
| Node | ≥ 22 LTS |

## Prerequisites

- Node.js ≥ 22 LTS
- A running Pangreksa Central Server on port 9000 (Docker image or native Windows for local dev)

## Quick Start

```bash
# 1. Install dependencies
npm install --legacy-peer-deps

# 2. Configure environment
cp .env.local.example .env.local
# Edit .env.local — set CENTRAL_SERVER_URL, ADMIN_API_KEY, NEXTAUTH_SECRET

# 3. Start development server
npm run dev
# → http://localhost:3000
```

## Environment Variables

Copy `.env.local.example` to `.env.local`.

| Variable | Required | Description |
|---|---|---|
| `CENTRAL_SERVER_URL` | ✅ | Base URL of the Central Server, e.g. `http://localhost:9000` |
| `ADMIN_API_KEY` | ✅ | Admin API key — injected server-side only, never sent to browser |
| `NEXTAUTH_SECRET` | ✅ | 32-byte hex string used to sign console session JWTs |
| `SESSION_COOKIE_NAME` | — | httpOnly cookie name (default: `pangreksa_session`) |
| `PROMETHEUS_URL` | — | Prometheus base URL for the Telemetry metrics explorer |
| `LOKI_URL` | — | Loki base URL (proxied through Central Server) |
| `JAEGER_URL` | — | Internal Jaeger URL for server-side queries |
| `NEXT_PUBLIC_APP_NAME` | — | App name shown in the browser tab |
| `NEXT_PUBLIC_JAEGER_EXTERNAL_URL` | — | Jaeger URL reachable from the browser (trace deep-links) |
| `NEXT_PUBLIC_SENTRY_DSN` | — | Sentry DSN for frontend error reporting |

> The Central Server runs as a Docker image in staging/production but can be started directly
> on Windows for local testing — both cases connect via `CENTRAL_SERVER_URL`.

## Commands

```bash
npm run dev          # Dev server with Turbopack hot reload
npm run build        # Production build (runs tsc --noEmit first)
npm run start        # Start production server
npm run type-check   # TypeScript strict type check
npm run lint         # ESLint — zero warnings enforced
npm run test         # Vitest unit tests with coverage report
npm run test:watch   # Vitest in watch mode
npm run test:e2e     # Playwright E2E tests (requires a running server)
npm run audit        # npm audit --audit-level=high
```

## Project Layout

```
web/
├── app/
│   ├── (auth)/login/          Login page
│   ├── (console)/             All authenticated routes
│   │   ├── layout.tsx         Carbon Shell (Header + SideNav) — RSC
│   │   ├── dashboard/         Observability Dashboard — 6 ECharts panels
│   │   ├── monitor/
│   │   │   ├── topology/      Pod topology — JointJS interactive graph
│   │   │   └── live/          Live telemetry feed — SSE + Carbon DataTable
│   │   ├── telemetry/
│   │   │   ├── metrics/       Prometheus explorer — ECharts + PromQL
│   │   │   ├── logs/          Log explorer — Virtuoso virtual scroll + Highlight.js
│   │   │   ├── traces/        Distributed trace list + Jaeger deep-links
│   │   │   └── request/[id]/  Request drill-down — RSC server-side fetch
│   │   ├── config/
│   │   │   ├── prompts/       Prompt Registry CRUD + Mermaid version history
│   │   │   ├── skills/        Skill Registry CRUD
│   │   │   ├── mcp/           MCP Server Registry CRUD + connectivity test
│   │   │   ├── guardrails/    Guardrail Policy Manager
│   │   │   ├── policies/      Budget + Rate Limit rule manager (tabbed)
│   │   │   ├── entitlements/  Per-user entitlement editor
│   │   │   └── audit/         Config change audit trail with JSON diffs
│   │   ├── reports/           Report builder wizard — DOCX / XLSX / PDF
│   │   └── settings/api-keys/ PAT / VAT management
│   ├── api/                   BFF proxy routes (all server-side)
│   │   ├── auth/              login / logout / refresh
│   │   ├── metrics/           → /admin/metrics
│   │   ├── budget/            → /admin/budget/summary
│   │   ├── pods/              → /admin/pods
│   │   ├── logs/              → /admin/logs
│   │   ├── traces/            → /admin/traces
│   │   ├── request/[id]/      → /admin/request/:id
│   │   ├── config/            → /admin/config/* (CRUD for all entities)
│   │   ├── reports/pdf|data/  → /admin/reports/pdf + data aggregation
│   │   ├── sse/telemetry/     → /sse/telemetry (streaming proxy)
│   │   ├── admin/invalidate/  → /admin/invalidate (daemon cache flush)
│   │   ├── telemetry/metrics/ → Prometheus /api/v1/query_range
│   │   ├── settings/api-keys/ → /settings/api-keys
│   │   └── health/            Docker health check (/api/health)
│   └── 403/ not-found/
│
├── components/
│   ├── charts/       MetricsChart, BudgetGauge, ComponentUsageBar
│   ├── monitor/      PodTopology, PodDetailDrawer, LiveFeed
│   ├── telemetry/    CodeBlock (Highlight.js), MermaidDiagram, LogExplorer
│   ├── config/       CrudTable — reusable CRUD pattern for all 6 config pages
│   ├── shell/        AppHeader, AppSideNav, ThemeProvider, ToastContainer
│   └── providers/    QueryProvider (TanStack Query), ErrorBoundary
│
├── hooks/
│   ├── useMetrics.ts    Time-range-aware metrics fetcher
│   ├── useBudget.ts     Budget summary; SSE budget_alert triggers invalidation
│   ├── usePods.ts       Pod list; SSE pod_heartbeat triggers invalidation
│   ├── useLiveFeed.ts   SSE consumer — feeds LiveFeed, cross-invalidates queries
│   └── useReports.ts    DOCX/XLSX/PDF generation orchestrator + download trigger
│
├── lib/
│   ├── api/central-server.ts         centralFetch<T>() — server-only BFF helper
│   ├── api/config-route-helpers.ts   makeConfigHandlers() — generic CRUD factory
│   ├── auth/session.ts               JWT sign/verify, cookie helpers — server-only
│   ├── auth/rbac.ts                  hasPermission / requirePermission — server-only
│   ├── charts/echarts-theme.ts       Carbon token → ECharts theme registration
│   ├── charts/jointjs-styles.ts      Carbon token values for JointJS node styling
│   ├── reports/docx-generator.ts     Client-side DOCX generation
│   ├── reports/xlsx-generator.ts     Client-side XLSX generation (3 sheets)
│   └── utils/format.ts | sse.ts | csrf.ts
│
├── store/
│   ├── ui.ts             timeRange, theme (white/g100), sideNavCollapsed
│   └── notifications.ts  Toast queue consumed by ToastContainer
│
├── types/
│   ├── api.ts    All API response shapes (never any — strict types throughout)
│   └── rbac.ts   ConsolePermission union + ROUTE_PERMISSION_MAP
│
├── middleware.ts        RBAC guard — runs before every page render
├── next.config.ts       standalone output, transpilePackages, security headers
├── tsconfig.json        strict + noUncheckedIndexedAccess + exactOptionalPropertyTypes
├── vitest.config.ts
├── playwright.config.ts
├── docker/Dockerfile.console   3-stage Node 22 Alpine build
├── test/                       Vitest unit tests (22 tests, 4 suites)
└── e2e/                        Playwright E2E specs (auth, dashboard, monitor, reports)
```

## Architecture

### BFF Proxy Pattern

All external service calls go through Next.js API route handlers (`app/api/**`).
The browser never communicates with the Central Server directly.

```
Browser  →  /api/pods  →  [Route Handler]  →  Central Server :9000/admin/pods
                               ↑
                   injects ADMIN_API_KEY from env (server-only)
                   injects org_id from verified JWT claim
```

### Auth Flow

```
1. POST /api/auth/login  (email + password from login page)
2. → centralFetch POST /auth/login  (Central Server validates credentials)
3. ← access_token + permissions[]
4. → signSession(payload)  (console JWT, 15 min, signed with NEXTAUTH_SECRET)
5. ← Set-Cookie: pangreksa_session  (httpOnly, Secure, SameSite=Strict)
6. → redirect /dashboard

Every route:
  middleware.ts → jwtVerify(cookie) → check permissions[] → redirect or pass
```

### Real-Time (SSE)

`/api/sse/telemetry` proxies the Central Server SSE stream.

| Event | Consumer | Effect |
|---|---|---|
| `transaction` | `useLiveFeed` | Prepended to live feed buffer (max 200 rows) |
| `pod_heartbeat` | `useLiveFeed` | `invalidateQueries(['pods'])` + in-place pod status update |
| `budget_alert` | `useLiveFeed` | `invalidateQueries(['budget'])` |

Reconnects with exponential backoff: 2s → 4s → 8s … max 60s (SRS-NFR-C-020).

### ECharts Theming

Two themes — `carbon-white` and `carbon-dark` — are registered using Carbon Design System
token hex values. The active theme tracks the Zustand `ui.ts` store; switching the header
theme toggle re-renders all charts without a page reload.

### CRUD Pattern

All six Configuration Manager pages reuse `<CrudTable>` from `components/config/CrudTable.tsx`.
New config entities require:
1. A BFF route using `makeConfigHandlers(basePath)` from `lib/api/config-route-helpers.ts`
2. A page that queries the BFF and renders `<CrudTable>` with a `Modal` form

## Deployment

### Docker

```bash
docker build -f docker/Dockerfile.console -t pangreksa-console:latest .

docker run -p 3000:3000 \
  -e CENTRAL_SERVER_URL=http://central-server:9000 \
  -e ADMIN_API_KEY=your-admin-key \
  -e NEXTAUTH_SECRET=your-32-byte-hex-secret \
  pangreksa-console:latest
```

### Nginx (TLS termination)

Nginx should terminate TLS on port 443 and proxy to Next.js on port 3000.
Set `X-Forwarded-Proto: https` so Next.js marks session cookies as `Secure`.

```nginx
location / {
    proxy_pass http://console:3000;
    proxy_set_header X-Forwarded-Proto https;
    proxy_set_header Host $host;
}
```

### Environments

| Environment | Infrastructure |
|---|---|
| Development | `npm run dev` + `.env.local` |
| Staging | Docker Compose (console + central-server + infra) |
| Production | Docker Swarm / Kubernetes, 2+ replicas, Nginx LB |

## Testing

### Unit Tests

```bash
npm run test           # single run + coverage
npm run test:watch     # watch mode
```

Coverage threshold: 80% lines and functions. Tests live in `test/`.

### E2E Tests

```bash
npm run build && npm run start &   # or use a staging deployment
npm run test:e2e
```

E2E specs live in `e2e/`. Tests requiring auth check for an `E2E_SESSION_COOKIE`
env var (pre-signed JWT injected by CI).
