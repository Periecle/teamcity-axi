GO ?= go
TEAMCITY_AXI_TEST_BINARY ?=

.PHONY: build test test-real-cli test-live format format-check docs docs-check vet check package test-package source-check
build:
	$(GO) build -trimpath -o bin/teamcity-axi ./cmd/teamcity-axi
format:
	gofmt -w assets.go cmd internal tests
format-check:
	@test -z "$$(gofmt -l assets.go cmd internal tests)" || (gofmt -l assets.go cmd internal tests; exit 1)
test: format-check
	$(GO) test ./...
test-real-cli:
	$(GO) run ./cmd/axi-dev verify-native "$(TEAMCITY_AXI_TEST_BINARY)"
	TEAMCITY_AXI_TEST_BINARY="$(TEAMCITY_AXI_TEST_BINARY)" $(GO) test -tags realcli ./...
test-live:
	$(GO) run ./cmd/axi-dev verify-native "$(TEAMCITY_AXI_TEST_BINARY)"
	TEAMCITY_AXI_TEST_BINARY="$(TEAMCITY_AXI_TEST_BINARY)" $(GO) test -tags live ./...
docs:
	$(GO) run ./cmd/axi-dev docs
docs-check:
	$(GO) run ./cmd/axi-dev docs --check
vet:
	$(GO) vet ./...
source-check:
	$(GO) run ./cmd/axi-dev verify-go-tree
check: test vet docs-check source-check
package: build
	mkdir -p release
	tar -czf release/teamcity-axi-$$(go env GOOS)-$$(go env GOARCH).tar.gz -C bin teamcity-axi -C .. LICENSE README.md CHANGELOG.md THIRD_PARTY_LICENSES.txt docs/commands.md docs/dependencies.md
test-package: package
	TEAMCITY_AXI_PACKAGE="$(CURDIR)/release/teamcity-axi-$$(go env GOOS)-$$(go env GOARCH).tar.gz" $(GO) test -tags packageaudit ./tests/distribution -count=1
