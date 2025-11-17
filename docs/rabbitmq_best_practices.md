# RabbitMQ Best Practices

## Message Design

### Use Message IDs
Always include a unique message ID for idempotency and tracking:

```go
headers := amqp.Table{
    "x-message-id": uuid.New().String(),
}
```

### Keep Messages Small
- Target < 1MB per message
- For large payloads, use object storage (S3) and send references
- Use compression for large JSON payloads

### Include Metadata
```go
headers := amqp.Table{
    "x-message-id":  messageID,
    "x-trace-id":    traceID,        // For distributed tracing
    "x-source":      "service-name",
    "x-timestamp":   time.Now().Unix(),
    "x-retry-count": 0,
}
```

## Connection Management

### Connection Pooling
- Use one connection per application
- Use multiple channels for concurrent operations
- Channels are NOT thread-safe - use one per goroutine or protect with mutex

```go
// ❌ Bad - sharing channel across goroutines
ch, _ := conn.Channel()
go publishMessage(ch, msg1)
go publishMessage(ch, msg2)

// ✅ Good - channel per goroutine
go func() {
    ch, _ := conn.Channel()
    defer ch.Close()
    publishMessage(ch, msg1)
}()
```

### Heartbeats
Configure appropriate heartbeat intervals:

```go
// For stable networks
conn, err := amqp.DialConfig(url, amqp.Config{
    Heartbeat: 10 * time.Second,
})
```

## Publisher Best Practices

### Always Use Publisher Confirms
```go
ch.Confirm(false)
confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

// Publish
ch.PublishWithContext(ctx, exchange, key, false, false, msg)

// Wait for confirm
select {
case confirm := <-confirms:
    if !confirm.Ack {
        // Handle nack
    }
}
```

### Set Mandatory Flag Carefully
```go
// Use mandatory=true to detect unroutable messages
ch.PublishWithContext(ctx, exchange, key, true, false, msg)

returns := ch.NotifyReturn(make(chan amqp.Return, 1))
// Handle returned messages
```

### Message Persistence
```go
msg := amqp.Publishing{
    DeliveryMode: amqp.Persistent, // Survive broker restart
    ContentType:  "application/json",
    Body:         body,
}
```

## Consumer Best Practices

### QoS and Prefetch
```go
// Limit unacked messages per consumer
ch.Qos(
    10,    // prefetch count - adjust based on processing time
    0,     // prefetch size (0 = no limit)
    false, // global (false = per consumer)
)
```

### Manual Acknowledgments
```go
// ✅ Good - manual ack after successful processing
msgs, _ := ch.Consume(queue, "", false, false, false, false, nil)

for msg := range msgs {
    if err := process(msg); err != nil {
        msg.Nack(false, false) // Don't requeue immediately
    } else {
        msg.Ack(false)
    }
}
```

### Worker Pool Pattern
```go
// Create worker pool
for i := 0; i < workerCount; i++ {
    go worker(i, deliveries)
}

func worker(id int, deliveries <-chan amqp.Delivery) {
    for msg := range deliveries {
        process(msg)
    }
}
```

### Idempotency
```go
// Check for duplicates before processing
if dedupeStore.Exists(msg.MessageId) {
    msg.Ack(false)
    return
}

// Process message
if err := process(msg); err == nil {
    dedupeStore.Add(msg.MessageId)
    msg.Ack(false)
}
```

## Error Handling

### Retry Strategy
```go
retryCount := getRetryCount(msg.Headers)

if retryCount >= maxRetries {
    // Send to DLQ
    sendToDLQ(msg)
    msg.Ack(false)
} else {
    // Send to retry queue with delay
    sendToRetryQueue(msg, retryCount+1)
    msg.Ack(false)
}
```

### Dead Letter Queue
Set up DLQ with TTL for automatic retry:

```bash
# Retry queue with 5-second TTL, routes back to main queue
x-dead-letter-exchange: ""
x-dead-letter-routing-key: "main_queue"
x-message-ttl: 5000
```

### Poison Messages
```go
// Detect poison messages
const maxRetries = 3
if retryCount > maxRetries {
    logger.WithFields(logrus.Fields{
        "message_id": msg.MessageId,
        "retry_count": retryCount,
    }).Error("Poison message detected, sending to DLQ")
    
    sendToDLQ(msg)
    msg.Ack(false)
    return
}
```

## Monitoring and Observability

### Structured Logging
```go
logger.WithFields(logrus.Fields{
    "message_id":  msg.MessageId,
    "exchange":    exchange,
    "routing_key": routingKey,
    "retry_count": retryCount,
    "duration_ms": duration.Milliseconds(),
}).Info("Message processed")
```

### Key Metrics to Track
- **Producer**: published_total, publish_failed_total, publish_duration
- **Consumer**: received_total, processed_total, failed_total, processing_duration
- **Queue**: queue_depth, consumer_count, message_rate
- **System**: connection_count, channel_count, memory_usage

### Health Checks
```go
func (c *Connector) HealthCheck() error {
    conn := c.GetConnection()
    if conn == nil || conn.IsClosed() {
        return errors.New("no connection")
    }
    
    // Try to open a channel
    ch, err := conn.Channel()
    if err != nil {
        return err
    }
    ch.Close()
    
    return nil
}
```

## Performance Optimization

### Batching
```go
// Publish in batches for better throughput
const batchSize = 100
batch := make([]Message, 0, batchSize)

for msg := range input {
    batch = append(batch, msg)
    
    if len(batch) >= batchSize {
        publishBatch(batch)
        batch = batch[:0]
    }
}
```

### Connection Locality
- Place consumers close to RabbitMQ (same datacenter/region)
- Minimize network latency
- Use local queues for high-throughput scenarios

### Message Size
- Keep messages < 1MB
- Use compression for large payloads
- Reference large objects instead of embedding

## Security

### TLS/SSL
```go
// Production connection with TLS
config := amqp.Config{
    TLSClientConfig: &tls.Config{
        MinVersion: tls.VersionTLS12,
    },
}

conn, err := amqp.DialConfig("amqps://host:5671/", config)
```

### Credentials Management
```go
// ❌ Bad - hardcoded credentials
url := "amqp://guest:guest@localhost:5672/"

// ✅ Good - from environment or secrets manager
url := os.Getenv("RABBITMQ_URL")

// ✅ Better - from secrets manager
url := getFromVault("rabbitmq/url")
```

### Permissions
- Use separate users for producers and consumers
- Grant minimum required permissions
- Use vhosts to isolate environments

## Capacity Planning

### Queue Length Monitoring
```go
// Alert when queue depth exceeds threshold
if queueDepth > 10000 {
    alert("Queue depth high", queueDepth)
}
```

### Consumer Scaling
```go
// Scale consumers based on queue depth
desiredWorkers := min(queueDepth/100, maxWorkers)
scaleWorkers(desiredWorkers)
```

### Message TTL
```go
// Set TTL to prevent queue buildup
msg := amqp.Publishing{
    Expiration: "60000", // 60 seconds
    Body:       body,
}
```

## Testing

### Unit Testing with Mocks
```go
func TestProducer(t *testing.T) {
    mockConn := new(MockConnection)
    mockCh := new(MockChannel)
    
    mockConn.On("Channel").Return(mockCh, nil)
    mockCh.On("PublishWithContext", ...).Return(nil)
    
    // Test your code
}
```

### Integration Testing
```go
// Use testcontainers for integration tests
func TestIntegration(t *testing.T) {
    if os.Getenv("INTEGRATION_TESTS") != "true" {
        t.Skip("Skipping integration test")
    }
    
    // Start RabbitMQ container
    // Run actual tests
}
```

## Common Pitfalls

### ❌ Not Closing Channels
```go
// Memory leak
ch, _ := conn.Channel()
ch.Publish(...)
// Missing: defer ch.Close()
```

### ❌ Blocking Reconnect
```go
// Blocks application startup
conn := connectWithRetry() // Bad if retries forever

// Use non-blocking reconnect loop instead
go reconnectLoop()
```

### ❌ Unbounded Prefetch
```go
// Can cause OOM
ch.Qos(0, 0, false) // Unlimited prefetch

// Use bounded prefetch
ch.Qos(10, 0, false)
```

### ❌ No Timeout on Operations
```go
// Can hang forever
confirm := <-confirms

// Use timeout
select {
case confirm := <-confirms:
    // Handle
case <-time.After(5 * time.Second):
    return errors.New("timeout")
}
```

## Production Checklist

- [ ] TLS enabled for connections
- [ ] Publisher confirms enabled
- [ ] Manual acknowledgments for consumers
- [ ] Retry queue with exponential backoff
- [ ] Dead letter queue configured
- [ ] Idempotency checks implemented
- [ ] Structured logging in place
- [ ] Metrics collection configured
- [ ] Health checks implemented
- [ ] Graceful shutdown handling
- [ ] Connection pooling optimized
- [ ] QoS/prefetch tuned
- [ ] Monitoring and alerting set up
- [ ] Load testing completed
- [ ] Documentation updated

## Resources

- [RabbitMQ Best Practices](https://www.rabbitmq.com/best-practices.html)
- [RabbitMQ Production Checklist](https://www.rabbitmq.com/production-checklist.html)
- [Go AMQP Client Documentation](https://pkg.go.dev/github.com/rabbitmq/amqp091-go)
