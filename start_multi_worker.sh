#!/bin/bash
# Start Tinpot with multiple specialized workers

set -e

echo "🚀 Starting Tinpot with Multiple Workers"
echo "========================================"

# Start all services
docker-compose -f docker-compose.multi-worker.yml up -d

echo ""
echo "✅ Services started:"
echo "   - Redis (broker)"
echo "   - API (port 8000)"
echo "   - Worker (default queue) - Maintenance, General"
echo "   - Worker (devops queue) - DevOps, Database"
echo ""

# Wait for services to be healthy
echo "⏳ Waiting for services to be ready..."
sleep 5

# Check worker status
echo ""
echo "📊 Worker Status:"
echo ""
docker-compose -f docker-compose.multi-worker.yml exec -T worker-default celery -A app.worker inspect active_queues 2>/dev/null || echo "Default worker: Starting..."
docker-compose -f docker-compose.multi-worker.yml exec -T worker-devops celery -A app.worker inspect active_queues 2>/dev/null || echo "DevOps worker: Starting..."

echo ""
echo "✅ Tinpot is running!"
echo ""
echo "📍 Access points:"
echo "   - Web UI: http://localhost:8000"
echo "   - API: http://localhost:8000/api/actions"
echo ""
echo "🛠️ Management commands:"
echo "   - View logs: docker-compose -f docker-compose.multi-worker.yml logs -f"
echo "   - Stop: docker-compose -f docker-compose.multi-worker.yml down"
echo "   - View queues: docker-compose -f docker-compose.multi-worker.yml exec redis redis-cli LLEN default"
echo "   - Scale workers: docker-compose -f docker-compose.multi-worker.yml up -d --scale worker-default=4"
echo ""
