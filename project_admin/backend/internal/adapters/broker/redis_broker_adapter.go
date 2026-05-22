// Package broker — Redis Streams implementation of ports.BrokerPort.
//
// Topology:
//   - One Redis stream per topic. Stream key = `stream:<topic>`.
//   - One consumer group per worker pool. Group name passed by caller.
//   - Each goroutine in the pool is a unique consumer in that group.
//   - Messages stay in the stream until XACK; unACK'd messages older
//     than reclaimIdle are returned to the pool via XAUTOCLAIM (60s
//     background sweep).
package broker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"keepstar-admin/internal/logger"
)

const (
	streamKeyPrefix = "stream:"
	payloadField    = "payload"
	reclaimIdle     = 5 * time.Minute
	reclaimEvery    = 60 * time.Second
	readBlock       = 5 * time.Second
)

// RedisBrokerAdapter implements ports.BrokerPort over Redis Streams.
type RedisBrokerAdapter struct {
	rdb *redis.Client
	log *logger.Logger

	reclaimOnce sync.Map // topic -> chan struct{} — closed on Close to stop reclaim loop
}

func NewRedisBrokerAdapter(redisURL string, log *logger.Logger) (*RedisBrokerAdapter, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	rdb := redis.NewClient(opt)
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &RedisBrokerAdapter{rdb: rdb, log: log}, nil
}

func (a *RedisBrokerAdapter) Publish(ctx context.Context, topic string, payload []byte) error {
	key := streamKeyPrefix + topic
	if err := a.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		Values: map[string]any{payloadField: payload},
	}).Err(); err != nil {
		return fmt.Errorf("xadd %s: %w", key, err)
	}
	return nil
}

func (a *RedisBrokerAdapter) Subscribe(ctx context.Context, topic, group, consumer string,
	handler func(ctx context.Context, payload []byte) error) error {

	key := streamKeyPrefix + topic
	if err := a.ensureGroup(ctx, key, group); err != nil {
		return err
	}
	a.startReclaimLoop(ctx, key, group, consumer)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		res, err := a.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumer,
			Streams:  []string{key, ">"},
			Count:    1,
			Block:    readBlock,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			a.log.Warn("broker_xreadgroup_failed", "key", key, "consumer", consumer, "error", err)
			// Back off a bit then retry. Avoids tight-looping if Redis is
			// briefly unreachable.
			time.Sleep(time.Second)
			continue
		}
		for _, stream := range res {
			for _, msg := range stream.Messages {
				a.dispatch(ctx, key, group, msg, handler)
			}
		}
	}
}

func (a *RedisBrokerAdapter) Close() error {
	if a.rdb != nil {
		return a.rdb.Close()
	}
	return nil
}

func (a *RedisBrokerAdapter) ensureGroup(ctx context.Context, key, group string) error {
	err := a.rdb.XGroupCreateMkStream(ctx, key, group, "$").Err()
	if err == nil {
		return nil
	}
	// BUSYGROUP means the group already exists — that's the happy path.
	if strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	return fmt.Errorf("xgroup create %s/%s: %w", key, group, err)
}

func (a *RedisBrokerAdapter) dispatch(ctx context.Context, key, group string, msg redis.XMessage,
	handler func(ctx context.Context, payload []byte) error) {

	payload, ok := extractPayload(msg.Values)
	if !ok {
		// Bad message — ACK and log so we don't get stuck on it.
		a.log.Warn("broker_message_missing_payload", "key", key, "id", msg.ID)
		_ = a.rdb.XAck(ctx, key, group, msg.ID).Err()
		return
	}
	if err := handler(ctx, payload); err != nil {
		a.log.Warn("broker_handler_failed", "key", key, "id", msg.ID, "error", err)
		// Don't ACK — XAUTOCLAIM will reassign after reclaimIdle.
		return
	}
	if ackErr := a.rdb.XAck(ctx, key, group, msg.ID).Err(); ackErr != nil {
		a.log.Warn("broker_xack_failed", "key", key, "id", msg.ID, "error", ackErr)
	}
}

func extractPayload(values map[string]any) ([]byte, bool) {
	raw, ok := values[payloadField]
	if !ok {
		return nil, false
	}
	switch v := raw.(type) {
	case string:
		return []byte(v), true
	case []byte:
		return v, true
	default:
		return nil, false
	}
}

// startReclaimLoop is started once per (key, group) on first Subscribe.
// It runs XAUTOCLAIM every reclaimEvery seconds to pull stuck messages
// back to the active pool.
func (a *RedisBrokerAdapter) startReclaimLoop(ctx context.Context, key, group, consumer string) {
	lockKey := key + ":" + group
	if _, loaded := a.reclaimOnce.LoadOrStore(lockKey, struct{}{}); loaded {
		return
	}
	go func() {
		ticker := time.NewTicker(reclaimEvery)
		defer ticker.Stop()
		var cursor string = "0-0"
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			msgs, nextCursor, err := a.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
				Stream:   key,
				Group:    group,
				Consumer: consumer,
				MinIdle:  reclaimIdle,
				Start:    cursor,
				Count:    10,
			}).Result()
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					a.log.Warn("broker_xautoclaim_failed", "key", key, "error", err)
				}
				continue
			}
			cursor = nextCursor
			if len(msgs) > 0 {
				a.log.Info("broker_reclaimed", "key", key, "count", len(msgs), "consumer", consumer)
			}
		}
	}()
}
