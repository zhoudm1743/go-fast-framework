package database

import "github.com/zhoudm1743/go-fast-framework/contracts"

// ConnectionConfig 是 contracts.ConnectionConfig 的类型别名，保持向后兼容。
// 具体定义已移至 contracts 包，以避免 database ↔ gormdriver 的循环依赖。
type ConnectionConfig = contracts.ConnectionConfig
