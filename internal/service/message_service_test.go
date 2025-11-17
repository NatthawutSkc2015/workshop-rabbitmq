package service

import (
	"context"
	"encoding/json"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

func TestMessageService_ProcessMessage(t *testing.T) {
	service := NewMessageService(nil, "test-exchange", "test.key")

	message := map[string]interface{}{
		"id":      "msg-1",
		"content": "test message",
	}
	body, _ := json.Marshal(message)

	delivery := amqp.Delivery{
		Body: body,
	}

	ctx := context.Background()
	err := service.ProcessMessage(ctx, delivery)

	assert.NoError(t, err)
}

func TestMessageService_ProcessMessageInvalidJSON(t *testing.T) {
	service := NewMessageService(nil, "test-exchange", "test.key")

	delivery := amqp.Delivery{
		Body: []byte("invalid json"),
	}

	ctx := context.Background()
	err := service.ProcessMessage(ctx, delivery)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid message format")
}
