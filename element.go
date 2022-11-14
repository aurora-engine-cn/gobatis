package sgo

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/antonmedv/expr"
	"github.com/beevik/etree"
	"reflect"
	"strings"
)

func StatementElement(element *etree.Element, template string, ctx map[string]any) (string, string, []any, error) {
	analysisTemplate, t, param, err := AnalysisTemplate(template, ctx)
	if err != nil {
		return "", "", param, fmt.Errorf("%s,%s,%s", element.Tag, element.SelectAttr("id").Value, err.Error())
	}
	return analysisTemplate, t, param, nil
}

func ForElement(element *etree.Element, template string, ctx map[string]any) (string, string, []any, error) {
	return forElement(element, template, ctx)
}

func IfElement(element *etree.Element, template string, ctx map[string]any) (string, string, []any, error) {
	return ifElement(element, template, ctx)
}

func forElement(element *etree.Element, template string, ctx map[string]any) (string, string, []any, error) {
	var slice, open, closes, column = "", "(", ")", ""
	separator := ","
	var attr *etree.Attr
	buf := bytes.Buffer{}
	templateBuf := bytes.Buffer{}
	params := make([]any, 0)
	if attr = element.SelectAttr("column"); attr != nil {
		column = attr.Value
	}
	if attr = element.SelectAttr("slice"); attr != nil {
		slice = attr.Value
	}
	if attr = element.SelectAttr("open"); attr != nil {
		open = attr.Value
	}
	if attr = element.SelectAttr("close"); attr != nil {
		closes = attr.Value
	}
	attr = element.SelectAttr("separator")
	if element.SelectAttr("separator"); attr != nil {
		separator = attr.Value
	}
	if column != "" {
		buf.WriteString(column + " IN ")
		templateBuf.WriteString(column + " IN ")
	}
	// 上下文中取出 数据
	t := UnTemplate(slice)
	keys := strings.Split(t[1], ".")
	v, err := ctxValue(ctx, keys)
	if err != nil {
		return "", "", nil, err
	}
	valueOf := reflect.ValueOf(v)
	buf.WriteString(open)
	templateBuf.WriteString(open)
	var result, temp string
	var param []any
	// 解析 slice 属性迭代
	combine := Combine{Value: v, Template: template, Separator: separator}
	switch valueOf.Kind() {
	case reflect.Slice, reflect.Array:
		combine.Politic = Slice{}
	case reflect.Struct:
		combine.Politic = Struct{}
	case reflect.Pointer:
		combine.Politic = Pointer{}
	}
	result, temp, param, err = combine.ForEach()
	if err != nil {
		return "", "", nil, err
	}
	params = append(params, param...)
	buf.WriteString(result)
	templateBuf.WriteString(temp)
	buf.WriteString(closes)
	templateBuf.WriteString(closes)
	return buf.String(), templateBuf.String(), params, nil
}

func ifElement(element *etree.Element, template string, ctx map[string]any) (string, string, []any, error) {
	var attr *etree.Attr
	attr = element.SelectAttr("expr")
	if attr == nil {
		return "", "", nil, fmt.Errorf("%s,attr 'expr' not found", element.Tag)
	}
	exprStr := attr.Value
	if exprStr == "" {
		return "", "", nil, fmt.Errorf("%s,attr 'expr' value is empty", element.Tag)
	}
	analysisExpr := AnalysisExpr(exprStr)
	compile, err := expr.Compile(analysisExpr)
	if err != nil {
		return "", "", nil, err
	}
	run, err := expr.Run(compile, ctx)
	if err != nil {
		return "", "", nil, err
	}
	var flag, f bool
	if flag, f = run.(bool); !f {
		return "", "", nil, fmt.Errorf("%s,expr result is not bool type", element.Tag)
	}
	if flag {
		analysisTemplate, t, param, err := AnalysisTemplate(template, ctx)
		if err != nil {
			return "", t, param, fmt.Errorf("%s,template '%s'. %s", element.Tag, template, err.Error())
		}
		return analysisTemplate, t, param, nil
	}
	return "", "", nil, nil
}

// 把 map 或者 结构体完全转化为 map[any]
func toMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	valueOf := reflect.ValueOf(value)
	if valueOf.Kind() != reflect.Map && valueOf.Kind() != reflect.Struct && valueOf.Kind() != reflect.Pointer {
		return map[string]any{}
	}
	if valueOf.Kind() == reflect.Pointer {
		valueOf = valueOf.Elem()
		return toMap(valueOf.Interface())
	}
	ctx := make(map[string]any)
	switch valueOf.Kind() {
	case reflect.Struct:
		structToMap(valueOf, ctx)
	case reflect.Map:
		mapToMap(valueOf, ctx)
	}
	return ctx
}

func structToMap(value reflect.Value, ctx map[string]any) {
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if !value.Type().Field(i).IsExported() {
			continue
		}
		key := value.Type().Field(i).Name
		key = strings.ToLower(key)
		v := field.Interface()
		if dataType(key, v, ctx) {
			continue
		}
		if field.Kind() == reflect.Slice {
			v = filedToMap(v)
		}
		if field.Kind() == reflect.Struct || field.Kind() == reflect.Pointer || field.Kind() == reflect.Map {
			v = toMap(v)
		}
		ctx[key] = v
	}
}

func mapToMap(value reflect.Value, ctx map[string]any) {
	mapIter := value.MapRange()
	for mapIter.Next() {
		key := mapIter.Key().Interface().(string)
		vOf := mapIter.Value()
		v := vOf.Interface()
		if vOf.Kind() == reflect.Interface {
			if vOf.Elem().Kind() == reflect.Slice {
				if vOf.Elem().Type().Elem().Kind() == reflect.Struct || vOf.Elem().Type().Elem().Kind() == reflect.Pointer || vOf.Elem().Type().Elem().Kind() == reflect.Map {
					v = filedToMap(v)
				}
			}
			if vOf.Elem().Kind() == reflect.Struct || vOf.Elem().Kind() == reflect.Map || vOf.Elem().Kind() == reflect.Pointer {
				v = toMap(v)
			}
		}
		if dataType(key, v, ctx) {
			continue
		}
		if vOf.Kind() == reflect.Slice {
			v = filedToMap(v)
		}
		if vOf.Kind() == reflect.Struct || vOf.Kind() == reflect.Map || vOf.Kind() == reflect.Pointer {
			v = toMap(v)
		}
		ctx[key] = v
	}
}

func filedToMap(value any) []map[string]any {
	valueOf := reflect.ValueOf(value)
	elem := valueOf.Type().Elem()
	arr := make([]map[string]any, 0)
	length := valueOf.Len()
	switch elem.Kind() {
	case reflect.Struct, reflect.Pointer:
		for i := 0; i < length; i++ {
			val := valueOf.Index(i)
			m := toMap(val.Interface())
			arr = append(arr, m)
		}
	case reflect.Map:
		for i := 0; i < length; i++ {
			val := valueOf.Index(i)
			iter := val.MapRange()
			m := map[string]any{}
			for iter.Next() {
				key := iter.Key().Interface().(string)
				v := iter.Value()
				var vals any
				if v.Kind() == reflect.Slice {
					vals = filedToMap(v.Interface())
				}
				if v.Kind() == reflect.Struct || v.Kind() == reflect.Pointer || v.Kind() == reflect.Map {
					vals = toMap(v.Interface())
				}
				m[key] = vals
				arr = append(arr, m)
			}
		}
	}
	return arr
}

// 校验复杂数据类型，不是复杂数据类型返回 false 让主程序继续处理，如果是复杂数据类型，应该直接添加到ctx，并返回true
func dataType(key string, value any, ctx map[string]any) bool {
	// TODO
	return false
}

// 模板解析处理复杂数据类型
func dataHandle(value any) string {
	// TODO
	return ""
}

// UnTemplate 解析 {xx} 模板 解析为三个部分 ["{","xx","}"]
func UnTemplate(template string) []string {
	length := len(template)
	return []string{template[0:1], template[1 : length-1], template[length-1:]}
}

// AnalysisExpr 翻译表达式
func AnalysisExpr(template string) string {
	buf := bytes.Buffer{}
	template = strings.TrimSpace(template)
	templateByte := []byte(template)
	starIndex := 0
	for i := starIndex; i < len(templateByte); {
		if templateByte[i] == '{' {
			starIndex = i
			endIndex := i
			for j := starIndex; j < len(templateByte); j++ {
				if templateByte[j] == '}' {
					endIndex = j
					break
				}
			}
			s := template[starIndex+1 : endIndex]
			buf.WriteString(" " + s + " ")
			i = endIndex + 1
			continue
		}
		buf.WriteByte(templateByte[i])
		i++
	}
	return buf.String()
}

// AnalysisTemplate 模板解析器
func AnalysisTemplate(template string, ctx map[string]any) (string, string, []any, error) {
	params := []any{}
	buf := bytes.Buffer{}
	templateBuf := bytes.Buffer{}
	template = strings.TrimSpace(template)
	templateByte := []byte(template)
	starIndex := 0
	for i := starIndex; i < len(templateByte); {
		if templateByte[i] == '{' {
			starIndex = i
			endIndex := i
			for j := starIndex; j < len(templateByte); j++ {
				if templateByte[j] == '}' {
					endIndex = j
					break
				}
			}
			s := template[starIndex+1 : endIndex]
			split := strings.Split(s, ".")
			value, err := ctxValue(ctx, split)
			if err != nil {
				return "", "", params, fmt.Errorf("%s,'%s' not found", template, s)
			}
			switch value.(type) {
			case string:
				buf.WriteString(fmt.Sprintf(" '%s' ", value.(string)))
				templateBuf.WriteString("?")
				params = append(params, value)
			case int:
				buf.WriteString(fmt.Sprintf(" %d ", value.(int)))
				templateBuf.WriteString("?")
				params = append(params, value)
			case float64:
				buf.WriteString(fmt.Sprintf(" %f ", value.(float64)))
				templateBuf.WriteString("?")
				params = append(params, value)
			default:
				// 其他复杂数据类型
				if handle := dataHandle(value); handle != "" {
					buf.WriteString(" " + handle + " ")
					templateBuf.WriteString(" " + handle + " ")
					params = append(params, handle)
				}
			}
			i = endIndex + 1
			continue
		}
		buf.WriteByte(templateByte[i])
		templateBuf.WriteByte(templateByte[i])
		i++
	}
	return buf.String(), templateBuf.String(), params, nil
}

// 上下文中取数据
func ctxValue(ctx map[string]any, keys []string) (any, error) {
	if ctx == nil {
		return nil, errors.New("ctx is nil")
	}
	kl := len(keys)
	var v any
	b := false
	for i := 0; i < kl; i++ {
		k := keys[i]
		if i == kl-1 {
			if v, b = ctx[k]; !b {
				return nil, fmt.Errorf("'slice' key %s not find ", k)
			}
		} else {
			if v, b = ctx[k]; !b {
				return nil, fmt.Errorf("'slice' key %s not find ", k)
			}
			if ctx, b = v.(map[string]any); !b {
				return nil, fmt.Errorf("'%s' is not map or struct", k)
			}
		}
	}
	return v, nil
}
