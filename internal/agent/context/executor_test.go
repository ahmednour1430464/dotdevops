package context

import (
	"testing"
)

func TestExecutor_ValidateCommand(t *testing.T) {
	tests := []struct {
		name    string
		ctx     *ExecutionContext
		cmd     []string
		wantErr bool
	}{
		{
			name: "empty command",
			ctx: &ExecutionContext{
				Name: "test",
			},
			cmd:     []string{},
			wantErr: true,
		},
		{
			name: "allowed command with no restrictions",
			ctx: &ExecutionContext{
				Name: "test",
			},
			cmd:     []string{"/bin/ls", "-la"},
			wantErr: false,
		},
		{
			name: "denied command by blacklist",
			ctx: &ExecutionContext{
				Name: "test",
				Process: ProcessConfig{
					DeniedExecutables: []string{"rm", "dd"},
				},
			},
			cmd:     []string{"/bin/rm", "-rf", "/"},
			wantErr: true,
		},
		{
			name: "allowed command with whitelist",
			ctx: &ExecutionContext{
				Name: "test",
				Process: ProcessConfig{
					AllowedExecutables: []string{"/bin/ls", "/usr/bin/cat"},
				},
			},
			cmd:     []string{"/bin/ls"},
			wantErr: false,
		},
		{
			name: "denied command not in whitelist",
			ctx: &ExecutionContext{
				Name: "test",
				Process: ProcessConfig{
					AllowedExecutables: []string{"/bin/ls"},
				},
			},
			cmd:     []string{"/bin/rm"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Executor{Context: tt.ctx}
			err := e.validateCommand(tt.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExecutor_ValidateFilePath(t *testing.T) {
	tests := []struct {
		name      string
		ctx       *ExecutionContext
		path      string
		operation FileOperation
		wantErr   bool
	}{
		{
			name: "denied path check",
			ctx: &ExecutionContext{
				Name: "test",
				Filesystem: FilesystemConfig{
					DeniedPaths: []string{"/etc"},
				},
			},
			path:      "/etc/passwd",
			operation: FileOpRead,
			wantErr:   true,
		},
		{
			name: "readable path allowed",
			ctx: &ExecutionContext{
				Name: "test",
				Filesystem: FilesystemConfig{
					ReadOnlyPaths: []string{"/tmp"},
				},
			},
			path:      "/tmp/test.txt",
			operation: FileOpRead,
			wantErr:   false,
		},
		{
			name: "writable path for write op",
			ctx: &ExecutionContext{
				Name: "test",
				Filesystem: FilesystemConfig{
					WritablePaths: []string{"/tmp"},
				},
			},
			path:      "/tmp/test.txt",
			operation: FileOpWrite,
			wantErr:   false,
		},
		{
			name: "no restrictions allows all",
			ctx: &ExecutionContext{
				Name: "test",
			},
			path:      "/anywhere/test.txt",
			operation: FileOpRead,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Executor{Context: tt.ctx}
			err := e.ValidateFilePath(tt.path, tt.operation)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
