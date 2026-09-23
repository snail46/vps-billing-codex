package provision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	domainEventsStream = "domain-events"
	provisionGroup     = "provision-triggers"
)

type TriggerConsumer struct {
	repository *Repository
	redis      *redis.Client
	consumer   string
}

func NewTriggerConsumer(repository *Repository, redisClient *redis.Client, consumer string) *TriggerConsumer {
	return &TriggerConsumer{repository: repository, redis: redisClient, consumer: consumer}
}

func (c *TriggerConsumer) ProcessBatch(ctx context.Context) (int, error) {
	if err := c.redis.XGroupCreateMkStream(ctx, domainEventsStream, provisionGroup, "0").Err(); err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return 0, err
	}
	streams, err := c.redis.XReadGroup(ctx, &redis.XReadGroupArgs{Group: provisionGroup, Consumer: c.consumer, Streams: []string{domainEventsStream, ">"}, Count: 50, Block: 10 * time.Millisecond}).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, stream := range streams {
		for _, message := range stream.Messages {
			if err := c.process(ctx, message); err != nil {
				return processed, err
			}
			if err := c.redis.XAck(ctx, domainEventsStream, provisionGroup, message.ID).Err(); err != nil {
				return processed, err
			}
			processed++
		}
	}
	return processed, nil
}

func (c *TriggerConsumer) process(ctx context.Context, message redis.XMessage) error {
	if fmt.Sprint(message.Values["event_type"]) != "payment.succeeded.v1" {
		return nil
	}
	var envelope struct {
		Data struct {
			OrderID uuid.UUID `json:"order_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(fmt.Sprint(message.Values["payload"])), &envelope); err != nil {
		return err
	}
	if envelope.Data.OrderID == uuid.Nil {
		return errors.New("payment event has no order_id")
	}
	_, err := c.repository.EnsureForPaidOrder(ctx, envelope.Data.OrderID, "payment:"+message.ID)
	if errors.Is(err, ErrPurchaseNotReady) {
		// Renewal payments have their own Subscription and intentionally do not provision.
		return nil
	}
	return err
}
