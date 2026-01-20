# FIX: Tab-Scoped JWT Storage for Multi-Tab Project Support

## Problem

When users work with multiple projects in different browser tabs, switching between tabs causes an infinite re-authentication loop. Each tab overwrites the shared JWT cookie, causing the other tab's project ID to mismatch.

### Reproduction Steps
1. Open Project1 in Tab1: `http://localhost:3443/projects/{pid1}?auth_url=http://localhost:5002`
2. Open Project2 in Tab2: `http://localhost:3443/projects/{pid2}?auth_url=http://localhost:5002`
3. Navigate within Tab1 (e.g., click a datasource, then Back)
4. Switch to Tab2 and navigate
5. **Bug**: Continuous re-authentication loop between tabs

### Root Cause

The JWT is stored in an **httpOnly cookie** (`ekaya_jwt`) with `Path: "/"`, meaning it's shared across all browser tabs. Each project requires a JWT with that project's ID in the claims. When Tab2 authenticates, it overwrites Tab1's JWT, causing Tab1 to get 403 (project mismatch) on its next API call.

```
Tab1 (Project1): Gets JWT with project_id: Project1 → stored in cookie
Tab2 (Project2): Gets JWT with project_id: Project2 → OVERWRITES same cookie
Tab1 API call:   JWT says Project2, URL says Project1 → 403 mismatch → re-auth
Tab1 re-auths:   Gets JWT with project_id: Project1 → OVERWRITES cookie
Tab2 API call:   JWT says Project1, URL says Project2 → 403 mismatch → re-auth
... infinite loop ...
```

### User Impact
- Users cannot work on multiple projects simultaneously in different tabs
- Constant re-authentication disrupts workflow
- Tab-scoped project isolation is broken

---

## Solution: Adopt ekaya-central's Tab-Scoped JWT Pattern

**ekaya-central already solved this problem** using `sessionStorage` (which is inherently per-tab) instead of cookies. ekaya-engine should adopt the same pattern.

### Architecture Comparison

| Aspect | ekaya-central (correct) | ekaya-engine (broken) |
|--------|------------------------|----------------------|
| JWT Storage | `sessionStorage` (per-tab) | httpOnly cookie (shared) |
| JWT Transmission | `Authorization: Bearer` header | Cookie auto-sent |
| Token Retrieval | `getProjectToken()` from sessionStorage | `r.Cookie("ekaya_jwt")` |

### Reference Implementation

**ekaya-central's auth-token.ts** (`../ekaya-central/src/lib/auth-token.ts`):
```typescript
const STORAGE_KEYS = {
  JWT: 'ekaya_jwt',
  PROJECT_ID: 'ekaya_project_id',
} as const;

export function storeProjectToken(jwt: string, projectId: string): void {
  sessionStorage.setItem(STORAGE_KEYS.JWT, jwt);
  sessionStorage.setItem(STORAGE_KEYS.PROJECT_ID, projectId);
}

export function getProjectToken(): string | null {
  return sessionStorage.getItem(STORAGE_KEYS.JWT);
}

export function clearProjectToken(): void {
  sessionStorage.removeItem(STORAGE_KEYS.JWT);
  sessionStorage.removeItem(STORAGE_KEYS.PROJECT_ID);
}
```

**ekaya-central's api-client.ts** - sends Bearer token:
```typescript
const authToken = token || getProjectToken();
const headers: HeadersInit = {
  Authorization: `Bearer ${authToken}`,
  'Content-Type': 'application/json',
};
```

---

## Implementation Plan

### Task 1: Create Frontend Auth Token Utilities ✅

**Status: COMPLETE**

**Files created:**
- `ui/src/lib/auth-token.ts` - Tab-scoped JWT storage utilities
- `ui/src/lib/auth-token.test.ts` - Comprehensive unit tests (13 tests covering all functions)

**Implementation notes:**
- Uses sessionStorage (inherently tab-scoped) instead of cookies
- Includes token expiry checking with 1-minute buffer
- All functions are pure and side-effect free (easy to test)
- Test coverage includes edge cases: malformed JWTs, expired tokens, missing claims
- Pattern matches ekaya-central's implementation exactly

**New file: `ui/src/lib/auth-token.ts`**

Create tab-scoped JWT storage utilities matching ekaya-central's pattern:

```typescript
/**
 * Tab-scoped JWT authentication utilities
 *
 * Uses sessionStorage (inherently tab-scoped) to store project tokens,
 * enabling users to work on different projects in different browser tabs.
 */

const STORAGE_KEYS = {
  JWT: 'ekaya_jwt',
  PROJECT_ID: 'ekaya_project_id',
} as const;

const TOKEN_EXPIRY_BUFFER_MS = 60 * 1000; // 1 minute

export function storeProjectToken(jwt: string, projectId: string): void {
  sessionStorage.setItem(STORAGE_KEYS.JWT, jwt);
  sessionStorage.setItem(STORAGE_KEYS.PROJECT_ID, projectId);
}

export function getProjectToken(): string | null {
  return sessionStorage.getItem(STORAGE_KEYS.JWT);
}

export function clearProjectToken(): void {
  sessionStorage.removeItem(STORAGE_KEYS.JWT);
  sessionStorage.removeItem(STORAGE_KEYS.PROJECT_ID);
}

export function getCurrentProjectId(): string | null {
  return sessionStorage.getItem(STORAGE_KEYS.PROJECT_ID);
}

export function isTokenExpired(jwt: string): boolean {
  try {
    const parts = jwt.split('.');
    if (parts.length !== 3) return true;

    const payload = JSON.parse(atob(parts[1]));
    if (typeof payload.exp !== 'number') return true;

    const expiresAtMs = payload.exp * 1000;
    return Date.now() > expiresAtMs - TOKEN_EXPIRY_BUFFER_MS;
  } catch {
    return true;
  }
}
```

### Task 2: Update Backend to Return JWT in Response Body ✅

**Status: COMPLETE** ✅

**Files Modified:**
- `pkg/handlers/auth.go` - Added token and project_id to response body
- `pkg/handlers/auth_test.go` - Added comprehensive tests

**Implementation Summary:**

Modified `CompleteOAuth` handler to return JWT and project_id in the response body (in addition to setting cookie for backward compatibility):

```go
// CompleteOAuthResponse - added Token and ProjectID fields
type CompleteOAuthResponse struct {
    Success     bool   `json:"success"`
    RedirectURL string `json:"redirect_url"`
    Token       string `json:"token"`       // JWT for sessionStorage
    ProjectID   string `json:"project_id"`  // Project ID extracted from JWT
}
```

**Key Implementation Details:**

1. **JWT Parsing:** Uses `jwt.NewParser(jwt.WithoutClaimsValidation()).ParseUnverified()` to extract project_id claim without full validation (since token is already validated by auth server)

2. **Error Handling:** JWT parsing failures are logged as warnings but do not fail the request - the token is still valid and usable

3. **Backward Compatibility:** Cookie is still set alongside response body fields to support gradual migration

4. **Test Coverage:**
   - `TestAuthHandler_CompleteOAuth_ReturnsTokenInBody` - Verifies valid JWT returns token and project_id in response
   - `TestAuthHandler_CompleteOAuth_HandlesInvalidJWTGracefully` - Verifies malformed JWTs don't cause request failures
   - Updated `TestAuthHandler_CompleteOAuth_Success` - Verifies token field is present

**Important Notes for Next Task:**
- The response now includes both `token` and `project_id` fields
- Frontend should check for both fields in response before attempting to extract project_id from JWT manually
- Cookie is still being set (can be removed in later cleanup task)

### Task 3: Update OAuthCallbackPage to Store JWT in sessionStorage ✅

**Status: COMPLETE** ✅

**Files Modified:**
- `ui/src/pages/OAuthCallbackPage.tsx:62-83` - Added `extractProjectIdFromJwt()` helper and JWT storage logic
- `ui/src/pages/OAuthCallbackPage.test.tsx` - Added 3 comprehensive test cases

**Implementation Summary:**

Updated OAuthCallbackPage to store JWT in sessionStorage after successful OAuth token exchange, enabling tab-scoped authentication. The component now handles both the primary path (token + project_id in response) and fallback path (extracting project_id from JWT payload).

**Key Implementation Details:**

1. **Helper Function:** Added `extractProjectIdFromJwt(jwt: string): string | null` (lines 62-70)
   - Safely parses JWT payload to extract project_id claim
   - Returns null on any parsing error (malformed JWT, missing claim)
   - Uses standard JWT structure: `header.payload.signature`

2. **Primary Storage Path:** When backend response includes both `token` and `project_id`
   - Stores directly using `storeProjectToken(data.token, data.project_id)`
   - This is the expected path after Task 2 backend changes

3. **Fallback Storage Path:** When response only includes `token`
   - Extracts project_id from JWT payload using helper function
   - Stores token only if extraction succeeds
   - Provides backward compatibility during transition

4. **Error Handling:** Gracefully handles malformed JWTs
   - No storage occurs if project_id extraction fails
   - OAuth flow completes successfully (cookie fallback still works)
   - No user-visible errors

5. **Storage Location:** Lines 73-83 in handleCallback function
   - Runs after successful `/api/auth/complete-oauth` response
   - Before redirect to return URL

**Test Coverage:**
- ✅ `should store JWT in sessionStorage when token and project_id are in response` - Verifies primary path works
- ✅ `should extract project_id from JWT if not in response` - Verifies fallback extraction works
- ✅ `should not store token if extraction fails and no project_id in response` - Verifies graceful error handling

**Why This Approach:**
- Primary path (token + project_id in response) avoids client-side JWT parsing where possible
- Fallback path ensures compatibility if backend doesn't include project_id
- Fail-safe: if storage fails, cookie-based auth still works (backend accepts both)

**Next Task Context:**
Task 4 will update `fetchWithAuth()` to read from sessionStorage and send Bearer tokens. Once that's complete, the tab-scoped authentication will be fully functional. The current implementation is ready for Task 4 - JWT is being stored correctly in sessionStorage with the project_id.

**Original Spec:**

**File: `ui/src/pages/OAuthCallbackPage.tsx`**

After successful token exchange, store the JWT in sessionStorage:

```typescript
import { storeProjectToken } from '../lib/auth-token';

// After response.json()...
const data = await response.json();

// Store JWT in sessionStorage (tab-scoped)
if (data.token && data.project_id) {
  storeProjectToken(data.token, data.project_id);
} else if (data.token) {
  // Fallback: extract project_id from JWT if not in response
  const projectId = extractProjectIdFromJwt(data.token);
  if (projectId) {
    storeProjectToken(data.token, projectId);
  }
}
```

### Task 4: Update fetchWithAuth to Use Bearer Token ✅

**Status: COMPLETE** ✅

**Files Modified:**
- `ui/src/lib/api.ts:1-84` - Complete rewrite to use Bearer token authentication
- `ui/src/lib/api.test.ts:1-235` - Updated all tests to verify Bearer token behavior

**Implementation Summary:**

Replaced cookie-based authentication with Bearer token authentication from sessionStorage. The new implementation:

1. **Pre-request Token Check:** Before making any request, checks if a valid token exists in sessionStorage
   - Calls `getProjectToken()` to retrieve token
   - Calls `isTokenExpired()` to verify token is still valid
   - If no token or expired, clears token and initiates OAuth flow

2. **Bearer Token Header:** Sends JWT as Authorization header instead of cookies
   - Added `Authorization: Bearer ${token}` header to all authenticated requests
   - Removed `credentials: 'include'` (no longer using cookies)
   - Preserves any custom headers provided by caller

3. **Response Error Handling:** Handles 401/403 by clearing token and re-authenticating
   - 401 Unauthorized: Invalid or missing token
   - 403 Forbidden: Token valid but project ID mismatch
   - Both cases: call `clearProjectToken()` and initiate OAuth flow

4. **Helper Function:** Added `extractProjectIdFromPath()` helper (lines 13-17)
   - Extracts project ID from URL patterns: `/projects/:id` or `/sdap/v1/:id`
   - Used when initiating OAuth flow to pass correct project_id

**Key Changes:**
- **Lines 3-4:** Import auth-token utilities (`getProjectToken`, `clearProjectToken`, `isTokenExpired`)
- **Lines 13-17:** New helper function to extract project ID from path
- **Lines 35-52:** Pre-request token validation and OAuth initiation
- **Lines 54-61:** Send Bearer token in Authorization header (no credentials)
- **Lines 66-80:** Handle 401/403 responses by clearing token and re-authenticating

**Test Coverage (7 tests):**
- ✅ Sends Authorization Bearer header with valid token
- ✅ Does NOT send credentials (no cookies)
- ✅ Initiates OAuth flow when no token present
- ✅ Clears token and re-auths on 401 response
- ✅ Clears token and re-auths on 403 response
- ✅ Extracts project_id from URL when re-authenticating
- ✅ Preserves custom headers and merges with Authorization

**Why This Approach:**
- Tab-scoped sessionStorage enables multi-tab project isolation (solves the root problem)
- Pre-request token validation reduces unnecessary API calls with expired tokens
- Bearer token is standard OAuth 2.0 pattern (better than cookies)
- Helper function centralizes URL parsing logic for maintainability

**Next Task Context:**
Task 5 will verify that the backend reads Authorization header before falling back to cookies. The frontend is now fully migrated to Bearer token authentication. Cookie support in backend is only needed for backward compatibility during transition.

**Original Spec:**

**File: `ui/src/lib/api.ts`**

Replace cookie-based auth with Bearer token from sessionStorage:

```typescript
import { getProjectToken, clearProjectToken, isTokenExpired } from './auth-token';
import { initiateOAuthFlow } from './oauth';

export async function fetchWithAuth(url: string, options: RequestInit = {}): Promise<Response> {
  const token = getProjectToken();

  // Check if we have a valid token
  if (!token || isTokenExpired(token)) {
    console.log('No valid token - initiating OAuth flow');
    clearProjectToken();

    const config = getCachedConfig();
    if (!config) {
      throw new Error('Configuration not available');
    }

    const projectId = extractProjectIdFromPath();
    await initiateOAuthFlow(config, projectId);
    return new Promise(() => {}); // Redirecting
  }

  // Send token as Bearer header
  const response = await fetch(url, {
    ...options,
    headers: {
      ...options.headers,
      'Authorization': `Bearer ${token}`,
    },
  });

  // Handle 401/403 - clear token and re-auth
  if (response.status === 401 || response.status === 403) {
    console.log(`${response.status} detected - clearing token and re-authenticating`);
    clearProjectToken();

    const config = getCachedConfig();
    if (!config) {
      throw new Error('Configuration not available');
    }

    const projectId = extractProjectIdFromPath();
    await initiateOAuthFlow(config, projectId);
    return new Promise(() => {});
  }

  return response;
}

function extractProjectIdFromPath(): string | undefined {
  const match = window.location.pathname.match(/\/projects\/([a-f0-9-]+)/);
  return match?.[1];
}
```

### Task 5: Ensure Backend Reads Authorization Header ✅

**Status: COMPLETE** ✅

**Files Modified:**
- `pkg/auth/service.go:54-81` - Reordered token extraction to check Authorization header first, then cookie
- `pkg/auth/service_test.go:75-135` - Updated and added tests for new precedence order

**Implementation Summary:**

Modified `ValidateRequest` function to prefer Authorization header over cookie, enabling tab-scoped authentication while maintaining backward compatibility:

1. **Authorization Header First (Preferred):** Checks `Authorization: Bearer` header as the primary method
   - If header present but malformed, returns `ErrInvalidAuthFormat`
   - If header is valid, extracts token and sets source to "header"

2. **Cookie Fallback (Backward Compatibility):** Falls back to `ekaya_jwt` cookie if no Authorization header
   - Only checked if Authorization header is absent
   - Sets source to "cookie" for logging/debugging

3. **Error Handling:** Returns `ErrMissingAuthorization` if neither header nor cookie present

**Test Coverage:**
- ✅ `TestAuthService_ValidateRequest_AuthorizationHeaderTakesPrecedence` - Verifies header wins when both present
- ✅ `TestAuthService_ValidateRequest_FallsBackToCookie` - Verifies cookie fallback when only cookie present
- ✅ `TestAuthService_ValidateRequest_MissingAuth` - Verifies error when neither present
- ✅ All existing tests pass (Cookie, AuthHeader, InvalidAuthFormat, TokenValidationError)

**Why This Approach:**
- Tab-scoped sessionStorage (frontend) → Bearer token (transport) → Header-first extraction (backend) enables multi-tab project isolation
- Cookie fallback ensures smooth transition and backward compatibility
- Source tracking ("header" vs "cookie") enables monitoring migration progress

**Implementation Details for Next Session:**
The change was straightforward - reversed the order of token extraction logic in `ValidateRequest()`:
1. First checks `Authorization` header for `Bearer <token>` format
2. Only falls back to `ekaya_jwt` cookie if no Authorization header present
3. All tests updated to verify this precedence order

The implementation maintains full backward compatibility - existing clients using cookies will continue to work, while new clients can send Bearer tokens via Authorization header for tab-scoped isolation.

**Original Spec:**

**File: `pkg/auth/service.go`**

The backend already supports reading from Authorization header. Verify this is checked BEFORE cookie:

```go
func (s *authService) ExtractToken(r *http.Request) (string, string) {
    // 1. Check Authorization header first (preferred for tab-scoped auth)
    authHeader := r.Header.Get("Authorization")
    if strings.HasPrefix(authHeader, "Bearer ") {
        return strings.TrimPrefix(authHeader, "Bearer "), "header"
    }

    // 2. Fall back to cookie (for backward compatibility during transition)
    if cookie, err := r.Cookie("ekaya_jwt"); err == nil {
        return cookie.Value, "cookie"
    }

    return "", ""
}
```

### Task 6: Remove Cookie Clearing from Frontend

**File: `ui/src/lib/api.ts`**

Remove the line that clears the cookie (no longer needed):

```typescript
// REMOVE this line:
document.cookie = 'ekaya_jwt=; Max-Age=0; path=/; SameSite=Strict';
```

---

## Files to Modify

### Frontend (TypeScript)
| File | Change |
|------|--------|
| `ui/src/lib/auth-token.ts` | **NEW** - Tab-scoped JWT utilities |
| `ui/src/pages/OAuthCallbackPage.tsx` | Store JWT in sessionStorage after auth |
| `ui/src/lib/api.ts` | Send Bearer token, remove cookie handling |

### Backend (Go)
| File | Change |
|------|--------|
| `pkg/handlers/auth.go` | Return JWT in response body |
| `pkg/auth/service.go` | Verify Authorization header is checked first |

---

## Testing

### Task 7: Unit Tests for auth-token.ts

**New file: `ui/src/lib/auth-token.test.ts`**

```typescript
import { describe, it, expect, beforeEach } from 'vitest';
import {
  storeProjectToken,
  getProjectToken,
  clearProjectToken,
  getCurrentProjectId,
  isTokenExpired,
} from './auth-token';

describe('Tab-Scoped JWT Storage', () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  describe('storeProjectToken', () => {
    it('should store JWT and project ID in sessionStorage', () => {
      storeProjectToken('test-jwt', 'project-123');

      expect(sessionStorage.getItem('ekaya_jwt')).toBe('test-jwt');
      expect(sessionStorage.getItem('ekaya_project_id')).toBe('project-123');
    });

    it('should overwrite existing values', () => {
      storeProjectToken('jwt-1', 'project-1');
      storeProjectToken('jwt-2', 'project-2');

      expect(getProjectToken()).toBe('jwt-2');
      expect(getCurrentProjectId()).toBe('project-2');
    });
  });

  describe('getProjectToken', () => {
    it('should return null when no token stored', () => {
      expect(getProjectToken()).toBeNull();
    });

    it('should return stored token', () => {
      sessionStorage.setItem('ekaya_jwt', 'my-token');
      expect(getProjectToken()).toBe('my-token');
    });
  });

  describe('clearProjectToken', () => {
    it('should remove both JWT and project ID', () => {
      storeProjectToken('test-jwt', 'project-123');
      clearProjectToken();

      expect(getProjectToken()).toBeNull();
      expect(getCurrentProjectId()).toBeNull();
    });
  });

  describe('isTokenExpired', () => {
    it('should return true for malformed JWT', () => {
      expect(isTokenExpired('not-a-jwt')).toBe(true);
      expect(isTokenExpired('only.two.parts')).toBe(true);
      expect(isTokenExpired('')).toBe(true);
    });

    it('should return true for expired token', () => {
      // Create JWT with exp in the past
      const payload = { exp: Math.floor(Date.now() / 1000) - 3600 }; // 1 hour ago
      const jwt = `header.${btoa(JSON.stringify(payload))}.signature`;

      expect(isTokenExpired(jwt)).toBe(true);
    });

    it('should return true for token expiring within buffer window', () => {
      // Create JWT expiring in 30 seconds (within 1 minute buffer)
      const payload = { exp: Math.floor(Date.now() / 1000) + 30 };
      const jwt = `header.${btoa(JSON.stringify(payload))}.signature`;

      expect(isTokenExpired(jwt)).toBe(true);
    });

    it('should return false for valid non-expired token', () => {
      // Create JWT expiring in 1 hour
      const payload = { exp: Math.floor(Date.now() / 1000) + 3600 };
      const jwt = `header.${btoa(JSON.stringify(payload))}.signature`;

      expect(isTokenExpired(jwt)).toBe(false);
    });

    it('should return true for token without exp claim', () => {
      const payload = { sub: 'user-123' }; // No exp
      const jwt = `header.${btoa(JSON.stringify(payload))}.signature`;

      expect(isTokenExpired(jwt)).toBe(true);
    });
  });
});
```

### Task 8: Unit Tests for fetchWithAuth Bearer Token

**Update file: `ui/src/lib/api.test.ts`**

Add tests to verify Bearer token is sent:

```typescript
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { fetchWithAuth } from './api';
import * as authToken from './auth-token';
import * as config from '../services/config';

// Mock the modules
vi.mock('./auth-token');
vi.mock('../services/config');

describe('fetchWithAuth - Bearer Token', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    global.fetch = vi.fn();
  });

  it('should send Authorization Bearer header with token', async () => {
    const mockToken = 'valid-jwt-token';
    vi.mocked(authToken.getProjectToken).mockReturnValue(mockToken);
    vi.mocked(authToken.isTokenExpired).mockReturnValue(false);
    vi.mocked(global.fetch).mockResolvedValue(new Response('{}', { status: 200 }));

    await fetchWithAuth('/api/test');

    expect(global.fetch).toHaveBeenCalledWith('/api/test', expect.objectContaining({
      headers: expect.objectContaining({
        'Authorization': `Bearer ${mockToken}`,
      }),
    }));
  });

  it('should NOT send credentials: include (no cookies)', async () => {
    vi.mocked(authToken.getProjectToken).mockReturnValue('token');
    vi.mocked(authToken.isTokenExpired).mockReturnValue(false);
    vi.mocked(global.fetch).mockResolvedValue(new Response('{}', { status: 200 }));

    await fetchWithAuth('/api/test');

    // Should not include credentials: 'include' since we're using Bearer token
    const fetchCall = vi.mocked(global.fetch).mock.calls[0];
    expect(fetchCall[1]?.credentials).toBeUndefined();
  });

  it('should initiate OAuth flow when no token present', async () => {
    vi.mocked(authToken.getProjectToken).mockReturnValue(null);
    vi.mocked(config.getCachedConfig).mockReturnValue({
      authServerUrl: 'https://auth.example.com',
      authorizationEndpoint: 'https://auth.example.com/authorize',
      tokenEndpoint: 'https://auth.example.com/token',
      oauthClientId: 'test-client',
      baseUrl: 'http://localhost:3443',
    });

    // Mock window.location for OAuth redirect
    const originalLocation = window.location;
    delete (window as any).location;
    window.location = { ...originalLocation, href: '', pathname: '/projects/test-123' } as any;

    const promise = fetchWithAuth('/api/test');

    // Should not call fetch when no token
    expect(global.fetch).not.toHaveBeenCalled();

    window.location = originalLocation;
  });

  it('should clear token and re-auth on 401 response', async () => {
    vi.mocked(authToken.getProjectToken).mockReturnValue('expired-token');
    vi.mocked(authToken.isTokenExpired).mockReturnValue(false);
    vi.mocked(global.fetch).mockResolvedValue(new Response('Unauthorized', { status: 401 }));
    vi.mocked(config.getCachedConfig).mockReturnValue({
      authServerUrl: 'https://auth.example.com',
      authorizationEndpoint: 'https://auth.example.com/authorize',
      tokenEndpoint: 'https://auth.example.com/token',
      oauthClientId: 'test-client',
      baseUrl: 'http://localhost:3443',
    });

    const originalLocation = window.location;
    delete (window as any).location;
    window.location = { ...originalLocation, href: '', pathname: '/projects/test-123' } as any;

    fetchWithAuth('/api/test');

    // Wait for async handling
    await new Promise(resolve => setTimeout(resolve, 0));

    expect(authToken.clearProjectToken).toHaveBeenCalled();

    window.location = originalLocation;
  });

  it('should clear token and re-auth on 403 response', async () => {
    vi.mocked(authToken.getProjectToken).mockReturnValue('wrong-project-token');
    vi.mocked(authToken.isTokenExpired).mockReturnValue(false);
    vi.mocked(global.fetch).mockResolvedValue(new Response('Forbidden', { status: 403 }));
    vi.mocked(config.getCachedConfig).mockReturnValue({
      authServerUrl: 'https://auth.example.com',
      authorizationEndpoint: 'https://auth.example.com/authorize',
      tokenEndpoint: 'https://auth.example.com/token',
      oauthClientId: 'test-client',
      baseUrl: 'http://localhost:3443',
    });

    const originalLocation = window.location;
    delete (window as any).location;
    window.location = { ...originalLocation, href: '', pathname: '/projects/test-123' } as any;

    fetchWithAuth('/api/test');

    await new Promise(resolve => setTimeout(resolve, 0));

    expect(authToken.clearProjectToken).toHaveBeenCalled();

    window.location = originalLocation;
  });
});
```

### Task 9: Unit Tests for OAuthCallbackPage Token Storage

**Update file: `ui/src/pages/OAuthCallbackPage.test.tsx`**

Add tests to verify JWT is stored in sessionStorage:

```typescript
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import OAuthCallbackPage from './OAuthCallbackPage';
import * as authToken from '../lib/auth-token';

vi.mock('../lib/auth-token');

describe('OAuthCallbackPage - Token Storage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    sessionStorage.clear();

    // Set up required OAuth session state
    sessionStorage.setItem('oauth_state', 'test-state-123');
    sessionStorage.setItem('oauth_code_verifier', 'test-verifier-456');
    sessionStorage.setItem('oauth_auth_server_url', 'https://auth.example.com');
    sessionStorage.setItem('oauth_return_url', '/projects/test-project');
  });

  it('should store JWT in sessionStorage after successful auth', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        success: true,
        redirect_url: '/projects/test-project',
        token: 'new-jwt-token',
        project_id: 'project-123',
      }),
    });

    render(
      <MemoryRouter initialEntries={['/oauth/callback?code=auth-code&state=test-state-123']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(authToken.storeProjectToken).toHaveBeenCalledWith('new-jwt-token', 'project-123');
    });
  });

  it('should extract project_id from JWT if not in response', async () => {
    // JWT with project_id in payload
    const payload = { project_id: 'extracted-project-456', exp: Date.now() / 1000 + 3600 };
    const mockJwt = `header.${btoa(JSON.stringify(payload))}.signature`;

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        success: true,
        redirect_url: '/projects/test-project',
        token: mockJwt,
        // No project_id in response - should extract from JWT
      }),
    });

    render(
      <MemoryRouter initialEntries={['/oauth/callback?code=auth-code&state=test-state-123']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(authToken.storeProjectToken).toHaveBeenCalledWith(mockJwt, 'extracted-project-456');
    });
  });
});
```

### Task 10: Backend Unit Tests

**Update file: `pkg/handlers/auth_test.go`**

Add tests to verify JWT is returned in response body:

```go
func TestCompleteOAuth_ReturnsTokenInBody(t *testing.T) {
    // Setup mock OAuth service that returns a valid token
    mockOAuthService := &mockOAuthService{
        exchangeFunc: func(ctx context.Context, req *services.TokenExchangeRequest) (string, error) {
            // Return a JWT with project_id claim
            return "eyJhbGciOiJIUzI1NiJ9.eyJwcm9qZWN0X2lkIjoicHJvamVjdC0xMjMiLCJleHAiOjk5OTk5OTk5OTl9.sig", nil
        },
    }

    handler := NewAuthHandler(mockOAuthService, nil, testConfig, zap.NewNop())

    reqBody := CompleteOAuthRequest{
        Code:         "auth-code",
        State:        "state-123",
        CodeVerifier: "verifier-456",
        AuthURL:      "https://auth.example.com",
        RedirectURI:  "http://localhost:3443/oauth/callback",
    }
    body, _ := json.Marshal(reqBody)

    req := httptest.NewRequest("POST", "/api/auth/complete-oauth", bytes.NewReader(body))
    rec := httptest.NewRecorder()

    handler.CompleteOAuth(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected status 200, got %d", rec.Code)
    }

    var resp CompleteOAuthResponse
    if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
        t.Fatalf("failed to decode response: %v", err)
    }

    // Verify token is in response body
    if resp.Token == "" {
        t.Error("expected token in response body")
    }

    // Verify project_id is in response body
    if resp.ProjectID == "" {
        t.Error("expected project_id in response body")
    }

    if resp.ProjectID != "project-123" {
        t.Errorf("expected project_id 'project-123', got '%s'", resp.ProjectID)
    }
}
```

**Update file: `pkg/auth/service_test.go`**

Add tests to verify Authorization header is preferred over cookie:

```go
func TestExtractToken_PrefersAuthorizationHeader(t *testing.T) {
    service := NewAuthService(/* ... */)

    req := httptest.NewRequest("GET", "/api/test", nil)
    req.Header.Set("Authorization", "Bearer header-token")
    req.AddCookie(&http.Cookie{Name: "ekaya_jwt", Value: "cookie-token"})

    token, source := service.ExtractToken(req)

    if token != "header-token" {
        t.Errorf("expected 'header-token', got '%s'", token)
    }
    if source != "header" {
        t.Errorf("expected source 'header', got '%s'", source)
    }
}

func TestExtractToken_FallsBackToCookie(t *testing.T) {
    service := NewAuthService(/* ... */)

    req := httptest.NewRequest("GET", "/api/test", nil)
    // No Authorization header
    req.AddCookie(&http.Cookie{Name: "ekaya_jwt", Value: "cookie-token"})

    token, source := service.ExtractToken(req)

    if token != "cookie-token" {
        t.Errorf("expected 'cookie-token', got '%s'", token)
    }
    if source != "cookie" {
        t.Errorf("expected source 'cookie', got '%s'", source)
    }
}

func TestExtractToken_ReturnsEmptyWhenNoAuth(t *testing.T) {
    service := NewAuthService(/* ... */)

    req := httptest.NewRequest("GET", "/api/test", nil)
    // No Authorization header, no cookie

    token, source := service.ExtractToken(req)

    if token != "" {
        t.Errorf("expected empty token, got '%s'", token)
    }
    if source != "" {
        t.Errorf("expected empty source, got '%s'", source)
    }
}
```

### Task 11: Integration Tests

**New file: `pkg/handlers/auth_integration_test.go`**

```go
func TestOAuthFlow_TabScopedJWT(t *testing.T) {
    // This test verifies the complete OAuth flow returns JWT in response body
    // and that the JWT can be used as Bearer token for subsequent requests

    // 1. Complete OAuth flow
    completeReq := CompleteOAuthRequest{
        Code:         "valid-code",
        State:        "valid-state",
        CodeVerifier: "valid-verifier",
        AuthURL:      "http://localhost:5002",
        RedirectURI:  "http://localhost:3443/oauth/callback",
    }
    body, _ := json.Marshal(completeReq)
    req := httptest.NewRequest("POST", "/api/auth/complete-oauth", bytes.NewReader(body))
    rec := httptest.NewRecorder()

    authHandler.CompleteOAuth(rec, req)

    var resp CompleteOAuthResponse
    json.NewDecoder(rec.Body).Decode(&resp)

    // 2. Verify JWT is in response
    if resp.Token == "" {
        t.Fatal("expected token in response")
    }

    // 3. Use JWT as Bearer token for API call
    apiReq := httptest.NewRequest("GET", "/api/projects/"+resp.ProjectID, nil)
    apiReq.Header.Set("Authorization", "Bearer "+resp.Token)
    apiRec := httptest.NewRecorder()

    projectHandler.GetProject(apiRec, apiReq)

    if apiRec.Code != http.StatusOK {
        t.Errorf("expected 200 with Bearer token, got %d", apiRec.Code)
    }
}
```

### Manual Testing Checklist

**Critical: Multi-Tab Isolation Test**

| Step | Action | Expected Result |
|------|--------|-----------------|
| 1 | Open Tab1: `http://localhost:3443/projects/{pid1}?auth_url=http://localhost:5002` | Auth redirects to localhost:5002 |
| 2 | Complete auth in Tab1 | Redirects back, JWT stored in Tab1's sessionStorage |
| 3 | Open Tab2: `http://localhost:3443/projects/{pid2}?auth_url=http://localhost:5002` | Auth redirects to localhost:5002 |
| 4 | Complete auth in Tab2 | Redirects back, JWT stored in Tab2's sessionStorage |
| 5 | In Tab1: Navigate to datasources and back | **No re-auth triggered** |
| 6 | In Tab2: Navigate to datasources and back | **No re-auth triggered** |
| 7 | Rapidly switch between tabs, making API calls | **No re-auth loop** |
| 8 | Open DevTools in each tab, check sessionStorage | Each tab has different `ekaya_jwt` value |

**Token Expiry Test**

| Step | Action | Expected Result |
|------|--------|-----------------|
| 1 | Authenticate in a tab | JWT stored |
| 2 | Wait until token is within 1 minute of expiry | - |
| 3 | Make API call | Should trigger graceful re-auth |

**Tab Close/Reopen Test**

| Step | Action | Expected Result |
|------|--------|-----------------|
| 1 | Authenticate in a tab | JWT stored in sessionStorage |
| 2 | Close the tab | sessionStorage cleared (browser behavior) |
| 3 | Open new tab to same project URL | Should require re-authentication |

**Cookie Removal Verification**

| Step | Action | Expected Result |
|------|--------|-----------------|
| 1 | Complete OAuth flow | Check DevTools > Application > Cookies |
| 2 | Verify no `ekaya_jwt` cookie is set | Cookie should NOT be present |
| 3 | Make API calls | Should work via Bearer token only |

---

## Migration Notes

### Backward Compatibility
During transition, the backend should:
1. Accept both cookie AND Authorization header
2. Prefer Authorization header when both are present
3. Continue setting cookie (can be removed in follow-up PR)

### Cleanup (Follow-up PR)
After frontend is deployed:
1. Remove cookie setting from `CompleteOAuth`
2. Remove cookie clearing from `Logout`
3. Remove cookie reading fallback from `ExtractToken` (optional, low risk to keep)

---

## Security Considerations

- **sessionStorage** is not accessible by other tabs or windows (same-origin, same-tab only)
- **sessionStorage** is cleared when tab is closed (more secure than persistent cookies)
- **Bearer token** in Authorization header is standard OAuth 2.0 pattern
- **httpOnly cookie removal** eliminates XSS cookie theft vector (though sessionStorage is still accessible to XSS)

---

## Related Files

- **Reference implementation**: `../ekaya-central/src/lib/auth-token.ts`
- **Reference API client**: `../ekaya-central/src/lib/api-client.ts`
- **Current broken implementation**: `ui/src/lib/api.ts`, `pkg/handlers/auth.go`
