package modelschema

import "reflect"

// validators are the checks the packages above this one registered for the
// declaration of a model, in the order they registered them.
var validators []func(typ reflect.Type) error

// RegisterValidator adds validate to the checks Validate holds the
// declaration of a model to. A package with a rule of its own for a model,
// the tenant's on unique indexes or the URL query's on parameter names,
// registers it while it initializes: the rule is then checked in every
// program holding the package, and this package names none of them.
func RegisterValidator(validate func(typ reflect.Type) error) {
	validators = append(validators, validate)
}

// Validate holds the declaration of the model typ to the rules its table is
// prepared under: the framework's columns keep their names (see
// ValidateFrameworkColumns), the columns the framework has a type for are
// declared by it (see ValidateColumnTypes), and every check registered with
// RegisterValidator passes, in the order they were registered. It returns
// the first error.
func Validate(typ reflect.Type) error {
	if err := ValidateFrameworkColumns(typ); err != nil {
		return err
	}
	if err := ValidateColumnTypes(typ); err != nil {
		return err
	}
	for _, validate := range validators {
		if err := validate(typ); err != nil {
			return err
		}
	}
	return nil
}
