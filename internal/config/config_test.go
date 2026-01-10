package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate_PlaceholderLength(t *testing.T) {
	t.Run("empty placeholder allowed", func(t *testing.T) {
		cfg := &Config{
			Server:  ServerConfig{Port: 8080},
			Logging: LoggingConfig{Level: "info"},
			Services: map[string]ServiceConfig{
				"test": {HostPattern: "example.com", Placeholder: ""},
			},
		}
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("short placeholder rejected", func(t *testing.T) {
		cfg := &Config{
			Server:  ServerConfig{Port: 8080},
			Logging: LoggingConfig{Level: "info"},
			Services: map[string]ServiceConfig{
				"test": {HostPattern: "example.com", Placeholder: "short"},
			},
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least 8 characters")
	})

	t.Run("7 character placeholder rejected", func(t *testing.T) {
		cfg := &Config{
			Server:  ServerConfig{Port: 8080},
			Logging: LoggingConfig{Level: "info"},
			Services: map[string]ServiceConfig{
				"test": {HostPattern: "example.com", Placeholder: "1234567"},
			},
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least 8 characters")
		assert.Contains(t, err.Error(), "got 7")
	})

	t.Run("8 character placeholder accepted", func(t *testing.T) {
		cfg := &Config{
			Server:  ServerConfig{Port: 8080},
			Logging: LoggingConfig{Level: "info"},
			Services: map[string]ServiceConfig{
				"test": {HostPattern: "example.com", Placeholder: "12345678"},
			},
		}
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("valid placeholder accepted", func(t *testing.T) {
		cfg := &Config{
			Server:  ServerConfig{Port: 8080},
			Logging: LoggingConfig{Level: "info"},
			Services: map[string]ServiceConfig{
				"test": {HostPattern: "example.com", Placeholder: "chap_test_12345678"},
			},
		}
		err := cfg.Validate()
		assert.NoError(t, err)
	})
}
