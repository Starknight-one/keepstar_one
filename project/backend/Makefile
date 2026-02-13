ENV_FILE := $(shell for f in .env ../.env ../../project/.env; do [ -f "$$f" ] && echo "$$f" && break; done)

# Export DATABASE_URL and ANTHROPIC_API_KEY from .env (handles & in URLs)
define load_env
	$(if $(ENV_FILE),export DATABASE_URL="$$(grep '^DATABASE_URL=' $(ENV_FILE) | cut -d= -f2-)"; \
	export ANTHROPIC_API_KEY="$$(grep '^ANTHROPIC_API_KEY=' $(ENV_FILE) | cut -d= -f2-)"; \
	export LLM_MODEL="$$(grep '^LLM_MODEL=' $(ENV_FILE) | cut -d= -f2-)";)
endef

.PHONY: test-unit test-integration test-usecase test-llm test-all

# Layer 1: Domain + Tools (pure Go, 0 deps, <1 sec)
test-unit:
	go test -v -count=1 -race \
		./internal/domain/ \
		./internal/tools/

# Layer 2 + 3: DB Integration + API Smoke (requires DATABASE_URL, ~30 sec)
test-integration:
	$(load_env) go test -v -count=1 -timeout 120s \
		./internal/adapters/postgres/ \
		./internal/handlers/

# Layer 4: Usecase Integration — real DB, mock LLM (~20 sec)
test-usecase:
	$(load_env) go test -v -count=1 -timeout 120s \
		./internal/usecases/

# Layer 5: LLM Integration (requires DATABASE_URL + ANTHROPIC_API_KEY, ~120 sec)
test-llm:
	$(load_env) go test -v -count=1 -timeout 300s -run "TestAgent1Execute_Integration|TestPromptCaching" \
		./internal/usecases/

# All layers except LLM (Layer 1-4)
test-all:
	$(load_env) go test -v -count=1 -race -timeout 180s \
		./internal/domain/ \
		./internal/tools/ \
		./internal/adapters/postgres/ \
		./internal/handlers/ \
		./internal/usecases/
