#!/bin/bash
# Test script for verifying subpath deployment

echo "Testing Tinpot Subpath Deployment"
echo "=================================="
echo ""

# Check if docker-compose is available
if ! command -v docker-compose &> /dev/null; then
    echo "❌ docker-compose not found. Please install docker-compose to run tests."
    echo ""
    echo "Alternative: Install dependencies and run manually:"
    echo "  pip install -r requirements.txt"
    echo "  uvicorn app.main:app --root-path /tinpot"
    exit 1
fi

echo "Using Docker Compose for testing..."
echo ""

# Test 1: Root path deployment
echo "Test 1: Root path deployment (ROOT_PATH not set)"
echo "Building and starting services..."

# Stop any running containers
docker-compose down -v 2>/dev/null || true

# Start services without ROOT_PATH
docker-compose up -d redis api 2>&1 | grep -v "WARNING"
sleep 5

echo "Testing root endpoint..."
curl -s http://127.0.0.1:8000/ | grep -q "Tinpot" && echo "✓ Index page served" || echo "✗ Index page failed"

echo "Testing API endpoint..."
curl -s http://127.0.0.1:8000/api/actions | grep -q "{" && echo "✓ API endpoint accessible" || echo "✗ API endpoint failed"

echo "Testing health endpoint..."
curl -s http://127.0.0.1:8000/health | grep -q "healthy" && echo "✓ Health check passed" || echo "✗ Health check failed"

echo "Checking BASE_PATH is empty (root deployment)..."
curl -s http://127.0.0.1:8000/ | grep -q 'window.BASE_PATH = ""' && echo "✓ BASE_PATH empty for root" || echo "ℹ BASE_PATH not empty"

echo "Stopping services..."
docker-compose down 2>&1 | grep -v "WARNING"
sleep 2

echo ""
echo "Test 2: Subpath deployment (ROOT_PATH=/tinpot)"

# Create temporary docker-compose override for subpath testing
cat > docker-compose.override.yml << 'EOF'
version: '3.8'
services:
  api:
    environment:
      - ROOT_PATH=/tinpot
    command: uvicorn app.main:app --host 0.0.0.0 --port 8000 --root-path /tinpot
EOF

echo "Starting services with ROOT_PATH=/tinpot..."
docker-compose up -d redis api 2>&1 | grep -v "WARNING"
sleep 5

echo "Testing root endpoint..."
curl -s http://127.0.0.1:8000/ | grep -q 'window.BASE_PATH = "/tinpot"' && echo "✓ BASE_PATH injected correctly" || echo "✗ BASE_PATH injection failed"

echo "Testing API endpoint (at root)..."
curl -s http://127.0.0.1:8000/api/actions | grep -q "{" && echo "✓ API endpoint accessible at root" || echo "✗ API endpoint failed"

echo "Testing OpenAPI docs path..."
curl -s http://127.0.0.1:8000/docs | grep -q "swagger" && echo "✓ OpenAPI docs accessible" || echo "✗ OpenAPI docs failed"

echo "Testing health endpoint..."
curl -s http://127.0.0.1:8000/health | grep -q "healthy" && echo "✓ Health check passed" || echo "✗ Health check failed"

echo "Stopping services..."
docker-compose down 2>&1 | grep -v "WARNING"

# Cleanup override file
rm -f docker-compose.override.yml

echo ""
echo "=================================="
echo "Testing complete!"
echo ""
echo "For nginx subpath deployment:"
echo "  1. Set ROOT_PATH=/tinpot in environment"
echo "  2. Start uvicorn with: --root-path /tinpot"
echo "  3. Configure nginx with rewrite rules (see DEPLOYMENT.md)"
echo ""
echo "The application will:"
echo "  - Serve all routes with correct OpenAPI paths"
echo "  - Inject BASE_PATH into frontend JavaScript"
echo "  - Work correctly behind nginx reverse proxy"
