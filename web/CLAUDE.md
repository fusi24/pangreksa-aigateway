@AGENTS.md

# Pangreksa Console — AI Development Guide

This file gives Claude Code the context it needs to work effectively in this codebase.
Read it fully before making any changes.

---

## What This Project Is

A Next.js 16 admin console for the Pangreksa AI Gateway Engine. The frontend lives in this
directory (`web/`). The backend — the **Central Server** (Go, port 9000) — is a **separate
project**. The console never calls the Central Server directly from the browser; all calls
go through Next.js BFF API routes in `app/api/`.

---

## Non-Obvious Rules You Must Follow

### 1. `server-only` is enforced — respect it

`lib/api/central-server.ts` and `lib/auth/session.ts` import `server-only`. If you import
either of them (or anything that transitively imports them) from a `'use client'` component
or a shared module that can be bundled client-side, the build will fail with a deliberate
error. This is intentional security: `ADMIN_API_KEY` must never reach the browser.

**Rule:** BFF logic (`centralFetch`, session helpers, `rbac.ts`) belongs in `app/api/**`
route handlers or RSC pages only.

### 2. `exactOptionalPropertyTypes: true` — conditional spreading required

TypeScript is configured with `exactOptionalPropertyTypes: true`. This means:

```typescript
// ❌ WRONG — 'undefined' is not assignable to optional 'string'
saveMutation.mutate({ ...values, id: editId ?? undefined });

// ✅ CORRECT — only spread id when it is defined
saveMutation.mutate(editId ? { ...values, id: editId } : values);
```

The same applies to any optional props passed to Carbon components. If a Carbon component
does not accept a prop (e.g. `SkeletonPlaceholder` does not accept `style`), wrap the
element in a `<div style={...}>` instead.

### 3. `noUncheckedIndexedAccess: true` — array access always possibly undefined

```typescript
// ❌ WRONG — items[index] is possibly undefined
const item = items[index];
item.name;

// ✅ CORRECT — guard before use
const item = items[index];
if (item) item.name;
```

### 4. Carbon `DataTable` — always destructure `key` from spread props

`getRowProps()` returns an object that includes a `key` property. Spreading it directly onto
`<TableRow>` causes "key specified more than once" TypeScript errors:

```typescript
// ❌ WRONG
<TableRow key={row.id} {...getRowProps({ row })}>

// ✅ CORRECT
const { key: rowKey, ...rowProps } = getRowProps({ row });
<TableRow key={rowKey} {...rowProps}>
```

### 5. `@joint/core` — not `jointjs`

The JointJS 4.x package is `@joint/core`, not `jointjs`. The `jointjs` npm package only has
versions up to 3.7.x. The open-source `@joint/core` package does **not** include the
`DirectedGraph` auto-layout (that is Rappid commercial only). Use manual x/y positioning for
topology nodes instead.

### 6. ECharts must be dynamically imported

ECharts and `echarts-for-react` require the DOM. Always lazy-load them:

```typescript
const ReactECharts = dynamic(() => import('echarts-for-react'), { ssr: false });
```

Same for JointJS (load inside `useEffect`) and Mermaid (load inside `useEffect`).

### 7. Carbon icons — verify exports before using

Not all icon names you might guess exist. Always check before using:
- `DocumentReport` does not exist — use `Report`
- `StructuredList` is not exported — use `StructuredListWrapper`
- Use `node -e "const c = require('@carbon/react'); console.log(Object.keys(c).filter(k => ...))"` to verify

### 8. After every config mutation — call `/api/admin/invalidate`

Every successful config change (prompts, skills, MCP, guardrails, policies, entitlements)
must be followed by `POST /api/admin/invalidate` to trigger Redis pub/sub cache flush on the
Central Server. The `makeConfigHandlers()` factory does this automatically. If you write a
custom mutation handler, add the invalidation call manually.

---

## BFF Route Anatomy

Every BFF route in `app/api/**` follows this exact pattern:

```typescript
import { NextRequest, NextResponse } from 'next/server';
import { centralFetch, ApiError } from '@/lib/api/central-server';
import { getSessionJwt, getSession } from '@/lib/auth/session';

export async function GET(req: NextRequest) {
  const session = await getSession();           // verify JWT, get org_id
  if (!session) return NextResponse.json({ error: 'Unauthenticated' }, { status: 401 });

  const jwt = getSessionJwt(req);              // raw token forwarded to Central Server
  try {
    const data = await centralFetch<MyType>('/admin/resource', {}, jwt);
    return NextResponse.json(data);
  } catch (e) {
    if (e instanceof ApiError) return NextResponse.json({ error: e.body }, { status: e.status });
    return NextResponse.json({ error: 'Internal error' }, { status: 500 });
  }
}
```

For mutating routes (POST/PUT/DELETE), add CSRF validation before the session check:
```typescript
import { validateCsrf } from '@/lib/utils/csrf';
try { validateCsrf(req); } catch { return NextResponse.json({ error: 'CSRF' }, { status: 403 }); }
```

For new config entities, use `makeConfigHandlers(basePath)` from
`lib/api/config-route-helpers.ts` — it handles all four HTTP verbs plus invalidation.

---

## Key Files Map

| File | What it does |
|---|---|
| `middleware.ts` | RBAC guard — runs on every request; JWT → permissions → redirect/rewrite |
| `lib/api/central-server.ts` | `centralFetch<T>()` — the one function all BFF routes use |
| `lib/auth/session.ts` | `getSession()`, `signSession()`, `getSessionJwt()` |
| `lib/auth/rbac.ts` | `hasPermission()`, `requirePermission()` |
| `lib/utils/sse.ts` | SSE client with backoff; used by `useLiveFeed` hook |
| `lib/utils/csrf.ts` | `validateCsrf(req)` — call in every mutating BFF route |
| `lib/api/config-route-helpers.ts` | `makeConfigHandlers(basePath)` — generic CRUD factory |
| `types/api.ts` | All API response shapes — add new shapes here |
| `types/rbac.ts` | `ConsolePermission` union + `ROUTE_PERMISSION_MAP` |
| `store/ui.ts` | `useUIStore` — timeRange, theme, sideNav |
| `store/notifications.ts` | `useNotificationStore` — toast queue |
| `components/config/CrudTable.tsx` | Reusable CRUD table for all config pages |
| `hooks/useLiveFeed.ts` | SSE consumer + cross-invalidation of pods/budget queries |
| `lib/charts/echarts-theme.ts` | `registerCarbonTheme()` — call once per chart component |
| `lib/charts/jointjs-styles.ts` | Carbon token constants for JointJS node styling |

---

## Adding a New Page

1. Create `app/(console)/your-module/page.tsx`
2. Add a SideNav entry in `components/shell/AppSideNav.tsx`
3. Add the route+permission mapping in `middleware.ts` (`ROUTE_PERMISSION_MAP` in `types/rbac.ts`)
4. If it needs data: add a BFF route in `app/api/your-module/route.ts`
5. If it's CRUD: use `makeConfigHandlers` for the BFF + `<CrudTable>` for the UI

## Adding a New Config Entity

1. BFF: `app/api/config/entity-name/[[...slug]]/route.ts` using `makeConfigHandlers('/admin/config/entity-name')`
2. Page: follow the pattern in `app/(console)/config/skills/page.tsx`
3. Type: add the record interface to `types/api.ts`

---

## State Management

| State type | Store | Invalidated by |
|---|---|---|
| Time range / theme | `store/ui.ts` (Zustand) | User interaction |
| Metrics data | TanStack Query `['metrics', {...}]` | Time range change, 30s interval |
| Pod list | TanStack Query `['pods']` | SSE `pod_heartbeat`, 15s interval |
| Budget summary | TanStack Query `['budget']` | SSE `budget_alert`, 30s interval |
| Config entities | TanStack Query `['config', 'entity']` | Mutation success |
| Toast notifications | `store/notifications.ts` (Zustand) | Auto-dismiss (5s) or user close |
| Live feed | Local state in `useLiveFeed` | Not cached — SSE stream only |

---

## Known Issues / Gotchas

- **Carbon peer dep:** `@carbon/react` declares `react@"^18"` peer dep but works with
  React 19. The project root `.npmrc` sets `legacy-peer-deps=true` so plain `npm install`
  works. Never remove that file.

- **ECharts `yAxis.name` with undefined:** The ECharts `EChartsOption` type from `echarts`
  does not allow `name: string | undefined` with `exactOptionalPropertyTypes`. Use conditional
  object spread: `yAxis: name ? { type: 'value', name } : { type: 'value' }`.

- **Carbon `DatePicker onChange` type:** The Carbon `DatePicker` `onChange` callback signature
  is `(dates: Date[]) => void`, not `([from, to]: [Date?, Date?]) => void`. Access elements
  by index: `dates[0]`, `dates[1]`.

- **Carbon `ContentSwitcher` requires `size` prop:** Without it TypeScript errors.
  Always pass `size="sm"`, `size="md"`, or `size="lg"`.

- **`MultiSelect` does not accept `style` prop:** Wrap in a `<div style={...}>` instead.

- **`SkeletonPlaceholder` does not accept `style` prop:** Same — use a wrapper `<div>`.

- **`middleware.ts` is now `proxy.ts` with a `proxy` export:** Next.js 16 renamed the
  route-interceptor from `middleware.ts` → `proxy.ts`, and the exported function from
  `middleware` → `proxy`. Both the filename and the function name must change:
  ```ts
  // proxy.ts (project root)
  export async function proxy(req: NextRequest) { … }
  export const config = { matcher: […] };
  ```

- **Carbon components require `'use client'` in RSC files:** Carbon v11 uses
  `React.createContext()` at module scope. Any file that imports from `@carbon/react`
  without `'use client'` will crash at runtime with `createContext only works in Client
  Components`. Rules:
  - If the file does **no** server-side data fetching → add `'use client'` at the top.
  - If the file **must** stay RSC (e.g. calls `centralFetch` or `getSession`) → extract the
    Carbon JSX into a separate `*View.tsx` client component and pass data as props.
  - `(console)/layout.tsx` is the exception: it stays RSC but avoids Carbon's `Content`
    component — uses a plain `<div>` instead (Carbon's `Content` uses context internally).

- **Carbon CSS import — use `@carbon/styles/css/styles.css`:** `@carbon/react` ships
  only SCSS, not pre-compiled CSS. The correct import for `app/layout.tsx` is:
  `import "@carbon/styles/css/styles.css"` — not `@carbon/react/css/components.css`.

- **Playwright E2E tests must not be in `test/`:** They live in `e2e/`. The vitest config
  has `include: ['test/**/*.{test,spec}.ts']` to prevent Playwright specs from running
  under Vitest (they would fail with "Playwright Test did not expect test() here").

---

## CI Checks (all must pass before merge)

```bash
npm run type-check   # zero TypeScript errors
npm run lint         # zero ESLint warnings
npm run test         # ≥80% coverage (vitest)
npm audit --audit-level=high   # zero high/critical CVEs
```
