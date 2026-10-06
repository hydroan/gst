package sample

// dynamic hands a value of interface type to a key-value pair, so the type
// of the field is decided at run time.
func dynamic(payload any) {
	log.Infow("received", "payload", payload)
}
