package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type RedisQueue struct {
	client *redis.Client
	key    string
}

func NewRedisQueue(client *redis.Client, key string) *RedisQueue {
	if key == "" {
		key = "thule:jobs"
	}
	return &RedisQueue{client: client, key: key}
}

func (q *RedisQueue) Enqueue(ctx context.Context, job Job) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	if err := q.client.RPush(ctx, q.key, payload).Err(); err != nil {
		return fmt.Errorf("redis enqueue: %w", err)
	}
	return nil
}

func (q *RedisQueue) Dequeue(ctx context.Context) (Job, error) {
	res, err := q.client.BLPop(ctx, 0, q.key).Result()
	if err != nil {
		return Job{}, err
	}
	if len(res) < 2 {
		return Job{}, fmt.Errorf("redis dequeue: empty result")
	}
	var job Job
	if err := json.Unmarshal([]byte(res[1]), &job); err != nil {
		return Job{}, fmt.Errorf("unmarshal job: %w", err)
	}
	return job, nil
}
