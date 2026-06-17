package reflectmeta_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/reflectmeta"
)

type Base struct {
	ID        string     `json:"id" gorm:"primaryKey" schema:"id" url:"-"`
	CreatedBy string     `json:"created_by,omitempty" gorm:"index" schema:"created_by" url:"-"`
	UpdatedBy string     `json:"updated_by,omitempty" gorm:"index" schema:"updated_by" url:"-"`
	CreatedAt *time.Time `json:"created_at,omitempty" gorm:"index" schema:"-" url:"-"`
	UpdatedAt *time.Time `json:"updated_at,omitempty" gorm:"index" schema:"-" url:"-"`
	Remark    *string    `json:"remark,omitempty" gorm:"size:10240" schema:"-" url:"-"`
	Order     *uint      `json:"order,omitempty" schema:"-" url:"-"`
}

type User struct {
	Base
	Name  string `json:"name"`
	Email string `schema:"email_address"`
	Age   int
}

// func TestGetStructMeta(t *testing.T) {
// 	meta := reflectmeta.GetStructMeta(reflect.TypeOf(User{}))
//
// 	if meta.Type.Name() != "User" {
// 		t.Errorf("expected type name 'User', got: %s", meta.Type.Name())
// 	}
//
// 	// expectJSON := []string{
// 	// 	"id", "created_by,omitempty", "updated_by,omitempty", "created_at,omitempty", "updated_at,omitempty", "remark,omitempty", "order,omitempty",
// 	// 	"name", "", // Email field no json tag
// 	// }
// 	// expectSchema := []string{
// 	// 	"id", "created_by", "updated_by", "-", "-", "-", "-",
// 	// 	"", "email_address", // Name field no schema tag
// 	// }
//
// 	// fmt.Println(len(meta.JSONNames), len(expectJSON))
// 	// fmt.Println(meta.JSONNames)
// 	// fmt.Println(expectJSON)
//
// 	// for i := range expectJSON {
// 	// 	if meta.AllJsonTag(i) != expectJSON[i] {
// 	// 		t.Errorf("json tag mismatch at index %d: expected %q, got %q", i, expectJSON[i], meta.JsonTag(i))
// 	// 	}
// 	// 	if meta.AllSchemaTag(i) != expectSchema[i] {
// 	// 		t.Errorf("schema tag mismatch at index %d: expected %q, got %q", i, expectSchema[i], meta.SchemaTag(i))
// 	// 	}
// 	// }
//
// 	// if idx, ok := meta.AllFieldMap["Email"]; !ok || idx != 8 {
// 	// 	t.Errorf("expected Email at index 8 in FieldMap, got index: %d, exists: %v", idx, ok)
// 	// }
// }

func TestUserGetStructMeta(t *testing.T) {
	meta := reflectmeta.GetStructMeta(reflect.TypeFor[User]())
	typ := reflect.TypeFor[User]()
	test(t, meta, typ)

	meta2 := reflectmeta.GetStructMeta(reflect.TypeFor[*User]())
	typ2 := reflect.TypeFor[*User]()
	test(t, meta2, typ2)
}

func test(t *testing.T, meta *reflectmeta.StructMeta, typ reflect.Type) {
	t.Helper()
	t.Run("NumField", func(t *testing.T) {
		if typ.NumField() != meta.NumField() {
			t.Fatalf("NumField got %d, want %d", meta.NumField(), typ.NumField())
		}
	})

	t.Run("Field", func(t *testing.T) {
		for i := range typ.NumField() {
			if typ.Field(i).Name != meta.Field(i).Name {
				t.Errorf("Field(%d).Name = %s, want %s", i, meta.Field(i).Name, typ.Field(i).Name)
			}
		}
	})
	t.Run("Tag", func(t *testing.T) {
		for i := range typ.NumField() {
			if typ.Field(i).Tag.Get("json") != meta.JSONTag(i) {
				t.Errorf("JsonTag(%d) = %s, want %s", i, meta.JSONTag(i), typ.Field(i).Tag.Get("json"))
			}
			if typ.Field(i).Tag.Get("schema") != meta.SchemaTag(i) {
				t.Errorf("SchemaTag(%d) = %s, want %s", i, meta.SchemaTag(i), typ.Field(i).Tag.Get("schema"))
			}
			if typ.Field(i).Tag.Get("gorm") != meta.GormTag(i) {
				t.Errorf("GormTag(%d) = %s, want %s", i, meta.GormTag(i), typ.Field(i).Tag.Get("gorm"))
			}
			if typ.Field(i).Tag.Get("url") != meta.URLTag(i) {
				t.Errorf("UrlTag(%d) = %s, want %s", i, meta.URLTag(i), typ.Field(i).Tag.Get("url"))
			}
			if typ.Field(i).Tag.Get("query") != meta.QueryTag(i) {
				t.Errorf("QueryTag(%d) = %s, want %s", i, meta.QueryTag(i), typ.Field(i).Tag.Get("query"))
			}
		}
	})
}

// func TestUserStructMeta_FieldByName(t *testing.T) {
// 	meta := reflectmeta.GetStructMeta(reflect.TypeOf(User{}))
// 	// Base 字段
// 	sf, ok := meta.FieldByName("ID")
// 	if !ok || sf.Name != "ID" {
// 		t.Errorf("FieldByName('ID') failed, got %+v", sf)
// 	}
// 	// User 字段
// 	sf, ok = meta.FieldByName("Age")
// 	if !ok || sf.Name != "Age" {
// 		t.Errorf("FieldByName('Age') failed, got %+v", sf)
// 	}
// }

// func TestUserStructMeta_FieldIndexByName(t *testing.T) {
// 	meta := reflectmeta.GetStructMeta(reflect.TypeOf(User{}))
// 	idx, ok := meta.FieldIndexByName("CreatedBy")
// 	if !ok {
// 		t.Fatalf("FieldIndexByName('CreatedBy') not found")
// 	}
// 	// CreatedBy 在 Base 匿名字段，第1个字段，index 应为 [0,1]
// 	if len(idx) != 2 {
// 		t.Errorf("FieldIndexByName('CreatedBy') len(idx) = %d, want 2", len(idx))
// 	}
// }

// func TestUserStructMeta_FieldByTag(t *testing.T) {
// 	meta := reflectmeta.GetStructMeta(reflect.TypeOf(User{}))
// 	// json tag
// 	sf, ok := meta.FieldByTag("json", "name")
// 	if !ok || sf.Name != "Name" {
// 		t.Errorf("FieldByTag(json, name) failed, got %+v", sf)
// 	}
// 	// schema tag
// 	sf, ok = meta.FieldByTag("schema", "email_address")
// 	if !ok || sf.Name != "Email" {
// 		t.Errorf("FieldByTag(schema, email_address) failed, got %+v", sf)
// 	}
// 	// gorm tag
// 	sf, ok = meta.FieldByTag("gorm", "primaryKey")
// 	if !ok || sf.Name != "ID" {
// 		t.Errorf("FieldByTag(gorm, primaryKey) failed, got %+v", sf)
// 	}
// }

// func TestUserStructMeta_FieldIndexByTag(t *testing.T) {
// 	meta := reflectmeta.GetStructMeta(reflect.TypeOf(User{}))
// 	idx, ok := meta.FieldIndexByTag("json", "id")
// 	if !ok {
// 		t.Fatalf("FieldIndexByTag(json, id) not found")
// 	}
// 	if len(idx) != 2 {
// 		t.Errorf("FieldIndexByTag(json, id) len(idx) = %d, want 2", len(idx))
// 	}
// 	idx, ok = meta.FieldIndexByTag("schema", "email_address")
// 	if !ok {
// 		t.Fatalf("FieldIndexByTag(schema, email_address) not found")
// 	}
// 	if len(idx) != 1 {
// 		t.Errorf("FieldIndexByTag(schema, email_address) len(idx) = %d, want 1", len(idx))
// 	}
// }

// func TestUserStructMeta_FieldNotExist(t *testing.T) {
// 	meta := reflectmeta.GetStructMeta(reflect.TypeOf(User{}))
// 	if _, ok := meta.FieldByName("NotExist"); ok {
// 		t.Errorf("FieldByName('NotExist') should not exist")
// 	}
// 	if _, ok := meta.FieldIndexByTag("json", "not_exist"); ok {
// 		t.Errorf("FieldIndexByTag(json, not_exist) should not exist")
// 	}
// }

// func TestUserStructMeta_TagValueEmpty(t *testing.T) {
// 	meta := reflectmeta.GetStructMeta(reflect.TypeOf(User{}))
// 	// 遍历所有最外层字段，找到 json tag 为空的字段名
// 	var emptyTagFields []string
// 	for i := 0; i < meta.NumField(); i++ {
// 		if meta.JsonTag(i) == "" {
// 			emptyTagFields = append(emptyTagFields, meta.Field(i).Name)
// 		}
// 	}
// 	// 你的User结构体最外层字段没有json tag的是 "Email" 和 "Age"
// 	want := map[string]struct{}{
// 		"Email": {},
// 		"Age":   {},
// 	}
// 	for _, f := range emptyTagFields {
// 		if _, ok := want[f]; !ok {
// 			t.Errorf("Unexpected field with empty json tag: %s", f)
// 		}
// 		delete(want, f)
// 	}
// 	for f := range want {
// 		t.Errorf("Expected field with empty json tag not found: %s", f)
// 	}
// }
