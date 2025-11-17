package rabbitmq

import (
	"context"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

// MessageHandler processes messages
type MessageHandler func(ctx context.Context, delivery amqp.Delivery) error

// DedupeStore interface for idempotency checking
type DedupeStore interface {
	Exists(messageID string) bool
	Add(messageID string)
}

// ConsumerMetrics interface for consumer observability
type ConsumerMetrics interface {
	IncReceived()
	IncProcessed()
	IncFailed()
	IncRetried()
}

type Consumer struct {
	connector     *Connector
	queue         string
	retryQueue    string
	dlq           string
	prefetchCount int
	workerCount   int
	maxRetries    int
	handler       MessageHandler
	dedupeStore   DedupeStore
	logger        *logrus.Logger
	metrics       ConsumerMetrics
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

func NewConsumer(
	connector *Connector,
	queue, retryQueue, dlq string,
	prefetchCount, workerCount, maxRetries int,
	handler MessageHandler,
	dedupeStore DedupeStore,
	logger *logrus.Logger,
	metrics ConsumerMetrics,
) *Consumer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Consumer{
		connector:     connector,
		queue:         queue,
		retryQueue:    retryQueue,
		dlq:           dlq,
		prefetchCount: prefetchCount,
		workerCount:   workerCount,
		maxRetries:    maxRetries,
		handler:       handler,
		dedupeStore:   dedupeStore,
		logger:        logger,
		metrics:       metrics,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start begins consuming messages with worker pool
func (c *Consumer) Start() error {
	conn := c.connector.GetConnection()
	if conn == nil {
		return ErrNoConnection
	}

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %w", err)
	}

	if err := ch.Qos(c.prefetchCount, 0, false); err != nil {
		ch.Close()
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	deliveries, err := ch.Consume(c.queue, "", false, false, false, false, nil)
	if err != nil {
		ch.Close()
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"queue":   c.queue,
		"workers": c.workerCount,
	}).Info("Consumer started")

	// Start worker pool
	for i := 0; i < c.workerCount; i++ {
		c.wg.Add(1)
		go c.worker(i, deliveries, ch)
	}

	// Monitor channel closure
	c.wg.Add(1)
	go c.monitorChannel(ch)

	return nil
}

func (c *Consumer) worker(id int, deliveries <-chan amqp.Delivery, ch AMQPChannel) {
	defer c.wg.Done()

	logger := c.logger.WithField("worker_id", id)
	logger.Info("Worker started")

	for {
		select {
		case <-c.ctx.Done():
			logger.Info("Worker shutting down")
			return
		case delivery, ok := <-deliveries:
			if !ok {
				logger.Warn("Delivery channel closed")
				return
			}
			c.processMessage(logger, delivery)
		}
	}
}

func (c *Consumer) processMessage(logger *logrus.Entry, delivery amqp.Delivery) {
	c.metrics.IncReceived()

	// Check for duplicate using message ID
	if messageID := delivery.MessageId; messageID != "" {
		if c.dedupeStore.Exists(messageID) {
			logger.WithField("message_id", messageID).Info("Duplicate message, skipping")
			delivery.Ack(false)
			return
		}
	}

	// Get retry count from headers
	retryCount := getRetryCount(delivery.Headers)

	logger = logger.WithFields(logrus.Fields{
		"message_id":  delivery.MessageId,
		"retry_count": retryCount,
	})

	// Process message
	if err := c.handler(c.ctx, delivery); err != nil {
		logger.WithError(err).Error("Message processing failed")
		c.metrics.IncFailed()
		c.handleFailure(logger, delivery, retryCount)
		return
	}

	// Success - mark as processed
	if err := delivery.Ack(false); err != nil {
		logger.WithError(err).Error("Failed to ack message")
	} else {
		c.metrics.IncProcessed()
		if delivery.MessageId != "" {
			c.dedupeStore.Add(delivery.MessageId)
		}
		logger.Debug("Message processed successfully")
	}
}

func (c *Consumer) handleFailure(logger *logrus.Entry, delivery amqp.Delivery, retryCount int) {
	if retryCount >= c.maxRetries {
		// Max retries exceeded - send to DLQ
		logger.Warn("Max retries exceeded, sending to DLQ")
		if err := c.sendToDLQ(delivery); err != nil {
			logger.WithError(err).Error("Failed to send to DLQ")
			delivery.Nack(false, true) // Requeue as fallback
		} else {
			delivery.Ack(false)
		}
		return
	}

	// Send to retry queue
	logger.Info("Sending to retry queue")
	c.metrics.IncRetried()
	if err := c.sendToRetryQueue(delivery, retryCount+1); err != nil {
		logger.WithError(err).Error("Failed to send to retry queue")
		delivery.Nack(false, true) // Requeue
	} else {
		delivery.Ack(false)
	}
}

func (c *Consumer) sendToRetryQueue(delivery amqp.Delivery, newRetryCount int) error {
	conn := c.connector.GetConnection()
	if conn == nil {
		return ErrNoConnection
	}

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	headers := delivery.Headers
	if headers == nil {
		headers = amqp.Table{}
	}
	headers["x-retry-count"] = newRetryCount

	msg := amqp.Publishing{
		ContentType:  delivery.ContentType,
		Body:         delivery.Body,
		Headers:      headers,
		DeliveryMode: amqp.Persistent,
	}

	return ch.PublishWithContext(c.ctx, "", c.retryQueue, false, false, msg)
}

func (c *Consumer) sendToDLQ(delivery amqp.Delivery) error {
	conn := c.connector.GetConnection()
	if conn == nil {
		return ErrNoConnection
	}

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	msg := amqp.Publishing{
		ContentType:  delivery.ContentType,
		Body:         delivery.Body,
		Headers:      delivery.Headers,
		DeliveryMode: amqp.Persistent,
	}

	return ch.PublishWithContext(c.ctx, "", c.dlq, false, false, msg)
}

func (c *Consumer) monitorChannel(ch AMQPChannel) {
	defer c.wg.Done()

	closeChan := make(chan *amqp.Error, 1)
	ch.NotifyClose(closeChan)

	select {
	case <-c.ctx.Done():
		ch.Close()
	case err := <-closeChan:
		if err != nil {
			c.logger.WithError(err).Error("Consumer channel closed")
		}
	}
}

// Stop gracefully shuts down consumer
func (c *Consumer) Stop() {
	c.logger.Info("Stopping consumer")
	c.cancel()
	c.wg.Wait()
	c.logger.Info("Consumer stopped")
}

func getRetryCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}
	if count, ok := headers["x-retry-count"].(int); ok {
		return count
	}
	if count, ok := headers["x-retry-count"].(int32); ok {
		return int(count)
	}
	return 0
}
