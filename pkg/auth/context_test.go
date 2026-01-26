package auth

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestGetUserIDFromContext(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected string
	}{
		{
			name: "valid user ID in context",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "user-123",
				},
			}),
			expected: "user-123",
		},
		{
			name:     "no claims in context",
			ctx:      context.Background(),
			expected: "",
		},
		{
			name:     "nil claims in context",
			ctx:      context.WithValue(context.Background(), ClaimsKey, (*Claims)(nil)),
			expected: "",
		},
		{
			name: "empty user ID in claims",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "",
				},
			}),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetUserIDFromContext(tt.ctx)
			if got != tt.expected {
				t.Errorf("GetUserIDFromContext() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGetProjectIDFromContext(t *testing.T) {
	validProjectID := uuid.New()
	tests := []struct {
		name     string
		ctx      context.Context
		expected uuid.UUID
	}{
		{
			name: "valid project ID in context",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				ProjectID: validProjectID.String(),
			}),
			expected: validProjectID,
		},
		{
			name:     "no claims in context",
			ctx:      context.Background(),
			expected: uuid.Nil,
		},
		{
			name:     "nil claims in context",
			ctx:      context.WithValue(context.Background(), ClaimsKey, (*Claims)(nil)),
			expected: uuid.Nil,
		},
		{
			name: "empty project ID in claims",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				ProjectID: "",
			}),
			expected: uuid.Nil,
		},
		{
			name: "invalid UUID format in project ID",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				ProjectID: "not-a-valid-uuid",
			}),
			expected: uuid.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetProjectIDFromContext(tt.ctx)
			if got != tt.expected {
				t.Errorf("GetProjectIDFromContext() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRequireUserIDFromContext(t *testing.T) {
	tests := []struct {
		name      string
		ctx       context.Context
		wantValue string
		wantErr   bool
	}{
		{
			name: "valid user ID in context",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "user-456",
				},
			}),
			wantValue: "user-456",
			wantErr:   false,
		},
		{
			name:      "no claims in context",
			ctx:       context.Background(),
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "nil claims in context",
			ctx:       context.WithValue(context.Background(), ClaimsKey, (*Claims)(nil)),
			wantValue: "",
			wantErr:   true,
		},
		{
			name: "empty user ID in claims",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "",
				},
			}),
			wantValue: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RequireUserIDFromContext(tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("RequireUserIDFromContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantValue {
				t.Errorf("RequireUserIDFromContext() = %q, want %q", got, tt.wantValue)
			}
		})
	}
}

func TestRequireProjectIDFromContext(t *testing.T) {
	validProjectID := uuid.New()
	tests := []struct {
		name      string
		ctx       context.Context
		wantValue uuid.UUID
		wantErr   bool
	}{
		{
			name: "valid project ID in context",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				ProjectID: validProjectID.String(),
			}),
			wantValue: validProjectID,
			wantErr:   false,
		},
		{
			name:      "no claims in context",
			ctx:       context.Background(),
			wantValue: uuid.Nil,
			wantErr:   true,
		},
		{
			name:      "nil claims in context",
			ctx:       context.WithValue(context.Background(), ClaimsKey, (*Claims)(nil)),
			wantValue: uuid.Nil,
			wantErr:   true,
		},
		{
			name: "empty project ID in claims",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				ProjectID: "",
			}),
			wantValue: uuid.Nil,
			wantErr:   true,
		},
		{
			name: "invalid UUID format returns error",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				ProjectID: "not-a-valid-uuid",
			}),
			wantValue: uuid.Nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RequireProjectIDFromContext(tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("RequireProjectIDFromContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantValue {
				t.Errorf("RequireProjectIDFromContext() = %v, want %v", got, tt.wantValue)
			}
		})
	}
}

func TestRequireClaimsFromContext(t *testing.T) {
	validProjectID := uuid.New()
	tests := []struct {
		name          string
		ctx           context.Context
		wantProjectID uuid.UUID
		wantUserID    string
		wantErr       bool
		errContains   string
	}{
		{
			name: "valid claims with both IDs",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				ProjectID: validProjectID.String(),
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "user-789",
				},
			}),
			wantProjectID: validProjectID,
			wantUserID:    "user-789",
			wantErr:       false,
		},
		{
			name:          "no claims in context",
			ctx:           context.Background(),
			wantProjectID: uuid.Nil,
			wantUserID:    "",
			wantErr:       true,
			errContains:   "no claims in context",
		},
		{
			name:          "nil claims in context",
			ctx:           context.WithValue(context.Background(), ClaimsKey, (*Claims)(nil)),
			wantProjectID: uuid.Nil,
			wantUserID:    "",
			wantErr:       true,
			errContains:   "no claims in context",
		},
		{
			name: "missing project ID",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				ProjectID: "",
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "user-789",
				},
			}),
			wantProjectID: uuid.Nil,
			wantUserID:    "",
			wantErr:       true,
			errContains:   "missing project ID",
		},
		{
			name: "missing user ID",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				ProjectID: validProjectID.String(),
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "",
				},
			}),
			wantProjectID: uuid.Nil,
			wantUserID:    "",
			wantErr:       true,
			errContains:   "missing user ID",
		},
		{
			name: "invalid project ID format",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				ProjectID: "not-a-uuid",
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "user-789",
				},
			}),
			wantProjectID: uuid.Nil,
			wantUserID:    "",
			wantErr:       true,
			errContains:   "invalid project ID format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotProjectID, gotUserID, err := RequireClaimsFromContext(tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("RequireClaimsFromContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !contains(err.Error(), tt.errContains) {
					t.Errorf("RequireClaimsFromContext() error = %v, want error containing %q", err, tt.errContains)
				}
			}
			if gotProjectID != tt.wantProjectID {
				t.Errorf("RequireClaimsFromContext() projectID = %v, want %v", gotProjectID, tt.wantProjectID)
			}
			if gotUserID != tt.wantUserID {
				t.Errorf("RequireClaimsFromContext() userID = %q, want %q", gotUserID, tt.wantUserID)
			}
		})
	}
}

func TestGetUserUUIDFromContext(t *testing.T) {
	validUserID := uuid.New()
	tests := []struct {
		name     string
		ctx      context.Context
		wantUUID uuid.UUID
		wantOK   bool
	}{
		{
			name: "valid UUID user ID in context",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: validUserID.String(),
				},
			}),
			wantUUID: validUserID,
			wantOK:   true,
		},
		{
			name:     "no claims in context",
			ctx:      context.Background(),
			wantUUID: uuid.Nil,
			wantOK:   false,
		},
		{
			name:     "nil claims in context",
			ctx:      context.WithValue(context.Background(), ClaimsKey, (*Claims)(nil)),
			wantUUID: uuid.Nil,
			wantOK:   false,
		},
		{
			name: "empty user ID in claims",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "",
				},
			}),
			wantUUID: uuid.Nil,
			wantOK:   false,
		},
		{
			name: "non-UUID user ID in claims",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "not-a-valid-uuid",
				},
			}),
			wantUUID: uuid.Nil,
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUUID, gotOK := GetUserUUIDFromContext(tt.ctx)
			if gotOK != tt.wantOK {
				t.Errorf("GetUserUUIDFromContext() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotUUID != tt.wantUUID {
				t.Errorf("GetUserUUIDFromContext() uuid = %v, want %v", gotUUID, tt.wantUUID)
			}
		})
	}
}

func TestRequireUserUUIDFromContext(t *testing.T) {
	validUserID := uuid.New()
	tests := []struct {
		name      string
		ctx       context.Context
		wantValue uuid.UUID
		wantErr   bool
	}{
		{
			name: "valid UUID user ID in context",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: validUserID.String(),
				},
			}),
			wantValue: validUserID,
			wantErr:   false,
		},
		{
			name:      "no claims in context",
			ctx:       context.Background(),
			wantValue: uuid.Nil,
			wantErr:   true,
		},
		{
			name:      "nil claims in context",
			ctx:       context.WithValue(context.Background(), ClaimsKey, (*Claims)(nil)),
			wantValue: uuid.Nil,
			wantErr:   true,
		},
		{
			name: "empty user ID in claims",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "",
				},
			}),
			wantValue: uuid.Nil,
			wantErr:   true,
		},
		{
			name: "non-UUID user ID in claims",
			ctx: context.WithValue(context.Background(), ClaimsKey, &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "not-a-valid-uuid",
				},
			}),
			wantValue: uuid.Nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RequireUserUUIDFromContext(tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("RequireUserUUIDFromContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantValue {
				t.Errorf("RequireUserUUIDFromContext() = %v, want %v", got, tt.wantValue)
			}
		})
	}
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
