package redis_driver

import (
	"fmt"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// pending 待派发任务（Builder 模式）。
type pending struct {
	mgr        *RedisDriver
	jobs       []contracts.QueueChain
	queue      string
	connection string
	delay      time.Time
	retry      int
}

func (p *pending) OnQueue(queue string) contracts.QueuePending {
	p.queue = queue
	return p
}

func (p *pending) OnConnection(connection string) contracts.QueuePending {
	p.connection = connection
	return p
}

func (p *pending) Delay(delay time.Time) contracts.QueuePending {
	p.delay = delay
	return p
}

func (p *pending) Retry(times int) contracts.QueuePending {
	p.retry = times
	return p
}

// Dispatch 推送到 Redis 队列异步执行。
func (p *pending) Dispatch() error {
	if p.queue == "" {
		p.queue = defaultQueueName
	}
	return p.mgr.enqueue(p)
}

// DispatchSync 同步立即执行（不走 Redis 队列）。
func (p *pending) DispatchSync() error {
	return execChain(p.jobs)
}

func execChain(chain []contracts.QueueChain) error {
	for _, item := range chain {
		args := make([]any, len(item.Args))
		for i, a := range item.Args {
			args[i] = a.Value
		}
		if err := item.Job.Handle(args...); err != nil {
			return fmt.Errorf("[GoFast] redis queue job %s failed: %w", item.Job.Signature(), err)
		}
	}
	return nil
}
