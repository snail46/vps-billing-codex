package provider

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestRegistryRequiresExplicitProviderIdentity(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Get(uuid.New()); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("Get() error=%v", err)
	}
	if err := registry.Register(uuid.Nil, nil); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("Register() error=%v", err)
	}
}
