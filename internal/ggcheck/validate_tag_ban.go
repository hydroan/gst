package ggcheck

import (
	"fmt"
	"go/ast"

	"github.com/hydroan/gst/internal/gghelper"
)

// ValidateTagBan keeps validation rules out of the validate struct tag.
var ValidateTagBan = Check{
	Name: "Validate tag ban",
	Rule: "struct fields must not declare validation rules under the validate tag, which the framework never runs; a request is validated by the rules its fields declare under the binding tag",
	run:  checkValidateTagBan,
}

// validateTag is the struct tag go-playground/validator reads rules from when
// it is used on its own, and the one the framework never reads.
const validateTag = "validate"

// checkValidateTagBan reports struct fields that carry a validate struct tag.
// The framework validates a model or a payload with gin's validator, which
// reads the rules of a field from its binding tag and from no other: rules
// under validate read like validation and never run, so the request they are
// there to refuse is served. A field carrying a binding tag beside the
// validate one is reported as well, the rules under validate being as dead
// there. The fields read are those of the project's own model files (see
// structFieldViolations).
func checkValidateTagBan(ignore gghelper.ProjectIgnore) []string {
	return structFieldViolations(ignore, func(structName string, field *ast.Field) string {
		rules, ok := structTagValue(field, validateTag)
		if !ok {
			return ""
		}
		return fmt.Sprintf(
			"field '%s.%s' declares its validation rules under the validate tag (%s), which the framework never runs; declare them under the binding tag instead",
			structName, fieldDisplayName(field), rules,
		)
	})
}
