# Init

Bootstrap ADW for this project. Scans structure, detects stack, generates config.

## Instructions

You are initializing the ADW (Agent Development Workflow) system for this project.

### Step 1: Scan Project Structure

Search for common patterns to detect project structure:

**Backend detection:**
- `go.mod` -> Go backend
- `requirements.txt` or `pyproject.toml` -> Python backend
- `package.json` with express/fastapi/nest -> Node backend
- `Cargo.toml` -> Rust backend

**Frontend detection:**
- `package.json` with react/vue/svelte/angular
- `vite.config.*`, `next.config.*`, `nuxt.config.*`

**Database detection:**
- `*.db` files -> SQLite
- `docker-compose.yml` with postgres/mysql
- Migration folders

**Common folder names:**
- Backend: `backend`, `server`, `api`, `src` (if no frontend)
- Frontend: `frontend`, `web`, `client`, `app`
- Database: `DB`, `db`, `data`, `database`

### Step 2: Generate adw.yaml

Based on detected structure, create `ADW/adw.yaml`:

1. Read `ADW/adw.yaml.template`
2. Fill in detected values
3. Set appropriate validation commands:

**Go backend:**
```yaml
cmd: "go build ./..."  # build
cmd: "go test ./..."   # test
```

**Node/TypeScript:**
```yaml
cmd: "npm run build"   # build
cmd: "npm test"        # test
cmd: "npm run lint"    # lint
```

**Python:**
```yaml
cmd: "python -m pytest"  # test
cmd: "ruff check ."      # lint
```

### Step 3: Create Expert Domains

For each detected domain, create expert structure:

```
.claude/commands/experts/{domain}/
├── expertise.yaml    # Empty template
├── question.md       # Standard question handler
└── self-improve.md   # Standard sync handler
```

Use templates from `.claude/commands/experts/_templates/`

### Step 4: Update Commands

If paths differ from defaults, update commands that reference paths:
- `sdlc.md`
- `test.md`
- `build.md`

### Step 5: Report

Output summary:
```
ADW Initialized

Project: {name}
Structure:
  Backend: {path} ({language}/{framework})
  Frontend: {path} ({framework})
  Database: {type}

Created:
  - ADW/adw.yaml
  - Experts: backend, frontend, database

Next steps:
  1. Review ADW/adw.yaml
  2. Run /cold_start to initialize session
  3. Run /sync-experts after building features
```

### Edge Cases

**Empty project:**
- Create minimal adw.yaml with empty paths
- Skip expert creation
- Suggest: "Add your app structure, then run /init again"

**Monorepo:**
- Detect multiple package.json or go.mod
- Ask user which paths to use

**Unknown stack:**
- Leave validation commands empty
- Note in output: "Could not detect stack, please fill ADW/adw.yaml manually"
