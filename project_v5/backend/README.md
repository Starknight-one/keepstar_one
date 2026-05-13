# Keepstar V5 backend

V5 chat engine. Replaces V4's Formation/Widget/Atom engine with a Go port of v9's scene-graph engine
(`/Users/starknight/Keepstar_project/Keepstar_one_v9/packages/domain/`), enriched with V4's binding
layer, sectional state with delta-stream, transition graph, and actions.

**Status**: in-progress. Engine port (chunk 1 of plan) underway. Not wired to HTTP/DB/LLM yet.

See `docs/v5-engine-plan.md` for the high-level plan and `docs/Updates/v5/` for session logs.

## Layout

```
cmd/server/             entry point (stub)
internal/
  domain/               typed entities (chunk 2+)
  engine/               v9 scene-graph port (chunk 1, current)
  ports/                interface definitions
  adapters/             concrete implementations (Postgres, Anthropic, …)
  usecases/             pipeline / agent1 / agent2 / state ops
  handlers/             HTTP
  tools/                LLM tools (search, visual_assembly v5)
  prompts/              system prompts
```

## Development

```sh
go build ./...
go test ./internal/engine/...
```
