package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	db "vps-billing/backend/internal/store/sqlc"
)

type Scheduler struct {
	repository *PostgresRepository
	now        func() time.Time
}

func NewScheduler(repository *PostgresRepository) *Scheduler {
	return &Scheduler{repository: repository, now: time.Now}
}

func (s *Scheduler) Reserve(ctx context.Context, operationID uuid.UUID, request ScheduleRequest) (Reservation, error) {
	if operationID == uuid.Nil || request.NodeGroupID == uuid.Nil || request.TTL <= 0 || !validCapacityRequest(request.Resources) {
		return Reservation{}, ErrInvalidRequest
	}
	requiredCapabilities, err := json.Marshal(request.RequiredCapabilities)
	if err != nil {
		return Reservation{}, ErrInvalidRequest
	}
	tx, err := s.repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Reservation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	if existing, findErr := queries.GetResourceReservationByOperation(ctx, operationID); findErr == nil {
		return reservationFromRow(existing), nil
	} else if !errors.Is(findErr, pgx.ErrNoRows) {
		return Reservation{}, findErr
	}
	params := db.SelectCandidateNodesForUpdateParams{
		NodeGroupID: &request.NodeGroupID, RequiredCapabilities: requiredCapabilities,
		CpuCores: request.Resources.CPUCores, MemoryMb: request.Resources.MemoryMB, DiskGb: request.Resources.DiskGB,
		Ipv4Count: request.Resources.IPv4Count, Ipv6Count: request.Resources.IPv6Count, NatPortCount: request.Resources.NATPortCount,
	}
	var placementPolicy string
	if err := tx.QueryRow(ctx, `SELECT placement_policy FROM node_groups WHERE id=$1`, request.NodeGroupID).Scan(&placementPolicy); err != nil {
		return Reservation{}, err
	}
	requestedResources, err := json.Marshal(request.Resources)
	if err != nil {
		return Reservation{}, ErrInvalidRequest
	}
	candidates, err := queries.SelectCandidateNodesForUpdate(ctx, params)
	if err != nil {
		return Reservation{}, err
	}
	if len(candidates) == 0 {
		if _, recordErr := tx.Exec(ctx, `INSERT INTO scheduler_decisions(id,operation_id,node_group_id,placement_policy,candidate_count,requested_resources,required_capabilities,result) VALUES($1,$2,$3,$4,0,$5,$6,'exhausted')`, newID(), operationID, request.NodeGroupID, placementPolicy, requestedResources, requiredCapabilities); recordErr != nil {
			return Reservation{}, recordErr
		}
		if err := tx.Commit(ctx); err != nil {
			return Reservation{}, err
		}
		return Reservation{}, ErrResourceExhausted
	}
	node := candidates[0]
	score := utilizationScore(node.CpuAllocated+node.CpuReserved+request.Resources.CPUCores, node.CpuTotal, float64(node.MemoryAllocatedMb+node.MemoryReservedMb+request.Resources.MemoryMB), float64(node.MemoryTotalMb), float64(node.DiskAllocatedGb+node.DiskReservedGb+request.Resources.DiskGB), float64(node.DiskTotalGb))
	if _, err := tx.Exec(ctx, `INSERT INTO scheduler_decisions(id,operation_id,node_group_id,selected_node_id,placement_policy,candidate_count,requested_resources,required_capabilities,selected_score,result) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'selected')`, newID(), operationID, request.NodeGroupID, node.ID, placementPolicy, len(candidates), requestedResources, requiredCapabilities, score); err != nil {
		return Reservation{}, err
	}
	if _, err := queries.AddNodeReservation(ctx, db.AddNodeReservationParams{
		NodeID: node.ID, CpuCores: request.Resources.CPUCores, MemoryMb: request.Resources.MemoryMB, DiskGb: request.Resources.DiskGB,
		Ipv4Count: request.Resources.IPv4Count, Ipv6Count: request.Resources.IPv6Count, NatPortCount: request.Resources.NATPortCount,
	}); errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, ErrResourceExhausted
	} else if err != nil {
		return Reservation{}, err
	}
	row, err := queries.CreateResourceReservation(ctx, db.CreateResourceReservationParams{
		ID: newID(), NodeID: node.ID, OperationID: operationID,
		CpuCores: request.Resources.CPUCores, MemoryMb: request.Resources.MemoryMB, DiskGb: request.Resources.DiskGB,
		Ipv4Count: request.Resources.IPv4Count, Ipv6Count: request.Resources.IPv6Count, NatPortCount: request.Resources.NATPortCount,
		ExpiresAt: optionalTime(pointerTime(s.now().UTC().Add(request.TTL))),
	})
	if err != nil {
		if isUniqueViolation(err) {
			_ = tx.Rollback(ctx)
			existing, findErr := s.repository.queries.GetResourceReservationByOperation(ctx, operationID)
			if findErr != nil {
				return Reservation{}, findErr
			}
			return reservationFromRow(existing), nil
		}
		return Reservation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Reservation{}, err
	}
	return reservationFromRow(row), nil
}

func utilizationScore(cpuUsed, cpuTotal, memoryUsed, memoryTotal, diskUsed, diskTotal float64) float64 {
	score := 0.0
	for _, pair := range [][2]float64{{cpuUsed, cpuTotal}, {memoryUsed, memoryTotal}, {diskUsed, diskTotal}} {
		if pair[1] > 0 && pair[0]/pair[1] > score {
			score = pair[0] / pair[1]
		}
	}
	return score
}

func (s *Scheduler) Commit(ctx context.Context, reservationID uuid.UUID) (Reservation, error) {
	return s.changeStatus(ctx, reservationID, "committed")
}

func (s *Scheduler) Release(ctx context.Context, reservationID uuid.UUID) (Reservation, error) {
	return s.changeStatus(ctx, reservationID, "released")
}

func (s *Scheduler) changeStatus(ctx context.Context, reservationID uuid.UUID, target string) (Reservation, error) {
	tx, err := s.repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Reservation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	current, err := queries.LockResourceReservation(ctx, reservationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, ErrReservationAbsent
	}
	if err != nil {
		return Reservation{}, err
	}
	if current.Status == target {
		return reservationFromRow(current), nil
	}
	if current.Status != "reserved" {
		return Reservation{}, ErrReservationState
	}
	resources := db.CommitNodeReservationParams{NodeID: current.NodeID, CpuCores: current.CpuCores, MemoryMb: current.MemoryMb, DiskGb: current.DiskGb, Ipv4Count: current.Ipv4Count, Ipv6Count: current.Ipv6Count, NatPortCount: current.NatPortCount}
	if target == "committed" {
		if _, err := queries.CommitNodeReservation(ctx, resources); err != nil {
			return Reservation{}, err
		}
	} else {
		if _, err := queries.ReleaseNodeReservation(ctx, db.ReleaseNodeReservationParams(resources)); err != nil {
			return Reservation{}, err
		}
	}
	updated, err := queries.SetResourceReservationStatus(ctx, db.SetResourceReservationStatusParams{ID: reservationID, Status: target})
	if err != nil {
		return Reservation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Reservation{}, err
	}
	return reservationFromRow(updated), nil
}

func validCapacityRequest(value Capacity) bool {
	return value.CPUCores > 0 && value.MemoryMB > 0 && value.DiskGB > 0 && value.IPv4Count >= 0 && value.IPv6Count >= 0 && value.NATPortCount >= 0
}

func pointerTime(value time.Time) *time.Time { return &value }

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
