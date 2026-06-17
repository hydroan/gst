package reflectmeta

import (
	"reflect"
	"sync"
)

var (
	metaCache        sync.Map // map[string]*StructMeta
	methodCache      sync.Map // map[methodCacheKey]*reflect.Method
	methodParamCache sync.Map // map[methodParamCacheKey]reflect.Type
)

type StructMeta struct {
	Type      reflect.Type
	represent string

	allFields   []reflect.StructField // All fields including anonymous sub-structs.
	fields      []reflect.StructField // Fields excluding anonymous sub-structs
	AllFieldMap map[string]int        // All fields including anonymous sub-structs, key is field name, value is index
	FieldMap    map[string]int        // Fields excluding anonymous sub-structs, key is field name, value is index

	numField int

	allJSONTags   []string
	allSchemaTags []string
	allGormTags   []string
	allQueryTags  []string
	allURLTags    []string

	jsonTags   []string
	schemaTags []string
	gormTags   []string
	queryTags  []string
	urlTags    []string

	FieldIndexes [][]int // 每个字段的 Index 路径（支持嵌套匿名字段）
}

func GetStructMeta(t reflect.Type) *StructMeta {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	key := t.PkgPath() + "|" + t.String()
	if meta, ok := metaCache.Load(key); ok {
		return meta.(*StructMeta) //nolint:errcheck
	}

	fieldCount := t.NumField()
	allFields := make([]reflect.StructField, 0, fieldCount)
	fields := make([]reflect.StructField, 0, fieldCount)
	allFieldMap := make(map[string]int)
	fieldMap := make(map[string]int)

	allJSONTags := make([]string, 0, fieldCount)
	allSchemaTags := make([]string, 0, fieldCount)
	allGormTags := make([]string, 0, fieldCount)
	allQueryTags := make([]string, 0, fieldCount)
	allURLTags := make([]string, 0, fieldCount)

	jsonTags := make([]string, 0, fieldCount)
	schemaTags := make([]string, 0, fieldCount)
	gormTags := make([]string, 0, fieldCount)
	queryTags := make([]string, 0, fieldCount)
	urlTags := make([]string, 0, fieldCount)

	allFieldIndexes := make([][]int, 0, fieldCount)

	var parseFields func(reflect.Type, []int, bool)
	parseFields = func(rt reflect.Type, parentIndex []int, isTopLevel bool) {
		for i := range rt.NumField() {
			field := rt.Field(i)

			indexPath := append(parentIndex, i)

			allFields = append(allFields, field)
			allFieldMap[field.Name] = len(allFields) - 1
			allFieldIndexes = append(allFieldIndexes, indexPath)
			allJSONTags = append(allJSONTags, field.Tag.Get("json"))
			allSchemaTags = append(allSchemaTags, field.Tag.Get("schema"))
			allGormTags = append(allGormTags, field.Tag.Get("gorm"))
			allQueryTags = append(allQueryTags, field.Tag.Get("query"))
			allURLTags = append(allURLTags, field.Tag.Get("url"))

			if isTopLevel {
				fields = append(fields, field)
			}
			if isTopLevel {
				fieldMap[field.Name] = len(fields) - 1
			}
			if isTopLevel {
				jsonTags = append(jsonTags, field.Tag.Get("json"))
				schemaTags = append(schemaTags, field.Tag.Get("schema"))
				gormTags = append(gormTags, field.Tag.Get("gorm"))
				queryTags = append(queryTags, field.Tag.Get("query"))
				urlTags = append(urlTags, field.Tag.Get("url"))
			}

			if field.Anonymous && field.Type.Kind() == reflect.Struct {
				parseFields(field.Type, indexPath, false)
				continue
			}
		}
	}

	parseFields(t, []int{}, true)

	meta := &StructMeta{
		Type:      t,
		represent: t.String(),
		numField:  fieldCount,

		allFields: allFields,
		fields:    fields,

		AllFieldMap: allFieldMap,
		FieldMap:    fieldMap,

		FieldIndexes: allFieldIndexes,

		allJSONTags:   allJSONTags,
		allSchemaTags: allSchemaTags,
		allGormTags:   allGormTags,
		allQueryTags:  allQueryTags,
		allURLTags:    allURLTags,

		jsonTags:   jsonTags,
		schemaTags: schemaTags,
		gormTags:   gormTags,
		queryTags:  queryTags,
		urlTags:    urlTags,
	}
	metaCache.Store(key, meta)
	return meta
}

// NumField returns the number of fields.
func (m *StructMeta) NumField() int { return m.numField }

// Field returns the StructField at index i
func (m *StructMeta) Field(i int) reflect.StructField { return m.fields[i] }

func (m *StructMeta) JSONTag(i int) string   { return m.jsonTags[i] }
func (m *StructMeta) SchemaTag(i int) string { return m.schemaTags[i] }
func (m *StructMeta) GormTag(i int) string   { return m.gormTags[i] }
func (m *StructMeta) QueryTag(i int) string  { return m.queryTags[i] }
func (m *StructMeta) URLTag(i int) string    { return m.urlTags[i] }

func (m *StructMeta) String() string { return m.represent }

type methodCacheKey struct {
	TypeName   string
	MethodName string
}

func GetCachedMethod(typ reflect.Type, methodName string) (reflect.Method, bool) {
	key := methodCacheKey{TypeName: typ.PkgPath() + "|" + typ.String(), MethodName: methodName}
	if m, ok := methodCache.Load(key); ok {
		return m.(reflect.Method), true //nolint:errcheck
	}
	method, ok := typ.MethodByName(methodName)
	if ok {
		methodCache.Store(key, method)
	}
	return method, ok
}

type methodParamCacheKey struct {
	TypeName   string
	MethodName string
	ParamIndex int
}

func GetCachedMethodParamType(typ reflect.Type, methodName string, idx int) (reflect.Type, bool) {
	key := methodParamCacheKey{TypeName: typ.PkgPath() + "|" + typ.String(), MethodName: methodName, ParamIndex: idx}
	if t, ok := methodParamCache.Load(key); ok {
		return t.(reflect.Type), true //nolint:errcheck
	}
	method, ok := GetCachedMethod(typ, methodName)
	if !ok {
		return nil, false
	}
	if method.Type.NumIn() <= idx {
		return nil, false
	}
	paramType := method.Type.In(idx)
	methodParamCache.Store(key, paramType)
	return paramType, true
}

func GetTypeOfModel[M any]() reflect.Type {
	typ := reflect.TypeFor[M]()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}

// // FieldByName returns the StructField and true if field with given name existssj
// func (m *StructMeta) FieldByName(name string) (reflect.StructField, bool) {
// 	if idx, ok := m.FieldMap[name]; ok {
// 		return m.fields[idx], true
// 	}
// 	return reflect.StructField{}, false
// }

// // FieldIndexByName returns the field index path for the given name.
// func (m *StructMeta) FieldIndexByName(name string) ([]int, bool) {
// 	if idx, ok := m.FieldMap[name]; ok {
// 		return m.FieldIndexes[idx], true
// 	}
// 	return nil, false
// }

// // FieldByTag returns the first StructField and true that matches the tagName:tagValue.
// func (m *StructMeta) FieldByTag(tagName, tagValue string) (reflect.StructField, bool) {
// 	var tagSlice []string
// 	switch tagName {
// 	case "json":
// 		tagSlice = m.allJSONTags
// 	case "schema":
// 		tagSlice = m.allSchemaTags
// 	case "gorm":
// 		tagSlice = m.allGormTags
// 	case "query":
// 		tagSlice = m.allQueryTags
// 	case "url":
// 		tagSlice = m.allUrlTags
// 	default:
// 		return reflect.StructField{}, false
// 	}
// 	for i, v := range tagSlice {
// 		if v == tagValue {
// 			return m.allFields[i], true
// 		}
// 	}
// 	return reflect.StructField{}, false
// }

// // FieldIndexByTag returns the index path for the first field that matches tagName:tagValue.
// func (m *StructMeta) FieldIndexByTag(tagName, tagValue string) ([]int, bool) {
// 	var tagSlice []string
// 	switch tagName {
// 	case "json":
// 		tagSlice = m.allJSONTags
// 	case "schema":
// 		tagSlice = m.allSchemaTags
// 	case "gorm":
// 		tagSlice = m.allGormTags
// 	case "query":
// 		tagSlice = m.allQueryTags
// 	case "url":
// 		tagSlice = m.allUrlTags
// 	default:
// 		return nil, false
// 	}
// 	for i, v := range tagSlice {
// 		if v == tagValue {
// 			return m.FieldIndexes[i], true
// 		}
// 	}
// 	return nil, false
// }

// func (m *StructMeta) AllJsonTag(i int) string   { return m.allJSONTags[i] }
// func (m *StructMeta) AllSchemaTag(i int) string { return m.allSchemaTags[i] }
// func (m *StructMeta) AllGormTag(i int) string   { return m.allGormTags[i] }
// func (m *StructMeta) AllQueryTag(i int) string  { return m.allQueryTags[i] }
// func (m *StructMeta) AllUrlTag(i int) string    { return m.allUrlTags[i] }
