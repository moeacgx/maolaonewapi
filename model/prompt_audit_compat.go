package model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// PromptAuditLargeText 用于保存可能超过 MySQL TEXT 上限的版本化密文。
// 通过 GORM 数据类型契约让普通迁移和快速迁移采用相同定义，无需额外执行
// INFORMATION_SCHEMA 查询或数据库专用 ALTER SQL。
type PromptAuditLargeText string

func (PromptAuditLargeText) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	if db != nil && db.Dialector != nil && db.Dialector.Name() == "mysql" {
		return "LONGTEXT"
	}
	return "TEXT"
}
