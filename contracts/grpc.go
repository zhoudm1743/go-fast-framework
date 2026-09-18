package contracts

import (
	"context"
	"time"

	"google.golang.org/grpc"
)

// ── gRPC 服务 ───────────────────────────────────────────────────────

// Grpc gRPC 服务契约。门面风格：facades.Grpc().Server() / facades.Grpc().Client()。
type Grpc interface {
	// Server 获取服务端封装（原生 Server 供生成代码 RegisterXxxServiceServer 使用）。
	Server() GrpcServer
	// Client 获取具名连接管理器。
	Client() GrpcClient
}

// GrpcServer gRPC 服务端契约。
type GrpcServer interface {
	// Server 返回原生 grpc.Server，供生成代码注册服务实现。
	Server() *grpc.Server
	// Address 返回实际监听地址（未启动时为空）。
	Address() string
	// GracefulStop 优雅停止：等待进行中的 RPC 完成后再关闭。
	GracefulStop()
}

// GrpcClient gRPC 客户端契约。
type GrpcClient interface {
	// Conn 获取具名连接，省略 name 时取 "default"。
	// 连接按配置 grpc.client.<name> 创建并缓存，首次 RPC 时才真正建立连接。
	Conn(name ...string) (*grpc.ClientConn, error)
	// Close 关闭全部已建立的连接。
	Close() error
}

// ── 注册发现 ────────────────────────────────────────────────────────

// ServiceInstance 注册的服务实例。
type ServiceInstance struct {
	ID      string            // 实例唯一 ID
	Name    string            // 服务名
	Address string            // 实例地址（host:port）
	Meta    map[string]string // 附加元数据
}

// Registry 服务注册发现契约。驱动可插拔：cache / etcd / nop。
type Registry interface {
	// Register 注册实例；实现通常内建心跳续期，直到 Unregister 或 ctx 结束。
	Register(ctx context.Context, inst ServiceInstance) error
	// Unregister 注销实例并停止心跳。
	Unregister(ctx context.Context, inst ServiceInstance) error
	// Discover 发现指定服务的存活实例列表（失联实例由实现自行过滤/清理）。
	Discover(name string) ([]ServiceInstance, error)
	// Watch 监听实例变更，返回实例列表推送通道与取消函数。
	// interval 为实现侧轮询间隔（原生 Watch 的实现可忽略）。
	Watch(name string, interval time.Duration) (<-chan []ServiceInstance, func(), error)
}
