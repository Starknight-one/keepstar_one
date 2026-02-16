# Project Memory

## Tools & Paths

- **psql**: `/opt/homebrew/Cellar/libpq/18.1_1/bin/psql`
- **psql (alt)**: `/opt/homebrew/Cellar/postgresql@15/15.15_1/bin/psql`

## Database

- Chat backend .env: `project/.env` (contains DATABASE_URL, TENANT_SLUG, API keys)
- Default tenant slug: `keepstart`

## Dev Servers

- Chat backend: `project/backend/` → port 8080
- Chat frontend: `project/frontend/` → port 5173
- Admin backend: `project_admin/backend/` → port 8081
- Admin frontend: `project_admin/frontend/` → port 5174
- Start scripts: `scripts/start.sh`, `scripts/start_admin.sh`, `scripts/start_all.sh`

## Crawled Data

- Enriched catalog: `project_admin/Crawler_results/crawl_enriched_967.json` (967 products, heybabes)
