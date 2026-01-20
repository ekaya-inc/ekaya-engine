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
      expect(isTokenExpired('only.two')).toBe(true);
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
