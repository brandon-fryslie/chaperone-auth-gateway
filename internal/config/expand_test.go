package config

import (
	"os"
	"testing"
)

func TestExpandVars(t *testing.T) {
	// Set up test environment variables
	os.Setenv("TEST_VAR", "test_value")
	os.Setenv("HOME", "/home/user")
	os.Setenv("PATH", "/usr/bin:/bin")
	defer func() {
		os.Unsetenv("TEST_VAR")
	}()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "no variables",
			input: "plain string",
			want:  "plain string",
		},
		{
			name:  "simple variable",
			input: "$TEST_VAR",
			want:  "test_value",
		},
		{
			name:  "braced variable",
			input: "${TEST_VAR}",
			want:  "test_value",
		},
		{
			name:  "variable in middle with braces",
			input: "prefix_${TEST_VAR}_suffix",
			want:  "prefix_test_value_suffix",
		},
		{
			name:  "variable followed by non-alphanum",
			input: "prefix-$TEST_VAR-suffix",
			want:  "prefix-test_value-suffix",
		},
		{
			name:  "multiple variables",
			input: "$HOME/bin:$PATH",
			want:  "/home/user/bin:/usr/bin:/bin",
		},
		{
			name:  "escaped dollar",
			input: "\\$TEST_VAR",
			want:  "$TEST_VAR",
		},
		{
			name:  "mixed escaped and real",
			input: "\\$TEST_VAR and $TEST_VAR",
			want:  "$TEST_VAR and test_value",
		},
		{
			name:    "undefined variable",
			input:   "$UNDEFINED_VAR",
			wantErr: true,
		},
		{
			name:    "undefined braced variable",
			input:   "${UNDEFINED_VAR}",
			wantErr: true,
		},
		{
			name:    "unclosed brace",
			input:   "${TEST_VAR",
			wantErr: true,
		},
		{
			name:    "empty variable name",
			input:   "${}",
			wantErr: true,
		},
		{
			name:    "dollar at end",
			input:   "test$",
			wantErr: true,
		},
		{
			name:  "variable with underscore",
			input: "$TEST_VAR",
			want:  "test_value",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandVars(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expandVars() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("expandVars() unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("expandVars() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExpandRunConfig(t *testing.T) {
	// Set up test environment variables
	os.Setenv("TEST_CMD", "echo")
	os.Setenv("TEST_ARG", "hello")
	os.Setenv("TEST_PATH", "/tmp/test.env")
	os.Setenv("TEST_SOCKET", "/tmp/test.sock")
	defer func() {
		os.Unsetenv("TEST_CMD")
		os.Unsetenv("TEST_ARG")
		os.Unsetenv("TEST_PATH")
		os.Unsetenv("TEST_SOCKET")
	}()

	tests := []struct {
		name    string
		input   *RunConfig
		want    *RunConfig
		wantErr bool
	}{
		{
			name:  "nil config",
			input: nil,
			want:  nil,
		},
		{
			name: "expand all fields",
			input: &RunConfig{
				Command:    "$TEST_CMD",
				Args:       []string{"$TEST_ARG", "world"},
				EnvFile:    "$TEST_PATH",
				SocketPath: "$TEST_SOCKET",
				Stdout:     "file:$TEST_PATH",
				Stderr:     "inherit",
			},
			want: &RunConfig{
				Command:    "echo",
				Args:       []string{"hello", "world"},
				EnvFile:    "/tmp/test.env",
				SocketPath: "/tmp/test.sock",
				Stdout:     "file:/tmp/test.env",
				Stderr:     "inherit",
			},
		},
		{
			name: "undefined variable in command",
			input: &RunConfig{
				Command: "$UNDEFINED",
			},
			wantErr: true,
		},
		{
			name: "undefined variable in args",
			input: &RunConfig{
				Command: "echo",
				Args:    []string{"$UNDEFINED"},
			},
			wantErr: true,
		},
		{
			name: "undefined variable in env_file",
			input: &RunConfig{
				Command: "echo",
				EnvFile: "$UNDEFINED",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying test input
			var input *RunConfig
			if tt.input != nil {
				copy := *tt.input
				if tt.input.Args != nil {
					copy.Args = make([]string, len(tt.input.Args))
					for i := range tt.input.Args {
						copy.Args[i] = tt.input.Args[i]
					}
				}
				input = &copy
			}

			err := expandRunConfig(input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expandRunConfig() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("expandRunConfig() unexpected error: %v", err)
				return
			}

			if tt.want == nil {
				if input != nil {
					t.Errorf("expandRunConfig() expected nil, got %+v", input)
				}
				return
			}

			if input.Command != tt.want.Command {
				t.Errorf("Command = %q, want %q", input.Command, tt.want.Command)
			}
			if input.EnvFile != tt.want.EnvFile {
				t.Errorf("EnvFile = %q, want %q", input.EnvFile, tt.want.EnvFile)
			}
			if input.SocketPath != tt.want.SocketPath {
				t.Errorf("SocketPath = %q, want %q", input.SocketPath, tt.want.SocketPath)
			}
			if input.Stdout != tt.want.Stdout {
				t.Errorf("Stdout = %q, want %q", input.Stdout, tt.want.Stdout)
			}
			if input.Stderr != tt.want.Stderr {
				t.Errorf("Stderr = %q, want %q", input.Stderr, tt.want.Stderr)
			}
			if len(input.Args) != len(tt.want.Args) {
				t.Errorf("Args length = %d, want %d", len(input.Args), len(tt.want.Args))
			}
			for i := range input.Args {
				if input.Args[i] != tt.want.Args[i] {
					t.Errorf("Args[%d] = %q, want %q", i, input.Args[i], tt.want.Args[i])
				}
			}
		})
	}
}

func TestIsAlphaNumericUnderscore(t *testing.T) {
	tests := []struct {
		input byte
		want  bool
	}{
		{'a', true},
		{'z', true},
		{'A', true},
		{'Z', true},
		{'0', true},
		{'9', true},
		{'_', true},
		{'-', false},
		{'.', false},
		{'$', false},
		{' ', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := isAlphaNumericUnderscore(tt.input)
			if got != tt.want {
				t.Errorf("isAlphaNumericUnderscore(%c) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
