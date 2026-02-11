package queue

import "context"

type Job struct {
	DeliveryID string
	EventType  string
	Repository string
	MergeReqID int64
	HeadSHA    string
}

type Queue interface {
	Enqueue(context.Context, Job) error
	Dequeue(context.Context) (Job, error)
}

type MemoryQueue struct {
	ch chan Job
}

func NewMemoryQueue(buffer int) *MemoryQueue {
	return &MemoryQueue{ch: make(chan Job, buffer)}
}

func (q *MemoryQueue) Enqueue(ctx context.Context, job Job) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case q.ch <- job:
		return nil
	}
}

func (q *MemoryQueue) Dequeue(ctx context.Context) (Job, error) {
	select {
	case <-ctx.Done():
		return Job{}, ctx.Err()
	case j := <-q.ch:
		return j, nil
	}
}
