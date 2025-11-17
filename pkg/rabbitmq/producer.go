package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

var (
	ErrNoConnection       = errors.New("no connection available")
	ErrPublishFailed      = errors.New("publish not confirmed")
	ErrMaxRetriesExceeded = errors.New("max retries exceeded")
)

// Metrics interface for observability
type MetricsRecorder interface {
	IncPublished()
	IncPublishFailed()
	IncPublishRetry()
}

type Producer struct {
	connector  *Connector
	logger     *logrus.Logger
	metrics    MetricsRecorder
	maxRetries int
	retryDelay time.Duration
}

func NewProducer(connector *Connector, logger *logrus.Logger, metrics MetricsRecorder, maxRetries int, retryDelay time.Duration) *Producer {
	return &Producer{
		connector:  connector,
		logger:     logger,
		metrics:    metrics,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
	}
}

// Publish publishes message with publisher confirms and retries
func (p *Producer) Publish(ctx context.Context, exchange, routingKey string, body []byte, headers amqp.Table) error {
	var lastErr error

	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			p.metrics.IncPublishRetry()
			p.logger.WithFields(logrus.Fields{
				"attempt":     attempt,
				"exchange":    exchange,
				"routing_key": routingKey,
			}).Info("Retrying publish")

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(p.retryDelay * time.Duration(attempt)):
			}
		}

		err := p.publishOnce(ctx, exchange, routingKey, body, headers)
		if err == nil {
			p.metrics.IncPublished()
			return nil
		}

		lastErr = err
		p.logger.WithError(err).WithFields(logrus.Fields{
			"attempt":     attempt + 1,
			"exchange":    exchange,
			"routing_key": routingKey,
		}).Warn("Publish attempt failed")
	}

	p.metrics.IncPublishFailed()
	return fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
}

func (p *Producer) publishOnce(ctx context.Context, exchange, routingKey string, body []byte, headers amqp.Table) error {
	conn := p.connector.GetConnection()
	if conn == nil {
		return ErrNoConnection
	}

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %w", err)
	}
	defer ch.Close()

	// Enable publisher confirms
	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("failed to enable confirms: %w", err)
	}

	confirms := make(chan amqp.Confirmation, 1)
	ch.NotifyPublish(confirms)

	msg := amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		Headers:      headers,
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
	}

	if err := ch.PublishWithContext(ctx, exchange, routingKey, false, false, msg); err != nil {
		return fmt.Errorf("failed to publish: %w", err)
	}

	// Wait for confirmation
	select {
	case <-ctx.Done():
		return ctx.Err()
	case confirm := <-confirms:
		if !confirm.Ack {
			return ErrPublishFailed
		}
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("confirmation timeout")
	}
}
