package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Grpc 获取 gRPC 服务实例。
func Grpc() contracts.Grpc {
	return App().MustMake("grpc").(contracts.Grpc)
}
