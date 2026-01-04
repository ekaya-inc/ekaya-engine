# PLAN: Add Agent Tools to MCP Server Configuration

## Overview

Add a new "Agent Tools" section to the MCP Server configuration page, positioned between "Business User Tools" and "Developer Tools". This enables AI Agents to access the database safely and securely through Pre-Approved Queries only.

**Key Design Principles:**
- One Agent API Key per project (regenerable)
- Keys encrypted at rest using PROJECT_CREDENTIALS_KEY (same pattern as datasource credentials)
- Agent authentication is separate from user JWT authentication
- Agents can only access approved_queries tools (NOT developer tools)
- Follow existing patterns: models → repositories → services → handlers → UI

---

## Implementation Steps

### Step 1: Database Migration ✅ COMPLETED

**Files Created:**
- `migrations/026_agent_api_keys.up.sql` - Adds `agent_api_key_encrypted` TEXT column to `engine_mcp_config` table with partial index
- `migrations/026_agent_api_keys.down.sql` - Removes the column and index

**Implementation Notes:**
- Column is nullable (no default) - keys are generated on-demand when agent tools are enabled
- Partial index only includes non-null values for efficient lookups
- Follows existing pattern from `migrations/003_datasources.up.sql` for encrypted credential storage

---

### Step 2: Update Models ✅ COMPLETED

**Files Modified:**
- `pkg/models/mcp_config.go` - Added `AgentAPIKeyEncrypted` field to `MCPConfig` struct
- `pkg/services/mcp_config.go` - Added `ToolGroupAgentTools` constant and registered in `validToolGroups`

**Implementation Notes:**
- `AgentAPIKeyEncrypted` uses `json:"-"` tag to prevent serialization (never exposed via API)
- `ToolGroupAgentTools` constant defined as `"agent_tools"` for consistency with existing patterns
- Both `ToolGroupApprovedQueries` and `ToolGroupAgentTools` constants are now exported for use by other packages

**Code Locations:**
- `pkg/models/mcp_config.go:25` - AgentAPIKeyEncrypted field
- `pkg/services/mcp_config.go:18-19` - ToolGroupAgentTools constant
- `pkg/services/mcp_config.go:23-27` - validToolGroups map with agent_tools entry

---

### Step 3: Update Repository [x] COMPLETED

**Files Modified:**
- `pkg/repositories/mcp_config_repository.go` - Extended interface and implementation for agent API key storage
- `pkg/services/mcp_config_test.go` - Updated mock to implement new interface methods

**What Was Done:**
1. Extended `MCPConfigRepository` interface with:
   - `GetAgentAPIKey(ctx, projectID) (string, error)` - retrieves encrypted key
   - `SetAgentAPIKey(ctx, projectID, encryptedKey) error` - stores encrypted key

2. Updated `Get` method:
   - Added `agent_api_key_encrypted` to SELECT query
   - Scans nullable column via `*string` intermediate variable
   - Copies to `config.AgentAPIKeyEncrypted` when non-nil

3. Updated `Upsert` method:
   - Includes `agent_api_key_encrypted` in INSERT/UPDATE
   - Converts empty string to nil for nullable column storage

4. Added `GetAgentAPIKey` method:
   - Returns empty string (not error) when key doesn't exist or is null
   - Uses nullable `*string` scan to handle NULL values

5. Added `SetAgentAPIKey` method:
   - Uses UPSERT pattern with default tool_groups for INSERT case
   - Only updates `agent_api_key_encrypted` and `updated_at` on conflict

6. Updated mock in test file:
   - Added `agentAPIKeyByProject map[uuid.UUID]string` field
   - Implemented `GetAgentAPIKey` and `SetAgentAPIKey` methods

**Code Locations:**
- `pkg/repositories/mcp_config_repository.go:24-26` - New interface methods
- `pkg/repositories/mcp_config_repository.go:44-47` - Get query with new column
- `pkg/repositories/mcp_config_repository.go:51-55,66-68` - Get scan with nullable handling
- `pkg/repositories/mcp_config_repository.go:90-103` - Upsert with agent key column
- `pkg/repositories/mcp_config_repository.go:116-160` - New GetAgentAPIKey and SetAgentAPIKey methods
- `pkg/services/mcp_config_test.go:19-47` - Updated mock implementation

---

### Step 4: Create Agent API Key Service [x] COMPLETED

**Files Created:**
- `pkg/services/agent_api_key.go` - Service interface and implementation
- `pkg/services/agent_api_key_test.go` - Comprehensive unit tests

**What Was Implemented:**

1. **AgentAPIKeyService interface** (`pkg/services/agent_api_key.go:18-31`):
   - `GenerateKey(ctx, projectID) (string, error)` - Creates 32-byte random key, encrypts and stores
   - `GetKey(ctx, projectID) (string, error)` - Retrieves and decrypts stored key
   - `RegenerateKey(ctx, projectID) (string, error)` - Generates new key (overwrites old)
   - `ValidateKey(ctx, projectID, providedKey) (bool, error)` - Validates key with constant-time comparison

2. **Service implementation** (`pkg/services/agent_api_key.go:33-135`):
   - Uses `crypto.CredentialEncryptor` with `PROJECT_CREDENTIALS_KEY` env var (same as datasource credentials)
   - Keys are 32 random bytes encoded as 64-character hex strings
   - Uses `crypto/subtle.ConstantTimeCompare` for validation (prevents timing attacks)
   - Follows existing service patterns: interface + implementation, compile-time interface check

3. **Comprehensive unit tests** (`pkg/services/agent_api_key_test.go`):
   - `TestAgentAPIKeyService_GenerateKey` - Verifies 64 hex chars, encrypted storage
   - `TestAgentAPIKeyService_GenerateKey_Unique` - Verifies cryptographic uniqueness
   - `TestAgentAPIKeyService_GetKey` - Roundtrip encryption/decryption
   - `TestAgentAPIKeyService_GetKey_NotExists` - Returns empty string (not error) for missing
   - `TestAgentAPIKeyService_RegenerateKey` - Verifies old key invalidated
   - `TestAgentAPIKeyService_ValidateKey_Valid` - Correct key validates
   - `TestAgentAPIKeyService_ValidateKey_Invalid` - Wrong key rejected
   - `TestAgentAPIKeyService_ValidateKey_NoKey` - Returns false (not error) when no key
   - `TestAgentAPIKeyService_ValidateKey_AfterRegenerate` - Old key fails, new key works
   - `TestAgentAPIKeyService_NewService_MissingEnvVar` - Error when env var missing
   - `TestAgentAPIKeyService_NewService_InvalidKey` - Error when key invalid

**Code Locations:**
- `pkg/services/agent_api_key.go:18-31` - AgentAPIKeyService interface
- `pkg/services/agent_api_key.go:39-60` - NewAgentAPIKeyService constructor
- `pkg/services/agent_api_key.go:63-88` - GenerateKey implementation
- `pkg/services/agent_api_key.go:118-132` - ValidateKey with constant-time comparison

**Pattern References:**
- `pkg/crypto/credentials.go` - Encryption pattern
- `pkg/services/mcp_config.go` - Service structure pattern

---

### Step 5: Create Agent API Key Handler

**File:** `pkg/handlers/agent_api_key.go`

```go
package handlers

import (
    "encoding/json"
    "net/http"

    "go.uber.org/zap"

    "github.com/ekaya-inc/ekaya-engine/pkg/auth"
    "github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// GetAgentAPIKeyResponse is the response for GET /api/projects/{pid}/mcp/agent-key
type GetAgentAPIKeyResponse struct {
    Key    string `json:"key"`    // Masked or full key depending on ?reveal=true
    Masked bool   `json:"masked"` // Whether key is masked
}

// RegenerateAgentAPIKeyResponse is the response for POST /api/projects/{pid}/mcp/agent-key/regenerate
type RegenerateAgentAPIKeyResponse struct {
    Key string `json:"key"` // New unmasked key
}

// AgentAPIKeyHandler handles agent API key HTTP requests.
type AgentAPIKeyHandler struct {
    agentKeyService services.AgentAPIKeyService
    logger          *zap.Logger
}

// NewAgentAPIKeyHandler creates a new agent API key handler.
func NewAgentAPIKeyHandler(agentKeyService services.AgentAPIKeyService, logger *zap.Logger) *AgentAPIKeyHandler {
    return &AgentAPIKeyHandler{
        agentKeyService: agentKeyService,
        logger:          logger,
    }
}

// RegisterRoutes registers the agent API key handler's routes on the given mux.
func (h *AgentAPIKeyHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
    keyBase := "/api/projects/{pid}/mcp/agent-key"

    mux.HandleFunc("GET "+keyBase,
        authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Get)))
    mux.HandleFunc("POST "+keyBase+"/regenerate",
        authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Regenerate)))
}

// Get handles GET /api/projects/{pid}/mcp/agent-key
func (h *AgentAPIKeyHandler) Get(w http.ResponseWriter, r *http.Request) {
    projectID, ok := ParseProjectID(w, r, h.logger)
    if !ok {
        return
    }

    key, err := h.agentKeyService.GetKey(r.Context(), projectID)
    if err != nil {
        h.logger.Error("Failed to get agent API key",
            zap.String("project_id", projectID.String()),
            zap.Error(err))
        if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get agent API key"); err != nil {
            h.logger.Error("Failed to write error response", zap.Error(err))
        }
        return
    }

    // Generate key if it doesn't exist
    if key == "" {
        key, err = h.agentKeyService.GenerateKey(r.Context(), projectID)
        if err != nil {
            h.logger.Error("Failed to generate agent API key",
                zap.String("project_id", projectID.String()),
                zap.Error(err))
            if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to generate agent API key"); err != nil {
                h.logger.Error("Failed to write error response", zap.Error(err))
            }
            return
        }
    }

    // Check if ?reveal=true query parameter is present
    reveal := r.URL.Query().Get("reveal") == "true"

    responseKey := key
    masked := false
    if !reveal {
        responseKey = "****"
        masked = true
    }

    response := ApiResponse{
        Success: true,
        Data: GetAgentAPIKeyResponse{
            Key:    responseKey,
            Masked: masked,
        },
    }

    if err := WriteJSON(w, http.StatusOK, response); err != nil {
        h.logger.Error("Failed to write response", zap.Error(err))
    }
}

// Regenerate handles POST /api/projects/{pid}/mcp/agent-key/regenerate
func (h *AgentAPIKeyHandler) Regenerate(w http.ResponseWriter, r *http.Request) {
    projectID, ok := ParseProjectID(w, r, h.logger)
    if !ok {
        return
    }

    newKey, err := h.agentKeyService.RegenerateKey(r.Context(), projectID)
    if err != nil {
        h.logger.Error("Failed to regenerate agent API key",
            zap.String("project_id", projectID.String()),
            zap.Error(err))
        if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to regenerate agent API key"); err != nil {
            h.logger.Error("Failed to write error response", zap.Error(err))
        }
        return
    }

    response := ApiResponse{
        Success: true,
        Data: RegenerateAgentAPIKeyResponse{
            Key: newKey,
        },
    }

    if err := WriteJSON(w, http.StatusOK, response); err != nil {
        h.logger.Error("Failed to write response", zap.Error(err))
    }
}
```

**Pattern reference:** `pkg/handlers/mcp_config.go` (handler structure and route registration)

**Wire up in:** `cmd/server/main.go` (add to handler initialization and route registration)

---

### Step 6: Update MCP Authentication Middleware

**File:** `pkg/mcp/auth/middleware.go` (modify existing file)

Add agent API key authentication path alongside existing JWT authentication:

```go
// Authenticate middleware extracts and validates either JWT or Agent API Key authentication.
// It supports two authentication methods:
// 1. JWT (existing): Authorization: Bearer <jwt>
// 2. Agent API Key: Authorization: api-key:<key> OR X-API-Key: <key>
func (m *Middleware) Authenticate(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()

        // Try JWT authentication first
        authHeader := r.Header.Get("Authorization")
        if strings.HasPrefix(authHeader, "Bearer ") {
            // Existing JWT path...
            token := strings.TrimPrefix(authHeader, "Bearer ")
            claims, err := m.jwtValidator.Validate(ctx, token)
            if err != nil {
                http.Error(w, "Invalid token", http.StatusUnauthorized)
                return
            }
            ctx = auth.WithClaims(ctx, claims)
            next.ServeHTTP(w, r.WithContext(ctx))
            return
        }

        // Try Agent API Key authentication
        apiKey := ""
        if strings.HasPrefix(authHeader, "api-key:") {
            apiKey = strings.TrimPrefix(authHeader, "api-key:")
        } else {
            apiKey = r.Header.Get("X-API-Key")
        }

        if apiKey != "" {
            // Extract project ID from URL path: /mcp/{project-id}
            projectID, err := extractProjectIDFromPath(r.URL.Path)
            if err != nil {
                http.Error(w, "Invalid project ID in path", http.StatusBadRequest)
                return
            }

            // Validate API key
            valid, err := m.agentKeyService.ValidateKey(ctx, projectID, apiKey)
            if err != nil {
                m.logger.Error("Failed to validate agent API key", zap.Error(err))
                http.Error(w, "Authentication failed", http.StatusInternalServerError)
                return
            }
            if !valid {
                http.Error(w, "Invalid API key", http.StatusUnauthorized)
                return
            }

            // Create synthetic claims for agent context
            claims := &auth.Claims{
                ProjectID: projectID,
                UserID:    "agent", // Special marker for agent authentication
                Email:     "agent@system",
                Name:      "Agent",
            }

            ctx = auth.WithClaims(ctx, claims)
            next.ServeHTTP(w, r.WithContext(ctx))
            return
        }

        // No valid authentication found
        http.Error(w, "Missing or invalid authentication", http.StatusUnauthorized)
    })
}

// extractProjectIDFromPath extracts project UUID from /mcp/{project-id} path.
func extractProjectIDFromPath(path string) (uuid.UUID, error) {
    // Expected format: /mcp/{project-id} or /mcp/{project-id}/...
    parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
    if len(parts) < 2 || parts[0] != "mcp" {
        return uuid.Nil, fmt.Errorf("invalid path format")
    }

    return uuid.Parse(parts[1])
}
```

**Dependencies:** Add `agentKeyService services.AgentAPIKeyService` to `Middleware` struct

---

### Step 7: Update Tool Filtering for Agent Tools

**File:** `pkg/mcp/tools/developer.go`

Update `NewToolFilter` function to handle agent_tools group:

```go
// NewToolFilter creates a ToolFilterFunc that filters tools based on MCP configuration.
func NewToolFilter(deps *DeveloperToolDeps) func(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
    return func(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
        // Get claims from context
        claims, ok := auth.GetClaims(ctx)
        if !ok {
            return []mcp.Tool{} // No auth, no tools
        }

        projectID := claims.ProjectID

        // Check if this is agent authentication
        isAgent := claims.UserID == "agent"

        // If agent_tools enabled, filter to only approved_queries tools
        if isAgent {
            agentEnabled, err := deps.MCPConfigService.IsToolGroupEnabled(ctx, projectID, services.ToolGroupAgentTools)
            if err != nil || !agentEnabled {
                return []mcp.Tool{} // Agent tools disabled
            }

            // Filter to only approved_queries tools
            filtered := []mcp.Tool{}
            for _, tool := range tools {
                if approvedQueryToolNames[tool.Name] {
                    filtered = append(filtered, tool)
                }
            }
            return filtered
        }

        // Existing logic for user authentication...
        // (developer tools, approved_queries, ontology tools filtering)
    }
}
```

Add approved query tool names map:

```go
// approvedQueryToolNames lists tools available with approved_queries or agent_tools.
var approvedQueryToolNames = map[string]bool{
    "list_approved_queries":   true,
    "execute_approved_query":  true,
}
```

**Pattern reference:** `pkg/mcp/tools/developer.go:96-147`

---

### Step 8: Frontend - Add UI Metadata

**File:** `ui/src/constants/mcpToolMetadata.ts`

Add to `TOOL_GROUP_IDS`:

```typescript
export const TOOL_GROUP_IDS = {
  DEVELOPER: 'developer',
  APPROVED_QUERIES: 'approved_queries',
  AGENT_TOOLS: 'agent_tools', // Add this
} as const;
```

Add to `TOOL_GROUP_METADATA`:

```typescript
[TOOL_GROUP_IDS.AGENT_TOOLS]: {
  name: 'Agent Tools',
  description:
    'Enable AI Agents to access the database safely and securely with logging and auditing capabilities. AI Agents can only use the enabled Pre-Approved Queries so that you have full control over access.',
  warning: 'Agent access requires API key authentication. Generate and distribute keys carefully.',
},
```

**Pattern reference:** `ui/src/constants/mcpToolMetadata.ts:19-56`

---

### Step 9: Frontend - Add API Methods

**File:** `ui/src/services/engineApi.ts`

Add methods:

```typescript
/**
 * Get agent API key for a project
 */
async getAgentAPIKey(
  projectId: string,
  reveal: boolean = false
): Promise<ApiResponse<{ key: string; masked: boolean }>> {
  const query = reveal ? '?reveal=true' : '';
  return this.makeRequest<{ key: string; masked: boolean }>(
    `/${projectId}/mcp/agent-key${query}`,
    { method: 'GET' }
  );
}

/**
 * Regenerate agent API key for a project
 */
async regenerateAgentAPIKey(
  projectId: string
): Promise<ApiResponse<{ key: string }>> {
  return this.makeRequest<{ key: string }>(
    `/${projectId}/mcp/agent-key/regenerate`,
    { method: 'POST' }
  );
}
```

**Pattern reference:** `ui/src/services/engineApi.ts:90-100`

---

### Step 10: Frontend - Create Agent API Key Display Component

**File:** `ui/src/components/mcp/AgentAPIKeyDisplay.tsx`

```typescript
import { Copy, RefreshCw } from 'lucide-react';
import { useEffect, useState } from 'react';

import { Button } from '../ui/Button';
import { Input } from '../ui/Input';
import { useToast } from '../../hooks/useToast';
import engineApi from '../../services/engineApi';

interface AgentAPIKeyDisplayProps {
  projectId: string;
}

export const AgentAPIKeyDisplay = ({ projectId }: AgentAPIKeyDisplayProps) => {
  const { toast } = useToast();
  const [key, setKey] = useState<string>('****');
  const [masked, setMasked] = useState(true);
  const [loading, setLoading] = useState(true);
  const [regenerating, setRegenerating] = useState(false);

  // Fetch initial key (masked)
  useEffect(() => {
    const fetchKey = async () => {
      try {
        setLoading(true);
        const response = await engineApi.getAgentAPIKey(projectId, false);
        if (response.success && response.data) {
          setKey(response.data.key);
          setMasked(response.data.masked);
        }
      } catch (error) {
        console.error('Failed to fetch agent API key:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchKey();
  }, [projectId]);

  // Reveal key on focus
  const handleFocus = async (e: React.FocusEvent<HTMLInputElement>) => {
    if (masked) {
      try {
        const response = await engineApi.getAgentAPIKey(projectId, true);
        if (response.success && response.data) {
          setKey(response.data.key);
          setMasked(false);
          // Auto-select text
          e.target.select();
        }
      } catch (error) {
        console.error('Failed to reveal agent API key:', error);
      }
    } else {
      // Already revealed, just select
      e.target.select();
    }
  };

  // Copy to clipboard
  const handleCopy = async () => {
    try {
      // Fetch unmasked key if needed
      let keyToCopy = key;
      if (masked) {
        const response = await engineApi.getAgentAPIKey(projectId, true);
        if (response.success && response.data) {
          keyToCopy = response.data.key;
        }
      }

      await navigator.clipboard.writeText(keyToCopy);
      toast({
        title: 'Success',
        description: 'Agent API key copied to clipboard',
      });
    } catch (error) {
      toast({
        title: 'Error',
        description: 'Failed to copy API key',
        variant: 'destructive',
      });
    }
  };

  // Regenerate key
  const handleRegenerate = async () => {
    const confirmed = window.confirm(
      'This will reset the API key. All previously configured Agents will fail to authenticate.'
    );

    if (!confirmed) return;

    try {
      setRegenerating(true);
      const response = await engineApi.regenerateAgentAPIKey(projectId);
      if (response.success && response.data) {
        setKey(response.data.key);
        setMasked(false);
        toast({
          title: 'Success',
          description: 'Agent API key regenerated',
        });
      }
    } catch (error) {
      toast({
        title: 'Error',
        description: 'Failed to regenerate API key',
        variant: 'destructive',
      });
    } finally {
      setRegenerating(false);
    }
  };

  if (loading) {
    return <div className="text-sm text-gray-500">Loading...</div>;
  }

  return (
    <div className="space-y-2">
      <label className="text-sm font-medium">Agent API Key</label>
      <div className="flex items-center gap-2">
        <Input
          type="text"
          value={key}
          onFocus={handleFocus}
          readOnly
          className="font-mono text-sm"
        />
        <Button
          size="icon"
          variant="outline"
          onClick={handleCopy}
          title="Copy to clipboard"
        >
          <Copy className="h-4 w-4" />
        </Button>
        <Button
          size="icon"
          variant="outline"
          onClick={handleRegenerate}
          disabled={regenerating}
          title="Regenerate key"
        >
          <RefreshCw className={`h-4 w-4 ${regenerating ? 'animate-spin' : ''}`} />
        </Button>
      </div>
      <p className="text-xs text-gray-500">
        Click the key to reveal. Use this key for agent authentication.
      </p>
    </div>
  );
};
```

---

### Step 11: Frontend - Update MCP Server Page

**File:** `ui/src/pages/MCPServerPage.tsx`

Add Agent Tools section between Business User Tools (approved_queries) and Developer Tools:

1. Import component:

```typescript
import { AgentAPIKeyDisplay } from '../components/mcp/AgentAPIKeyDisplay';
```

2. Add section rendering (insert after approved_queries section, before developer section):

```typescript
{/* Agent Tools Section */}
<MCPToolGroup
  groupId={TOOL_GROUP_IDS.AGENT_TOOLS}
  state={config.toolGroups[TOOL_GROUP_IDS.AGENT_TOOLS]}
  onToggle={handleToggleToolGroup}
  onToggleSubOption={handleToggleSubOption}
  disabled={updating}
  additionalInfo={
    config.toolGroups[TOOL_GROUP_IDS.AGENT_TOOLS]?.enabled ? (
      <div className="mt-4 p-4 bg-gray-50 rounded-md">
        <AgentAPIKeyDisplay projectId={pid!} />
      </div>
    ) : null
  }
/>
```

**Pattern reference:** `ui/src/pages/MCPServerPage.tsx:33-150` (existing tool group rendering)

---

### Step 12: Update TypeScript Types

**File:** `ui/src/types/index.ts` (or types file)

Ensure `ToolGroupState` includes agent_tools:

```typescript
export interface MCPConfigResponse {
  serverUrl: string;
  toolGroups: {
    developer?: ToolGroupState;
    approved_queries?: ToolGroupState;
    agent_tools?: ToolGroupState; // Add this
  };
}
```

---

## Testing Strategy

### Unit Tests

1. **Service tests:** `pkg/services/agent_api_key_test.go`
   - Test key generation (32 bytes, 64 hex chars)
   - Test encryption/decryption roundtrip
   - Test regeneration invalidates old key
   - Test validation (valid, invalid, missing key)

2. **Handler tests:** `pkg/handlers/agent_api_key_test.go`
   - Test GET endpoint (masked vs revealed)
   - Test POST regenerate endpoint
   - Test authentication requirements

3. **Repository tests:** `pkg/repositories/mcp_config_repository_test.go`
   - Test GetAgentAPIKey (existing, missing)
   - Test SetAgentAPIKey (insert, update)
   - Test that agent_api_key_encrypted is properly handled in Get/Upsert

### Integration Tests

1. **MCP auth middleware:** `pkg/mcp/auth/middleware_test.go`
   - Test JWT authentication (existing)
   - Test API key authentication (Bearer api-key:xxx)
   - Test API key authentication (X-API-Key header)
   - Test invalid API key rejection
   - Test missing API key rejection
   - Test project ID extraction from path

2. **Tool filtering:** `pkg/mcp/tools/developer_test.go`
   - Test agent authentication only sees approved_queries tools
   - Test user authentication sees all enabled tools
   - Test agent_tools disabled = no tools for agents

### Manual Testing

1. **UI workflow:**
   - Navigate to `/projects/{pid}/mcp-server`
   - Enable Agent Tools
   - Verify API key display (masked by default)
   - Click key to reveal
   - Click copy button
   - Click regenerate, confirm dialog
   - Verify new key displayed

2. **MCP authentication:**
   - Use MCP client with `Authorization: api-key:<key>` header
   - Verify connection succeeds
   - Use MCP client with `X-API-Key: <key>` header
   - Verify connection succeeds
   - List tools, verify only approved_queries tools appear
   - Regenerate key, verify old key fails authentication

---

## Security Considerations

1. **Encryption at rest:** Keys encrypted with PROJECT_CREDENTIALS_KEY (AES-256-GCM)
2. **Constant-time comparison:** Use `==` (Go's string comparison is not constant-time, consider using `crypto/subtle.ConstantTimeCompare` for production)
3. **Key regeneration:** Immediately invalidates all previous keys
4. **Scoped access:** Agents can ONLY access approved_queries tools, NOT developer tools
5. **Audit logging:** All MCP requests logged (existing infrastructure)
6. **Key distribution:** Keys revealed only on explicit user action (focus or reveal=true)

---

## Key Patterns from Existing Code

### Encryption Pattern
From `pkg/crypto/credentials.go:20-99`:
- Use `crypto.CredentialEncryptor` with `PROJECT_CREDENTIALS_KEY` env var
- `Encrypt(plaintext) → base64(nonce || ciphertext || tag)`
- `Decrypt(encrypted) → plaintext`

### Repository Pattern
From `pkg/repositories/mcp_config_repository.go`:
- Interface in same file as implementation
- Use `database.GetTenantScope(ctx)` for RLS
- Return `nil, nil` for "not found" (not an error)
- Use `pgx.ErrNoRows` check

### Handler Pattern
From `pkg/handlers/mcp_config.go`:
- `RegisterRoutes(mux, authMiddleware, tenantMiddleware)`
- Use `ParseProjectID(w, r, logger)` helper
- Use `ErrorResponse(w, statusCode, errorCode, message)`
- Use `WriteJSON(w, statusCode, ApiResponse{Success: true, Data: ...})`

### Service Pattern
From `pkg/services/mcp_config.go`:
- Interface + implementation in same file
- Compile-time check: `var _ Interface = (*implementation)(nil)`
- Logger in struct, log errors at ERROR level
- Return errors immediately (fail fast)

### UI Pattern
From `ui/src/constants/mcpToolMetadata.ts`:
- Tool group IDs as const object
- Metadata separate from state (backend returns state, UI provides display text)

---

## Implementation Order

1. Database migration (026_agent_api_keys.up.sql)
2. Update models (mcp_config.go constants)
3. Update repository (mcp_config_repository.go)
4. Create service (agent_api_key.go)
5. Create handler (agent_api_key.go)
6. Wire handler in main.go
7. Update MCP auth middleware
8. Update tool filtering (developer.go)
9. Frontend: UI metadata (mcpToolMetadata.ts)
10. Frontend: API methods (engineApi.ts)
11. Frontend: Display component (AgentAPIKeyDisplay.tsx)
12. Frontend: Page integration (MCPServerPage.tsx)
13. Write tests

**Estimated effort:** 4-6 hours for a developer familiar with the codebase.
