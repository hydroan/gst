package gen

import (
	"github.com/hydroan/gst/types/consts"
	"github.com/stoewer/go-strcase"
)

var Methods = []string{
	strcase.UpperCamelCase(string(consts.PHASE_CREATE_BEFORE)),
	strcase.UpperCamelCase(string(consts.PHASE_CREATE_AFTER)),
	strcase.UpperCamelCase(string(consts.PHASE_DELETE_BEFORE)),
	strcase.UpperCamelCase(string(consts.PHASE_DELETE_AFTER)),
	strcase.UpperCamelCase(string(consts.PHASE_UPDATE_BEFORE)),
	strcase.UpperCamelCase(string(consts.PHASE_UPDATE_AFTER)),
	strcase.UpperCamelCase(string(consts.PHASE_PATCH_BEFORE)),
	strcase.UpperCamelCase(string(consts.PHASE_PATCH_AFTER)),
	strcase.UpperCamelCase(string(consts.PHASE_LIST_BEFORE)),
	strcase.UpperCamelCase(string(consts.PHASE_LIST_AFTER)),
	strcase.UpperCamelCase(string(consts.PHASE_GET_BEFORE)),
	strcase.UpperCamelCase(string(consts.PHASE_GET_AFTER)),
	strcase.UpperCamelCase(string(consts.PHASE_CREATE_MANY_BEFORE)),
	strcase.UpperCamelCase(string(consts.PHASE_CREATE_MANY_AFTER)),
	strcase.UpperCamelCase(string(consts.PHASE_DELETE_MANY_BEFORE)),
	strcase.UpperCamelCase(string(consts.PHASE_DELETE_MANY_AFTER)),
	strcase.UpperCamelCase(string(consts.PHASE_UPDATE_MANY_BEFORE)),
	strcase.UpperCamelCase(string(consts.PHASE_UPDATE_MANY_AFTER)),
	strcase.UpperCamelCase(string(consts.PHASE_PATCH_MANY_BEFORE)),
	strcase.UpperCamelCase(string(consts.PHASE_PATCH_MANY_AFTER)),
}
