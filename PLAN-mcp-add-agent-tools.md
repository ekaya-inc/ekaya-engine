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

### Step 5: Create Agent API Key Handler ✅ COMPLETED

**Files Created:**
- `pkg/handlers/agent_api_key.go` - Handler implementation
- `pkg/handlers/agent_api_key_test.go` - Unit tests

**Files Modified:**
- `main.go` - Wired up service and handler

**What Was Done:**
1. Created `AgentAPIKeyHandler` with `Get` and `Regenerate` endpoints
2. GET `/api/projects/{pid}/mcp/agent-key` - Returns masked key by default, full key with `?reveal=true`
3. POST `/api/projects/{pid}/mcp/agent-key/regenerate` - Generates new key, invalidates old
4. Auto-generates key if one doesn't exist on GET request
5. Added comprehensive unit tests covering:
   - Masked and revealed key responses
   - Auto-generation when key doesn't exist
   - Error handling for both endpoints
   - Invalid project ID validation
6. Wired up AgentAPIKeyService and AgentAPIKeyHandler in main.go

**Code Locations:**
- `pkg/handlers/agent_api_key.go:50-102` - Get handler with auto-generation and masking logic
- `pkg/handlers/agent_api_key.go:106-133` - Regenerate handler
- `pkg/handlers/agent_api_key.go:38-45` - RegisterRoutes with auth/tenant middleware
- `main.go:351-357` - Service and handler wiring

**Implementation Notes for Future Sessions:**
- The handler follows existing patterns from `pkg/handlers/mcp_config.go`
- Uses `ParseProjectID`, `ErrorResponse`, `WriteJSON`, and `ApiResponse` helper functions
- Response types defined: `GetAgentAPIKeyResponse` (key + masked bool) and `RegenerateAgentAPIKeyResponse` (key only)
- Routes use same auth/tenant middleware pattern as other protected endpoints
- Tests use a mock service implementing `services.AgentAPIKeyService` interface

**Pattern Reference:** `pkg/handlers/mcp_config.go` (handler structure and route registration)

---

### Step 6: Update MCP Authentication Middleware ✅ COMPLETED

**Files Modified:**
- `pkg/mcp/auth/middleware.go` - Extended middleware to support agent API key auth
- `pkg/mcp/auth/middleware_test.go` - Comprehensive tests for agent key auth
- `pkg/handlers/mcp_handler_test.go` - Updated mock to include agentKeyService parameter
- `pkg/handlers/mcp_integration_test.go` - Updated mock to include agentKeyService parameter
- `main.go` - Moved agentAPIKeyService creation before mcpAuthMiddleware

**What Was Done:**

1. **Extended Middleware struct** (`pkg/mcp/auth/middleware.go:23-27`):
   - Added `agentKeyService services.AgentAPIKeyService` field
   - Updated `NewMiddleware` to accept optional agentKeyService (can be nil)

2. **Updated RequireAuth** (`pkg/mcp/auth/middleware.go:46-68`):
   - Checks for API key authentication first (Authorization: api-key:xxx or X-API-Key header)
   - Falls through to JWT authentication if no API key provided
   - Refactored into separate handlers for clarity

3. **Added handleAgentKeyAuth** (`pkg/mcp/auth/middleware.go:71-136`):
   - Validates agentKeyService is configured (returns 500 if nil)
   - Extracts project ID from path param or falls back to URL path parsing
   - Validates API key via agentKeyService.ValidateKey()
   - Creates synthetic claims with `Subject = "agent"` marker
   - Injects claims into context (no token for agent auth)

4. **Added handleJWTAuth** (`pkg/mcp/auth/middleware.go:139-179`):
   - Extracted existing JWT logic into separate method
   - No functional changes to JWT flow

5. **Added extractProjectIDFromPath** (`pkg/mcp/auth/middleware.go:182-190`):
   - Parses project UUID from /mcp/{project-id} path format
   - Used as fallback when PathValue not set

6. **Comprehensive test coverage** (`pkg/mcp/auth/middleware_test.go:305-661`):
   - `TestMiddleware_RequireAuth_AgentAPIKey_AuthorizationHeader` - Authorization: api-key:xxx
   - `TestMiddleware_RequireAuth_AgentAPIKey_XAPIKeyHeader` - X-API-Key header
   - `TestMiddleware_RequireAuth_AgentAPIKey_InvalidKey` - Wrong key rejected
   - `TestMiddleware_RequireAuth_AgentAPIKey_NoKeyConfigured` - No key for project
   - `TestMiddleware_RequireAuth_AgentAPIKey_InvalidProjectID` - Invalid UUID format
   - `TestMiddleware_RequireAuth_AgentAPIKey_ServiceError` - DB error handling
   - `TestMiddleware_RequireAuth_AgentAPIKey_NoService` - Nil service handling
   - `TestMiddleware_RequireAuth_AgentAPIKey_ExtractProjectFromPath` - Fallback path parsing
   - `TestExtractProjectIDFromPath` - Path extraction edge cases

7. **Wiring in main.go** (`main.go:135-140, 318`):
   - Moved agentAPIKeyService creation earlier (before MCP handler setup)
   - Passed agentKeyService to mcpauth.NewMiddleware()

**Code Locations:**
- `pkg/mcp/auth/middleware.go:23-27` - Middleware struct with agentKeyService
- `pkg/mcp/auth/middleware.go:30-37` - NewMiddleware constructor
- `pkg/mcp/auth/middleware.go:71-136` - handleAgentKeyAuth method
- `pkg/mcp/auth/middleware.go:124-128` - Synthetic claims for agent (Subject = "agent")
- `main.go:318` - mcpAuthMiddleware wiring with agentAPIKeyService

**Key Implementation Notes for Future Sessions:**
- Agent auth uses `claims.Subject = "agent"` to identify agent vs user authentication
- Tool filtering (Step 7) should check `claims.Subject == "agent"` to apply agent restrictions
- API key checked BEFORE JWT to prioritize explicit agent authentication
- All RFC 6750 error responses maintained for both auth methods

---

### Step 7: Update Tool Filtering for Agent Tools ✅ COMPLETED

**Files Modified:**
- `pkg/mcp/tools/developer.go` - Updated NewToolFilter and added filterAgentTools function
- `pkg/mcp/tools/developer_filter_test.go` - Added comprehensive tests for agent tool filtering

**What Was Done:**

1. **Added agentToolNames map** (`pkg/mcp/tools/developer.go:82-87`):
   - Lists tools available to agents: `list_approved_queries` and `execute_approved_query`
   - Agents can only access approved_queries tools, not developer tools

2. **Updated NewToolFilter** (`pkg/mcp/tools/developer.go:134-151`):
   - Checks `claims.Subject == "agent"` to identify agent authentication (set by MCP auth middleware)
   - If agent auth, checks if `agent_tools` is enabled via `MCPConfigService.IsToolGroupEnabled`
   - Calls `filterAgentTools` for agent-specific filtering
   - Falls through to existing user tool filtering logic for non-agent auth

3. **Added filterAgentTools function** (`pkg/mcp/tools/developer.go:235-258`):
   - Health tool always available (consistent with user auth)
   - When `agent_tools` disabled: only health tool available
   - When `agent_tools` enabled: health + approved_queries tools (list_approved_queries, execute_approved_query)
   - Developer tools, schema tools, and ontology tools are never available to agents

4. **Comprehensive test coverage** (`pkg/mcp/tools/developer_filter_test.go`):
   - `TestFilterAgentTools_Disabled` - Unit test for filter function with disabled flag
   - `TestFilterAgentTools_Enabled` - Unit test for filter function with enabled flag
   - `TestNewToolFilter_AgentAuth_AgentToolsEnabled` - Integration test with agent_tools enabled
   - `TestNewToolFilter_AgentAuth_AgentToolsDisabled` - Integration test with agent_tools disabled
   - `TestNewToolFilter_AgentAuth_NoConfig` - Integration test with no config (defaults to disabled)
   - `TestNewToolFilter_UserAuth_AgentToolsEnabledDoesNotAffectUsers` - Verifies user auth is unaffected by agent_tools config

**Key Implementation Notes for Future Sessions:**
- Agent authentication is identified by `claims.Subject == "agent"` (set in `pkg/mcp/auth/middleware.go:127`)
- The plan mentioned checking `claims.UserID == "agent"` but the actual implementation uses `claims.Subject`
- Agent tool filtering is completely separate from user tool filtering - agents cannot access developer tools regardless of developer tool group settings

**Code Locations:**
- `pkg/mcp/tools/developer.go:82-87` - agentToolNames map
- `pkg/mcp/tools/developer.go:134-151` - Agent auth handling in NewToolFilter
- `pkg/mcp/tools/developer.go:235-258` - filterAgentTools function

**Original Plan File Reference:**

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
