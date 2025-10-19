package field

import (
	"github.com/suifengpiao14/commonlanguage"
	"github.com/suifengpiao14/sqlbuilder"
)

func NewId[T int | []int](id T) *sqlbuilder.Field {
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
