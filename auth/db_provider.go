package auth

import (
	"fmt"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// DBUserProviderConfig 数据库用户加载配置。
type DBUserProviderConfig struct {
	Table           string   // 用户表名
	IDColumn        string   // 主键列，默认 "id"
	IdentityColumns []string // 登录身份列（OR 匹配），默认 ["username"]
}

// dbUserProvider 基于 contracts.Query 的用户提供者（内置 driver）。
type dbUserProvider struct {
	db      contracts.DB
	cfg     DBUserProviderConfig
	factory func() contracts.AuthUser
}

// NewDBUserProvider 创建数据库用户提供者。
// factory 返回业务用户模型的空指针实例（须实现 contracts.AuthUser）。
func NewDBUserProvider(db contracts.DB, cfg DBUserProviderConfig, factory func() contracts.AuthUser) contracts.UserProvider {
	if cfg.IDColumn == "" {
		cfg.IDColumn = "id"
	}
	if len(cfg.IdentityColumns) == 0 {
		cfg.IdentityColumns = []string{"username"}
	}
	return &dbUserProvider{db: db, cfg: cfg, factory: factory}
}

func (p *dbUserProvider) RetrieveByID(_ contracts.Context, id any) (contracts.AuthUser, error) {
	user := p.factory()
	err := p.db.Query().Table(p.cfg.Table).
		Where(p.cfg.IDColumn+" = ?", id).
		First(user)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUserNotFound, err)
	}
	return user, nil
}

func (p *dbUserProvider) RetrieveByIdentity(_ contracts.Context, identity string) (contracts.AuthUser, error) {
	conds := make([]string, len(p.cfg.IdentityColumns))
	args := make([]any, len(p.cfg.IdentityColumns))
	for i, col := range p.cfg.IdentityColumns {
		conds[i] = col + " = ?"
		args[i] = identity
	}
	user := p.factory()
	err := p.db.Query().Table(p.cfg.Table).
		Where("("+strings.Join(conds, " OR ")+")", args...).
		First(user)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUserNotFound, err)
	}
	return user, nil
}
