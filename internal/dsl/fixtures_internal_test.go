package dsl

import (
	"go/parser"
	"go/token"
	"os"
	"testing"
)

var (
	userSource        string
	user2Source       string
	user3And4Source   string
	user4Source       string
	user5Source       string
	user6And7Source   string
	user8And9Source   string
	user10And11Source string
	user12Source      string
	user13Source      string
)

func init() {
	data1, err := os.ReadFile("./testdata/user.go")
	if err != nil {
		panic(err)
	}
	userSource = string(data1)

	data2, err := os.ReadFile("./testdata/user2.go")
	if err != nil {
		panic(err)
	}
	user2Source = string(data2)

	data3, err := os.ReadFile("./testdata/user3_4.go")
	if err != nil {
		panic(err)
	}
	user3And4Source = string(data3)

	data4, err := os.ReadFile("./testdata/user4.go")
	if err != nil {
		panic(err)
	}
	user4Source = string(data4)

	data5, err := os.ReadFile("./testdata/user5.go")
	if err != nil {
		panic(err)
	}
	user5Source = string(data5)

	data6, err := os.ReadFile("./testdata/user6_7.go")
	if err != nil {
		panic(err)
	}
	user6And7Source = string(data6)

	data7, err := os.ReadFile("./testdata/user8_9.go")
	if err != nil {
		panic(err)
	}
	user8And9Source = string(data7)

	data8, err := os.ReadFile("./testdata/user10_11.go")
	if err != nil {
		panic(err)
	}
	user10And11Source = string(data8)

	data9, err := os.ReadFile("./testdata/user12.go")
	if err != nil {
		panic(err)
	}
	user12Source = string(data9)

	data10, err := os.ReadFile("./testdata/user13.go")
	if err != nil {
		panic(err)
	}
	user13Source = string(data10)
}

// parseDesignFromSource parses src and returns the design Parse builds for
// the model named modelName, failing the test when src declares no such model.
func parseDesignFromSource(t *testing.T, src, modelName string) *Design {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse source failed: %v", err)
	}

	designs := Parse(file)
	design, ok := designs[modelName]
	if !ok {
		t.Fatalf("model %s not found", modelName)
	}
	return design
}
