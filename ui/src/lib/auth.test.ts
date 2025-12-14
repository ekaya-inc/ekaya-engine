import { describe, it, expect } from 'vitest';

import { generatePKCE } from './auth';

describe('auth.js - PKCE generation', () => {
  it('should generate PKCE challenge and verifier', async () => {
    const result = await generatePKCE();

    expect(result).toHaveProperty('code_verifier');
    expect(result).toHaveProperty('code_challenge');
    expect(typeof result.code_verifier).toBe('string');
    expect(typeof result.code_challenge).toBe('string');
  });

  it('should generate verifier of sufficient length', async () => {
    const { code_verifier } = await generatePKCE();

    // pkce-challenge library generates verifiers between 43-128 chars
    expect(code_verifier.length).toBeGreaterThanOrEqual(43);
    expect(code_verifier.length).toBeLessThanOrEqual(128);
  });

  it('should generate base64url encoded challenge', async () => {
    const { code_challenge } = await generatePKCE();

    // Base64url should not contain +, /, or =
    expect(code_challenge).not.toMatch(/[+/=]/);
    expect(code_challenge.length).toBeGreaterThan(0);
  });

  it('should generate unique values on each call', async () => {
    const first = await generatePKCE();
    const second = await generatePKCE();

    expect(first.code_verifier).not.toBe(second.code_verifier);
    expect(first.code_challenge).not.toBe(second.code_challenge);
  });
});
