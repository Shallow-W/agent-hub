<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex or another agent-capable tool, additional project-scoped helpers may live in:
- `.agents/skills/` — reusable Trellis skills
- `.codex/agents/` — optional custom subagents

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

<!-- TRELLIS:END -->

## Project

AgentHub — IM-chat-driven multi-agent collaboration platform. Users chat with multiple AI Agents (Claude Code, Codex, OpenCode) through a Feishu/WeChat-style interface. Supports 1-on-1 chat, group chat, task dispatch, and artifact preview.

## Monorepo Layout

```
src/
  backend/        Go backend (module: github.com/agent-hub/backend)
    cmd/server/main.go          — entrypoint, all DI wiring happens here
    internal/{handler,service,repository,model,middleware}/
    pkg/{ws,redis}/             — shared packages (WebSocket hub, Redis client)
    config/config.yaml          — gitignored; copy from config.example.yaml
    migrations/001..023_*.sql   — numbered SQL migrations, applied in order
  frontend/       React SPA (Vite + TypeScript)
    src/{api,components,hooks,store,views,types,layout}/
    e2e/                         — Playwright E2E tests
  daemon/         Go daemon for local agent scanning & process management (separate go.mod)
  daemon-npm/     npm wrapper @agenthub/daemon for the Go daemon
scripts/
  dev.sh          starts postgres + runs migrations + backend + frontend
  build.sh        builds backend binary + frontend dist
```

## Dev Commands

```bash
# Start infra (PostgreSQL 15 + Redis 7)
docker compose up -d

# Backend
cd src/backend
cp config/config.example.yaml config/config.yaml   # edit JWT secret + DB password
go run ./cmd/server/                                 # starts on :8080
# Live-reload: air (config in .air.toml)

# Frontend
cd src/frontend
npm install
npm run dev        # starts on :5173, proxies /api→:8080 and /ws→ws://:8080

# Build both
bash scripts/build.sh   # outputs bin/server + src/frontend/dist/

# Go tests (no external deps needed for unit tests — they use fakes)
cd src/backend
go test ./internal/service/...    # run all service tests
go test ./internal/handler/...    # handler tests

# E2E (requires running backend + frontend)
cd src/frontend
npx playwright install            # first time only
npm run test:e2e:agent            # runs e2e/agent-connect.spec.ts
```

## Key Architecture Facts

- **Backend DI**: All dependency wiring is in `cmd/server/main.go`. Handlers receive Service interfaces, Services receive Repository interfaces. No `init()` or global state.
- **WebSocket**: Two separate WS endpoints — `/ws?token=` for user chat and a daemon WS (token-authenticated via `config.yaml → daemon.token`). The WS hub is in `pkg/ws/`.
- **Migrations**: Sequential numbered SQL files in `src/backend/migrations/`. Applied by `dev.sh` via `psql` or manually. No migration tool binary in the build.
- **Frontend proxy**: Vite dev server proxies `/api` and `/ws` to the Go backend on `:8080`. In production, the Go server serves the frontend dist and handles routing.
- **Daemon ↔ Backend**: The daemon connects over WebSocket with a shared token from config. It scans local machines for agent processes and reports status.
- **Orchestrator**: `internal/service/orchestrator.go` — handles group-chat intent parsing, task splitting, and multi-agent dispatch.

## Conventions

- **Comments**: Chinese (explain "why"). **Names**: English.
- **Line endings**: LF only (not CRLF).
- **Go**: tabs, `log/slog` for structured logging, errors wrapped with `%w`, interfaces defined at consumer side.
- **Frontend**: 2-space indent, CSS Modules (`*.module.css`), path alias `@/` → `src/`, strict TypeScript (`noUnusedLocals`, `noUnusedParameters`, `noUncheckedIndexedAccess`).
- **Commit format**: `type(scope): 中文描述` — e.g. `feat(chat): 实现WebSocket流式消息推送`. Scopes: `agent|api|auth|chat|daemon|db|orchestrator|preview|ui|conventions|doc`.
- **Branches**: `feature/<desc>`, `fix/<desc>`, `refactor/<desc>`, `docs/<desc>`. No direct push to `main`.

## Zustand Pitfalls

These have caused real bugs in this codebase:
- Never inline `?? []` or `?? {}` in selectors — use module-level constants to avoid infinite re-renders.
- Use precise selectors (`s.messages[convId]`), not the whole store slice.
- `useEffect` deps must be primitives (IDs), not object references.
- Never use `Set`/`Map` in Zustand stores — use `Record<string, boolean>`.

## Config

`src/backend/config/config.example.yaml` documents all fields. `config.yaml` is gitignored. Default DB credentials: `agenthub:agenthub@localhost:5432/agenthub`. Redis optional — degrades gracefully when absent.

## Docs

| Need | Location |
|------|----------|
| Product requirements | `doc/需求文档.md` |
| Architecture | `doc/architecture/overview.md` |
| Project structure | `doc/conventions/project-structure.md` |
| Frontend conventions | `doc/conventions/frontend-conventions.md` |
| Backend conventions | `doc/conventions/backend-conventions.md` |
| Git conventions | `doc/conventions/git-conventions.md` |
| API reference | `doc/reference/api.md` |
| Module tasks | `doc/task/M0-基础设施.md` ~ `doc/task/M10-Pin上下文.md` |
| Task progress | `doc/TASKLIST.md` |
