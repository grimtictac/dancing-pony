# Production load testing, included from the Makefile.
#
# PROD_SSH reaches the droplet as root (add "-i KEY" to it if needed).
# PROD_CA is DigitalOcean's CA certificate for the database. The fixture
# target opens the same SSH tunnel as deploy/README.md describes.
PROD_URL ?= https://silson.dev
PROD_SSH ?= ssh root@silson.dev
PROD_DB_HOST ?= private-dancing-pony-postgres18-do-user-6189174-0.i.db.ondigitalocean.com
PROD_CA ?=
PROD_TUNNEL := /tmp/pony-prod-tunnel
PROD_PSQL := docker run --rm -i --network host -e PGPASSWORD -v "$(PROD_CA):/ca.crt:ro" \
	postgres:18.6-alpine@sha256:123b17998ccaf47e53b502e8c6a1e79afd8f579c6f3eec83c563da026f957395 \
	psql -q -v ON_ERROR_STOP=1 "postgres://doadmin@127.0.0.1:15434/dancingpony?sslmode=verify-ca&sslrootcert=/ca.crt"

.PHONY: load-fixture-prod load-test-prod
load-fixture-prod: ## Add the load-test restaurant to production (PROD_CA=...; asks for doadmin's password)
	@test -n "$(PROD_CA)" || { echo "set PROD_CA to the database CA certificate"; exit 1; }
	@printf "doadmin password: "; stty -echo; read -r PGPASSWORD; stty echo; echo; export PGPASSWORD; \
	$(PROD_SSH) -S $(PROD_TUNNEL) -O exit 2>/dev/null; \
	$(PROD_SSH) -M -S $(PROD_TUNNEL) -f -N -o ExitOnForwardFailure=yes \
		-L 15434:$(PROD_DB_HOST):25060 || exit 1; \
	trap '$(PROD_SSH) -S $(PROD_TUNNEL) -O exit 2>/dev/null' EXIT; \
	$(PROD_PSQL) -c "SELECT rolname, rolsuper, rolbypassrls FROM pg_roles WHERE rolname IN ('doadmin', 'pony_app')" && \
	$(PROD_PSQL) < loadtest/fixture.sql

load-test-prod: ## Browse the load-test restaurant in production from here (RATE=100 DURATION=30s)
	@$(PROD_SSH) curl -sf 127.0.0.1:9091/metrics > /tmp/pony-metrics-before.txt
	-$(K6) run -e BASE_URL=$(PROD_URL) -e REMOTE=1 -e RATE=$(RATE) -e DURATION=$(DURATION) browse.js
	@$(PROD_SSH) curl -sf 127.0.0.1:9091/metrics > /tmp/pony-metrics-after.txt
	@echo; echo "Server-side latency, inside the app (no network):"
	@python3 loadtest/latency.py /tmp/pony-metrics-before.txt /tmp/pony-metrics-after.txt
