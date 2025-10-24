package treemodel

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cast"
	"github.com/suifengpiao14/sqlbuilder"
	"github.com/suifengpiao14/treemodel/field"
	"gitlab.huishoubao.com/gopackage/treeflat"
)

type _TreeModel struct {
	Id       int
	ParentId int
	Path     string //数据库路径
	calPath  string //程序中计算出的路径
	children []*_TreeModel
}

func (m *_TreeModel) Fields() sqlbuilder.Fields {
	return sqlbuilder.Fields{
		sqlbuilder.GetField(field.NewId).BindValue(&m.Id),
		sqlbuilder.GetField(field.NewParentId).BindValue(&m.ParentId),
		sqlbuilder.GetField(field.NewPath).BindValue(&m.Path),
	}
}

func (m *_TreeModel) GetID() string {
	return cast.ToString(m.Id)
}
func (m *_TreeModel) GetParentID() string {
	return cast.ToString(m.ParentId)
}
func (m *_TreeModel) GetChildren() []*_TreeModel {
	return m.children
}
func (m *_TreeModel) SetChildren(children []*_TreeModel) {
	m.children = children
}

func (m *_TreeModel) SetPath(path string) {
	m.calPath = path //先设置到程序中计算的路径
}

type TreeMiddleware struct {
}

var _DBHandler = sqlbuilder.NewFieldIDBHandler(sqlbuilder.GetDB)

func (s TreeMiddleware) GetTable() sqlbuilder.TableConfig {
	var tableName = "t_tree"
	var topic = fmt.Sprintf(`%s_be6097ae461af2796fc9c3c0bd1cd370`, tableName)
	tableTree := sqlbuilder.NewTableConfig(tableName).WithHandler(_DBHandler).AddColumns(
		sqlbuilder.NewColumn("Fid", sqlbuilder.GetField(field.NewId).SetModelRequered(true)),
		sqlbuilder.NewColumn("Fparent_id", sqlbuilder.GetField(field.NewParentId).SetModelRequered(true)),
		sqlbuilder.NewColumn("Fpath", sqlbuilder.GetField(field.NewPath)),
	)

	tableTree.WithTopic(topic).WithConsumerMakers(func(table sqlbuilder.TableConfig) (consumer sqlbuilder.Consumer) {
		publishTable := tableTree.WithHandler(table.GetHandler()).WithTableName(table.Name) //因为订阅自身模型事件，所以这里从自身运行时的表中获取 handler和table.Name
		return sqlbuilder.MakeIdentityEventSubscriber(publishTable, func(model _TreeModel) (err error) {
			err = s.fillPath(table, model.Id)
			if err != nil {
				return err
			}
			return nil
		})
	})

	return tableTree
}

func (s TreeMiddleware) Insert() sqlbuilder.ModelMiddleware {
	return func(ctx *sqlbuilder.ModelMiddlewareContext, fs *sqlbuilder.Fields) (err error) {
		table := fs.FirstMust().GetTable()

		parentIdField, err := fs.GetByNameAsError(sqlbuilder.GetFieldName(field.NewParentId))
		if err != nil {
			return err
		}

		parentId := cast.ToInt(parentIdField.GetOriginalValue())
		if parentId > 0 {
			_, err := s.getNode(table, parentId)
			if err != nil {
				return err
			}
		}

		err = ctx.Next(fs) //执行下一个中间件

		if err != nil {
			return err
		}
		idField, err := fs.GetByNameAsError(sqlbuilder.FieldName_lastInsertId)
		if err != nil {
			return err
		}
		id := cast.ToInt(idField.GetOriginalValue())
		err = s.publishEvent(id, sqlbuilder.Event_Operation_Insert)
		if err != nil {
			return err
		}
		return nil
	}
}

func (s TreeMiddleware) publishEvent(id int, operation string) (err error) {
	event := sqlbuilder.IdentityEvent{
		Operation:         operation,
		IdentityValue:     cast.ToString(id),
		IdentityFieldName: sqlbuilder.GetFieldName(field.NewId),
	}
	err = s.GetTable().Publish(event) //固定使用Table_tree 表名发布事件,避免多model中间件发布重复事件,避免重复消费事件
	if err != nil {
		return err
	}
	return nil
}

func (s TreeMiddleware) MoveNode() sqlbuilder.ModelMiddleware {
	return func(ctx *sqlbuilder.ModelMiddlewareContext, fs *sqlbuilder.Fields) (err error) {
		table := fs.FirstMust().GetTable()
		idField, err := fs.GetByNameAsError(sqlbuilder.GetFieldName(field.NewId))
		if err != nil {
			return err
		}
		id := cast.ToInt(idField.GetOriginalValue())
		if id == 0 {
			return fmt.Errorf("id is required")
		}
		parentId := 0
		parentIdField, exists := fs.GetByName(sqlbuilder.GetFieldName(field.NewParentId))
		if exists {
			parentIdAny, err := parentIdField.GetValue(sqlbuilder.Layer_all, *fs...) // 需要排除where部分
			if err != nil {
				if errors.Is(err, sqlbuilder.ErrValueNil) { //如果为空，则不校验父节点是否存在
					err = nil
				} else {
					return err
				}
			}
			parentId = cast.ToInt(parentIdAny)
			if parentId > 0 {
				_, err := s.getNode(table, parentId)
				if err != nil {
					return err
				}
			}
		}

		err = ctx.Next(fs) //执行下一个中间件
		if err != nil {
			return err
		}

		if parentId > 0 { //如果父节点有变化，则需要重新计算路径
			err = s.publishEvent(id, sqlbuilder.Event_Operation_Update)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func (s TreeMiddleware) GetSubTree() sqlbuilder.ModelMiddleware {
	return func(ctx *sqlbuilder.ModelMiddlewareContext, fs *sqlbuilder.Fields) (err error) {
		pathField, err := fs.GetByNameAsError(sqlbuilder.GetFieldName(field.NewPath))
		if err != nil {
			return err
		}
		//给 path 增加where条件
		pathField.SetRequired(true).SetAllowZero(true).AppendWhereFn(sqlbuilder.ValueFnWhereLikev2(false, true))
		err = ctx.Next(fs) //执行下一个中间件
		if err != nil {
			return err
		}
		return nil
	}
}

func (s TreeMiddleware) GetAncestors() sqlbuilder.ModelMiddleware {
	return func(ctx *sqlbuilder.ModelMiddlewareContext, fs *sqlbuilder.Fields) (err error) {
		pathField, err := fs.GetByNameAsError(sqlbuilder.GetFieldName(field.NewPath))
		if err != nil {
			return err
		}
		// 将path 字段转为 ids 数组，做为where条件
		path := cast.ToString(pathField.GetOriginalValue())
		pathField.AppendValueFn(sqlbuilder.ValueFnSetFormat(nil)) // 屏蔽path字段
		if path == "" {
			return nil
		}
		ids := s.splitPath(path)
		*fs = append(*fs, field.NewId(0).SetValue(ids).SetRequired(true).AppendWhereFn(sqlbuilder.ValueFnForward))
		err = ctx.Next(fs) //执行下一个中间件
		if err != nil {
			return err
		}

		return nil
	}
}

func (s TreeMiddleware) getNode(table sqlbuilder.TableConfig, id int) (model *_TreeModel, err error) {
	fields := sqlbuilder.Fields{
		field.NewId(id).SetRequired(true).AppendWhereFn(sqlbuilder.ValueFnForward),
	}
	model = &_TreeModel{}
	err = table.Repository().FirstMustExists(model, fields)
	if err != nil {
		return nil, err
	}
	return model, nil
}

var (
	PathDelimiter = "/"
)

func (s TreeMiddleware) buildPath(parentPath string, id int) string {
	return fmt.Sprintf("%s%s%d", parentPath, PathDelimiter, id)
}
func (s TreeMiddleware) splitPath(parentPath string) []int {
	ids := make([]int, 0)
	for _, idStr := range strings.Split(parentPath, PathDelimiter) {
		if idStr != "" {
			ids = append(ids, cast.ToInt(idStr))
		}
	}
	return ids
}

// fillPath 更新path字段
func (s TreeMiddleware) fillPath(table sqlbuilder.TableConfig, id int) (err error) {
	oldNode, err := s.getNode(table, id)
	if err != nil {
		return err
	}
	oldPath := oldNode.Path
	var parent = &_TreeModel{}
	if oldNode.ParentId > 0 {
		parent, err = s.getNode(table, oldNode.ParentId)
		if err != nil {
			return err
		}
	}
	newPath := s.buildPath(parent.Path, id)
	if newPath == oldPath {
		return nil
	}
	err = s.reBuildPath(table, id, newPath, oldPath)
	if err != nil {
		return err
	}

	//检测parentPath 准确性，发现异常，则修正
	ancestors, err := s.getAncestors(table, oldNode.Path)
	if err != nil {
		return err
	}

	trees := treeflat.BuildTree(ancestors) //计算path
	ancestors = treeflat.FlattenTree(trees)
	for _, ancestor := range ancestors {
		if ancestor.Path != ancestor.calPath {
			err = s.reBuildPath(table, ancestor.Id, ancestor.calPath, ancestor.Path)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s TreeMiddleware) reBuildPath(table sqlbuilder.TableConfig, id int, newPath string, oldPath string) (err error) {
	pathFs := sqlbuilder.Fields{
		field.NewId(id).SetRequired(true).ShieldUpdate(true).AppendWhereFn(sqlbuilder.ValueFnForward),
		field.NewPath(newPath),
	}
	err = table.Repository().Update(pathFs)
	if err != nil {
		return err
	}

	if oldPath != "" {
		// 更新子节点的path
		pathFs := sqlbuilder.Fields{
			field.NewId(id).SetRequired(true).ShieldUpdate(true).AppendWhereFn(sqlbuilder.ValueFnForward),
			field.NewPath(newPath).AppendWhereFn(sqlbuilder.ValueFnWhereLikev2(false, true)).Apply(func(f *sqlbuilder.Field, fs ...*sqlbuilder.Field) {
				f.ValueFns.Append(sqlbuilder.ValueFnOnlyForData(sqlbuilder.ValueFnReplace(oldPath)))
			}),
		}
		err = table.Repository().Update(pathFs)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s TreeMiddleware) getAncestors(table sqlbuilder.TableConfig, path string) (models []*_TreeModel, err error) {
	ids := s.splitPath(path)
	if len(ids) == 0 {
		return nil, nil
	}
	fields := sqlbuilder.Fields{
		field.NewId(0).SetValue(ids).SetRequired(true).SetAllowZero(true).AppendWhereFn(sqlbuilder.ValueFnForward),
		field.NewDeletedAt(),
	}
	err = table.Repository().All(&models, fields)
	if err != nil {
		return nil, err
	}
	return nil, nil
}
