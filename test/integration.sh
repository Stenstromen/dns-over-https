#!/bin/bash
set -e

echo "Starting integration tests..."

# Function to cleanup on exit
cleanup() {
    echo "Cleaning up..."
    podman-compose down
}

# Register cleanup function
trap cleanup EXIT

# Start the services
echo "Building and starting services..."
podman-compose build --no-cache
podman-compose up -d

# Wait for services to be ready
echo "Waiting for services to be ready..."
sleep 3

# First request - should be a cache MISS
echo "ℹ️ Testing first DNS request..."
RESPONSE=$(curl -s -H "Accept: application/json" "http://localhost:8053/getnsrecord?name=podman-desktop.io&type=TXT")
ANSWER=$(echo $RESPONSE | jq -r '.Answer[0].data' | tr -d '"')
EXPECTED="google-site-verification=quQGxmY-gXc3frcrpUE5WSGdxP4MLkDrYb5ObzJscaE"

if [ "$ANSWER" != "$EXPECTED" ]; then
    echo "❌ Test failed: Expected '$EXPECTED' but got '$ANSWER'"
    exit 1
fi

echo "✅ First request successful, testing cache..."

# Second request - should be a cache HIT
CACHE_STATUS=$(curl -s -H "Accept: application/json" -I "http://localhost:8053/getnsrecord?name=podman-desktop.io&type=TXT" | grep -i "x-cache-status" | tr -d '\r')

if [[ "$CACHE_STATUS" != *"HIT"* ]]; then
    echo "❌ Cache test failed: Expected 'X-Cache-Status: HIT' but got '$CACHE_STATUS'"
    exit 1
fi

echo "✅ Cache test successful"
echo "✅ All tests passed!" 
