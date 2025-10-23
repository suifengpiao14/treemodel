package treemodel

import (
	"fmt"
	"strings"

	"github.com/spf13/cast"
	"github.com/suifengpiao14/sqlbuilder"
	"github.com/suifengpiao14/treemodel/field"
)

type TreeService struct {
	table          sqlbuilder.TableConfig
	treeMiddleware _TreeMiddleware
}

func NewTreeService(table sqlbuilder.TableConfig) TreeService {
	err := table.CheckMissOutFieldName(Table_tree)
	if err != nil {
		panic(err)
	}
	return TreeService{
		table: table,
	}
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

func (s TreeService) AddNode(in AddNodeIn) (err error) {
	_, _, err = s.table.Repository().InsertWithLastId(in.Fields(), func(p *sqlbuilder.InsertParam) {
		p.WithModelMiddleware(s.treeMiddleware.Add())
	})
	if err != nil {
		return err
	}

	return nil
}

// getNode 获取节点信息 内部使用,因为固定返回TreeModel 模型
func (s TreeService) getNode(id int) (model *TreeModel, err error) {
	fields := sqlbuilder.Fields{
		field.NewId(id).SetRequired(true).AppendWhereFn(sqlbuilder.ValueFnForward),
		field.NewDeletedAt(),
	}
	model = &TreeModel{}
	err = s.table.Repository().FirstMustExists(model, fields)
	if err != nil {
		return nil, err
	}
	return model, nil
}

type UpdateIn struct {
	Id          int64  `json:"id"`
	Title       string `json:"title"`
	ExtraFields sqlbuilder.Fields
}

func (in UpdateIn) Fields() sqlbuilder.Fields {
	return sqlbuilder.Fields{
		field.NewId(int(in.Id)).SetRequired(true).ShieldUpdate(true).AppendWhereFn(sqlbuilder.ValueFnForward), // where id = ?
		field.NewTitle(in.Title),
	}.Add(in.ExtraFields...)
}

// 修改节点
func (s TreeService) UpdateNode(in UpdateIn) error {
	err := s.table.Repository().Update(in.Fields())
	if err != nil {
		return err
	}
	return err
}

// 删除节点（逻辑删除）
func (s TreeService) DeleteNode(id int, fs ...*sqlbuilder.Field) error {
	fields := sqlbuilder.Fields{
		field.NewId(id).SetRequired(true).ShieldUpdate(true).AppendWhereFn(sqlbuilder.ValueFnForward),
		field.NewDeletedAt().SetRequired(true),
	}.Add(fs...)
	err := s.table.Repository().Update(fields)
	if err != nil {
		return err
	}
	return err
}

func (s TreeService) GetNodes(ids []int, models any) (err error) {
	fields := sqlbuilder.Fields{
		field.NewId(ids).SetRequired(true).AppendWhereFn(sqlbuilder.ValueFnForward),
		field.NewDeletedAt(),
	}
	err = s.table.Repository().All(models, fields)
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
func (s TreeService) MoveNode(id int, newParentID int) (err error) {
	ids := []int{id, newParentID}
	models := TreeModels{}
	err = s.GetNodes(ids, &models)
	if err != nil {
		return err
	}
	parent, exists := models.GetById(newParentID)
	if !exists {
		parent = &TreeModel{}
	}
	model, exists := models.GetById(id)
	if !exists {
		err = fmt.Errorf("id: %d not found", id)
		return err
	}

	fields := sqlbuilder.Fields{
		field.NewId(id).SetRequired(true).ShieldUpdate(true).AppendWhereFn(sqlbuilder.ValueFnForward),
		field.NewParentId(newParentID).SetRequired(true).SetAllowZero(true),
	}

	oldPath := model.Path
	newPath := s.buildPath(parent.Path, id)

	/*
				UPDATE t_tree
		SET Fpath = REPLACE(Fpath, ?, ?)
		WHERE Fpath LIKE CONCAT(?, '%');
	*/
	pathFs := sqlbuilder.Fields{
		field.NewPath(newPath).AppendWhereFn(sqlbuilder.ValueFnWhereLikev2(false, true)).Apply(func(f *sqlbuilder.Field, fs ...*sqlbuilder.Field) {
			f.ValueFns.Append(sqlbuilder.ValueFnOnlyForData(sqlbuilder.ValueFnReplace(oldPath)))
		}),
	}

	err = s.table.Repository().Transaction(func(txRepository sqlbuilder.Repository) (err error) {
		err = txRepository.Update(fields)
		if err != nil {
			return err
		}
		if !pathFs.IsEmpty() {
			err = txRepository.Update(pathFs)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil

}

// 查子树：where path like "prefix%"
func (s TreeService) GetSubTree(pathPrefix string, dst any) (err error) {
	fields := sqlbuilder.Fields{
		field.NewPath(pathPrefix).SetRequired(true).SetAllowZero(true).AppendWhereFn(sqlbuilder.ValueFnWhereLikev2(false, true)),
		field.NewDeletedAt(),
	}
	err = s.table.Repository().All(dst, fields)
	if err != nil {
		return err
	}
	return nil
}

// 查祖先节点：根据 path 拆分 id，再 where in
func (s TreeService) GetAncestors(path string, dst any) (err error) {
	// 假设 path = "1/3/8"
	arr := strings.Split(path, "/")
	ids := make([]int, 0)
	for _, id := range arr {
		ids = append(ids, cast.ToInt(id))
	}

	fields := sqlbuilder.Fields{
		field.NewId(ids).SetRequired(true).SetAllowZero(true).AppendWhereFn(sqlbuilder.ValueFnForward),
		field.NewDeletedAt(),
	}
	err = s.table.Repository().All(dst, fields)
	if err != nil {
		return err
	}
	return nil
}

func (s TreeService) buildPath(parentPath string, id int) string {
	return s.treeMiddleware.buildPath(parentPath, id)
}
