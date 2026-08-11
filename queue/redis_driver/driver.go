package redis_driver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

var bgCtx = context.Background()

const defaultQueueName = "default"
const defaultWorkers = 4

// Config Redis 队列驱动配置。
type Config struct {
	Host     string
	Port     int
	Password string
	DB       int
	Prefix   string
	Workers  int
}

// serialized 序列化的任务载荷。
type serialized struct {
	Signatures []string          `json:"signatures"`
	Jobs       []json.RawMessage `json:"jobs"`
	Queue      string            `json:"queue"`
	Retry      int               `json:"retry"`
	Attempts   int               `json:"attempts"`
	CreatedAt  int64             `json:"created_at"`
}

// RedisDriver 基于 Redis 的队列驱动。
type RedisDriver struct {
	client  *redis.Client
	log     contracts.Log
	prefix  string
	workers int
	stopCh  chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	stopped bool
}

// New 创建 Redis 队列驱动并启动 worker。
func New(log contracts.Log, cfg Config) (*RedisDriver, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := client.Ping(bgCtx).Err(); err != nil {
		return nil, fmt.Errorf("[GoFast] redis queue connect: %w", err)
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "gofast"
	}
	if cfg.Workers <= 0 {
		cfg.Workers = defaultWorkers
	}
	r := &RedisDriver{
		client:  client,
		log:     log,
		prefix:  cfg.Prefix,
		workers: cfg.Workers,
		stopCh:  make(chan struct{}),
	}
	r.startWorkers()
	return r, nil
}

// queueKey Redis 列表键（待处理任务）。
func (r *RedisDriver) queueKey(name string) string {
	if name == "" {
		name = defaultQueueName
	}
	return fmt.Sprintf("%s:queue:%s", r.prefix, name)
}

// delayedKey Redis ZSET 键（延迟任务）。
func (r *RedisDriver) delayedKey() string {
	return fmt.Sprintf("%s:delayed", r.prefix)
}

func (r *RedisDriver) startWorkers() {
	for i := 0; i < r.workers; i++ {
		r.wg.Add(1)
		go r.worker()
	}
}

// worker 轮询 Redis 队列并执行任务。
func (r *RedisDriver) worker() {
	defer r.wg.Done()
	keys := r.allQueueKeys()

	for {
		select {
		case <-r.stopCh:
			return
		default:
		}

		// 检查延迟任务是否到期
		r.processDelayed()

		// BLPOP 阻塞等待新任务（5 秒超时以支持优雅关闭）
		result, err := r.client.BLPop(bgCtx, 5*time.Second, keys...).Result()
		if err != nil || len(result) < 2 {
			continue
		}

		var payload serialized
		if err := json.Unmarshal([]byte(result[1]), &payload); err != nil {
			r.log.Errorf("[GoFast] redis queue unmarshal: %v", err)
			continue
		}

		r.execute(payload)
	}
}

func (r *RedisDriver) allQueueKeys() []string {
	return []string{r.queueKey(defaultQueueName)}
}

// processDelayed 将到期的延迟任务移到主队列。
func (r *RedisDriver) processDelayed() {
	now := time.Now().Unix()
	jobs, err := r.client.ZRangeByScore(bgCtx, r.delayedKey(), &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%d", now),
	}).Result()
	if err != nil || len(jobs) == 0 {
		return
	}
	for _, job := range jobs {
		r.client.LPush(bgCtx, r.queueKey(defaultQueueName), job)
	}
	r.client.ZRemRangeByScore(bgCtx, r.delayedKey(), "0", fmt.Sprintf("%d", now))
}

// execute 执行单个任务（含重试处理）。
func (r *RedisDriver) execute(payload serialized) {
	defer func() {
		if rec := recover(); rec != nil {
			r.log.Errorf("[GoFast] redis queue panic in %v: %v", payload.Signatures, rec)
		}
	}()

	for _, rawJob := range payload.Jobs {
		job, err := resolveJob(rawJob)
		if err != nil {
			r.log.Errorf("[GoFast] redis queue resolve job: %v", err)
			return
		}
		if err := job.Handle(); err != nil {
			if payload.Attempts <= payload.Retry {
				payload.Attempts++
				r.retryJob(payload)
			} else {
				r.log.WithError(err).WithField("job", payload.Signatures).
					Errorf("[GoFast] redis queue job failed after %d retries, discarded", payload.Retry)
			}
			return
		}
	}
}

// retryJob 将失败任务重新放回延迟队列。
func (r *RedisDriver) retryJob(payload serialized) {
	backoff := time.Duration(payload.Attempts) * 500 * time.Millisecond
	payload.CreatedAt = time.Now().Add(backoff).Unix()
	data, _ := json.Marshal(payload)
	r.client.LPush(bgCtx, r.queueKey(payload.Queue), string(data))
}

// Stop 优雅关闭驱动。
func (r *RedisDriver) Stop() {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	r.mu.Unlock()
	close(r.stopCh)
	r.wg.Wait()
	_ = r.client.Close()
}

// enqueue 将任务推送到 Redis 队列。
func (r *RedisDriver) enqueue(p *pending) error {
	payload := serialized{
		Queue:     p.queue,
		Retry:     p.retry,
		Attempts:  1,
		CreatedAt: time.Now().Unix(),
	}

	for _, item := range p.jobs {
		payload.Signatures = append(payload.Signatures, item.Job.Signature())
		job, err := marshalJob(item.Job, item.Args)
		if err != nil {
			return fmt.Errorf("[GoFast] redis queue marshal: %w", err)
		}
		payload.Jobs = append(payload.Jobs, job)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("[GoFast] redis queue marshal payload: %w", err)
	}

	// 延迟任务写入 ZSET，即发任务写入 LPUSH
	if !p.delay.IsZero() {
		return r.client.ZAdd(bgCtx, r.delayedKey(), redis.Z{
			Score:  float64(p.delay.Unix()),
			Member: string(data),
		}).Err()
	}
	return r.client.LPush(bgCtx, r.queueKey(p.queue), string(data)).Err()
}

// ── contracts.Queue 实现 ─────────────────────────────────────────────

func (r *RedisDriver) Job(job contracts.QueueJob, args []contracts.QueueArg) contracts.QueuePending {
	return &pending{
		mgr:  r,
		jobs: []contracts.QueueChain{{Job: job, Args: args}},
	}
}

func (r *RedisDriver) Chain(jobs []contracts.QueueChain) contracts.QueuePending {
	return &pending{mgr: r, jobs: jobs}
}
