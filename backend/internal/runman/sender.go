package runman

import (
	"context"
	"github.com/google/uuid"
	"time"
)

type ConnectionSender struct{ store Store }

func NewConnectionSender(store Store) *ConnectionSender { return &ConnectionSender{store} }
func (s *ConnectionSender) Online(id uuid.UUID) bool {
	ok, _ := s.store.Healthy(context.Background(), id, 90*time.Second)
	return ok
}
func (s *ConnectionSender) Send(context.Context, Command) error { return nil }
