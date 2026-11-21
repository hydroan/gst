package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckValidateTag runs the check over a fixture module. Two fields of
// the module model sources (internal/model/sample/record.go) are reported:
// one declaring its rules under validate (RecordCreateReq.Title) and one
// carrying a validate tag beside a binding one (ItemCreateReq.Code). Nothing
// is reported for a field declaring its rules under binding (Remark), for a
// tag that merely holds the word (Note), for a test file (record_test.go), or
// for a struct declared outside the module model sources
// (internal/service/sample and pkg/other).
func TestCheckValidateTag(t *testing.T) {
	root, pkgs := loadFixture(t, "testdata/validatetag/module")
	violations, err := checkValidateTag(root, pkgs)
	require.NoError(t, err)
	require.Equal(t, []violation{
		{
			File:    "internal/model/sample/record.go",
			Message: "Field 'ItemCreateReq.Code' at internal/model/sample/record.go:13 declares its validation rules under the validate tag (len=6), which the framework never runs: declare them under the binding tag",
		},
		{
			File:    "internal/model/sample/record.go",
			Message: "Field 'RecordCreateReq.Title' at internal/model/sample/record.go:6 declares its validation rules under the validate tag (required,max=64), which the framework never runs: declare them under the binding tag",
		},
	}, violations)
}
