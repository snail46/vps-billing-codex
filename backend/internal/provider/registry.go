package provider

import (
	"errors"
	"sync"

	"github.com/google/uuid"
)

var ErrNotRegistered = errors.New("provider adapter is not registered")

type Registry struct {
	mu       sync.RWMutex
	adapters map[uuid.UUID]Provider
}

func NewRegistry() *Registry { return &Registry{adapters: make(map[uuid.UUID]Provider)} }

func (r *Registry) Register(id uuid.UUID, adapter Provider) error {
	if id == uuid.Nil || adapter == nil {
		return ErrNotRegistered
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[id]; exists {
		return errors.New("provider adapter is already registered")
	}
	r.adapters[id] = adapter
	return nil
}

func (r *Registry) Get(id uuid.UUID) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[id]
	if !ok {
		return nil, ErrNotRegistered
	}
	return adapter, nil
}
