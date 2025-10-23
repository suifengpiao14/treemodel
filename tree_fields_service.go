package treemodel

import (
	"fmt"

	"github.com/spf13/cast"
	"github.com/suifengpiao14/sqlbuilder"
	"github.com/suifengpiao14/treemodel/field"
)

var DBHandler = sqlbuilder.NewFieldIDBHandler(sqlbuilder.GetDB)

var Table_tree = sqlbuilder.NewTableConfig("t_tree").WithHandler(DBHandler).AddColumns(
	sqlbuilder.NewColumn("Fid", sqlbuilder.GetField(field.NewId[int]).SetModelRequered(true)),
	sqlbuilder.NewColumn("Fparent_id", sqlbuilder.GetField(field.NewParentId).SetModelRequered(true)),
	sqlbuilder.NewColumn("Fpath", sqlbuilder.GetField(field.NewPath)),
	sqlbuilder.NewColumn("Ftitle", sqlbuilder.GetField(field.NewTitle)),
	sqlbuilder.NewColumn("Fdeleted_at", field.NewDeletedAt()),
)

type TreeModel struct {
	Id       int
	ParentId int
	Path     string
	Title    string
}

func (m *TreeModel) Fields() sqlbuilder.Fields {
	return sqlbuilder.Fields{
		sqlbuilder.GetField(field.NewId[int]).BindValue(&m.Id),
		sqlbuilder.GetField(field.NewParentId).BindValue(&m.ParentId),
		sqlbuilder.GetField(field.NewPath).BindValue(&m.Path),
		sqlbuilder.GetField(field.NewTitle).BindValue(&m.Title),
	}
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

type _TreeMiddleware struct {
}

var TreeMiddleware = _TreeMiddleware{}

func (s _TreeMiddleware) Add() sqlbuilder.ModelMiddleware {
	return func(ctx *sqlbuilder.ModelMiddlewareContext, fs *sqlbuilder.Fields) (err error) {
		table := fs.FirstMust().GetTable()
		addNodeIn := &AddNodeIn{}
		err = fs.UmarshalModel(addNodeIn)
		if err != nil {
			return err
		}
		var parent TreeModel
		if addNodeIn.ParentId > 0 {
			parentRef, err := s.getNode(table, addNodeIn.ParentId)
			if err != nil {
				return err
			}
			parent = *parentRef
		}
		err = ctx.Next(fs) //执行下一个中间件

		if err != nil {
			return err
		}

		//写入db后，更新path字段
		if table.Columns.Fields().Contains(sqlbuilder.GetField(field.NewPath)) { // 如果表中有path字段，则需要更新path
			idField, err := fs.GetByNameAsError(sqlbuilder.FieldName_lastInsertId)
			if err != nil {
				return err
			}
			id := cast.ToInt(idField.GetOriginalValue())
			path := s.buildPath(parent.Path, id)
			pathFs := sqlbuilder.Fields{
				field.NewId(id).SetRequired(true).ShieldUpdate(true).AppendWhereFn(sqlbuilder.ValueFnForward), // where id = ?
				field.NewPath(path),
			}
			err = table.Repository().Update(pathFs)
			if err != nil {
				return err
			}
		}
		return nil
	}

}

func (s _TreeMiddleware) getNode(table sqlbuilder.TableConfig, id int) (model *TreeModel, err error) {
	fields := sqlbuilder.Fields{
		field.NewId(id).SetRequired(true).AppendWhereFn(sqlbuilder.ValueFnForward),
		field.NewDeletedAt(),
	}
	model = &TreeModel{}
	err = table.Repository().FirstMustExists(model, fields)
	if err != nil {
		return nil, err
	}
	return model, nil
}

func (s _TreeMiddleware) buildPath(parentPath string, id int) string {
	return fmt.Sprintf("%s/%d", parentPath, id)
}
