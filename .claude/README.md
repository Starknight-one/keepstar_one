# Claude Code Configuration

Agent layer for development automation.

## Structure

```
.claude/
├── commands/           # Slash commands
│   ├── init.md         # Auto-configure project
│   ├── sdlc.md         # Full pipeline
│   ├── feature.md      # Plan features
│   ├── bug.md          # Plan bug fixes
│   ├── chore.md        # Plan tech tasks
│   ├── build.md        # Implement plans
│   ├── test.md         # Run validations
│   ├── accept.md       # Commit & push
│   ├── commit.md       # Git commit
│   ├── cold_start.md   # Session init
│   ├── sync-experts.md # Sync expertise
│   └── experts/
│       └── _templates/ # Expert templates
└── agents/             # Custom agents
```

## First Time Setup

1. Copy agent-kit to your project
2. Build your app structure
3. Run `/init` to auto-configure
4. Run `/cold_start` to verify

## Commands

### SDLC Pipeline

| Command | Purpose |
|---------|---------|
| `/sdlc "desc"` | Full automation |
| `/feature "desc"` | Create feature plan |
| `/bug "desc"` | Create bug fix plan |
| `/chore "desc"` | Create tech task plan |
| `/build specs/...` | Implement from plan |
| `/test` | Run validations |
| `/accept` | Commit and push |

### Setup & Maintenance

| Command | Purpose |
|---------|---------|
| `/init` | Auto-detect and configure |
| `/cold_start` | Initialize session |
| `/sync-experts` | Update expertise |

## Adding Experts

1. Copy templates from `experts/_templates/`
2. Create `experts/{domain}/` folder
3. Add to `ADW/adw.yaml` experts list
4. Run `/sync-experts` to populate
