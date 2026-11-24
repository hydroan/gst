package sample

// fixture drives a validator of the test's own.
type fixture struct {
	Code string `validate:"required"`
}
