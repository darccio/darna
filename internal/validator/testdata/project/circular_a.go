package main

// FuncA calls FuncB, creating a circular dependency.
func FuncA() string {
	return "A->" + FuncB()
}

// HelperA is a helper that doesn't create circular dependency.
func HelperA() string {
	return "HelperA"
}
