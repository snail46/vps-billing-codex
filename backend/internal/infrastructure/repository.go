package infrastructure

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "vps-billing/backend/internal/store/sqlc"
)

type PostgresRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, queries: db.New(pool)}
}

func (r *PostgresRepository) CreateProvider(ctx context.Context, record ProviderRecord) (ProviderRecord, error) {
	row, err := r.queries.CreateInfrastructureProvider(ctx, db.CreateInfrastructureProviderParams{
		ID: record.ID, Name: record.Name, ProviderType: record.ProviderType,
		Endpoint: optionalText(record.Endpoint), CredentialRef: optionalText(record.CredentialRef),
		Status: record.Status, Config: nonNilJSON(record.Config), Capabilities: nonNilJSON(record.Capabilities),
	})
	return providerFromRow(row), err
}

func (r *PostgresRepository) CreateNodeGroup(ctx context.Context, group NodeGroup) (NodeGroup, error) {
	row, err := r.queries.CreateNodeGroup(ctx, db.CreateNodeGroupParams{ID: group.ID, Name: group.Name, Region: group.Region, Status: group.Status})
	return NodeGroup{ID: row.ID, Name: row.Name, Region: row.Region, Status: row.Status}, err
}

func (r *PostgresRepository) CreateNode(ctx context.Context, node Node) (Node, error) {
	row, err := r.queries.CreateNode(ctx, db.CreateNodeParams{
		ID: node.ID, ProviderID: node.ProviderID, NodeGroupID: node.NodeGroupID,
		ProviderNodeID: optionalText(node.ProviderNodeID), Name: node.Name, Region: node.Region, Status: node.Status,
		CpuTotal: node.Total.CPUCores, MemoryTotalMb: node.Total.MemoryMB, DiskTotalGb: node.Total.DiskGB,
		Ipv4Total: node.Total.IPv4Count, Ipv6Total: node.Total.IPv6Count, NatPortTotal: node.Total.NATPortCount,
		Weight: node.Weight, Capabilities: nonNilJSON(node.Capabilities), LastSeenAt: optionalTime(node.LastSeenAt),
	})
	return nodeFromRow(row), err
}

func (r *PostgresRepository) ListProviders(ctx context.Context) ([]ProviderRecord, error) {
	rows, err := r.queries.ListInfrastructureProviders(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ProviderRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, providerFromRow(row))
	}
	return result, nil
}

func (r *PostgresRepository) ListNodeGroups(ctx context.Context) ([]NodeGroup, error) {
	rows, err := r.queries.ListNodeGroups(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]NodeGroup, 0, len(rows))
	for _, row := range rows {
		result = append(result, NodeGroup{ID: row.ID, Name: row.Name, Region: row.Region, Status: row.Status})
	}
	return result, nil
}

func (r *PostgresRepository) ListNodes(ctx context.Context) ([]Node, error) {
	rows, err := r.queries.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Node, 0, len(rows))
	for _, row := range rows {
		result = append(result, nodeFromRow(row))
	}
	return result, nil
}

func providerFromRow(row db.Provider) ProviderRecord {
	return ProviderRecord{ID: row.ID, Name: row.Name, ProviderType: row.ProviderType, Endpoint: textPointer(row.Endpoint), CredentialRef: textPointer(row.CredentialRef), Status: row.Status, Config: row.Config, Capabilities: row.Capabilities}
}

func nodeFromRow(row db.Node) Node {
	return Node{
		ID: row.ID, ProviderID: row.ProviderID, NodeGroupID: row.NodeGroupID, ProviderNodeID: textPointer(row.ProviderNodeID),
		Name: row.Name, Region: row.Region, Status: row.Status,
		Total:     Capacity{CPUCores: row.CpuTotal, MemoryMB: row.MemoryTotalMb, DiskGB: row.DiskTotalGb, IPv4Count: row.Ipv4Total, IPv6Count: row.Ipv6Total, NATPortCount: row.NatPortTotal},
		Allocated: Capacity{CPUCores: row.CpuAllocated, MemoryMB: row.MemoryAllocatedMb, DiskGB: row.DiskAllocatedGb, IPv4Count: row.Ipv4Allocated, IPv6Count: row.Ipv6Allocated, NATPortCount: row.NatPortAllocated},
		Reserved:  Capacity{CPUCores: row.CpuReserved, MemoryMB: row.MemoryReservedMb, DiskGB: row.DiskReservedGb, IPv4Count: row.Ipv4Reserved, IPv6Count: row.Ipv6Reserved, NATPortCount: row.NatPortReserved},
		Weight:    row.Weight, Capabilities: row.Capabilities, LastSeenAt: timePointer(row.LastSeenAt), Version: row.Version,
	}
}

func reservationFromRow(row db.ResourceReservation) Reservation {
	return Reservation{ID: row.ID, NodeID: row.NodeID, OperationID: row.OperationID, Resources: Capacity{CPUCores: row.CpuCores, MemoryMB: row.MemoryMb, DiskGB: row.DiskGb, IPv4Count: row.Ipv4Count, IPv6Count: row.Ipv6Count, NATPortCount: row.NatPortCount}, Status: row.Status, ExpiresAt: row.ExpiresAt.Time.UTC()}
}

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}
func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
func optionalTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}
func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}
func nonNilJSON(value json.RawMessage) []byte {
	if len(value) == 0 {
		return []byte(`{}`)
	}
	return value
}

func newID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New()
	}
	return id
}
