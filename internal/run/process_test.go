package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSpawnChild_Success(t *testing.T) {
	ctx := context.Background()

	cfg := ProcessConfig{
		Command:    []string{"echo", "hello"},
		ProxyURL:   "http://127.0.0.1:8080",
		CACertPath: "/tmp/test-ca.pem",
	}

	result, err := SpawnChild(ctx, cfg)
	if err != nil {
		t.Fatalf("SpawnChild failed: %v", err)
	}

	exitCode := result.Wait(ctx)
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestSpawnChild_InvalidCommand(t *testing.T) {
	ctx := context.Background()

	cfg := ProcessConfig{
		Command:    []string{"/nonexistent/command"},
		ProxyURL:   "http://127.0.0.1:8080",
		CACertPath: "/tmp/test-ca.pem",
	}

	_, err := SpawnChild(ctx, cfg)
	if err == nil {
		t.Fatal("expected error for nonexistent command, got nil")
	}
}

func TestSpawnChild_EmptyCommand(t *testing.T) {
	ctx := context.Background()

	cfg := ProcessConfig{
		Command:    []string{},
		ProxyURL:   "http://127.0.0.1:8080",
		CACertPath: "/tmp/test-ca.pem",
	}

	_, err := SpawnChild(ctx, cfg)
	if err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
	if !strings.Contains(err.Error(), "command is required") {
		t.Errorf("expected 'command is required' error, got: %v", err)
	}
}

func TestSpawnChild_NonZeroExit(t *testing.T) {
	ctx := context.Background()

	cfg := ProcessConfig{
		Command:    []string{"sh", "-c", "exit 42"},
		ProxyURL:   "http://127.0.0.1:8080",
		CACertPath: "/tmp/test-ca.pem",
	}

	result, err := SpawnChild(ctx, cfg)
	if err != nil {
		t.Fatalf("SpawnChild failed: %v", err)
	}

	exitCode := result.Wait(ctx)
	if exitCode != 42 {
		t.Errorf("expected exit code 42, got %d", exitCode)
	}
}

func TestWaitWithSentinel_NaturalExit(t *testing.T) {
	ctx := context.Background()
	sentinelChan := make(chan struct{})

	cfg := ProcessConfig{
		Command:    []string{"echo", "hello"},
		ProxyURL:   "http://127.0.0.1:8080",
		CACertPath: "/tmp/test-ca.pem",
	}

	result, err := SpawnChild(ctx, cfg)
	if err != nil {
		t.Fatalf("SpawnChild failed: %v", err)
	}

	exitCode := result.WaitWithSentinel(ctx, sentinelChan)
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestWaitWithSentinel_SentinelTriggered(t *testing.T) {
	ctx := context.Background()
	sentinelChan := make(chan struct{})

	// Use a long-running command
	cfg := ProcessConfig{
		Command:    []string{"sleep", "60"},
		ProxyURL:   "http://127.0.0.1:8080",
		CACertPath: "/tmp/test-ca.pem",
	}

	result, err := SpawnChild(ctx, cfg)
	if err != nil {
		t.Fatalf("SpawnChild failed: %v", err)
	}

	// Trigger sentinel after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(sentinelChan)
	}()

	exitCode := result.WaitWithSentinel(ctx, sentinelChan)
	if exitCode != 0 {
		t.Errorf("expected exit code 0 (sentinel triggered), got %d", exitCode)
	}
}

func TestBuildProcessEnvironment_Defaults(t *testing.T) {
	ctx := context.Background()

	cfg := ProcessConfig{
		Command:    []string{"echo", "test"},
		ProxyURL:   "http://127.0.0.1:8080",
		CACertPath: "/tmp/test-ca.pem",
	}

	env, err := BuildProcessEnvironment(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildProcessEnvironment failed: %v", err)
	}

	// Check proxy vars are set
	hasHTTPProxy := false
	hasHTTPSProxy := false
	hasNodeUseEnvProxy := false
	hasSSLCertFile := false

	for _, e := range env {
		if strings.HasPrefix(e, "HTTP_PROXY=") {
			hasHTTPProxy = true
			if !strings.Contains(e, "127.0.0.1:8080") {
				t.Errorf("HTTP_PROXY should contain proxy URL, got: %s", e)
			}
		}
		if strings.HasPrefix(e, "HTTPS_PROXY=") {
			hasHTTPSProxy = true
		}
		if strings.HasPrefix(e, "NODE_USE_ENV_PROXY=1") {
			hasNodeUseEnvProxy = true
		}
		if strings.HasPrefix(e, "SSL_CERT_FILE=") {
			hasSSLCertFile = true
			if !strings.Contains(e, "/tmp/test-ca.pem") {
				t.Errorf("SSL_CERT_FILE should contain CA path, got: %s", e)
			}
		}
	}

	if !hasHTTPProxy {
		t.Error("HTTP_PROXY not set in environment")
	}
	if !hasHTTPSProxy {
		t.Error("HTTPS_PROXY not set in environment")
	}
	if !hasNodeUseEnvProxy {
		t.Error("NODE_USE_ENV_PROXY not set in environment")
	}
	if !hasSSLCertFile {
		t.Error("SSL_CERT_FILE not set in environment")
	}
}

func TestBuildProcessEnvironment_WithEnvFile(t *testing.T) {
	ctx := context.Background()

	// Create temporary env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	err := os.WriteFile(envFile, []byte("TEST_VAR=test_value\n"), 0600)
	if err != nil {
		t.Fatalf("failed to create test env file: %v", err)
	}

	cfg := ProcessConfig{
		Command:    []string{"echo", "test"},
		ProxyURL:   "http://127.0.0.1:8080",
		CACertPath: "/tmp/test-ca.pem",
		EnvFile:    envFile,
	}

	env, err := BuildProcessEnvironment(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildProcessEnvironment failed: %v", err)
	}

	// Check that env file var is present
	hasTestVar := false
	for _, e := range env {
		if strings.HasPrefix(e, "TEST_VAR=test_value") {
			hasTestVar = true
			break
		}
	}

	if !hasTestVar {
		t.Error("TEST_VAR from env file not found in environment")
	}
}

func TestBuildProcessEnvironment_WithUserVars(t *testing.T) {
	ctx := context.Background()

	cfg := ProcessConfig{
		Command:    []string{"echo", "test"},
		ProxyURL:   "http://127.0.0.1:8080",
		CACertPath: "/tmp/test-ca.pem",
		UserEnvVars: map[string]string{
			"CUSTOM_VAR1": "value1",
			"CUSTOM_VAR2": "value2",
		},
	}

	env, err := BuildProcessEnvironment(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildProcessEnvironment failed: %v", err)
	}

	// Check that user vars are present
	hasVar1 := false
	hasVar2 := false
	for _, e := range env {
		if strings.HasPrefix(e, "CUSTOM_VAR1=value1") {
			hasVar1 = true
		}
		if strings.HasPrefix(e, "CUSTOM_VAR2=value2") {
			hasVar2 = true
		}
	}

	if !hasVar1 {
		t.Error("CUSTOM_VAR1 not found in environment")
	}
	if !hasVar2 {
		t.Error("CUSTOM_VAR2 not found in environment")
	}
}

func TestBuildProcessEnvironment_CustomCAEnvVars(t *testing.T) {
	ctx := context.Background()

	cfg := ProcessConfig{
		Command:    []string{"echo", "test"},
		ProxyURL:   "http://127.0.0.1:8080",
		CACertPath: "/tmp/test-ca.pem",
		CAEnvVars:  []string{"SSL_CERT_FILE", "CUSTOM_CA_VAR"},
	}

	env, err := BuildProcessEnvironment(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildProcessEnvironment failed: %v", err)
	}

	// Check that only specified CA vars are set
	hasSSLCertFile := false
	hasCustomCAVar := false
	hasRequestsCABundle := false

	for _, e := range env {
		if strings.HasPrefix(e, "SSL_CERT_FILE=") {
			hasSSLCertFile = true
		}
		if strings.HasPrefix(e, "CUSTOM_CA_VAR=") {
			hasCustomCAVar = true
		}
		if strings.HasPrefix(e, "REQUESTS_CA_BUNDLE=") {
			hasRequestsCABundle = true
		}
	}

	if !hasSSLCertFile {
		t.Error("SSL_CERT_FILE not set in environment")
	}
	if !hasCustomCAVar {
		t.Error("CUSTOM_CA_VAR not set in environment")
	}
	if hasRequestsCABundle {
		t.Error("REQUESTS_CA_BUNDLE should not be set (not in custom list)")
	}
}
