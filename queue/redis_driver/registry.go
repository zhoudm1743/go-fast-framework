package redis_driver

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

var (
	jobRegistry   = make(map[string]func() contracts.QueueJob)
	jobRegistryMu sync.RWMutex
)

// Register 注册任务类型（签名 → 工厂函数），供 Redis worker 反序列化后重新实例化。
func Register(signature string, factory func() contracts.QueueJob) {
	jobRegistryMu.Lock()
	defer jobRegistryMu.Unlock()
	jobRegistry[signature] = factory
}

// GetJob 从注册表中获取任务的新实例。
func GetJob(signature string) contracts.QueueJob {
	jobRegistryMu.RLock()
	factory, ok := jobRegistry[signature]
	jobRegistryMu.RUnlock()
	if !ok {
		return nil
	}
	return factory()
}

// payloadJob 序列化载荷中的单个任务。
type payloadJob struct {
	Signature string               `json:"signature"`
	Args      []contracts.QueueArg `json:"args"`
}

// marshalJob 将任务序列化为 JSON。
func marshalJob(job contracts.QueueJob, args []contracts.QueueArg) (json.RawMessage, error) {
	pj := payloadJob{
		Signature: job.Signature(),
		Args:      args,
	}
	data, err := json.Marshal(pj)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// resolveJob 从序列化数据反序列化任务并返回可执行函数。
// 返回一个包装了 Handle 调用的函数。
func resolveJob(raw json.RawMessage) (*resolvedJob, error) {
	var pj payloadJob
	if err := json.Unmarshal(raw, &pj); err != nil {
		return nil, fmt.Errorf("解析任务失败: %w", err)
	}
	return &resolvedJob{
		signature: pj.Signature,
		args:      pj.Args,
	}, nil
}

// resolvedJob 反序列化后的可执行任务。
type resolvedJob struct {
	signature string
	args      []contracts.QueueArg
}

func (r *resolvedJob) Handle() error {
	job := GetJob(r.signature)
	if job == nil {
		return fmt.Errorf("[GoFast] redis queue: 未注册的任务类型 %q", r.signature)
	}
	anyArgs := make([]any, len(r.args))
	for i, a := range r.args {
		anyArgs[i] = a.Value
	}
	return job.Handle(anyArgs...)
}
