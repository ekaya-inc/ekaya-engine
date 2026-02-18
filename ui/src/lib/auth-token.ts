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

export function getUserRoles(): string[] {
  const jwt = getProjectToken();
  if (!jwt) return [];

  try {
    const parts = jwt.split('.');
    if (parts.length !== 3) return [];

    const payloadPart = parts[1];
    if (!payloadPart) return [];

    const payload = JSON.parse(atob(payloadPart));
    if (!Array.isArray(payload.roles)) return [];

    return payload.roles;
  } catch {
    return [];
  }
}

export function isTokenExpired(jwt: string): boolean {
  try {
    const parts = jwt.split('.');
    if (parts.length !== 3) return true;

    const payloadPart = parts[1];
    if (!payloadPart) return true;

    const payload = JSON.parse(atob(payloadPart));
    if (typeof payload.exp !== 'number') return true;

    const expiresAtMs = payload.exp * 1000;
    return Date.now() > expiresAtMs - TOKEN_EXPIRY_BUFFER_MS;
  } catch {
    return true;
  }
}
