package rabbitmq

import (
	"context"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

// Connector manages RabbitMQ connection with auto-reconnect
type Connector struct {
	url              string
	conn             AMQPConnection
	factory          ConnectionFactory
	logger           *logrus.Logger
	mu               sync.RWMutex
	reconnectBackoff time.Duration
	maxReconnectWait time.Duration
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
}

func NewConnector(url string, factory ConnectionFactory, logger *logrus.Logger, reconnectBackoff, maxReconnectWait time.Duration) *Connector {
	ctx, cancel := context.WithCancel(context.Background())
	return &Connector{
		url:              url,
		factory:          factory,
		logger:           logger,
		reconnectBackoff: reconnectBackoff,
		maxReconnectWait: maxReconnectWait,
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Connect establishes initial connection and starts reconnect loop
func (c *Connector) Connect() error {
	if err := c.connect(); err != nil {
		return err
	}
	c.wg.Add(1)
	go c.reconnectLoop()
	return nil
}

func (c *Connector) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := c.factory.Dial(c.url)
	if err != nil {
		return err
	}
	c.conn = conn
	c.logger.Info("Connected to RabbitMQ")
	return nil
}

// reconnectLoop monitors connection and reconnects with exponential backoff
func (c *Connector) reconnectLoop() {
	defer c.wg.Done()

	for {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(c.reconnectBackoff):
				c.attemptReconnect()
			}
			continue
		}

		closeChan := make(chan *amqp.Error, 1)
		conn.NotifyClose(closeChan)

		select {
		case <-c.ctx.Done():
			return
		case err := <-closeChan:
			if err != nil {
				c.logger.WithError(err).Warn("Connection closed, attempting reconnect")
			}
			c.attemptReconnect()
		}
	}
}

func (c *Connector) attemptReconnect() {
	backoff := c.reconnectBackoff
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(backoff):
			c.logger.Info("Attempting to reconnect to RabbitMQ")
			if err := c.connect(); err != nil {
				c.logger.WithError(err).Error("Reconnect failed")
				backoff *= 2
				if backoff > c.maxReconnectWait {
					backoff = c.maxReconnectWait
				}
			} else {
				c.logger.Info("Reconnected successfully")
				return
			}
		}
	}
}

// GetConnection returns current connection (may be nil if disconnected)
func (c *Connector) GetConnection() AMQPConnection {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

// Close gracefully shuts down connector
func (c *Connector) Close() error {
	c.cancel()
	c.wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
