package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvBuilder_InheritParent(t *testing.T) {
	// Set a test env var
	os.Setenv("TEST_INHERIT_VAR", "test_value")
	defer os.Unsetenv("TEST_INHERIT_VAR")

	eb := NewEnvBuilder()
	eb.InheritParent()

	env := eb.Build()

	// Should have inherited TEST_INHERIT_VAR
	found := false
	for _, e := range env {
		if strings.HasPrefix(e, "TEST_INHERIT_VAR=") {
			if e != "TEST_INHERIT_VAR=test_value" {
				t.Errorf("Got %q, want TEST_INHERIT_VAR=test_value", e)
			}
			found = true
			break
		}
	}

	if !found {
		t.Error("TEST_INHERIT_VAR not found in inherited environment")
	}

	// Should have many inherited vars (PATH, HOME, etc.)
	if len(env) < 5 {
		t.Errorf("Expected at least 5 inherited env vars, got %d", len(env))
	}
}

func TestEnvBuilder_Set(t *testing.T) {
	eb := NewEnvBuilder()
	eb.Set("FOO", "bar").Set("BAZ", "qux")

	env := eb.Build()

	if len(env) != 2 {
		t.Errorf("Expected 2 env vars, got %d", len(env))
	}

	vars := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			vars[parts[0]] = parts[1]
		}
	}

	if vars["FOO"] != "bar" {
		t.Errorf("FOO=%q, want bar", vars["FOO"])
	}
	if vars["BAZ"] != "qux" {
		t.Errorf("BAZ=%q, want qux", vars["BAZ"])
	}
}

func TestEnvBuilder_SetCAEnvVars(t *testing.T) {
	tests := []struct {
		name        string
		caCertPath  string
		caEnvVars   []string
		wantEnvVars map[string]string
	}{
		{
			name:       "default sets all standard vars",
			caCertPath: "/tmp/ca-cert.pem",
			caEnvVars:  nil,
			wantEnvVars: map[string]string{
				"SSL_CERT_FILE":                  "/tmp/ca-cert.pem",
				"CURL_CA_BUNDLE":                 "/tmp/ca-cert.pem",
				"NODE_EXTRA_CA_CERTS":            "/tmp/ca-cert.pem",
				"REQUESTS_CA_BUNDLE":             "/tmp/ca-cert.pem",
				"HTTPX_CA_BUNDLE":                "/tmp/ca-cert.pem",
				"GIT_SSL_CAINFO":                 "/tmp/ca-cert.pem",
				"PERL_LWP_SSL_CA_FILE":           "/tmp/ca-cert.pem",
				"HTTPS_CA_FILE":                  "/tmp/ca-cert.pem",
				"AWS_CA_BUNDLE":                  "/tmp/ca-cert.pem",
				"HOMEBREW_CERTIFICATE_AUTHORITY": "/tmp/ca-cert.pem",
				"CHAPERONE_CA_CERT":              "/tmp/ca-cert.pem",
			},
		},
		{
			name:       "empty slice sets all standard vars",
			caCertPath: "/tmp/ca-cert.pem",
			caEnvVars:  []string{},
			wantEnvVars: map[string]string{
				"SSL_CERT_FILE":                  "/tmp/ca-cert.pem",
				"CURL_CA_BUNDLE":                 "/tmp/ca-cert.pem",
				"NODE_EXTRA_CA_CERTS":            "/tmp/ca-cert.pem",
				"REQUESTS_CA_BUNDLE":             "/tmp/ca-cert.pem",
				"HTTPX_CA_BUNDLE":                "/tmp/ca-cert.pem",
				"GIT_SSL_CAINFO":                 "/tmp/ca-cert.pem",
				"PERL_LWP_SSL_CA_FILE":           "/tmp/ca-cert.pem",
				"HTTPS_CA_FILE":                  "/tmp/ca-cert.pem",
				"AWS_CA_BUNDLE":                  "/tmp/ca-cert.pem",
				"HOMEBREW_CERTIFICATE_AUTHORITY": "/tmp/ca-cert.pem",
				"CHAPERONE_CA_CERT":              "/tmp/ca-cert.pem",
			},
		},
		{
			name:       "custom vars only sets specified",
			caCertPath: "/tmp/ca.pem",
			caEnvVars:  []string{"NODE_EXTRA_CA_CERTS"},
			wantEnvVars: map[string]string{
				"NODE_EXTRA_CA_CERTS": "/tmp/ca.pem",
				"CHAPERONE_CA_CERT":   "/tmp/ca.pem",
			},
		},
		{
			name:       "multiple custom vars",
			caCertPath: "/tmp/ephemeral-ca.pem",
			caEnvVars:  []string{"SSL_CERT_FILE", "REQUESTS_CA_BUNDLE"},
			wantEnvVars: map[string]string{
				"SSL_CERT_FILE":      "/tmp/ephemeral-ca.pem",
				"REQUESTS_CA_BUNDLE": "/tmp/ephemeral-ca.pem",
				"CHAPERONE_CA_CERT":  "/tmp/ephemeral-ca.pem",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewEnvBuilder()
			builder.SetCAEnvVars(tt.caCertPath, tt.caEnvVars)

			env := builder.Build()
			envMap := parseEnvSlice(env)

			// Check expected vars are present with correct values
			for key, expectedVal := range tt.wantEnvVars {
				if envMap[key] != expectedVal {
					t.Errorf("env[%s] = %q, want %q", key, envMap[key], expectedVal)
				}
			}

			// Check no extra CA-related vars are set
			caRelatedVars := []string{
				"SSL_CERT_FILE",
				"CURL_CA_BUNDLE",
				"NODE_EXTRA_CA_CERTS",
				"REQUESTS_CA_BUNDLE",
				"HTTPX_CA_BUNDLE",
				"GIT_SSL_CAINFO",
				"PERL_LWP_SSL_CA_FILE",
				"HTTPS_CA_FILE",
				"AWS_CA_BUNDLE",
				"HOMEBREW_CERTIFICATE_AUTHORITY",
				"CHAPERONE_CA_CERT",
			}
			for _, varName := range caRelatedVars {
				if _, expected := tt.wantEnvVars[varName]; !expected {
					if val, exists := envMap[varName]; exists {
						t.Errorf("unexpected env var %s=%q", varName, val)
					}
				}
			}
		})
	}
}

func TestEnvBuilder_LoadEnvFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "simple key=value",
			content: `FOO=bar
BAZ=qux`,
			want: map[string]string{
				"FOO": "bar",
				"BAZ": "qux",
			},
		},
		{
			name: "quoted values",
			content: `FOO="bar with spaces"
BAZ='single quoted'`,
			want: map[string]string{
				"FOO": "bar with spaces",
				"BAZ": "single quoted",
			},
		},
		{
			name: "comments and empty lines",
			content: `# Comment
FOO=bar

# Another comment
BAZ=qux
`,
			want: map[string]string{
				"FOO": "bar",
				"BAZ": "qux",
			},
		},
		{
			name: "whitespace handling",
			content: `  FOO = bar
  BAZ = qux  `,
			want: map[string]string{
				"FOO": "bar",
				"BAZ": "qux",
			},
		},
		{
			name: "empty value",
			content: `FOO=
BAZ=qux`,
			want: map[string]string{
				"FOO": "",
				"BAZ": "qux",
			},
		},
		{
			name:    "invalid format",
			content: `INVALID LINE`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			envFile := filepath.Join(tmpDir, ".env")
			if err := os.WriteFile(envFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			eb := NewEnvBuilder()
			err := eb.LoadEnvFile(envFile)

			if tt.wantErr {
				if err == nil {
					t.Error("LoadEnvFile() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("LoadEnvFile() unexpected error: %v", err)
				return
			}

			env := eb.Build()
			vars := make(map[string]string)
			for _, e := range env {
				parts := strings.SplitN(e, "=", 2)
				if len(parts) == 2 {
					vars[parts[0]] = parts[1]
				}
			}

			for key, want := range tt.want {
				got := vars[key]
				if got != want {
					t.Errorf("%s=%q, want %q", key, got, want)
				}
			}

			// Check no extra vars
			if len(vars) != len(tt.want) {
				t.Errorf("Got %d vars, want %d", len(vars), len(tt.want))
			}
		})
	}
}

func TestEnvBuilder_LoadEnvFile_NotFound(t *testing.T) {
	eb := NewEnvBuilder()
	err := eb.LoadEnvFile("/nonexistent/file.env")
	if err == nil {
		t.Error("LoadEnvFile() with nonexistent file should error")
	}
}

func TestEnvBuilder_Chaining(t *testing.T) {
	// Test that builder methods can be chained
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	os.WriteFile(envFile, []byte("FILE_VAR=from_file"), 0644)

	eb := NewEnvBuilder()
	err := eb.InheritParent().
		Set("CUSTOM_VAR", "custom_value").
		LoadEnvFile(envFile)
	if err != nil {
		t.Fatalf("Failed to load env file: %v", err)
	}
	eb.SetProxyVars("http://127.0.0.1:8080", "myservice")

	env := eb.Build()
	vars := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			vars[parts[0]] = parts[1]
		}
	}

	// Should have parent vars
	if vars["PATH"] == "" {
		t.Error("PATH not inherited")
	}

	// Should have custom var
	if vars["CUSTOM_VAR"] != "custom_value" {
		t.Errorf("CUSTOM_VAR=%q, want custom_value", vars["CUSTOM_VAR"])
	}

	// Should have file var
	if vars["FILE_VAR"] != "from_file" {
		t.Errorf("FILE_VAR=%q, want from_file", vars["FILE_VAR"])
	}

	// Should have proxy vars
	if vars["HTTP_PROXY"] != "http://127.0.0.1:8080" {
		t.Errorf("HTTP_PROXY=%q, want http://127.0.0.1:8080", vars["HTTP_PROXY"])
	}
}

func TestEnvBuilder_Chaining_WithCAEnvVars(t *testing.T) {
	// Test that SetCAEnvVars can be chained with other methods
	eb := NewEnvBuilder()
	eb.InheritParent()
	eb.SetProxyVars("http://127.0.0.1:8080", "myservice")
	eb.SetCAEnvVars("/tmp/ca-cert.pem", nil)

	env := eb.Build()
	vars := parseEnvSlice(env)

	// Should have proxy vars
	if vars["HTTP_PROXY"] != "http://127.0.0.1:8080" {
		t.Errorf("HTTP_PROXY=%q, want http://127.0.0.1:8080", vars["HTTP_PROXY"])
	}

	// Should have CA vars
	expectedCAVars := []string{
		"SSL_CERT_FILE",
		"CURL_CA_BUNDLE",
		"NODE_EXTRA_CA_CERTS",
		"REQUESTS_CA_BUNDLE",
		"HTTPX_CA_BUNDLE",
		"GIT_SSL_CAINFO",
		"PERL_LWP_SSL_CA_FILE",
		"HTTPS_CA_FILE",
		"AWS_CA_BUNDLE",
		"HOMEBREW_CERTIFICATE_AUTHORITY",
		"CHAPERONE_CA_CERT",
	}
	for _, varName := range expectedCAVars {
		if vars[varName] != "/tmp/ca-cert.pem" {
			t.Errorf("%s=%q, want /tmp/ca-cert.pem", varName, vars[varName])
		}
	}
}

// Helper function to parse env slice into map
func parseEnvSlice(env []string) map[string]string {
	result := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}
