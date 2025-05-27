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
	@echo "Starting integration tests..."
	@(trap 'echo "Cleaning up..."; podman-compose down' EXIT; \
	echo "Building and starting services..."; \
	podman-compose build --no-cache; \
	podman-compose up -d; \
	echo "Waiting for services to be ready..."; \
	sleep 3; \
	echo "ℹ️ Testing first DNS request..."; \
	RESPONSE=$$(curl -s -H "Accept: application/json" "http://localhost:8053/getnsrecord?name=podman-desktop.io&type=TXT"); \
	ANSWER=$$(echo $$RESPONSE | jq -r '.Answer[0].data' | tr -d '"'); \
	EXPECTED="google-site-verification=quQGxmY-gXc3frcrpUE5WSGdxP4MLkDrYb5ObzJscaE"; \
	if [ "$$ANSWER" != "$$EXPECTED" ]; then \
		echo "❌ Test failed: Expected '$$EXPECTED' but got '$$ANSWER'" && exit 1; \
	fi; \
	echo "✅ First request successful, testing cache..."; \
	CACHE_STATUS=$$(curl -s -H "Accept: application/json" -I "http://localhost:8053/getnsrecord?name=podman-desktop.io&type=TXT" | grep -i "x-cache-status" | tr -d '\r'); \
	if [[ "$$CACHE_STATUS" != *"HIT"* ]]; then \
		echo "❌ Cache test failed: Expected 'X-Cache-Status: HIT' but got '$$CACHE_STATUS'" && exit 1; \
	fi; \
	echo "✅ Cache test successful"; \
	echo "✅ All tests passed!")