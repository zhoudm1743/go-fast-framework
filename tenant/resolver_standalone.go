//go:build standalone

package tenant

import "github.com/zhoudm1743/go-fast-framework/contracts"

// fixedSchema 独立版固定 schema，编译期烧死，运行期不可改。
// 不读配置、不读环境变量，以杜绝被现场篡改开启多租户的可能。
const fixedSchema = "public"

type standaloneResolver struct{}

func (standaloneResolver) SchemaFromCtx(contracts.Context) string { return fixedSchema }

func newResolver() contracts.TenantResolver { return standaloneResolver{} }
