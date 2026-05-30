MODULES := ./pkg/... ./services/gateway/... ./services/post/... ./services/feed/... ./services/user/...

.PHONY: build tidy test compose-up

build:
	@go build $(MODULES)

test:
	@go test $(MODULES)

compose-up:
	@docker compose up -d

tidy:
	@for dir in pkg services/gateway services/post services/feed services/user; do \
		echo "=> tidy $$dir"; \
		(cd $$dir && go mod tidy); \
	done
