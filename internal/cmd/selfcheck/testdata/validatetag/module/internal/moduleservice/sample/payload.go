package sample

// Payload is declared outside the module model sources.
type Payload struct {
	Code string `validate:"required"`
}
