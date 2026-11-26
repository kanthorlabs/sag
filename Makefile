GO ?= go
GOTOOLCHAIN ?= auto
export GOTOOLCHAIN

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE ?= localhost/sag:e2e
CONFORMANCE_IMAGE ?= localhost/sag-conformance:e2e
export SAG_E2E_IMAGE := $(IMAGE)
export SAG_E2E_CONFORMANCE_IMAGE := $(CONFORMANCE_IMAGE)
LAYER ?= 1
PG ?= 17
PGS ?= 14 17 19
PKG ?= ./...
RUN ?=
SUITE ?= Main
PHASE ?= A
AC_SKIP =
REPORT := e2e/report
DEVX_IMAGE := localhost/sag:devx-e2e
LDFLAGS := -X github.com/kanthorlabs/sag/internal/cli.Version=$(VERSION)
E2E_TEST := $(GO) test -tags e2e -json -count=1 -timeout 30m
SHARED_ROOT := $(patsubst %/.git,%,$(shell git rev-parse --path-format=absolute --git-common-dir))
DEVX_LOCK := $(SHARED_ROOT)/.dev/claims/devx-e2e.lock

.PHONY: build build-devxtest image image-conformance test test-integration e2e e2e-devx e2e-matrix contract-check dev-up dev-down

build:
	CGO_ENABLED=1 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/sag ./cmd/sag

build-devxtest:
	CGO_ENABLED=1 $(GO) build -tags devxtest -trimpath -ldflags "$(LDFLAGS)" -o bin/sag-devxtest ./cmd/sag

define build_image
	old=$$(podman image inspect --format '{{.Id}}' $(1) 2>/dev/null); \
	podman build --layers=false --build-arg VERSION=$(VERSION) $(if $(4),--label $(4)) -t $(1) -f $(2) $(3) && \
	new=$$(podman image inspect --format '{{.Id}}' $(1)) && \
	if [ -n "$$old" ] && [ "$$old" != "$$new" ]; then podman rmi "$$old" >/dev/null 2>&1 || true; fi
endef

image:
	$(call build_image,$(IMAGE),deploy/Containerfile,.)

image-conformance:
	$(call build_image,$(CONFORMANCE_IMAGE),e2e/conformance/Containerfile,.)

test:
	$(GO) vet ./...
	$(GO) test -count=1 ./...

test-integration:
	SAG_TEST_PG_MAJOR=$(PG) $(GO) test -tags integration -count=1 $(if $(RUN),-run '$(RUN)') $(PKG)

e2e: test image image-conformance
	mkdir -p $(REPORT)/pg$(PG)
	SAG_TEST_PG_MAJOR=$(PG) SAG_AC_SKIP='$(AC_SKIP)' $(E2E_TEST) $(if $(RUN),-run 'TestE2E/.*/($(RUN))') ./e2e/... > $(REPORT)/pg$(PG)/events.json; \
	$(GO) run ./e2e/cmd/acreport -layer $(LAYER) -skip '$(AC_SKIP)' $(if $(RUN),-run '$(RUN)') -in $(REPORT)/pg$(PG)/events.json -out $(REPORT)

e2e-matrix: test image image-conformance
	for m in $(PGS); do $(MAKE) test-integration PG=$$m PKG=./... RUN= || exit 1; done
	pids=""; \
	for m in $(PGS); do \
		mkdir -p $(REPORT)/pg$$m; \
		SAG_TEST_PG_MAJOR=$$m SAG_AC_SKIP='$(AC_SKIP)' $(E2E_TEST) ./e2e/... > $(REPORT)/pg$$m/events.json & \
		pids="$$pids $$!"; \
	done; \
	for p in $$pids; do wait $$p || true; done; \
	$(GO) run ./e2e/cmd/acreport -layer $(LAYER) -skip '$(AC_SKIP)' $(foreach m,$(PGS),-in $(REPORT)/pg$(m)/events.json) -out $(REPORT)

e2e-devx:
	@mkdir -p "$(SHARED_ROOT)/.dev/claims"; \
	owner="$(DEVX_LOCK)/owner"; \
	pid=$$(cat "$$owner" 2>/dev/null | awk '{print $$1}'); \
	if [ -n "$$pid" ] && ! kill -0 "$$pid" 2>/dev/null; then \
		echo "Removing stale devx-e2e lock (PID $$pid not running)"; \
		rm -rf "$(DEVX_LOCK)"; \
	fi; \
	until mkdir "$(DEVX_LOCK)" 2>/dev/null; do echo "Waiting for devx-e2e lock $(DEVX_LOCK)..."; sleep 30; done; \
	token=$$(openssl rand -hex 16); \
	echo "$$PPID $$token" > "$(DEVX_LOCK)/owner"; \
	trap 'rm -rf "$(DEVX_LOCK)"; exit 130' INT; trap 'rm -rf "$(DEVX_LOCK)"' EXIT TERM; \
	$(MAKE) build build-devxtest || exit 1; \
	$(call build_image,$(DEVX_IMAGE),deploy/Containerfile,.,sag.devx-e2e=1) || exit 1; \
	mkdir -p $(REPORT)/devx-$(SUITE); \
	git rev-parse HEAD > $(REPORT)/devx-$(SUITE)/commit; \
	SAG_TEST_IMAGE=$(DEVX_IMAGE) DEVX_RUN_TOKEN=$$token $(GO) test -tags devxe2e -json -count=1 -timeout 90m \
		-run '^TestDevx$(SUITE)$$/.*$(if $(RUN),/($(RUN)))' \
		./e2e/devx/ > $(REPORT)/devx-$(SUITE)/events.json; \
	$(GO) run ./e2e/cmd/acreport -suite devx -layer $(LAYER) -phase $(PHASE) \
		$(if $(RUN),-run '$(RUN)' -in $(REPORT)/devx-$(SUITE)/events.json,$$(for f in $(REPORT)/devx-*/events.json; do printf -- '-in %s ' "$$f"; done)) \
		-out $(REPORT)

contract-check:
	$(GO) build ./...
	$(GO) vet ./...
	$(GO) vet -tags integration ./...
	$(GO) vet -tags e2e ./e2e/...
	$(GO) vet -tags devxtest ./...
	$(GO) vet -tags devxe2e ./e2e/devx/...
	$(GO) test -count=1 ./internal/core/...

dev-up:
	./scripts/dev-up.sh

dev-down:
	./scripts/dev-down.sh
