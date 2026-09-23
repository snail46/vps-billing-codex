package runman

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	pb "vps-billing/backend/internal/runman/proto"
)

func TestGatewayAuthenticateHeartbeatResultAndReconnect(t *testing.T) {
	nodeID, commandID, instanceID := uuid.New(), uuid.New(), uuid.New()
	store := newMemoryStore(nodeID)
	store.commands[commandID] = Command{ID: commandID, NodeID: nodeID, Type: "start", Payload: json.RawMessage(`{"vm_id":"` + instanceID.String() + `"}`), Status: "queued"}

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterAgentGatewayServer(server, NewGateway(store))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := pb.NewAgentGatewayClient(connection)

	unauthenticated, err := client.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = unauthenticated.Recv(); err == nil {
		t.Fatal("connection without bearer token was accepted")
	}

	ctx, cancel := context.WithCancel(metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer valid-token"))
	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if first.GetCommandId() != commandID.String() || first.GetStartVm().GetVmId() != instanceID.String() {
		t.Fatalf("unexpected replayed command: %#v", first)
	}
	heartbeat := &pb.AgentEnvelope{MessageId: uuid.NewString(), Payload: &pb.AgentEnvelope_Heartbeat{Heartbeat: &pb.Heartbeat{Timestamp: time.Now().Add(24 * time.Hour).Unix(), Vms: []*pb.VMSummary{{VmId: instanceID.String(), Status: pb.VMStatus_VM_STATUS_RUNNING}}}}}
	if err = stream.Send(heartbeat); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { store.mu.Lock(); defer store.mu.Unlock(); return len(store.heartbeats) == 1 })
	if err = stream.Send(heartbeat); err != nil {
		t.Fatal(err)
	}
	time.Sleep(25 * time.Millisecond)
	store.mu.Lock()
	if len(store.heartbeats) != 1 {
		t.Fatalf("duplicate message applied %d times", len(store.heartbeats))
	}
	if !store.heartbeats[0].Timestamp.Before(time.Now().Add(time.Minute)) {
		t.Fatal("gateway trusted future agent timestamp")
	}
	store.mu.Unlock()
	cancel()

	reconnectCtx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer valid-token")
	reconnected, err := client.Connect(reconnectCtx)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := reconnected.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if replayed.GetCommandId() != commandID.String() {
		t.Fatalf("reconnected command id = %q", replayed.GetCommandId())
	}
	if err = reconnected.Send(&pb.AgentEnvelope{MessageId: uuid.NewString(), Payload: &pb.AgentEnvelope_CmdResult{CmdResult: &pb.CommandResult{CommandId: commandID.String(), Success: true, Data: []byte(`{"ok":true}`)}}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return store.commands[commandID].Status == "succeeded"
	})
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}

type memoryStore struct {
	mu          sync.Mutex
	nodeID      uuid.UUID
	connections int
	messages    map[string]struct{}
	commands    map[uuid.UUID]Command
	heartbeats  []Heartbeat
}

func newMemoryStore(nodeID uuid.UUID) *memoryStore {
	return &memoryStore{nodeID: nodeID, messages: map[string]struct{}{}, commands: map[uuid.UUID]Command{}}
}

func (s *memoryStore) Authenticate(_ context.Context, token []byte) (uuid.UUID, error) {
	if string(token) != "valid-token" {
		return uuid.Nil, ErrNotFound
	}
	return s.nodeID, nil
}
func (s *memoryStore) Connected(context.Context, uuid.UUID, uuid.UUID, string) error {
	s.mu.Lock()
	s.connections++
	s.mu.Unlock()
	return nil
}
func (s *memoryStore) Disconnected(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s *memoryStore) ClaimMessage(_ context.Context, _ uuid.UUID, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.messages[id]; ok {
		return false, nil
	}
	s.messages[id] = struct{}{}
	return true, nil
}
func (s *memoryStore) ReleaseMessage(_ context.Context, _ uuid.UUID, id string) error {
	s.mu.Lock()
	delete(s.messages, id)
	s.mu.Unlock()
	return nil
}
func (s *memoryStore) Heartbeat(_ context.Context, _ uuid.UUID, heartbeat Heartbeat) error {
	s.mu.Lock()
	s.heartbeats = append(s.heartbeats, heartbeat)
	s.mu.Unlock()
	return nil
}
func (s *memoryStore) CreateCommand(context.Context, uuid.UUID, *uuid.UUID, string, string, any) (Command, error) {
	return Command{}, errors.New("not implemented")
}
func (s *memoryStore) Pending(_ context.Context, node uuid.UUID) ([]Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []Command
	for _, command := range s.commands {
		if command.NodeID == node && (command.Status == "queued" || command.Status == "dispatched") {
			result = append(result, command)
		}
	}
	return result, nil
}
func (s *memoryStore) MarkDispatched(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	command := s.commands[id]
	command.Status = "dispatched"
	s.commands[id] = command
	return nil
}
func (s *memoryStore) Complete(_ context.Context, id uuid.UUID, success bool, result json.RawMessage, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	command := s.commands[id]
	if success {
		command.Status = "succeeded"
	} else {
		command.Status = "failed"
	}
	command.Result, command.Error = result, message
	s.commands[id] = command
	return nil
}
func (s *memoryStore) Command(_ context.Context, id uuid.UUID) (Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	command, ok := s.commands[id]
	if !ok {
		return Command{}, ErrNotFound
	}
	return command, nil
}
func (s *memoryStore) VM(context.Context, uuid.UUID, uuid.UUID) (VMState, error) {
	return VMState{}, ErrNotFound
}
func (s *memoryStore) InstanceSpec(context.Context, uuid.UUID) (InstanceSpec, error) {
	return InstanceSpec{}, ErrNotFound
}
func (s *memoryStore) Images(context.Context, uuid.UUID) ([]Image, error) { return nil, nil }
func (s *memoryStore) PortForwards(context.Context, uuid.UUID, uuid.UUID) ([]PortForward, error) {
	return nil, nil
}
func (s *memoryStore) SavePortForwards(context.Context, uuid.UUID, uuid.UUID, []PortForward) error {
	return nil
}
func (s *memoryStore) Healthy(context.Context, uuid.UUID, time.Duration) (bool, error) {
	return true, nil
}
