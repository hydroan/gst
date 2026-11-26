package modelschema

import (
	"reflect"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// validatedSample keeps every rule of the package, so only a registered check
// can refuse it.
type validatedSample struct {
	ID   string `gorm:"primaryKey"`
	Name string `gorm:"size:64"`
}

// renamedIDSample maps the framework's id to a column of its own, which the
// package's own check refuses.
type renamedIDSample struct {
	ID string `gorm:"column:sample_id;primaryKey"`
}

// TestValidateRunsTheRegisteredChecksAfterItsOwn pins the order Validate
// documents: a registered check is asked of a model the package's own checks
// pass and its error is the one returned, while a model one of the package's
// own checks refuses never reaches the registered ones. The check is
// registered for the test alone: the checks registered before it are put back
// when the test ends, which the package's own test alone can do.
func TestValidateRunsTheRegisteredChecksAfterItsOwn(t *testing.T) {
	registered := validators
	t.Cleanup(func() { validators = registered })

	errRefused := errors.New("the sample is refused")
	var asked []reflect.Type
	RegisterValidator(func(typ reflect.Type) error {
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		switch typ {
		case reflect.TypeFor[validatedSample]():
			asked = append(asked, typ)
			return errRefused
		case reflect.TypeFor[renamedIDSample]():
			asked = append(asked, typ)
		}
		return nil
	})

	require.ErrorIs(t, Validate(reflect.TypeFor[*validatedSample]()), errRefused)
	require.Equal(t, []reflect.Type{reflect.TypeFor[validatedSample]()}, asked)

	err := Validate(reflect.TypeFor[*renamedIDSample]())
	require.ErrorContains(t, err, `framework field ID maps to column "sample_id" instead of "id"`)
	require.Len(t, asked, 1, "a model the package's own check refuses reaches no registered check")
}
