// Package edition 在编译期固定当前构建形态（SaaS / 独立版）。
//
// 设计原则：
//   - 版本完全由构建标签决定，运行期不可切换（不读配置、不读环境变量）。
//   - 严禁在业务层写 `if edition.IsSaaS() { ... }` 分支；有差异请拆成
//     带构建标签的两份文件。本包仅供启动横幅、日志、调试信息等非业务场景使用。
package edition

type Kind string

const (
	SaaS       Kind = "saas"
	Standalone Kind = "standalone"
)

func Current() Kind      { return current }
func IsSaaS() bool       { return current == SaaS }
func IsStandalone() bool { return current == Standalone }
