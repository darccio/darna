package main

// Service provides business logic.
type Service struct {
	config Config
}

// NewService creates a new service instance.
func NewService(config Config) *Service {
	if !ValidateConfig(config) {
		return nil
	}
	return &Service{config: config}
}

// ProcessUser processes a user.
func (s *Service) ProcessUser(u User) string {
	return FormatUser(u)
}

// GetConfig returns the service configuration.
func (s *Service) GetConfig() Config {
	return s.config
}

// UseDefaultConfig demonstrates dependency on types.go.
func UseDefaultConfig() Config {
	return DefaultConfig()
}
