package rabbitmq

import (
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

// TestHelper provides utilities for testing
type TestHelper struct {
	Logger  *logrus.Logger
	Metrics *SimpleMetrics
	Dedupe  *InMemoryDedupeStore
}

func NewTestHelper() *TestHelper {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	return &TestHelper{
		Logger:  logger,
		Metrics: NewSimpleMetrics(),
		Dedupe:  NewInMemoryDedupeStore(),
	}
}

// CreateTestDelivery creates a test amqp.Delivery
func CreateTestDelivery(messageID string, body []byte, headers amqp.Table) amqp.Delivery {
	if headers == nil {
		headers = amqp.Table{}
	}

	return amqp.Delivery{
		MessageId:    messageID,
		Body:         body,
		Headers:      headers,
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
	}
}

// WaitForMetric waits for a metric to reach expected value (for async tests)
func WaitForMetric(getter func() uint64, expected uint64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if getter() == expected {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
