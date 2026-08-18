# LinkedIn Inbox Downloader — builds run only inside Docker (no host Go required).

.DEFAULT_GOAL := help

GO_IMAGE      := golang:1.24-bookworm
MODULE        := github.com/derkalle4/linkedin-inbox-downloader
BIN_NAME      := linkedin-inbox-downloader
BIN_DIR       := bin
VERSION       := $(shell tr -d '[:space:]' < VERSION)
LDFLAGS_PROD  := -s -w -X $(MODULE)/internal/version.Version=$(VERSION)
# Run the container as the invoking user so bin/ is not owned by root.
UID_GID       := $(shell id -u):$(shell id -g)
# Named Docker volumes for Go module + build caches (removed by make clean).
GOMOD_VOL     := linkedin-inbox-downloader-gomod
GOCACHE_VOL   := linkedin-inbox-downloader-gocache

DOCKER_RUN := docker run --rm \
	--user $(UID_GID) \
	-v "$(CURDIR):/src" \
	-v "$(GOMOD_VOL):/cache/go-mod" \
	-v "$(GOCACHE_VOL):/cache/go-build" \
	-w /src \
	-e HOME=/tmp \
	-e GOMODCACHE=/cache/go-mod \
	-e GOCACHE=/cache/go-build \
	-e CGO_ENABLED=0 \
	-e GOOS=linux \
	-e GOARCH=amd64 \
	-e GOTOOLCHAIN=auto \
	$(GO_IMAGE)

.PHONY: help prod debug clean check-docker prepare-cache

help: ## Show this help
	@echo "LinkedIn Inbox Downloader  v$(VERSION)"
	@echo ""
	@echo "Builds run inside Docker as your user ($(UID_GID)) — no local Go install required."
	@echo "Go module/build caches live in Docker volumes (removed by make clean)."
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

check-docker:
	@command -v docker >/dev/null 2>&1 || { \
		echo "Error: Docker is required but was not found in PATH."; \
		echo "Install Docker, then run make again."; \
		exit 1; \
	}

prepare-cache: check-docker
	@docker volume create $(GOMOD_VOL) >/dev/null
	@docker volume create $(GOCACHE_VOL) >/dev/null
	@# Named volumes are root-owned on first create; make them writable for --user.
	@docker run --rm \
		-v "$(GOMOD_VOL):/cache/go-mod" \
		-v "$(GOCACHE_VOL):/cache/go-build" \
		alpine:3.21 \
		chown -R $(UID_GID) /cache/go-mod /cache/go-build
	@# Recreate or reclaim bin/ if a previous root-owned build left it unwritable.
	@if [ -e "$(BIN_DIR)" ] && [ ! -w "$(BIN_DIR)" ]; then \
		echo "Reclaiming $(BIN_DIR)/ ownership via Docker…"; \
		docker run --rm -v "$(CURDIR):/src" -w /src alpine:3.21 \
			chown -R $(UID_GID) /src/$(BIN_DIR); \
	fi
	@mkdir -p $(BIN_DIR)
	@if [ -e "$(BIN_DIR)/$(BIN_NAME)" ] && [ ! -w "$(BIN_DIR)/$(BIN_NAME)" ]; then \
		docker run --rm -v "$(CURDIR):/src" -w /src alpine:3.21 \
			chown $(UID_GID) /src/$(BIN_DIR)/$(BIN_NAME); \
	fi

prod: prepare-cache ## Build stripped linux/amd64 binary → bin/
	$(DOCKER_RUN) \
		go build -trimpath -buildvcs=false -ldflags "$(LDFLAGS_PROD)" \
			-o /src/$(BIN_DIR)/$(BIN_NAME) \
			./cmd/linkedin-inbox-downloader
	@echo "Built $(BIN_DIR)/$(BIN_NAME) (prod, v$(VERSION))"

debug: prepare-cache ## Build unstripped debug binary → bin/
	$(DOCKER_RUN) \
		go build -buildvcs=false -gcflags "all=-N -l" \
			-ldflags "-X $(MODULE)/internal/version.Version=$(VERSION)" \
			-o /src/$(BIN_DIR)/$(BIN_NAME) \
			./cmd/linkedin-inbox-downloader
	@echo "Built $(BIN_DIR)/$(BIN_NAME) (debug, v$(VERSION))"

clean: check-docker ## Remove bin/* and Docker Go cache volumes
	@rm -rf $(BIN_DIR) dist 2>/dev/null || true
	@# Drop leftover host .cache from older Makefile versions (may be root-owned).
	@if [ -e .cache ]; then \
		docker run --rm -v "$(CURDIR):/src" -w /src alpine:3.21 rm -rf /src/.cache; \
	fi
	-docker volume rm $(GOMOD_VOL) $(GOCACHE_VOL) >/dev/null 2>&1
	@echo "Removed $(BIN_DIR)/ and cache volumes $(GOMOD_VOL), $(GOCACHE_VOL)"
