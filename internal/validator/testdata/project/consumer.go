package main

// ConsumeConstant uses the MaxRetries constant from constants.go.
func ConsumeConstant() int {
	return MaxRetries * 2
}

// ConsumeVariable uses the DefaultTimeout variable from variables.go.
func ConsumeVariable() int {
	return DefaultTimeout + 10
}
