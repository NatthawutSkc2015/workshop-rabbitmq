package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go-queue/pkg/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
)

// MessageService handles business logic for message processing
type MessageService struct {
	producer   *rabbitmq.Producer
	exchange   string
	routingKey string
}

func NewMessageService(producer *rabbitmq.Producer, exchange, routingKey string) *MessageService {
	return &MessageService{
		producer:   producer,
		exchange:   exchange,
		routingKey: routingKey,
	}
}

// SendMessage sends a message through RabbitMQ
func (s *MessageService) SendMessage(ctx context.Context, messageID, content string) error {
	message := map[string]interface{}{
		"id":        messageID,
		"content":   content,
		"timestamp": time.Now(),
	}

	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	headers := amqp.Table{
		"x-message-id": messageID,
		"x-source":     "message-service",
	}

	return s.producer.Publish(ctx, s.exchange, s.routingKey, body, headers)
}

// ProcessMessage processes an incoming message (business logic)
func (s *MessageService) ProcessMessage(ctx context.Context, delivery amqp.Delivery) error {
	var message map[string]interface{}
	if err := json.Unmarshal(delivery.Body, &message); err != nil {
		return fmt.Errorf("invalid message format: %w", err)
	}

	// Business logic here
	// For example: validate, transform, store to database, call external APIs, etc.

	// Simulate processing
	time.Sleep(50 * time.Millisecond)

	return nil
}
