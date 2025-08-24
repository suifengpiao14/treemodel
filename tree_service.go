package treemodel

import (
	"fmt"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/spf13/cast"
	"github.com/suifengpiao14/sqlbuilder"
)

type TreeService struct {
	repo sqlbuilder.Repository
}

var Table_tree = sqlbuilder.NewTableConfig("t_tree").AddColumns(
	sqlbuilder.NewColumn("Fid", sqlbuilder.GetField(NewId[int])),
	sqlbuilder.NewColumn("Fparent_id", sqlbuilder.GetField(NewParentId)),
	sqlbuilder.NewColumn("Fpath", sqlbuilder.GetField(NewPath)),
	sqlbuilder.NewColumn("Ftitle", sqlbuilder.GetField(NewTitle)),
	sqlbuilder.NewColumn("Fdeleted_at", NewDeletedAt()),
)

type TreeModel struct {
	Id       int    `gorm:"column:Fid" json:"id"`
	ParentId int    `gorm:"column:Fparent_id" json:"parentId"`
	Path     string `gorm:"column:Fpath" json:"path"`
	Title    string `gorm:"column:Ftitle" json:"title"`
}
type TreeModels []TreeModel

func (ms TreeModels) GetById(id int) (m *TreeModel, exists bool) {
	for i := range ms {
		if ms[i].Id == id {
			return &ms[i], true
		}
	}
	return nil, false
}

func NewTreeService(tableConfig sqlbuilder.TableConfig) TreeService {
	err := sqlbuilder.CheckMissFieldName(tableConfig,
		sqlbuilder.GetFieldName(NewId[int]),
		sqlbuilder.GetFieldName(NewParentId),
		sqlbuilder.GetFieldName(NewPath),
		sqlbuilder.GetFieldName(NewTitle),
		NewDeletedAt().Name,
	)
	if err != nil {
		panic(err)
	}
	return TreeService{
		repo: sqlbuilder.NewRepository(tableConfig),
	}
}

// 新增节点
type AddNodeIn struct {
	ParentId    int    `json:"parentId"`
	Title       string `json:"title"`
	ExtraFields sqlbuilder.Fields
}

func (s TreeService) AddNode(in AddNodeIn) (err error) {
	parent := &TreeModel{}
	if in.ParentId > 0 {
		parent, err = s.getNode(in.ParentId)
		if err != nil {
			return err
		}
	}

	err = s.repo.Transaction(func(txRepository sqlbuilder.Repository) (err error) {
		fields := sqlbuilder.Fields{
			NewParentId(in.ParentId),
			NewTitle(in.Title),
		}.Add(in.ExtraFields...)
		id, _, err := txRepository.InsertWithLastId(fields)
		if err != nil {
			return err
		}
		path := s.buildPath(parent.Path, int(id))
		pathFs := sqlbuilder.Fields{
			NewId(int(id)).SetRequired(true).ShieldUpdate(true).AppendWhereFn(sqlbuilder.ValueFnForward), // where id = ?
			NewPath(path),
		}
		err = s.repo.Update(pathFs)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// getNode 获取节点信息 内部使用,因为固定返回TreeModel 模型
func (s TreeService) getNode(id int) (model *TreeModel, err error) {
	fields := sqlbuilder.Fields{
		NewId(id).SetRequired(true).AppendWhereFn(sqlbuilder.ValueFnForward),
		NewDeletedAt(),
	}
	model = &TreeModel{}
	err = s.repo.FirstMustExists(model, fields)
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

// 修改节点
func (s TreeService) UpdateNode(in UpdateIn) error {
	fields := sqlbuilder.Fields{
		NewId(int(in.Id)).SetRequired(true).ShieldUpdate(true).AppendWhereFn(sqlbuilder.ValueFnForward), // where id = ?
		NewTitle(in.Title),
	}.Add(in.ExtraFields...)
	err := s.repo.Update(fields)
	if err != nil {
		return err
	}
	return err
}

// 删除节点（逻辑删除）
func (s TreeService) DeleteNode(id int, fs ...*sqlbuilder.Field) error {
	fields := sqlbuilder.Fields{
		NewId(id).SetRequired(true).ShieldUpdate(true).AppendWhereFn(sqlbuilder.ValueFnForward),
		NewDeletedAt(),
	}.Add(fs...)
	err := s.repo.Update(fields)
	if err != nil {
		return err
	}
	return err
}

func (s TreeService) getNodes(ids ...int) (models TreeModels, err error) {
	fields := sqlbuilder.Fields{
		NewId(ids).SetRequired(true).AppendWhereFn(sqlbuilder.ValueFnForward),
		NewDeletedAt(),
	}
	err = s.repo.All(&models, fields)
	if err != nil {
		return nil, err
	}
	return models, nil
}

type MoveNodeIn struct {
	Id          int64 `json:"id"`
	NewParentID int64 `json:"newParentId"`
}

// 移动节点（修改 parentId 与 path）
func (s TreeService) MoveNode(id int, newParentID int) (err error) {

	models, err := s.getNodes(id, int(newParentID))
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
		NewId(id).SetRequired(true).ShieldUpdate(true).AppendWhereFn(sqlbuilder.ValueFnForward),
		NewParentId(newParentID),
	}

	oldPath := model.Path
	newPath := s.buildPath(parent.Path, id)

	/*
				UPDATE t_tree
		SET Fpath = REPLACE(Fpath, ?, ?)
		WHERE Fpath LIKE CONCAT(?, '%');
	*/
	pathFs := sqlbuilder.Fields{
		NewPath(oldPath).AppendWhereFn(sqlbuilder.ValueFnWhereLikev2(false, true)).Apply(func(f *sqlbuilder.Field, fs ...*sqlbuilder.Field) {
			f.ValueFns.Append(sqlbuilder.ValueFnOnlyForData(func(inputValue any, f *sqlbuilder.Field, fs ...*sqlbuilder.Field) (any, error) {
				val := goqu.L(fmt.Sprintf("REPLACE(%s,?,?)", f.DBColumnName().BaseNameWithQuotes()), oldPath, newPath)
				return val, nil
			}))
		}),
	}

	err = s.repo.Transaction(func(txRepository sqlbuilder.Repository) (err error) {
		err = txRepository.Update(fields)
		if err != nil {
			return err
		}
		err = txRepository.Update(pathFs)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil

}
func (s TreeService) buildPath(parentPath string, id int) string {
	return fmt.Sprintf("%s/%d", parentPath, id)
}

// 查子树：where path like "prefix%"
func (s TreeService) GetSubTree(pathPrefix string, dst any) (err error) {
	fields := sqlbuilder.Fields{
		NewPath(pathPrefix).AppendWhereFn(sqlbuilder.ValueFnWhereLikev2(false, true)),
		NewDeletedAt(),
	}
	err = s.repo.All(dst, fields)
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
		NewId(ids).AppendWhereFn(sqlbuilder.ValueFnForward),
		NewDeletedAt(),
	}
	err = s.repo.All(dst, fields)
	if err != nil {
		return err
	}
	return nil
}
