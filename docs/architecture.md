# Architecture Documentation

## System Overview

This RabbitMQ implementation follows clean architecture principles with clear separation of concerns and dependency inversion.

## Layer Structure

```
┌─────────────────────────────────────────────────────┐
│              Application Layer (cmd/)               │
│         Producer CLI          Consumer CLI          │
└────────────────────┬────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────┐
│           Service Layer (internal/service/)         │
│              Business Logic & Orchestration         │
└────────────────────┬────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────┐
│          Transport Layer (pkg/rabbitmq/)            │
│    Producer, Consumer, Connector, Interfaces        │
└────────────────────┬────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────┐
│              RabbitMQ (AMQP 0.9.1)                  │
│            Exchange, Queues, Bindings               │
└─────────────────────────────────────────────────────┘
```

## Component Diagram

```
┌──────────────────────────────────────────────────────────────┐
│                      Application                              │
│                                                               │
│  ┌─────────────┐              ┌──────────────┐              │
│  │  Producer   │              │  Consumer    │              │
│  │    CLI      │              │    CLI       │              │
│  └──────┬──────┘              └──────┬───────┘              │
│         │                             │                       │
│         │         ┌───────────────────┘                       │
│         │         │                                           │
│  ┌──────▼─────────▼──────┐                                   │
│  │  Message Service      │  ← Business Logic                 │
│  └──────┬────────────────┘                                   │
│         │                                                     │
└─────────┼─────────────────────────────────────────────────────┘
          │
┌─────────▼─────────────────────────────────────────────────────┐
│                   RabbitMQ Package                            │
│                                                               │
│  ┌─────────────┐    ┌──────────────┐    ┌──────────────┐   │
│  │  Connector  │◄───┤  Producer    │    │  Consumer    │   │
│  └──────┬──────┘    └──────────────┘    └──────┬───────┘   │
│         │                                        │            │
│         │           ┌────────────────────────────┘            │
│         │           │                                         │
│  ┌──────▼───────────▼────┐                                   │
│  │   Connection Pool     │                                   │
│  │   (with reconnect)    │                                   │
│  └───────────────────────┘                                   │
│                                                               │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │   Metrics    │  │   Dedupe     │  │   Logging    │      │
│  │   Recorder   │  │   Store      │  │              │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└───────────────────────────────────────────────────────────────┘
          │
          │ AMQP Protocol
          ▼
┌─────────────────────────────────────────────────────────────┐
│                      RabbitMQ Broker                         │
│                                                              │
│  ┌─────────┐      ┌──────────┐      ┌──────────┐          │
│  │Exchange │─────►│  Queue   │─────►│  Queue   │          │
│  │ (topic) │      │  (main)  │      │ (retry)  │          │
│  └─────────┘      └────┬─────┘      └────┬─────┘          │
│                        │                   │                 │
│                        └───────┬───────────┘                 │
│                                ▼                             │
│                         ┌──────────┐                         │
│                         │   DLQ    │                         │
│                         └──────────┘                         │
└─────────────────────────────────────────────────────────────┘
```

## Connection Management Flow

```
Application Start
      ↓
┌─────────────────┐
│ Create Factory  │
└────────┬────────┘
         ↓
┌─────────────────┐
│Create Connector │
└────────┬────────┘
         ↓
┌─────────────────┐
│  Connect()      │──────┐
└────────┬────────┘      │
         │               │
         ↓               │
   ┌──────────┐          │
   │Connected?│──No──────┘ Retry with backoff
   └────┬─────┘
        │Yes
        ↓
   Start Reconnect Loop (goroutine)
        │
        ├──► Monitor connection close
        │
        └──► Auto-reconnect on failure
             (exponential backoff)
```

## Producer Flow

```
Application
    │
    ├──► Create Message
    │
    ├──► Call producer.Publish()
    │        │
    │        ├──► Get Connection
    │        │
    │        ├──► Open Channel
    │        │
    │        ├──► Enable Confirms
    │        │
    │        ├──► Publish Message
    │        │
    │        ├──► Wait for Confirmation
    │        │        │
    │        │        ├──► ACK → Success
    │        │        │
    │        │        └──► NACK → Retry
    │        │                    │
    │        └────────────────────┘
    │                (with exponential backoff)
    │
    └──► Return Result
```

## Consumer Flow

```
Start Consumer
    │
    ├──► Create Worker Pool
    │        │
    │        └──► Workers (goroutines)
    │
    ├──► Start Consuming
    │        │
    │        └──► Receive Messages
    │                    │
    │                    ├──► Check Duplicate?
    │                    │    (Yes) → ACK & Skip
    │                    │
    │                    ├──► Process Message
    │                    │        │
    │                    │        ├──► Success
    │                    │        │    │
    │                    │        │    ├──► ACK
    │                    │        │    └──► Add to Dedupe Store
    │                    │        │
    │                    │        └──► Failure
    │                    │             │
    │                    │             ├──► Check Retry Count
    │                    │             │
    │                    │             ├──► < Max Retries
    │                    │             │    │
    │                    │             │    ├──► Send to Retry Queue
    │                    │             │    └──► ACK Original
    │                    │             │
    │                    │             └──► ≥ Max Retries
    │                    │                  │
    │                    │                  ├──► Send to DLQ
    │                    │                  └──► ACK Original
    │                    │
    │                    └──► Loop
    │
    └──► Graceful Shutdown
         │
         ├──► Cancel Context
         ├──► Wait for Workers
         └──► Close Connections
```

## Retry and DLQ Pattern

```
Message Processing Failed
         │
         ├──► Get Retry Count from Headers
         │
         ├──► Retry Count < Max?
         │        │
         │        │ Yes
         │        ├──► Increment Retry Count
         │        ├──► Publish to Retry Queue
         │        │    (with x-retry-count header)
         │        └──► ACK Original Message
         │
         │ No
         └──► Retry Count ≥ Max
              │
              ├──► Publish to DLQ
              └──► ACK Original Message


Retry Queue:
  - Has x-message-ttl (e.g., 5000ms)
  - Has x-dead-letter-exchange: ""
  - Has x-dead-letter-routing-key: "main_queue"
  │
  └──► After TTL expires → Routes back to Main Queue
       │
       └──► Consumer tries again
```

## Message Structure

```json
{
  "headers": {
    "x-message-id": "uuid",
    "x-trace-id": "trace-uuid",
    "x-retry-count": 0,
    "x-source": "service-name",
    "x-timestamp": 1234567890
  },
  "properties": {
    "content_type": "application/json",
    "delivery_mode": 2,
    "timestamp": "2024-01-01T00:00:00Z"
  },
  "body": {
    "id": "msg-123",
    "content": "actual message data",
    "timestamp": "2024-01-01T00:00:00Z"
  }
}
```

## Interface Design

The project uses interfaces for testability and flexibility:

```go
// Core abstractions
AMQPConnection interface
    ├── Channel() (AMQPChannel, error)
    ├── Close() error
    └── NotifyClose(chan *amqp.Error) chan *amqp.Error

AMQPChannel interface
    ├── Qos(int, int, bool) error
    ├── Confirm(bool) error
    ├── PublishWithContext(...) error
    └── Consume(...) (<-chan Delivery, error)

ConnectionFactory interface
    └── Dial(string) (AMQPConnection, error)

// Observability
MetricsRecorder interface
    ├── IncPublished()
    ├── IncPublishFailed()
    └── IncPublishRetry()

DedupeStore interface
    ├── Exists(messageID string) bool
    └── Add(messageID string)
```

## Concurrency Model

### Producer
- Single connection shared across goroutines
- Each publish operation opens a new channel
- Channels are NOT shared (not thread-safe)
- Publisher confirms ensure message delivery

### Consumer
- Worker pool pattern (configurable workers)
- Each worker has its own goroutine
- Shared delivery channel from RabbitMQ
- Manual acknowledgments prevent message loss

### Connector
- Background goroutine for reconnection monitoring
- Mutex-protected connection access
- Non-blocking reconnect loop
- Graceful shutdown with WaitGroup

## Error Handling Strategy

```
Error Occurs
    │
    ├──► Transient Error? (network, connection)
    │    │
    │    └──► Retry with exponential backoff
    │         └──► Max retries → Return error
    │
    ├──► Application Error? (validation, business logic)
    │    │
    │    └──► Send to DLQ immediately
    │         └──► ACK original message
    │
    └──► Poison Message? (repeated failures)
         │
         └──► Send to DLQ
              └──► ACK original message
              └──► Alert/Log for investigation
```

## Configuration Management

```
Environment Variables
         │
         ├──► Load at startup
         │
         ├──► Validate
         │
         ├──► Create Config struct
         │
         └──► Pass to components

Config Hierarchy:
  ├── Connection (URL, timeouts)
  ├── Exchange/Queue names
  ├── Consumer settings (prefetch, workers)
  ├── Retry settings (max retries, delays)
  └── Reconnection settings (backoff, max wait)
```

## Metrics and Observability

```
Application
    │
    ├──► Producer Metrics
    │    ├── published_total
    │    ├── publish_failed_total
    │    └── publish_retry_total
    │
    ├──► Consumer Metrics
    │    ├── received_total
    │    ├── processed_total
    │    ├── failed_total
    │    └── retried_total
    │
    ├──► Structured Logging
    │    ├── Request ID tracking
    │    ├── Error context
    │    └── Performance metrics
    │
    └──► Health Checks
         ├── Connection status
         ├── Channel status
         └── Queue depth
```

## Testing Strategy

### Unit Tests
- Mock all external dependencies (Connection, Channel)
- Test business logic in isolation
- Test error handling and retry logic
- Test concurrency patterns

### Integration Tests (Optional)
- Use real RabbitMQ instance (Docker)
- Test end-to-end flows
- Test failure scenarios
- Test performance under load

## Deployment Considerations

### Scalability
- Horizontal: Multiple consumer instances
- Vertical: Increase worker count per instance
- Queue partitioning for high throughput

### High Availability
- RabbitMQ cluster with mirrored queues
- Multiple consumer instances
- Load balancer for producers
- Health checks for auto-scaling

### Monitoring
- Queue depth alerts
- Consumer lag monitoring
- Error rate tracking
- Connection pool metrics

## Dependencies

```
Direct Dependencies:
├── github.com/rabbitmq/amqp091-go (AMQP client)
├── github.com/sirupsen/logrus (Logging)
└── github.com/stretchr/testify (Testing)

Indirect Dependencies:
└── Standard library (context, sync, time, etc.)
```

## Design Decisions

### Why rabbitmq/amqp091-go?
- Official maintained fork of streadway/amqp
- Active development and bug fixes
- Compatible with RabbitMQ 3.x+
- Go modules support

### Why Manual ACKs?
- Prevents message loss on processing errors
- Enables retry logic
- Better control over message flow

### Why Publisher Confirms?
- Ensures messages are persisted
- Detects routing failures
- Critical for data consistency

### Why Exponential Backoff?
- Prevents overwhelming the broker during issues
- Gives system time to recover
- Standard pattern for distributed systems

### Why Worker Pool?
- Controlled concurrency
- Prevents resource exhaustion
- Better backpressure management
- Easier to tune performance
