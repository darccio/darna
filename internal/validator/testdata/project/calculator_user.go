package main

// UseCalculator uses the Calculator type and its Add method from calculator.go.
func UseCalculator() int {
	calc := &Calculator{value: 0}
	return calc.Add(5)
}
