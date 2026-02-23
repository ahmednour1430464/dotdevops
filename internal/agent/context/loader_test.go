package context

import (
	"testing"
)

func TestValidateContext(t *testing.T) {
	tests := []struct {
		name    string
		ctx     *ExecutionContext
		wantErr bool
	}{
		{
			name: "valid minimal context",
			ctx: &ExecutionContext{
				Name:       "test",
				TrustLevel: TrustLevelLow,
				Identity: IdentityConfig{
					User: "testuser",
				},
				Audit: AuditConfig{
					Level: AuditLevelMinimal,
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			ctx: &ExecutionContext{
				TrustLevel: TrustLevelLow,
				Identity: IdentityConfig{
					User: "testuser",
				},
				Audit: AuditConfig{
					Level: AuditLevelMinimal,
				},
			},
			wantErr: true,
		},
		{
			name: "missing trust level",
			ctx: &ExecutionContext{
				Name: "test",
				Identity: IdentityConfig{
					User: "testuser",
				},
				Audit: AuditConfig{
					Level: AuditLevelMinimal,
				},
			},
			wantErr: true,
		},
		{
			name: "missing user",
			ctx: &ExecutionContext{
				Name:       "test",
				TrustLevel: TrustLevelLow,
				Audit: AuditConfig{
					Level: AuditLevelMinimal,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid trust level",
			ctx: &ExecutionContext{
				Name:       "test",
				TrustLevel: "invalid",
				Identity: IdentityConfig{
					User: "testuser",
				},
				Audit: AuditConfig{
					Level: AuditLevelMinimal,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid audit level",
			ctx: &ExecutionContext{
				Name:       "test",
				TrustLevel: TrustLevelLow,
				Identity: IdentityConfig{
					User: "testuser",
				},
				Audit: AuditConfig{
					Level: "invalid",
				},
			},
			wantErr: true,
		},
		{
			name: "escalation without commands",
			ctx: &ExecutionContext{
				Name:       "test",
				TrustLevel: TrustLevelLow,
				Identity: IdentityConfig{
					User: "testuser",
				},
				Privilege: PrivilegeConfig{
					AllowEscalation: true,
					SudoCommands:    []string{},
				},
				Audit: AuditConfig{
					Level: AuditLevelMinimal,
				},
			},
			wantErr: true,
		},
		{
			name: "relative path in readable",
			ctx: &ExecutionContext{
				Name:       "test",
				TrustLevel: TrustLevelLow,
				Identity: IdentityConfig{
					User: "testuser",
				},
				Filesystem: FilesystemConfig{
					ReadOnlyPaths: []string{"relative/path"},
				},
				Audit: AuditConfig{
					Level: AuditLevelMinimal,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContext(tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateContext() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCompareTrustLevel(t *testing.T) {
	tests := []struct {
		name string
		a    TrustLevel
		b    TrustLevel
		want int
	}{
		{"low < medium", TrustLevelLow, TrustLevelMedium, -1},
		{"low < high", TrustLevelLow, TrustLevelHigh, -1},
		{"medium < high", TrustLevelMedium, TrustLevelHigh, -1},
		{"low == low", TrustLevelLow, TrustLevelLow, 0},
		{"medium == medium", TrustLevelMedium, TrustLevelMedium, 0},
		{"high == high", TrustLevelHigh, TrustLevelHigh, 0},
		{"high > medium", TrustLevelHigh, TrustLevelMedium, 1},
		{"high > low", TrustLevelHigh, TrustLevelLow, 1},
		{"medium > low", TrustLevelMedium, TrustLevelLow, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareTrustLevel(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareTrustLevel(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
