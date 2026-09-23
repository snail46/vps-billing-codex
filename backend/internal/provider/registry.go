package provider

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	db "vps-billing/backend/internal/store/sqlc"
)

var ErrNotRegistered = errors.New("provider adapter is not registered")

type Registry struct {
	mu       sync.RWMutex
	adapters map[uuid.UUID]Provider
}

type Resolver interface {
	Resolve(context.Context, uuid.UUID) (Provider, error)
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

func (r *Registry) Resolve(_ context.Context, id uuid.UUID) (Provider, error) {
	return r.Get(id)
}

type FactoryConfig struct {
	Endpoint      string
	CredentialRef *string
	Config        json.RawMessage
}

type Factory func(FactoryConfig) (Provider, error)

type DynamicRegistry struct {
	registry  *Registry
	queries   *db.Queries
	mu        sync.RWMutex
	factories map[string]Factory
}

func NewDynamicRegistry(pool *pgxpool.Pool) *DynamicRegistry {
	return &DynamicRegistry{registry: NewRegistry(), queries: db.New(pool), factories: make(map[string]Factory)}
}

func (r *DynamicRegistry) RegisterFactory(providerType string, factory Factory) error {
	if providerType == "" || factory == nil {
		return ErrNotRegistered
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[providerType]; exists {
		return errors.New("provider factory is already registered")
	}
	r.factories[providerType] = factory
	return nil
}

func (r *DynamicRegistry) Resolve(ctx context.Context, id uuid.UUID) (Provider, error) {
	if adapter, err := r.registry.Get(id); err == nil {
		return adapter, nil
	}
	record, err := r.queries.GetInfrastructureProviderByID(ctx, id)
	if err != nil {
		return nil, ErrNotRegistered
	}
	r.mu.RLock()
	factory := r.factories[record.ProviderType]
	r.mu.RUnlock()
	if factory == nil {
		return nil, ErrNotRegistered
	}
	var credentialRef *string
	if record.CredentialRef.Valid {
		value := record.CredentialRef.String
		credentialRef = &value
	}
	adapter, err := factory(FactoryConfig{Endpoint: record.Endpoint.String, CredentialRef: credentialRef, Config: record.Config})
	if err != nil {
		return nil, err
	}
	if err := r.registry.Register(id, adapter); err != nil {
		return r.registry.Get(id)
	}
	return adapter, nil
}
