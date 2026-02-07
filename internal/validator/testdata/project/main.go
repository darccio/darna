package main

import "fmt"

func main() {
	config := Config{
		Port:    8080,
		Timeout: 30,
	}

	svc := NewService(config)
	if svc == nil {
		fmt.Println("Failed to create service")
		return
	}

	user := User{
		ID:   1,
		Name: "Alice",
	}

	result := svc.ProcessUser(user)
	fmt.Println(result)
	fmt.Println(Helper())
}
