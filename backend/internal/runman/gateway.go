package runman

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	pb "vps-billing/backend/internal/runman/proto"
)

type connection struct {
	id   uuid.UUID
	send chan Command
	sent map[uuid.UUID]struct{}
}
type Gateway struct {
	pb.UnimplementedAgentGatewayServer
	store       Store
	mu          sync.RWMutex
	connections map[uuid.UUID]*connection
}

func NewGateway(store Store) *Gateway {
	return &Gateway{store: store, connections: map[uuid.UUID]*connection{}}
}
func (g *Gateway) Online(id uuid.UUID) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.connections[id] != nil
}
func (g *Gateway) Send(ctx context.Context, c Command) error {
	g.mu.RLock()
	v := g.connections[c.NodeID]
	g.mu.RUnlock()
	if v == nil {
		return ErrNodeOffline
	}
	select {
	case v.send <- c:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *Gateway) Connect(stream pb.AgentGateway_ConnectServer) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	values := md.Get("authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return status.Error(codes.Unauthenticated, "agent token required")
	}
	node, err := g.store.Authenticate(stream.Context(), []byte(strings.TrimPrefix(values[0], "Bearer ")))
	if err != nil {
		return status.Error(codes.Unauthenticated, "invalid agent token")
	}
	conn := &connection{id: uuid.New(), send: make(chan Command, 64), sent: make(map[uuid.UUID]struct{})}
	remote := ""
	if p, ok := peer.FromContext(stream.Context()); ok {
		remote = p.Addr.String()
	}
	if err = g.store.Connected(stream.Context(), node, conn.id, remote); err != nil {
		return status.Error(codes.Internal, "persist connection")
	}
	g.mu.Lock()
	g.connections[node] = conn
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		if g.connections[node] == conn {
			delete(g.connections, node)
		}
		g.mu.Unlock()
		_ = g.store.Disconnected(context.Background(), node, conn.id)
	}()
	pending, _ := g.store.Pending(stream.Context(), node)
	for _, c := range pending {
		if err = g.sendCommand(stream, conn, c); err != nil {
			return err
		}
	}
	incoming := make(chan *pb.AgentEnvelope)
	receiveErrors := make(chan error, 1)
	go func() {
		for {
			m, e := stream.Recv()
			if e != nil {
				receiveErrors <- e
				return
			}
			incoming <- m
		}
	}()
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	dispatch := time.NewTicker(250 * time.Millisecond)
	defer dispatch.Stop()
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case e := <-receiveErrors:
			if errors.Is(e, io.EOF) {
				return nil
			}
			return e
		case c := <-conn.send:
			if sendErr := g.sendCommand(stream, conn, c); sendErr != nil {
				return sendErr
			}
		case m := <-incoming:
			if handleErr := g.handle(stream.Context(), node, m); handleErr != nil {
				return handleErr
			}
		case <-ping.C:
			if sendErr := stream.Send(&pb.PlatformEnvelope{CommandId: uuid.NewString(), Payload: &pb.PlatformEnvelope_Ping{Ping: &pb.Ping{}}}); sendErr != nil {
				return sendErr
			}
		case <-dispatch.C:
			queued, _ := g.store.Pending(stream.Context(), node)
			for _, command := range queued {
				if _, sent := conn.sent[command.ID]; sent {
					continue
				}
				if sendErr := g.sendCommand(stream, conn, command); sendErr != nil {
					return sendErr
				}
			}
		}
	}
}

func (g *Gateway) sendCommand(stream pb.AgentGateway_ConnectServer, conn *connection, command Command) error {
	envelope, err := commandEnvelope(command)
	if err != nil {
		_ = g.store.Complete(stream.Context(), command.ID, false, nil, err.Error())
		return nil
	}
	if err = stream.Send(envelope); err != nil {
		return err
	}
	conn.sent[command.ID] = struct{}{}
	return g.store.MarkDispatched(stream.Context(), command.ID)
}

func (g *Gateway) handle(ctx context.Context, node uuid.UUID, m *pb.AgentEnvelope) error {
	if m.GetMessageId() == "" {
		return status.Error(codes.InvalidArgument, "message_id is required")
	}
	claimed, err := g.store.ClaimMessage(ctx, node, m.GetMessageId())
	if err != nil {
		return status.Error(codes.Internal, "persist message")
	}
	if !claimed {
		return nil
	}
	err = g.handleClaimed(ctx, node, m)
	if err != nil {
		_ = g.store.ReleaseMessage(ctx, node, m.GetMessageId())
	}
	return err
}

func (g *Gateway) handleClaimed(ctx context.Context, node uuid.UUID, m *pb.AgentEnvelope) error {
	if h := m.GetHeartbeat(); h != nil {
		// Availability is based on receipt time. An agent-controlled timestamp must
		// never keep a disconnected node healthy indefinitely.
		at := time.Now().UTC()
		vms := []VMState{}
		for _, v := range h.GetVms() {
			id, e := uuid.Parse(v.GetVmId())
			if e != nil {
				continue
			}
			vms = append(vms, VMState{InstanceID: id, Status: vmStatus(v.GetStatus()), CPUPercent: float64(v.GetCpuPct()), RAMUsedMB: v.GetRamUsedMb(), TrafficIn: v.GetTrafficInBytes(), TrafficOut: v.GetTrafficOutBytes(), MonthlyIn: v.GetMonthlyTrafficIn(), MonthlyOut: v.GetMonthlyTrafficOut(), IPs: v.GetIps(), ObservedAt: at})
		}
		images := []Image{}
		for _, v := range h.GetOsImages() {
			images = append(images, Image{ID: v.GetId(), Name: v.GetName()})
		}
		return g.store.Heartbeat(ctx, node, Heartbeat{Timestamp: at, CPUPercent: float64(h.GetCpuPct()), RAMUsedMB: h.GetRamUsedMb(), DiskUsedGB: h.GetDiskUsedGb(), NetInBPS: h.GetNetInBps(), NetOutBPS: h.GetNetOutBps(), RAMTotalMB: h.GetRamTotalMb(), DiskTotalGB: h.GetDiskTotalGb(), CPUs: h.GetCpus(), BandwidthMbps: h.GetBandwidthMbps(), VirtType: h.GetVirtType(), EntryHost: h.GetEntryHost(), EntryIPv6: h.GetEntryIpv6(), VMs: vms, Images: images})
	}
	if r := m.GetCmdResult(); r != nil {
		id, e := uuid.Parse(r.GetCommandId())
		if e != nil {
			return nil
		}
		return g.store.Complete(ctx, id, r.GetSuccess(), r.GetData(), r.GetError())
	}
	if list := m.GetPortFwdList(); list != nil {
		id, e := uuid.Parse(list.GetCommandId())
		if e != nil {
			return nil
		}
		cmd, e := g.store.Command(ctx, id)
		if e != nil {
			return nil
		}
		var p commandPayload
		_ = json.Unmarshal(cmd.Payload, &p)
		vm, e := uuid.Parse(p.VMID)
		if e != nil {
			return nil
		}
		items := []PortForward{}
		for _, v := range list.GetEntries() {
			protocol := "tcp"
			if v.GetProtocol() == pb.Protocol_PROTOCOL_UDP {
				protocol = "udp"
			}
			items = append(items, PortForward{Protocol: protocol, HostPort: v.GetHostPort(), GuestPort: v.GetGuestPort(), Description: v.GetDescription()})
		}
		if e = g.store.SavePortForwards(ctx, node, vm, items); e != nil {
			return e
		}
		return g.store.Complete(ctx, id, true, []byte(`null`), "")
	}
	return nil
}

func vmStatus(v pb.VMStatus) string {
	switch v {
	case pb.VMStatus_VM_STATUS_CREATING:
		return "provisioning"
	case pb.VMStatus_VM_STATUS_RUNNING:
		return "running"
	case pb.VMStatus_VM_STATUS_STOPPED:
		return "stopped"
	case pb.VMStatus_VM_STATUS_ERROR:
		return "error"
	default:
		return "unknown"
	}
}

type commandPayload struct {
	VMID        string `json:"vm_id"`
	CPU         int32  `json:"cpu"`
	RAMMB       int64  `json:"ram_mb"`
	DiskGB      int64  `json:"disk_gb"`
	Bandwidth   int32  `json:"bandwidth_mbps"`
	Image       string `json:"os_image"`
	Password    string `json:"root_password"`
	Protocol    string `json:"protocol"`
	Description string `json:"description"`
	HostPort    int32  `json:"host_port"`
	GuestPort   int32  `json:"guest_port"`
	Force       bool   `json:"force"`
}

func commandEnvelope(c Command) (*pb.PlatformEnvelope, error) {
	var p commandPayload
	if err := json.Unmarshal(c.Payload, &p); err != nil {
		return nil, err
	}
	e := &pb.PlatformEnvelope{CommandId: c.ID.String()}
	switch c.Type {
	case "create":
		e.Payload = &pb.PlatformEnvelope_CreateVm{CreateVm: &pb.CmdCreateVM{VmId: p.VMID, Cpu: p.CPU, RamMb: p.RAMMB, DiskGb: p.DiskGB, BandwidthMbps: p.Bandwidth, OsImage: p.Image, RootPassword: p.Password}}
	case "start":
		e.Payload = &pb.PlatformEnvelope_StartVm{StartVm: &pb.CmdStartVM{VmId: p.VMID}}
	case "stop":
		e.Payload = &pb.PlatformEnvelope_StopVm{StopVm: &pb.CmdStopVM{VmId: p.VMID, Force: p.Force}}
	case "restart":
		e.Payload = &pb.PlatformEnvelope_RestartVm{RestartVm: &pb.CmdRestartVM{VmId: p.VMID}}
	case "delete":
		e.Payload = &pb.PlatformEnvelope_DeleteVm{DeleteVm: &pb.CmdDeleteVM{VmId: p.VMID}}
	case "reinstall":
		e.Payload = &pb.PlatformEnvelope_ReinstallVm{ReinstallVm: &pb.CmdReinstallVM{VmId: p.VMID, OsImage: p.Image, RootPassword: p.Password, Cpu: p.CPU, RamMb: p.RAMMB, DiskGb: p.DiskGB, BandwidthMbps: p.Bandwidth}}
	case "reset_password":
		e.Payload = &pb.PlatformEnvelope_ResetPassword{ResetPassword: &pb.CmdResetPassword{VmId: p.VMID, NewPassword: p.Password}}
	case "list_ports":
		e.Payload = &pb.PlatformEnvelope_GetPortFwds{GetPortFwds: &pb.CmdGetPortForwards{VmId: p.VMID}}
	case "add_port":
		proto := pb.Protocol_PROTOCOL_TCP
		if p.Protocol == "udp" {
			proto = pb.Protocol_PROTOCOL_UDP
		}
		e.Payload = &pb.PlatformEnvelope_SetPortFwd{SetPortFwd: &pb.CmdSetPortForward{VmId: p.VMID, Protocol: proto, HostPort: p.HostPort, GuestPort: p.GuestPort, Description: p.Description}}
	case "delete_port":
		proto := pb.Protocol_PROTOCOL_TCP
		if p.Protocol == "udp" {
			proto = pb.Protocol_PROTOCOL_UDP
		}
		e.Payload = &pb.PlatformEnvelope_DelPortFwd{DelPortFwd: &pb.CmdDelPortForward{VmId: p.VMID, Protocol: proto, HostPort: p.HostPort}}
	default:
		return nil, errors.New("unsupported runman command")
	}
	return e, nil
}
