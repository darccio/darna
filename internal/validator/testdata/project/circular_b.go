package main

// FuncB calls FuncA, creating a circular dependency.
func FuncB() string {
	return "B->" + FuncA()
}

// HelperB is a helper that doesn't create circular dependency.
func HelperB() string {
	return "HelperB"
}
