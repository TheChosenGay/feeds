MODULES := ./pkg/... ./services/gateway/... ./services/feed/... ./services/user/...
GO_VERSION := 1.25.3

.PHONY: build tidy test compose-up proto bootstrap pin-proto-go

build:
	@go build $(MODULES)

test:
	@go test $(MODULES)

compose-up:
	@docker compose up -d

# tidy all go modules (auto-discovered via go.work)
tidy:
	@-go work sync
	@for dir in $$(find . -name 'go.mod' -not -path '*/.git/*' | sed 's|/go.mod||'); do \
		echo "=> tidy $$dir"; \
		(cd $$dir && go mod tidy); \
	done

# buf generate writes go.mod with the local Go version; pin to GO_VERSION afterward.
pin-proto-go:
	@for f in proto/gen/*/go.mod; do \
		sed -i '' 's/^go .*/go $(GO_VERSION)/' "$$f"; \
	done

proto:
	@buf generate
	@$(MAKE) pin-proto-go

# first-time setup: generate proto + init go.mod for generated code
bootstrap: proto
	@for dir in $$(find proto/gen -maxdepth 1 -mindepth 1 -type d 2>/dev/null); do \
		if [ ! -f $$dir/go.mod ]; then \
			mod=$$(echo $$dir | sed 's|proto/gen/|github.com/TheChosenGay/feeds/proto/gen/|'); \
			echo "=> init $$mod"; \
			(cd $$dir && go mod init $$mod && go mod tidy); \
		fi; \
	done
	@$(MAKE) pin-proto-go
	@$(MAKE) tidy
