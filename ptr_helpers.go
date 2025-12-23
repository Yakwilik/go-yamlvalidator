package yamlvalidator

// Ptr returns a pointer to the value. Useful for setting Min/Max fields.
func Ptr[T any](v T) *T { return &v }

// PtrInt returns a pointer to an int.
func PtrInt(v int) *int { return &v }

// PtrFloat returns a pointer to a float64.
func PtrFloat(v float64) *float64 { return &v }
