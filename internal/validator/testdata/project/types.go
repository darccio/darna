package main

// User represents a user in the system.
type User struct {
	ID   int
	Name string
}

// Config holds application configuration.
type Config struct {
	Port    int
	Timeout int
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Port:    8080,
		Timeout: 30,
	}
}
