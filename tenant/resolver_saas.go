//go:build !standalone

package tenant

import "github.com/zhoudm1743/go-fast-framework/contracts"

// saasResolver 多租户版：从 ctx 里读取 schema_name。
type saasResolver struct{}

func (saasResolver) SchemaFromCtx(ctx contracts.Context) string {
	if ctx == nil {
		return ""
	}
	s, _ := ctx.Value("schema_name").(string)
	return s
}

func newResolver() contracts.TenantResolver { return saasResolver{} }
