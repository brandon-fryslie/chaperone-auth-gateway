package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bmf/chaperone/internal/config"
)

// hostileConfig is what a malicious repository would plant: a service whose
// credential_ref points at the victim's real secret and whose host pattern is
// attacker-controlled. If this config is ever loaded without explicit operator
// action, the proxy would fetch the real secret and inject it into requests
// bound for the attacker's domain.
const hostileConfig = `
[server]
address = "127.0.0.1"
port = 4010

[services.openai]
host_pattern = "api.attacker.example"
auth_strategy = "bearer"
credential_ref = "env:OPENAI_API_KEY"
`

const trustedConfig = `
[server]
address = "127.0.0.1"
port = 4010

[services.openai]
host_pattern = "api.openai.com"
auth_strategy = "bearer"
credential_ref = "env:OPENAI_API_KEY"
`

// plantHostileCWD creates a directory holding both CWD config filenames any
// mode has ever consulted (chaperone.toml for inject/examine/check,
// .chaperone.toml for run-mode merge) and chdirs into it.
func plantHostileCWD(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"chaperone.toml", ".chaperone.toml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(hostileConfig), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	return dir
}

// TestConfigResolutionIgnoresCWD is the acceptance test for refusing untrusted
// CWD config: with a hostile ./chaperone.toml and ./.chaperone.toml present and
// no -c flag, no service backed by a user secret is resolvable. All modes
// (inject, run, examine, check) share getConfigPath, so this seam is the single
// enforcer under test.
func TestConfigResolutionIgnoresCWD(t *testing.T) {
	t.Run("no user config: fails loudly instead of loading CWD config", func(t *testing.T) {
		plantHostileCWD(t)
		t.Setenv("HOME", t.TempDir()) // empty home → no user config

		origCfgFile := cfgFile
		cfgFile = ""
		t.Cleanup(func() { cfgFile = origCfgFile })

		path, err := getConfigPath()
		if err == nil {
			t.Fatalf("expected resolution to fail with hostile CWD config present, got path %q", path)
		}
		if !strings.Contains(err.Error(), "-c") {
			t.Errorf("error should document the explicit -c opt-in, got: %v", err)
		}
	})

	t.Run("user config present: it wins and CWD services never appear", func(t *testing.T) {
		plantHostileCWD(t)

		home := t.TempDir()
		userCfgDir := filepath.Join(home, ".config", "chaperone")
		if err := os.MkdirAll(userCfgDir, 0o700); err != nil {
			t.Fatal(err)
		}
		userCfgPath := filepath.Join(userCfgDir, "chaperone.toml")
		if err := os.WriteFile(userCfgPath, []byte(trustedConfig), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HOME", home)

		origCfgFile := cfgFile
		cfgFile = ""
		t.Cleanup(func() { cfgFile = origCfgFile })

		path, err := getConfigPath()
		if err != nil {
			t.Fatalf("expected user config to resolve, got error: %v", err)
		}
		if path != userCfgPath {
			t.Fatalf("expected user config path %q, got %q", userCfgPath, path)
		}

		cfg, err := config.Load(path)
		if err != nil {
			t.Fatalf("failed to load resolved config: %v", err)
		}
		svc, ok := cfg.Services["openai"]
		if !ok {
			t.Fatal("trusted service missing from loaded config")
		}
		if svc.HostPattern != "api.openai.com" {
			t.Errorf("CWD config altered a trusted service: host_pattern = %q", svc.HostPattern)
		}
	})

	t.Run("explicit -c is the documented opt-in for project config", func(t *testing.T) {
		dir := plantHostileCWD(t)
		t.Setenv("HOME", t.TempDir())

		origCfgFile := cfgFile
		cfgFile = filepath.Join(dir, "chaperone.toml")
		t.Cleanup(func() { cfgFile = origCfgFile })

		path, err := getConfigPath()
		if err != nil {
			t.Fatalf("explicit -c should resolve: %v", err)
		}
		if path != filepath.Join(dir, "chaperone.toml") {
			t.Fatalf("explicit -c should be honored verbatim, got %q", path)
		}
	})
}
