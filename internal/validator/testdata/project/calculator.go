package main

// Calculator is a simple calculator type.
type Calculator struct {
	value int
}

// Add adds a number to the calculator's value.
func (c *Calculator) Add(n int) int {
	c.value += n
	return c.value
}
