package sgo

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/beevik/etree"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

var banner = " __ __ _  \n(_ /__/ \\ \n__)\\_|\\_/ \n"

func New(db *sql.DB) *Build {
	if db == nil {
		panic("db nil")
	}
	return &Build{
		DB:         db,
		db:         reflect.ValueOf(db),
		NameSpaces: map[string]*Sql{},
	}
}

type Build struct {
	// DB 用于执行 sql 语句
	DB *sql.DB
	db reflect.Value
	// SqlSource 用于保存 xml 配置的文件的根路径配置信息，Build会通过SqlSource属性去加载 xml 文件
	SqlSource string
	// NameSpaces 保存了每个 xml 配置的根元素构建出来的 Sql 对象
	NameSpaces map[string]*Sql
}

func (build *Build) Source(source string) {
	if source != "" {
		build.SqlSource = source
	}
	getwd, err := os.Getwd()
	if err != nil {
		return
	}
	root := filepath.Join(getwd, build.SqlSource)
	fmt.Print(banner)
	// 解析 xml
	filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if strings.HasSuffix(path, ".xml") {
			document := etree.NewDocument()
			err = document.ReadFromFile(path)
			if err != nil {
				return err
			}
			element := document.Root()
			attr := element.SelectAttr("namespace")
			s := NewSql(element)
			s.LoadSqlElement()
			build.NameSpaces[attr.Value] = s
			Info("load mapper file path:[" + path + "]")
		}
		return nil
	})
}

// ScanMappers 扫描解析
func (build *Build) ScanMappers(mappers ...any) {
	Info("Start scanning the mapper mapping function")
	for i := 0; i < len(mappers); i++ {
		mapper := mappers[i]
		vf := reflect.ValueOf(mapper)
		if vf.Kind() != reflect.Pointer {
			panic("")
		}
		if vf.Elem().Kind() != reflect.Struct {
			panic("")
		}
		vf = vf.Elem()
		namespace := vf.Type().String()
		namespace = Namespace(namespace)
		Info("Starts loading the '" + namespace + "' mapping resolution")
		for j := 0; j < vf.NumField(); j++ {
			key := make([]string, 0)
			key = append(key, namespace)
			structField := vf.Type().Field(j)
			field := vf.Field(j)
			if !structField.IsExported() || structField.Type.Kind() != reflect.Func {
				continue
			}
			// mapper 函数校验规范
			if flag, err := MapperCheck(field); !flag {
				Panic(namespace+"."+structField.Name, ",", err.Error())
			}
			key = append(key, structField.Name)
			build.initMapper(key, field)
			Info(namespace + "." + structField.Name)
		}
	}
}

func (build *Build) Sql(id string, value any) (string, error) {
	ids := strings.Split(id, ".")
	if len(ids) != 2 {
		return "", errors.New("id error")
	}
	ctx := toMap(value)
	if sql, b := build.NameSpaces[ids[0]]; b {
		if element, f := sql.Statement[ids[1]]; f {
			analysis, _, err := Analysis(element, ctx)
			if err != nil {
				return "", err
			}
			join := strings.Join(analysis, " ")
			return join, nil
		}
	}
	return "", nil
}

func (build *Build) Get(id []string, value any) (string, string, error) {
	if len(id) != 2 {
		return "", "", errors.New("id error")
	}
	ctx := toMap(value)
	if sql, b := build.NameSpaces[id[0]]; b {
		if element, f := sql.Statement[id[1]]; f {
			analysis, tag, err := Analysis(element, ctx)
			if err != nil {
				return "", "", err
			}
			join := strings.Join(analysis, " ")
			return join, tag, nil
		}
	}
	return "", "", fmt.Errorf("not found sql statement element")
}

// Analysis 解析xml标签
func Analysis(element *etree.Element, ctx map[string]any) ([]string, string, error) {
	var err error
	sql := []string{}
	// 解析根标签 开始之后的文本
	sqlStar := element.Text()
	// 处理字符串前后空格
	sqlStar = strings.TrimSpace(sqlStar)
	//更具标签类型，对应解析字符串
	sqlStar, err = Element(element, sqlStar, ctx)
	if err != nil {
		return nil, "", err
	}
	sql = append(sql, sqlStar)
	// if 标签解析 逻辑不通过
	if sqlStar != "" && err == nil {
		// 解析子标签内容
		child := element.ChildElements()
		for _, childElement := range child {
			analysis, _, err := Analysis(childElement, ctx)
			if err != nil {
				return nil, "", fmt.Errorf("%s -> %s error,%s", element.Tag, childElement.Tag, err.Error())
			}
			sql = append(sql, analysis...)
		}
	}
	endSql := element.Tail()
	endSql = strings.TrimSpace(endSql)
	if endSql != "" {
		endSql, err = Element(element.Parent(), endSql, ctx)
		if err != nil {
			return nil, "", err
		}
		sql = append(sql, endSql)
	}
	return sql, element.Tag, nil
}

func Element(element *etree.Element, template string, ctx map[string]any) (string, error) {
	// 检擦 节点标签类型
	tag := element.Tag
	switch tag {
	case For:
		return ForElement(element, template, ctx)
	case If:
		return IfElement(element, template, ctx)
	case Select, Update, Delete, Insert:
		return StatementElement(element, template, ctx)
	case Mapper:
		// 对根标签不做任何处理
		return "", nil
	}
	return "", errors.New("error")
}

func Namespace(namespace string) string {
	if index := strings.LastIndex(namespace, "."); index != -1 {
		return namespace[index+1:]
	}
	return namespace
}

// MapperCheck 检查 不同类别的sql标签 Mapper 函数是否符合规范
// 规则: 入参只能有一个并且只能是 map 或者 结构体，对返回值最后一个参数必须是error
func MapperCheck(fun reflect.Value) (bool, error) {
	// 只能有一个入参
	if fun.Type().NumIn() != 1 {
		return false, errors.New("there can only be one argument")
	}

	// 至少有一个返回值
	if fun.Type().NumOut() < 1 {
		return false, errors.New("at least one return value is required")
	}

	// 只有一个参数接收时候，只能是 结果集对应类型
	if fun.Type().NumOut() == 1 {
		err := fun.Type().Out(0)
		if !err.Implements(reflect.TypeOf(new(error)).Elem()) {
			return false, errors.New("the second return value must be error")
		}
	}

	// 多个参数接收时候，最后一个返回值只能是 error
	if fun.Type().NumOut() > 2 {
		// 校验最后一个参数必须是 error
		err := fun.Type().Out(fun.Type().NumOut() - 1)
		if !err.Implements(reflect.TypeOf(new(error)).Elem()) {
			return false, errors.New("the second return value must be error")
		}
	}
	return true, nil
}
