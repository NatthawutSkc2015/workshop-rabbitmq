package rabbitmq

import (
	"sync/atomic"
)

// SimpleMetrics is a basic in-memory metrics implementation
// Can be replaced with Prometheus metrics in production
type SimpleMetrics struct {
	published     uint64
	publishFailed uint64
	publishRetry  uint64
	received      uint64
	processed     uint64
	failed        uint64
	retried       uint64
}

func NewSimpleMetrics() *SimpleMetrics {
	return &SimpleMetrics{}
}

// Producer metrics
func (m *SimpleMetrics) IncPublished() {
	atomic.AddUint64(&m.published, 1)
}

func (m *SimpleMetrics) IncPublishFailed() {
	atomic.AddUint64(&m.publishFailed, 1)
}

func (m *SimpleMetrics) IncPublishRetry() {
	atomic.AddUint64(&m.publishRetry, 1)
}

// Consumer metrics
func (m *SimpleMetrics) IncReceived() {
	atomic.AddUint64(&m.received, 1)
}

func (m *SimpleMetrics) IncProcessed() {
	atomic.AddUint64(&m.processed, 1)
}

func (m *SimpleMetrics) IncFailed() {
	atomic.AddUint64(&m.failed, 1)
}

func (m *SimpleMetrics) IncRetried() {
	atomic.AddUint64(&m.retried, 1)
}

// Getters for testing
func (m *SimpleMetrics) GetPublished() uint64 {
	return atomic.LoadUint64(&m.published)
}

func (m *SimpleMetrics) GetPublishFailed() uint64 {
	return atomic.LoadUint64(&m.publishFailed)
}

func (m *SimpleMetrics) GetPublishRetry() uint64 {
	return atomic.LoadUint64(&m.publishRetry)
}

func (m *SimpleMetrics) GetReceived() uint64 {
	return atomic.LoadUint64(&m.received)
}

func (m *SimpleMetrics) GetProcessed() uint64 {
	return atomic.LoadUint64(&m.processed)
}

func (m *SimpleMetrics) GetFailed() uint64 {
	return atomic.LoadUint64(&m.failed)
}

func (m *SimpleMetrics) GetRetried() uint64 {
	return atomic.LoadUint64(&m.retried)
}
