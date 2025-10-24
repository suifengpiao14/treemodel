package example

import (
	"strings"

	"github.com/spf13/cast"
	"github.com/suifengpiao14/sqlbuilder"
	"github.com/suifengpiao14/treemodel"
	"github.com/suifengpiao14/treemodel/field"
)

// ExampleTreeService 服务类案例
type ExampleTreeService struct {
	table          sqlbuilder.TableConfig
	treeMiddleware treemodel.TreeMiddleware
}

func NewTreeService(table sqlbuilder.TableConfig) ExampleTreeService {
	s := ExampleTreeService{
		table: table,
	}
	treeTable := s.treeMiddleware.GetTable()
	err := table.CheckMissOutFieldName(treeTable)
	if err != nil {
		panic(err)
	}
	return s
}

// 新增节点
type AddNodeIn struct {
	ParentId int    `json:"parentId"`
	Title    string `json:"title"`
}

func (in *AddNodeIn) Fields() sqlbuilder.Fields {
	return sqlbuilder.Fields{
		field.NewParentId(0).BindValue(&in.ParentId).SetRequired(true).SetAllowZero(true),
		field.NewTitle("").BindValue(&in.Title).SetRequired(true),
	}
}

func (s ExampleTreeService) AddNode(in AddNodeIn) (err error) {
	_, _, err = s.table.Repository().InsertWithLastId(in.Fields(), func(p *sqlbuilder.InsertParam) {
		p.WithModelMiddleware(s.treeMiddleware.Insert())
	})
	if err != nil {
		return err
	}

	return nil
}

type MoveNodeIn struct {
	Id          int64 `json:"id"`
	NewParentID int64 `json:"newParentId"`
}

// 移动节点（修改 parentId 与 path）
func (s ExampleTreeService) MoveNode(id int, newParentID int) (err error) {
	fields := sqlbuilder.Fields{
		field.NewId(id).SetRequired(true).ShieldUpdate(true).AppendWhereFn(sqlbuilder.ValueFnForward),
		field.NewParentId(newParentID).SetRequired(true).SetAllowZero(true),
	}

	err = s.table.Repository().Update(fields, func(p *sqlbuilder.UpdateParam) {
		p.WithModelMiddleware(s.treeMiddleware.Update())
	})
	if err != nil {
		return err
	}
	return nil

}

// 查子树：where path like "prefix%"
func (s ExampleTreeService) GetSubTree(pathPrefix string, dst any) (err error) {
	fields := sqlbuilder.Fields{
		field.NewPath(pathPrefix),
		field.NewDeletedAt(),
	}
	err = s.table.Repository().All(dst, fields, func(p *sqlbuilder.ListParam) {
		p.WithModelMiddleware(s.treeMiddleware.GetSubTree())
	})
	if err != nil {
		return err
	}
	return nil
}

// 查祖先节点：根据 path 拆分 id，再 where in
func (s ExampleTreeService) GetAncestors(path string, dst any) (err error) {
	// 假设 path = "1/3/8"
	arr := strings.Split(path, "/")
	ids := make([]int, 0)
	for _, id := range arr {
		ids = append(ids, cast.ToInt(id))
	}

	fields := sqlbuilder.Fields{
		field.NewPath(path),
		field.NewDeletedAt(),
	}
	err = s.table.Repository().All(dst, fields, func(p *sqlbuilder.ListParam) {
		p.WithModelMiddleware(s.treeMiddleware.GetAncestors())
	})
	if err != nil {
		return err
	}
	return nil
}
