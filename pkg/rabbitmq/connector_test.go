package rabbitmq

import (
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestConnector_ConnectSuccess(t *testing.T) {
	mockConn := new(MockConnection)
	mockFactory := new(MockConnectionFactory)
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	mockFactory.On("Dial", "amqp://test").Return(mockConn, nil)
	closeChan := make(chan *amqp.Error)
	mockConn.On("NotifyClose", closeChan).Return(closeChan)
	mockConn.On("Close").Return(nil)

	connector := NewConnector("amqp://test", mockFactory, logger, 100*time.Millisecond, 5*time.Second)
	err := connector.Connect()

	assert.NoError(t, err)
	assert.NotNil(t, connector.GetConnection())

	// Cleanup
	connector.Close()
	mockFactory.AssertExpectations(t)
}

func TestConnector_ConnectFailure(t *testing.T) {
	mockFactory := new(MockConnectionFactory)
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	mockFactory.On("Dial", "amqp://test").Return(nil, assert.AnError)

	connector := NewConnector("amqp://test", mockFactory, logger, 100*time.Millisecond, 5*time.Second)
	err := connector.Connect()

	assert.Error(t, err)
	assert.Nil(t, connector.GetConnection())

	mockFactory.AssertExpectations(t)
}

func TestConnector_Reconnect(t *testing.T) {
	mockConn1 := new(MockConnection)
	mockConn2 := new(MockConnection)
	mockFactory := new(MockConnectionFactory)
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// First connection succeeds
	closeChan1 := make(chan *amqp.Error, 1)
	mockFactory.On("Dial", "amqp://test").Return(mockConn1, nil).Once()
	mockConn1.On("NotifyClose", closeChan1).Return(closeChan1).Once()

	connector := NewConnector("amqp://test", mockFactory, logger, 50*time.Millisecond, 1*time.Second)
	err := connector.Connect()
	assert.NoError(t, err)

	// Simulate connection close
	closeChan2 := make(chan *amqp.Error, 1)
	mockFactory.On("Dial", "amqp://test").Return(mockConn2, nil).Once()
	mockConn2.On("NotifyClose", closeChan2).Return(closeChan2).Once()
	mockConn2.On("Close").Return(nil).Once()

	// Trigger close
	closeChan1 <- amqp.ErrClosed

	// Wait for reconnect
	time.Sleep(200 * time.Millisecond)

	// Verify reconnected
	conn := connector.GetConnection()
	assert.NotNil(t, conn)
	assert.Equal(t, mockConn2, conn)

	// Cleanup
	connector.Close()
	mockFactory.AssertExpectations(t)
}

func TestConnector_GracefulShutdown(t *testing.T) {
	mockConn := new(MockConnection)
	mockFactory := new(MockConnectionFactory)
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	closeChan := make(chan *amqp.Error)
	mockFactory.On("Dial", "amqp://test").Return(mockConn, nil)
	mockConn.On("NotifyClose", closeChan).Return(closeChan)
	mockConn.On("Close").Return(nil)

	connector := NewConnector("amqp://test", mockFactory, logger, 100*time.Millisecond, 5*time.Second)
	err := connector.Connect()
	assert.NoError(t, err)

	// Close and verify cleanup
	err = connector.Close()
	assert.NoError(t, err)

	mockConn.AssertExpectations(t)
}
