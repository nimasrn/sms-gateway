# SMS Gateway

High-performance SMS delivery system built with Go, Redis Streams, and PostgreSQL. Routes messages through multiple providers with intelligent load balancing, circuit breaking, and priority queues.

## Features

- 🚀 **High Throughput**: 10M+ messages/day with worker pool architecture
- 🔄 **Smart Routing**: Weight-based provider selection with automatic failover
- ⚡ **Priority Queues**: Normal and express lanes for time-sensitive messages
- 🛡️ **Circuit Breaker**: Automatic provider health monitoring and recovery
- 📊 **Observability**: Prometheus metrics, structured logging, distributed tracing
- 💾 **Read/Write Split**: PostgreSQL with replica support for scalability
- 🔁 **Retry Logic**: Exponential backoff with Dead Letter Queue (DLQ)

## Architecture

```
┌─────────────┐      ┌──────────────┐      ┌─────────────┐
│   Client    │─────▶│  API Server  │─────▶│Redis Streams│
└─────────────┘      └──────────────┘      └─────────────┘
                            │                      │
                            ▼                      ▼
                     ┌──────────────┐      ┌─────────────┐
                     │  PostgreSQL  │      │  Processor  │
                     │ (Read/Write) │      │ (100 Workers)│
                     └──────────────┘      └─────────────┘
                                                   │
                                                   ▼
                                           ┌──────────────┐
                                           │SMS Providers │
                                           │ (3 with weights)│
                                           └──────────────┘
```

**Current Capacity**: 10M messages/day
**Target (with Kafka migration)**: 100M+ messages/day

## Architecture Diagrams

Detailed C4 model diagrams available in `docs/diagrams/`:

### Current State
- [System Context](docs/diagrams/01-context.puml) - High-level overview
- [Container Diagram](docs/diagrams/02-container.puml) - Services and data stores
- [Deployment](docs/diagrams/09-deployment.puml) - Infrastructure setup

### Target Architecture (100M+ msg/day)
- [Target Architecture](docs/diagrams/10-target-architecture.puml) - Kafka microservices design
- [Migration Trade-offs](docs/diagrams/11-architecture-tradeoffs.puml) - Current vs Target comparison

**View diagrams**: Use [PlantUML VSCode extension](https://marketplace.visualstudio.com/items?itemName=jebbs.plantuml) or [PlantUML Online](http://www.plantuml.com/plantuml/uml/)

### Key Architectural Decisions

| Aspect | Current (10M/day) | Target (100M+/day) |
|--------|-------------------|-------------------|
| **Queue** | Redis Streams (50K ops/sec) | Apache Kafka (605 MB/s, 15x faster) |
| **Architecture** | Monolithic (API + Processor) | Microservices (10 services) |
| **Provider Protocol** | HTTP APIs (10-20 msg/sec) | SMPP (1,000 msg/sec per bind) |
| **Scaling** | Vertical (bigger instances) | Horizontal (auto-scaling) |

See `docs/diagrams/11-architecture-tradeoffs.puml` for detailed migration roadmap.

## Quick Start

### Prerequisites

- Go 1.21+
- PostgreSQL 15+
- Redis 7+

### Installation

```bash
# Clone repository
git clone <repository-url>
cd sms-gateway

# Install dependencies
make tidy

# Copy environment template
cp .env.example .env

# Run database migrations
make run-cli

# Start services (separate terminals)
make run-api        # API Server :8080
make run-processor  # Message Processor :9100
make run-operator   # Mock Provider :8081 (dev only)
```

### Docker Compose

```bash
docker-compose up -d
```

## Usage

### Send SMS

```bash
curl -X POST http://localhost:8080/api/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "+1234567890",
    "message": "Hello from SMS Gateway",
    "customer_id": 1
  }'
```

### Send Priority SMS (Express Queue)

```bash
curl -X POST http://localhost:8080/api/v1/messages/express \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "+1234567890",
    "message": "OTP: 123456",
    "customer_id": 1
  }'
```

### Check Message Status

```bash
curl http://localhost:8080/api/v1/messages/1/delivery-reports
```

## Configuration

Key environment variables (see `.env.example`):

```bash
# API Server
APP_ADDR=:8080

# Database (Read/Write Split)
POSTGRES_WRITE_HOST=localhost
POSTGRES_WRITE_PORT=5432
POSTGRES_READ_HOST=localhost
POSTGRES_READ_PORT=5432

# Redis Streams
REDIS_HOST=localhost:6379
QUEUE_NAME=sms-queue
QUEUE_EXPRESS_NAME=sms-queue-express

# Provider URLs (weights: 100, 80, 60)
PROVIDER_PRIMARY_URL=http://primary-provider.com
PROVIDER_SECONDARY_URL=http://secondary-provider.com
PROVIDER_BACKUP_URL=http://backup-provider.com
```

## Monitoring

### Prometheus Metrics

```bash
curl http://localhost:9100/metrics
```

### Health Check

```bash
curl http://localhost:8080/health
```

## Development

### Commands

```bash
make run-api          # Run API server
make run-processor    # Run message processor
make run-operator     # Run mock provider
make run-cli          # Run migrations

make test             # Run tests
make test-coverage    # Generate coverage report

make build            # Build all binaries
make tidy             # Update dependencies

make docker-up        # Start with Docker
make docker-down      # Stop Docker services
```

### Project Structure

```
├── cmd/
│   ├── api/          # API server entrypoint
│   ├── processor/    # Message processor
│   ├── operator/     # Mock SMS provider
│   └── cli/          # Migration tool
├── internal/
│   ├── handlers/     # HTTP request handlers
│   ├── services/     # Business logic
│   ├── repository/   # Database access
│   ├── gateways/     # Provider integration
│   ├── queue/        # Redis Streams wrapper
│   └── processor/    # Message processing service
├── pkg/
│   ├── http/         # FastHTTP server
│   ├── pg/           # PostgreSQL with read/write split
│   ├── redis/        # Redis adapter
│   ├── logger/       # Structured logging
│   ├── prom/         # Prometheus metrics
│   └── worker/       # Worker pool
├── migrations/       # Database migrations
└── docs/            # Architecture diagrams
```

## Performance

**Current Architecture (Redis Streams + Monolith)**:
- 100 concurrent workers
- 10 Redis Stream consumers
- ~1,000 messages/sec sustained
- ~10M messages/day realistic

**Bottlenecks**:
- Redis Streams single-node (50K ops/sec max)
- HTTP provider APIs (10-20 msg/sec per connection)
- No horizontal auto-scaling

**Migration Path to 100M/day**:
1. Redis Streams → Apache Kafka (15x throughput)
2. Monolith → Microservices (independent scaling)
3. HTTP APIs → SMPP protocol (50-100x throughput)

See `docs/diagrams/10-target-architecture.puml` for target state.

## Testing

```bash
# Unit tests
go test ./...

# With race detector
go test -race ./...

# Coverage
make test-coverage
open coverage.html

# Load testing
ab -n 100000 -c 100 http://localhost:8080/api/v1/messages
```

## Contributing

1. Fork the repository
2. Create feature branch (`git checkout -b feature/amazing`)
3. Commit changes (`git commit -am 'Add amazing feature'`)
4. Push to branch (`git push origin feature/amazing`)
5. Open Pull Request

## License

MIT License

## Documentation

- 📖 **[Architecture Guide](docs/ARCHITECTURE.md)** - Complete C4 diagrams and design docs
