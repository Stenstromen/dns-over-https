.PHONY: build compose-up compose-down test test-deps

build:
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o doh-server/doh-server ./doh-server

compose-up:
	podman-compose build --no-cache
	podman-compose up

compose-down:
	podman-compose down

test-deps:
	@which podman >/dev/null 2>&1 || (echo "podman is required but not installed. Aborting." && exit 1)
	@which curl >/dev/null 2>&1 || (echo "curl is required but not installed. Aborting." && exit 1)
	@which jq >/dev/null 2>&1 || (echo "jq is required but not installed. Aborting." && exit 1)

test: test-deps
	@./test/integration.sh