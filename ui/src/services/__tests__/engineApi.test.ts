import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock fetchWithAuth before importing engineApi
vi.mock('../../lib/api', () => ({
  fetchWithAuth: vi.fn(),
}));

import { fetchWithAuth } from '../../lib/api';
import engineApi from '../engineApi';

const mockFetchWithAuth = vi.mocked(fetchWithAuth);

describe('EngineApiService.makeRequest', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should handle 204 No Content without trying to parse JSON body', async () => {
    // This is the exact scenario: DELETE /ai-config returns 204 with no body.
    // Before the fix, makeRequest called response.json() unconditionally,
    // which threw "Unexpected end of JSON input" on empty 204 responses.
    mockFetchWithAuth.mockResolvedValue({
      status: 204,
      ok: true,
      json: () => Promise.reject(new SyntaxError('Unexpected end of JSON input')),
    } as unknown as Response);

    // Should not throw
    await expect(
      engineApi.deleteAIConfig('test-project-id')
    ).resolves.not.toThrow();
  });

  it('should still parse JSON for normal 200 responses', async () => {
    const responseData = {
      data: { llmBaseUrl: 'https://api.openai.com/v1', llmModel: 'gpt-4o' },
    };

    mockFetchWithAuth.mockResolvedValue({
      status: 200,
      ok: true,
      json: () => Promise.resolve(responseData),
    } as unknown as Response);

    const result = await engineApi.getAIConfig('test-project-id');
    expect(result).toEqual(responseData);
  });

  it('should throw on non-ok responses with error message', async () => {
    mockFetchWithAuth.mockResolvedValue({
      status: 400,
      ok: false,
      statusText: 'Bad Request',
      json: () => Promise.resolve({ error: 'Invalid configuration' }),
    } as unknown as Response);

    await expect(
      engineApi.getAIConfig('test-project-id')
    ).rejects.toThrow('Invalid configuration');
  });
});
