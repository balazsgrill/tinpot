#!/bin/bash
# Test runner script - executes tests in Docker environment

set -e

echo "🧪 Starting Tinpot Test Suite"
echo "================================"

# Start all services
echo "📦 Starting services..."
docker-compose up -d redis api worker selenium-chrome

# Wait for services to be healthy
echo "⏳ Waiting for services to be ready..."
sleep 5

# Run tests
echo "🚀 Running tests..."
docker-compose --profile test run --rm test-runner

# Capture exit code
TEST_EXIT_CODE=$?

# Show test results location
echo ""
echo "================================"
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "✅ All tests passed!"
else
    echo "❌ Some tests failed (exit code: $TEST_EXIT_CODE)"
fi

echo "📊 Test results saved to: ./test-results/results.xml"
echo ""

# Optional: Keep services running for debugging
echo "🛑 Stopping services..."
docker-compose down

exit $TEST_EXIT_CODE
