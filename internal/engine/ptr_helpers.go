package yamlvalidator

// Ptr returns a pointer to the value. Useful for setting Min/Max fields.
func Ptr[T any](v T) *T { return &v }
