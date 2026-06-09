package config

import "testing"

// validBase is a Config that passes Validate, so grantable cases test only the
// grantable section.
func validBase() *Config {
	return &Config{
		Server:  ServerConfig{Address: "127.0.0.1", Port: 0},
		Logging: LoggingConfig{Level: "info"},
	}
}

func TestValidate_Grantable(t *testing.T) {
	cases := []struct {
		name      string
		pairing   GrantableConfig
		wantError bool
	}{
		{
			name: "valid env pairing",
			pairing: GrantableConfig{
				CredentialRef: "env:OPENAI_API_KEY",
				HostPattern:   "api.openai.com",
				AuthStrategy:  "bearer",
				MaxBodyBytes:  1048576,
			},
		},
		{
			name:      "missing host",
			pairing:   GrantableConfig{CredentialRef: "env:KEY", AuthStrategy: "bearer"},
			wantError: true,
		},
		{
			name:      "missing auth strategy",
			pairing:   GrantableConfig{CredentialRef: "env:KEY", HostPattern: "api.x.com"},
			wantError: true,
		},
		{
			name:      "missing credential ref",
			pairing:   GrantableConfig{HostPattern: "api.x.com", AuthStrategy: "bearer"},
			wantError: true,
		},
		{
			name:      "credential ref without known scheme",
			pairing:   GrantableConfig{CredentialRef: "OPENAI_API_KEY", HostPattern: "api.x.com", AuthStrategy: "bearer"},
			wantError: true,
		},
		{
			name:      "negative max body bytes",
			pairing:   GrantableConfig{CredentialRef: "file:/k", HostPattern: "api.x.com", AuthStrategy: "bearer", MaxBodyBytes: -1},
			wantError: true,
		},
		{
			name:    "keychain scheme accepted",
			pairing: GrantableConfig{CredentialRef: "keychain:svc/acct", HostPattern: "api.x.com", AuthStrategy: "header:X-API-Key"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := validBase()
			cfg.Grantable = []GrantableConfig{c.pairing}
			err := cfg.Validate()
			if (err != nil) != c.wantError {
				t.Fatalf("Validate() error = %v, wantError = %v", err, c.wantError)
			}
		})
	}
}
