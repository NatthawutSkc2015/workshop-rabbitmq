package rabbitmq

import "sync"

// InMemoryDedupeStore is a simple in-memory implementation for testing/demo
// In production, use Redis or similar distributed store
type InMemoryDedupeStore struct {
	mu    sync.RWMutex
	store map[string]bool
}

func NewInMemoryDedupeStore() *InMemoryDedupeStore {
	return &InMemoryDedupeStore{
		store: make(map[string]bool),
	}
}

func (d *InMemoryDedupeStore) Exists(messageID string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.store[messageID]
}

func (d *InMemoryDedupeStore) Add(messageID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.store[messageID] = true
}

func (d *InMemoryDedupeStore) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.store = make(map[string]bool)
}
