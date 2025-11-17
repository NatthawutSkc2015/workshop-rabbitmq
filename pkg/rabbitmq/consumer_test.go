package rabbitmq

import (
	"context"
	"errors"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockAcknowledger mocks amqp.Acknowledger
type MockAcknowledger struct {
	mock.Mock
}

func (m *MockAcknowledger) Ack(tag uint64, multiple bool) error {
	args := m.Called(tag, multiple)
	return args.Error(0)
}

func (m *MockAcknowledger) Nack(tag uint64, multiple bool, requeue bool) error {
	args := m.Called(tag, multiple, requeue)
	return args.Error(0)
}

func (m *MockAcknowledger) Reject(tag uint64, requeue bool) error {
	args := m.Called(tag, requeue)
	return args.Error(0)
}

func TestConsumer_ProcessMessageSuccess(t *testing.T) {
	mockConn := new(MockConnection)
	// mockCh := new(MockChannel)
	mockFactory := new(MockConnectionFactory)
	metrics := NewSimpleMetrics()
	dedupeStore := NewInMemoryDedupeStore()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	connector := NewConnector("amqp://test", mockFactory, logger, time.Second, 30*time.Second)
	connector.conn = mockConn

	handlerCalled := false
	handler := func(ctx context.Context, delivery amqp.Delivery) error {
		handlerCalled = true
		return nil
	}

	consumer := NewConsumer(
		connector,
		"test-queue",
		"test-retry",
		"test-dlq",
		10, 1, 3,
		handler,
		dedupeStore,
		logger,
		metrics,
	)

	// Create mock delivery
	mockAck := new(MockAcknowledger)
	delivery := amqp.Delivery{
		Acknowledger: mockAck,
		MessageId:    "msg-123",
		Body:         []byte("test message"),
		Headers:      amqp.Table{},
	}

	mockAck.On("Ack", uint64(0), false).Return(nil)

	// Process message
	consumer.processMessage(logger.WithField("test", "test"), delivery)

	assert.True(t, handlerCalled)
	assert.Equal(t, uint64(1), metrics.GetReceived())
	assert.Equal(t, uint64(1), metrics.GetProcessed())
	assert.Equal(t, uint64(0), metrics.GetFailed())
	assert.True(t, dedupeStore.Exists("msg-123"))
	mockAck.AssertExpectations(t)
}

func TestConsumer_ProcessMessageDuplicate(t *testing.T) {
	mockConn := new(MockConnection)
	mockFactory := new(MockConnectionFactory)
	metrics := NewSimpleMetrics()
	dedupeStore := NewInMemoryDedupeStore()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	connector := NewConnector("amqp://test", mockFactory, logger, time.Second, 30*time.Second)
	connector.conn = mockConn

	// Pre-add message ID
	dedupeStore.Add("msg-123")

	handlerCalled := false
	handler := func(ctx context.Context, delivery amqp.Delivery) error {
		handlerCalled = true
		return nil
	}

	consumer := NewConsumer(
		connector,
		"test-queue",
		"test-retry",
		"test-dlq",
		10, 1, 3,
		handler,
		dedupeStore,
		logger,
		metrics,
	)

	mockAck := new(MockAcknowledger)
	delivery := amqp.Delivery{
		Acknowledger: mockAck,
		MessageId:    "msg-123",
		Body:         []byte("test message"),
	}

	mockAck.On("Ack", uint64(0), false).Return(nil)

	consumer.processMessage(logger.WithField("test", "test"), delivery)

	assert.False(t, handlerCalled, "Handler should not be called for duplicate")
	assert.Equal(t, uint64(1), metrics.GetReceived())
	mockAck.AssertExpectations(t)
}

func TestConsumer_ProcessMessageFailureWithRetry(t *testing.T) {
	mockConn := new(MockConnection)
	mockCh := new(MockChannel)
	mockFactory := new(MockConnectionFactory)
	metrics := NewSimpleMetrics()
	dedupeStore := NewInMemoryDedupeStore()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	connector := NewConnector("amqp://test", mockFactory, logger, time.Second, 30*time.Second)
	connector.conn = mockConn

	handler := func(ctx context.Context, delivery amqp.Delivery) error {
		return errors.New("processing failed")
	}

	consumer := NewConsumer(
		connector,
		"test-queue",
		"test-retry",
		"test-dlq",
		10, 1, 3,
		handler,
		dedupeStore,
		logger,
		metrics,
	)

	mockAck := new(MockAcknowledger)
	delivery := amqp.Delivery{
		Acknowledger: mockAck,
		MessageId:    "msg-456",
		Body:         []byte("test message"),
		Headers:      amqp.Table{"x-retry-count": int32(0)},
	}

	// Expect retry queue publish
	mockConn.On("Channel").Return(mockCh, nil).Once()
	mockCh.On("PublishWithContext", mock.Anything, "", "test-retry", false, false, mock.MatchedBy(func(msg amqp.Publishing) bool {
		return msg.Headers["x-retry-count"] == 1
	})).Return(nil).Once()
	mockCh.On("Close").Return(nil).Once()
	mockAck.On("Ack", uint64(0), false).Return(nil)

	consumer.processMessage(logger.WithField("test", "test"), delivery)

	assert.Equal(t, uint64(1), metrics.GetReceived())
	assert.Equal(t, uint64(1), metrics.GetFailed())
	assert.Equal(t, uint64(1), metrics.GetRetried())
	mockConn.AssertExpectations(t)
	mockCh.AssertExpectations(t)
	mockAck.AssertExpectations(t)
}

func TestConsumer_ProcessMessageFailureMaxRetries(t *testing.T) {
	mockConn := new(MockConnection)
	mockCh := new(MockChannel)
	mockFactory := new(MockConnectionFactory)
	metrics := NewSimpleMetrics()
	dedupeStore := NewInMemoryDedupeStore()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	connector := NewConnector("amqp://test", mockFactory, logger, time.Second, 30*time.Second)
	connector.conn = mockConn

	handler := func(ctx context.Context, delivery amqp.Delivery) error {
		return errors.New("processing failed")
	}

	consumer := NewConsumer(
		connector,
		"test-queue",
		"test-retry",
		"test-dlq",
		10, 1, 3,
		handler,
		dedupeStore,
		logger,
		metrics,
	)

	mockAck := new(MockAcknowledger)
	delivery := amqp.Delivery{
		Acknowledger: mockAck,
		MessageId:    "msg-789",
		Body:         []byte("test message"),
		Headers:      amqp.Table{"x-retry-count": int32(3)}, // Max retries reached
	}

	// Expect DLQ publish
	mockConn.On("Channel").Return(mockCh, nil).Once()
	mockCh.On("PublishWithContext", mock.Anything, "", "test-dlq", false, false, mock.Anything).Return(nil).Once()
	mockCh.On("Close").Return(nil).Once()
	mockAck.On("Ack", uint64(0), false).Return(nil)

	consumer.processMessage(logger.WithField("test", "test"), delivery)

	assert.Equal(t, uint64(1), metrics.GetReceived())
	assert.Equal(t, uint64(1), metrics.GetFailed())
	assert.Equal(t, uint64(0), metrics.GetRetried()) // No retry, went to DLQ
	mockConn.AssertExpectations(t)
	mockCh.AssertExpectations(t)
	mockAck.AssertExpectations(t)
}

func TestConsumer_GetRetryCount(t *testing.T) {
	tests := []struct {
		name     string
		headers  amqp.Table
		expected int
	}{
		{"nil headers", nil, 0},
		{"no retry count", amqp.Table{}, 0},
		{"int retry count", amqp.Table{"x-retry-count": 5}, 5},
		{"int32 retry count", amqp.Table{"x-retry-count": int32(7)}, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRetryCount(tt.headers)
			assert.Equal(t, tt.expected, result)
		})
	}
}
