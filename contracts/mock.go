package contracts

// MockManager Mock 管理服务契约。
// 用于测试环境中临时替换容器内的服务实例，并在测试结束后还原。
type MockManager interface {
	// Swap 将指定 key 的服务替换为 mock 实例。
	Swap(key string, instance any)
	// Restore 还原一个或多个被替换的服务。
	Restore(keys ...string)
	// RestoreAll 还原所有被替换的服务。
	RestoreAll()
	// Cache 获取缓存 Mock（若未替换则创建并自动 Swap）。
	Cache() Cache
	// DB 获取数据库 Mock（若未替换则创建并自动 Swap）。
	DB() DB
	// Queue 获取队列 Mock（若未替换则创建并自动 Swap）。
	Queue() Queue
	// Event 获取事件 Mock（若未替换则创建并自动 Swap）。
	Event() Event
	// Storage 获取存储 Mock（若未替换则创建并自动 Swap）。
	Storage() Storage
	// Log 获取日志 Mock（若未替换则创建并自动 Swap）。
	Log() Log
}
