package openapigen

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
)

// User is a user struct for testing
type User struct {
	Name string // User name
	Addr string // User address

	model.Base
}

// UserWithDoc is a user struct for testing documentation comments
type UserWithDoc struct {
	// User's name
	Name string

	/* 用户邮箱地址 */
	Email string

	// User's age - this is a line comment
	Age int // This is another line comment, should be ignored

	model.Base
}

// UserWithMultiLineDoc is a user struct for testing multi-line comments
type UserWithMultiLineDoc struct {
	// User's full name
	// Including first and last name
	FullName string

	/*
		用户的详细地址信息
		包括街道、城市、邮编等
	*/
	Address string

	model.Base
}

func Test_parseModelDocs(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		m    types.Model
		want map[string]string
	}{
		{
			name: "test_line_comments",
			m:    new(User),
			want: map[string]string{
				"Name": "User name",
				"Addr": "User address",
			},
		},
		{
			name: "test_doc_comments_priority",
			m:    new(UserWithDoc),
			want: map[string]string{
				"Name":  "User's name",
				"Email": "用户邮箱地址",
				"Age":   "User's age - this is a line comment",
			},
		},
		{
			name: "test_multiline_comments",
			m:    new(UserWithMultiLineDoc),
			want: map[string]string{
				"FullName": "User's full name Including first and last name",
				"Address":  "用户的详细地址信息 包括街道、城市、邮编等",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseModelDocs(tt.m)
			fmt.Println(got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseModelDocs() = %v, want %v", got, tt.want)
			}
		})
	}
}
