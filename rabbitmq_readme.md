# RabbitMQ Go Production-Ready Demo

Production-ready RabbitMQ implementation in Go with comprehensive unit tests, automatic reconnection, retry logic, dead-letter queues, and graceful shutdown.

## Features

### Architecture
- Clean architecture with separated layers (transport, service, config)
- Interface-based design for easy testing and mocking
- Comprehensive error handling and logging
- Graceful shutdown with context cancellation

### Connection Management
- Automatic reconnection with exponential backoff
- Connection health monitoring
- Non-blocking reconnect loop
- Proper resource cleanup

### Producer
- Publisher confirms for reliable message delivery
- Configurable retry logic with exponential backoff
- Structured logging with message tracking
- Metrics for observability

### Consumer
- Manual acknowledgments (Ack/Nack)
- Worker pool with configurable concurrency
- QoS/prefetch for backpressure management
- Retry queue with TTL → Dead Letter Queue (DLQ) pattern
- Idempotency support with deduplication store
- Graceful shutdown

### Observability
- Structured logging with logrus
- Basic metrics (published, received, processed, failed, retries)
- Extendable to Prometheus/other monitoring systems

## Project Structure

```
.
├── cmd/
│   ├── producer/          # Producer CLI application
│   └── consumer/          # Consumer CLI application
├── configs/               # Configuration management
├── internal/
│   └── service/          # Business logic layer
├── pkg/
│   └── rabbitmq/         # RabbitMQ client library
│       ├── interfaces.go  # Abstractions for testing
│       ├── connector.go   # Connection management
│       ├── producer.go    # Producer implementation
│       ├── consumer.go    # Consumer implementation
│       ├── dedupe.go      # Idempotency support
│       ├── metrics.go     # Metrics collection
│       └── *_test.go     # Unit tests
├── docker-compose.yml     # RabbitMQ for local development
├── Makefile              # Common commands
└── README.md
```

## Dependencies

This project uses:
- **github.com/rabbitmq/amqp091-go** - Official RabbitMQ Go client (maintained fork of streadway/amqp)
- **github.com/sirupsen/logrus** - Structured logging
- **github.com/stretchr/testify** - Testing assertions and mocking

## Prerequisites

- Go 1.21 or higher
- Docker and Docker Compose (for local RabbitMQ)
- Make (optional, for convenience commands)

## Quick Start

### 1. Start RabbitMQ

```bash
# Start RabbitMQ container
make docker-up

# Setup queues and exchanges
make setup-queues
```

RabbitMQ Management UI will be available at http://localhost:15672 (guest/guest)

### 2. Run Tests

```bash
# Run all unit tests
make test

# Run tests with coverage
make test-coverage
```

### 3. Run Producer

```bash
# Terminal 1 - Run producer
make run-producer
```

### 4. Run Consumer

```bash
# Terminal 2 - Run consumer  
make run-consumer
```

## Configuration

All configuration is done through environment variables with sensible defaults:

| Variable | Description | Default |
|----------|-------------|---------|
| `RABBITMQ_URL` | RabbitMQ connection URL | `amqp://guest:guest@localhost:5672/` |
| `EXCHANGE` | Exchange name | `demo_exchange` |
| `QUEUE` | Main queue name | `demo_queue` |
| `ROUTING_KEY` | Routing key for messages | `demo.routing.key` |
| `RETRY_QUEUE` | Retry queue name | `demo_queue_retry` |
| `DLQ` | Dead letter queue name | `demo_queue_dlq` |
| `PREFETCH_COUNT` | Consumer prefetch count | `10` |
| `WORKER_COUNT` | Number of consumer workers | `5` |
| `MAX_RETRIES` | Max retry attempts | `3` |
| `RETRY_DELAY_MS` | Delay between retries (ms) | `1000` |
| `RECONNECT_BACKOFF_MS` | Initial reconnect backoff (ms) | `1000` |
| `MAX_RECONNECT_WAIT_MS` | Max reconnect wait time (ms) | `30000` |

### Example with custom config:

```bash
RABBITMQ_URL=amqp://user:pass@prod-host:5672/ \
WORKER_COUNT=10 \
MAX_RETRIES=5 \
go run cmd/consumer/main.go
```

## Testing

### Unit Tests

All components have comprehensive unit tests with mocked dependencies:

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with race detector
go test -race ./...

# Run specific package tests
go test -v ./pkg/rabbitmq/
```

### Test Coverage

The project includes tests for:
- ✅ Successful message publishing with confirms
- ✅ Publish retry logic on transient failures
- ✅ Consumer ack/nack logic
- ✅ Retry → DLQ routing
- ✅ Connection reconnection logic
- ✅ Duplicate message handling
- ✅ Graceful shutdown

### Integration Testing

For integration tests with real RabbitMQ:

```bash
# Start RabbitMQ
make docker-up
make setup-queues

# Run integration tests (if implemented)
INTEGRATION_TESTS=true go test ./...
```

## Production Considerations

### Idempotency

The project includes an `InMemoryDedupeStore` for demonstration. **In production**, replace this with a distributed store:

```go
// Example with Redis
type RedisDedupeStore struct {
    client *redis.Client
    ttl    time.Duration
}

func (r *RedisDedupeStore) Exists(messageID string) bool {
    exists, _ := r.client.Exists(ctx, messageID).Result()
    return exists > 0
}

func (r *RedisDedupeStore) Add(messageID string) {
    r.client.Set(ctx, messageID, "1", r.ttl)
}
```

### Metrics

The `SimpleMetrics` implementation is for demonstration. In production, integrate with Prometheus:

```go
import "github.com/prometheus/client_golang/prometheus"

type PrometheusMetrics struct {
    published prometheus.Counter
    failed    prometheus.Counter
    // ... other metrics
}
```

### Security

- Use TLS for production: `amqps://user:pass@host:5671/`
- Store credentials in secrets management (Vault, AWS Secrets Manager)
- Use separate credentials for producer and consumer
- Enable RabbitMQ authentication and authorization

### High Availability

- Use RabbitMQ cluster with mirrored/quorum queues
- Deploy multiple consumer instances
- Use load balancer for producers
- Monitor queue depths and consumer lag

### Message Persistence

All messages in this demo use `DeliveryMode: amqp.Persistent` for durability. Ensure queues are also declared as durable.

## Queue Topology

```
Producer → Exchange (demo_exchange)
              ↓
           Queue (demo_queue) ← Consumer
              ↓ (on failure)
           Retry Queue (demo_queue_retry, TTL: 5s)
              ↓ (after TTL)
           Back to main queue
              ↓ (max retries exceeded)
           Dead Letter Queue (demo_queue_dlq)
```

## Troubleshooting

### Connection Issues

```bash
# Check RabbitMQ is running
docker ps | grep rabbitmq

# Check RabbitMQ logs
docker logs rabbitmq-demo

# Check connection from Go app
RABBITMQ_URL=amqp://guest:guest@localhost:5672/ go run cmd/producer/main.go
```

### Queue Not Found

```bash
# Re-run setup
make setup-queues

# Verify queues exist
docker exec rabbitmq-demo rabbitmqctl list_queues
```

### High Memory Usage

- Reduce `PREFETCH_COUNT`
- Reduce `WORKER_COUNT`
- Check for message processing bottlenecks
- Monitor queue depths

## Development

### Adding New Features

1. Add interface in `pkg/rabbitmq/interfaces.go`
2. Implement in appropriate file
3. Write unit tests with mocks
4. Update README

### Code Quality

```bash
# Format code
go fmt ./...

# Run linter
make lint  # requires golangci-lint

# Run tests
make test
```

## License

MIT License

## References

- [RabbitMQ Go Client](https://github.com/rabbitmq/amqp091-go)
- [RabbitMQ Documentation](https://www.rabbitmq.com/documentation.html)
- [Go Testing Best Practices](https://go.dev/doc/tutorial/add-a-test)
