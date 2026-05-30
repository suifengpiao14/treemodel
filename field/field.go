package field

import (
	"gitlab.huishoubao.com/gopackage/commonlanguage"
	"gitlab.huishoubao.com/gopackage/sqlbuilder"
)

func NewId(id int) *sqlbuilder.Field {
	return commonlanguage.NewId(id)
}

func NewParentId(pid int) *sqlbuilder.Field {
	return commonlanguage.NewId(pid).SetName("parentId")
}

func NewPath(path string) *sqlbuilder.Field {
	return sqlbuilder.NewStringField(path, "path", "路径", 2048)
}

func NewTitle(title string) *sqlbuilder.Field {
	return sqlbuilder.NewStringField(title, "title", "标题", 256)
}

var NewDeletedAt = commonlanguage.NewDeletedAt
