ENV_FILE := $(shell for f in .env ../.env ../../project/.env; do [ -f "$$f" ] && echo "$$f" && break; done)

# Export DATABASE_URL and ANTHROPIC_API_KEY from .env (handles & in URLs)
define load_env
	$(if $(ENV_FILE),export DATABASE_URL="$$(grep '^DATABASE_URL=' $(ENV_FILE) | cut -d= -f2-)"; \
	export ANTHROPIC_API_KEY="$$(grep '^ANTHROPIC_API_KEY=' $(ENV_FILE) | cut -d= -f2-)"; \
	export LLM_MODEL="$$(grep '^LLM_MODEL=' $(ENV_FILE) | cut -d= -f2-)";)
endef
