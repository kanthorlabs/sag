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
AC_SKIP ?=
REPORT := e2e/report
LDFLAGS := -X github.com/kanthorlabs/sag/internal/cli.Version=$(VERSION)
E2E_TEST := $(GO) test -tags e2e -json -count=1 -timeout 30m

.PHONY: build image image-conformance test test-integration e2e e2e-matrix contract-check dev-up dev-down

build:
	CGO_ENABLED=1 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/sag ./cmd/sag

image:
	podman build --build-arg VERSION=$(VERSION) -t $(IMAGE) -f deploy/Containerfile .

image-conformance:
	podman build --build-arg VERSION=$(VERSION) -t $(CONFORMANCE_IMAGE) -f e2e/conformance/Containerfile .

test:
	$(GO) vet ./...
	$(GO) test -count=1 ./...

test-integration:
	SAG_TEST_PG_MAJOR=$(PG) $(GO) test -tags integration -count=1 $(if $(RUN),-run '$(RUN)') $(PKG)

e2e: test image image-conformance
	mkdir -p $(REPORT)/pg$(PG)
	SAG_TEST_PG_MAJOR=$(PG) SAG_AC_SKIP='$(AC_SKIP)' $(E2E_TEST) $(if $(RUN),-run 'TestE2E/.*/$(RUN)') ./e2e/... > $(REPORT)/pg$(PG)/events.json; \
	$(GO) run ./e2e/cmd/acreport -layer $(LAYER) -in $(REPORT)/pg$(PG)/events.json -out $(REPORT)

e2e-matrix: test image image-conformance
	for m in $(PGS); do $(MAKE) test-integration PG=$$m PKG=./... RUN= || exit 1; done
	pids=""; \
	for m in $(PGS); do \
		mkdir -p $(REPORT)/pg$$m; \
		SAG_TEST_PG_MAJOR=$$m SAG_AC_SKIP='$(AC_SKIP)' $(E2E_TEST) ./e2e/... > $(REPORT)/pg$$m/events.json & \
		pids="$$pids $$!"; \
	done; \
	for p in $$pids; do wait $$p || true; done; \
	$(GO) run ./e2e/cmd/acreport -layer $(LAYER) $(foreach m,$(PGS),-in $(REPORT)/pg$(m)/events.json) -out $(REPORT)

contract-check:
	$(GO) build ./...
	$(GO) vet ./...
	$(GO) vet -tags integration ./...
	$(GO) vet -tags e2e ./e2e/...
	$(GO) test -count=1 ./internal/core/...

dev-up:
	./scripts/dev-up.sh

dev-down:
	./scripts/dev-down.sh
