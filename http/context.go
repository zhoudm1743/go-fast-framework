// Package http 提供框架无关的 HTTP 抽象层（validator / session / view / 驱动注册）。
// 具体引擎驱动已拆为独立插件：
//   - github.com/zhoudm1743/gofast-fiber — 基于 Fiber v2
//   - github.com/zhoudm1743/gofast-gin   — 基于 Gin
package http
