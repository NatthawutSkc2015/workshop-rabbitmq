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

// MockConnection mocks AMQPConnection
type MockConnection struct {
	mock.Mock
}

func (m *MockConnection) Channel() (AMQPChannel, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(AMQPChannel), args.Error(1)
}

func (m *MockConnection) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockConnection) NotifyClose(receiver chan *amqp.Error) chan *amqp.Error {
	args := m.Called(receiver)
	return args.Get(0).(chan *amqp.Error)
}

func (m *MockConnection) IsClosed() bool {
	args := m.Called()
	return args.Bool(0)
}

// MockChannel mocks AMQPChannel
type MockChannel struct {
	mock.Mock
}

func (m *MockChannel) Qos(prefetchCount, prefetchSize int, global bool) error {
	args := m.Called(prefetchCount, prefetchSize, global)
	return args.Error(0)
}

func (m *MockChannel) Confirm(noWait bool) error {
	args := m.Called(noWait)
	return args.Error(0)
}

func (m *MockChannel) NotifyPublish(confirm chan amqp.Confirmation) chan amqp.Confirmation {
	args := m.Called(confirm)
	return args.Get(0).(chan amqp.Confirmation)
}

func (m *MockChannel) NotifyReturn(returns chan amqp.Return) chan amqp.Return {
	args := m.Called(returns)
	return args.Get(0).(chan amqp.Return)
}

func (m *MockChannel) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	args := m.Called(ctx, exchange, key, mandatory, immediate, msg)
	return args.Error(0)
}

func (m *MockChannel) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	callArgs := m.Called(queue, consumer, autoAck, exclusive, noLocal, noWait, args)
	if callArgs.Get(0) == nil {
		return nil, callArgs.Error(1)
	}
	return callArgs.Get(0).(<-chan amqp.Delivery), callArgs.Error(1)
}

func (m *MockChannel) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockChannel) NotifyClose(receiver chan *amqp.Error) chan *amqp.Error {
	args := m.Called(receiver)
	return args.Get(0).(chan *amqp.Error)
}

// MockConnectionFactory mocks ConnectionFactory
type MockConnectionFactory struct {
	mock.Mock
}

func (m *MockConnectionFactory) Dial(url string) (AMQPConnection, error) {
	args := m.Called(url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(AMQPConnection), args.Error(1)
}

func TestProducer_PublishSuccess(t *testing.T) {
	mockConn := new(MockConnection)
	mockCh := new(MockChannel)
	mockFactory := new(MockConnectionFactory)
	metrics := NewSimpleMetrics()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Setup connector
	connector := NewConnector("amqp://test", mockFactory, logger, time.Second, 30*time.Second)
	connector.conn = mockConn

	producer := NewProducer(connector, logger, metrics, 3, 100*time.Millisecond)

	// Mock expectations
	mockConn.On("Channel").Return(mockCh, nil)
	mockCh.On("Confirm", false).Return(nil)

	confirmCh := make(chan amqp.Confirmation, 1)
	mockCh.On("NotifyPublish", mock.AnythingOfType("chan amqp.Confirmation")).Return(confirmCh)
	mockCh.On("PublishWithContext", mock.Anything, "test-exchange", "test.key", false, false, mock.Anything).Return(nil)
	mockCh.On("Close").Return(nil)

	// Send confirmation in goroutine
	go func() {
		time.Sleep(10 * time.Millisecond)
		confirmCh <- amqp.Confirmation{Ack: true}
	}()

	// Test
	ctx := context.Background()
	err := producer.Publish(ctx, "test-exchange", "test.key", []byte("test"), nil)

	assert.NoError(t, err)
	assert.Equal(t, uint64(1), metrics.GetPublished())
	assert.Equal(t, uint64(0), metrics.GetPublishFailed())
	mockConn.AssertExpectations(t)
	mockCh.AssertExpectations(t)
}

func TestProducer_PublishWithRetries(t *testing.T) {
	mockConn := new(MockConnection)
	mockCh1 := new(MockChannel)
	mockCh2 := new(MockChannel)
	mockFactory := new(MockConnectionFactory)
	metrics := NewSimpleMetrics()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	connector := NewConnector("amqp://test", mockFactory, logger, time.Second, 30*time.Second)
	connector.conn = mockConn

	producer := NewProducer(connector, logger, metrics, 2, 10*time.Millisecond)

	// First attempt fails
	mockConn.On("Channel").Return(mockCh1, nil).Once()
	mockCh1.On("Confirm", false).Return(nil).Once()
	confirmCh1 := make(chan amqp.Confirmation, 1)
	mockCh1.On("NotifyPublish", mock.AnythingOfType("chan amqp.Confirmation")).Return(confirmCh1).Once()
	mockCh1.On("PublishWithContext", mock.Anything, "test-exchange", "test.key", false, false, mock.Anything).Return(nil).Once()
	mockCh1.On("Close").Return(nil).Once()

	go func() {
		time.Sleep(5 * time.Millisecond)
		confirmCh1 <- amqp.Confirmation{Ack: false} // Failed
	}()

	// Second attempt succeeds
	mockConn.On("Channel").Return(mockCh2, nil).Once()
	mockCh2.On("Confirm", false).Return(nil).Once()
	confirmCh2 := make(chan amqp.Confirmation, 1)
	mockCh2.On("NotifyPublish", mock.AnythingOfType("chan amqp.Confirmation")).Return(confirmCh2).Once()
	mockCh2.On("PublishWithContext", mock.Anything, "test-exchange", "test.key", false, false, mock.Anything).Return(nil).Once()
	mockCh2.On("Close").Return(nil).Once()

	go func() {
		time.Sleep(5 * time.Millisecond)
		confirmCh2 <- amqp.Confirmation{Ack: true}
	}()

	// Test
	ctx := context.Background()
	err := producer.Publish(ctx, "test-exchange", "test.key", []byte("test"), nil)

	assert.NoError(t, err)
	assert.Equal(t, uint64(1), metrics.GetPublished())
	assert.Equal(t, uint64(1), metrics.GetPublishRetry())
	mockConn.AssertExpectations(t)
}

func TestProducer_PublishMaxRetriesExceeded(t *testing.T) {
	mockConn := new(MockConnection)
	mockFactory := new(MockConnectionFactory)
	metrics := NewSimpleMetrics()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	connector := NewConnector("amqp://test", mockFactory, logger, time.Second, 30*time.Second)
	connector.conn = mockConn

	producer := NewProducer(connector, logger, metrics, 2, 10*time.Millisecond)

	// All attempts fail
	for i := 0; i < 3; i++ {
		mockConn.On("Channel").Return(nil, errors.New("channel error")).Once()
	}

	// Test
	ctx := context.Background()
	err := producer.Publish(ctx, "test-exchange", "test.key", []byte("test"), nil)

	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrMaxRetriesExceeded)
	assert.Equal(t, uint64(0), metrics.GetPublished())
	assert.Equal(t, uint64(1), metrics.GetPublishFailed())
	assert.Equal(t, uint64(2), metrics.GetPublishRetry())
}

func TestProducer_PublishNoConnection(t *testing.T) {
	mockFactory := new(MockConnectionFactory)
	metrics := NewSimpleMetrics()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	connector := NewConnector("amqp://test", mockFactory, logger, time.Second, 30*time.Second)
	connector.conn = nil // No connection

	producer := NewProducer(connector, logger, metrics, 2, 10*time.Millisecond)

	ctx := context.Background()
	err := producer.Publish(ctx, "test-exchange", "test.key", []byte("test"), nil)

	assert.Error(t, err)
	assert.Equal(t, uint64(1), metrics.GetPublishFailed())
}
