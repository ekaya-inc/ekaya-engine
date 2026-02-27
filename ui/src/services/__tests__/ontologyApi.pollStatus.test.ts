import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mock fetchWithAuth before importing ontologyApi
vi.mock('../../lib/api', () => ({
  fetchWithAuth: vi.fn(),
}));

import { fetchWithAuth } from '../../lib/api';
import ontologyApi from '../ontologyApi';

const mockFetchWithAuth = vi.mocked(fetchWithAuth);

function mockJsonResponse(data: unknown, status = 200) {
  return {
    status,
    ok: status >= 200 && status < 300,
    statusText: status === 200 ? 'OK' : 'Error',
    json: () => Promise.resolve(data),
  } as unknown as Response;
}

function makeStatus(overrides: Record<string, unknown> = {}) {
  return {
    workflow_id: 'wf-1',
    current_phase: 'extracting',
    completed_phases: [],
    confidence_score: 0.5,
    iteration_count: 1,
    is_complete: false,
    status_label: 'Processing',
    status_type: 'processing',
    can_start_new: false,
    has_result: false,
    ...overrides,
  };
}

describe('ontologyApi.pollStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('resolves immediately when first status is complete', async () => {
    const completeStatus = makeStatus({ is_complete: true, current_phase: 'done' });
    mockFetchWithAuth.mockResolvedValueOnce(
      mockJsonResponse({ data: completeStatus })
    );

    const result = await ontologyApi.pollStatus('proj-1');

    expect(result).toEqual(completeStatus);
    expect(mockFetchWithAuth).toHaveBeenCalledTimes(1);
    expect(mockFetchWithAuth).toHaveBeenCalledWith(
      '/api/projects/proj-1/ontology/workflow',
      expect.objectContaining({
        headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
      })
    );
  });

  it('polls at the specified interval until complete', async () => {
    const inProgressStatus = makeStatus({ is_complete: false });
    const completeStatus = makeStatus({ is_complete: true, current_phase: 'done' });

    // First two polls return in-progress, third returns complete
    mockFetchWithAuth
      .mockResolvedValueOnce(mockJsonResponse({ data: inProgressStatus }))
      .mockResolvedValueOnce(mockJsonResponse({ data: inProgressStatus }))
      .mockResolvedValueOnce(mockJsonResponse({ data: completeStatus }));

    const pollPromise = ontologyApi.pollStatus('proj-1', { intervalMs: 1000 });

    // First poll happens immediately; flush its microtask
    await vi.advanceTimersByTimeAsync(0);
    expect(mockFetchWithAuth).toHaveBeenCalledTimes(1);

    // Advance past the first interval → second poll
    await vi.advanceTimersByTimeAsync(1000);
    expect(mockFetchWithAuth).toHaveBeenCalledTimes(2);

    // Advance past the second interval → third poll (completes)
    await vi.advanceTimersByTimeAsync(1000);
    expect(mockFetchWithAuth).toHaveBeenCalledTimes(3);

    const result = await pollPromise;
    expect(result).toEqual(completeStatus);
  });

  it('uses default 2000ms interval when none specified', async () => {
    const inProgressStatus = makeStatus({ is_complete: false });
    const completeStatus = makeStatus({ is_complete: true });

    mockFetchWithAuth
      .mockResolvedValueOnce(mockJsonResponse({ data: inProgressStatus }))
      .mockResolvedValueOnce(mockJsonResponse({ data: completeStatus }));

    const pollPromise = ontologyApi.pollStatus('proj-1');

    // First poll runs immediately
    await vi.advanceTimersByTimeAsync(0);
    expect(mockFetchWithAuth).toHaveBeenCalledTimes(1);

    // Advancing 1999ms should NOT trigger the next poll
    await vi.advanceTimersByTimeAsync(1999);
    expect(mockFetchWithAuth).toHaveBeenCalledTimes(1);

    // Advancing 1 more ms (total 2000ms) should trigger the next poll
    await vi.advanceTimersByTimeAsync(1);
    expect(mockFetchWithAuth).toHaveBeenCalledTimes(2);

    const result = await pollPromise;
    expect(result).toEqual(completeStatus);
  });

  it('calls onStatusUpdate callback at each poll', async () => {
    const inProgressStatus = makeStatus({ is_complete: false, iteration_count: 1 });
    const completeStatus = makeStatus({ is_complete: true, iteration_count: 2 });

    mockFetchWithAuth
      .mockResolvedValueOnce(mockJsonResponse({ data: inProgressStatus }))
      .mockResolvedValueOnce(mockJsonResponse({ data: completeStatus }));

    const onStatusUpdate = vi.fn();
    const pollPromise = ontologyApi.pollStatus('proj-1', {
      intervalMs: 500,
      onStatusUpdate,
    });

    // First poll
    await vi.advanceTimersByTimeAsync(0);
    expect(onStatusUpdate).toHaveBeenCalledTimes(1);
    expect(onStatusUpdate).toHaveBeenCalledWith(inProgressStatus);

    // Second poll
    await vi.advanceTimersByTimeAsync(500);
    expect(onStatusUpdate).toHaveBeenCalledTimes(2);
    expect(onStatusUpdate).toHaveBeenCalledWith(completeStatus);

    await pollPromise;
  });

  it('rejects when workflow has errors with severity "error"', async () => {
    const errorStatus = makeStatus({
      is_complete: false,
      errors: [
        {
          phase: 'extraction',
          message: 'LLM token limit exceeded',
          timestamp: '2024-01-01T00:00:00Z',
          severity: 'error',
        },
      ],
    });

    mockFetchWithAuth.mockResolvedValueOnce(
      mockJsonResponse({ data: errorStatus })
    );

    const pollPromise = ontologyApi.pollStatus('proj-1');

    await expect(pollPromise).rejects.toThrow('LLM token limit exceeded');
    expect(mockFetchWithAuth).toHaveBeenCalledTimes(1);
  });

  it('rejects with combined message when multiple error-severity errors exist', async () => {
    const errorStatus = makeStatus({
      is_complete: false,
      errors: [
        {
          phase: 'extraction',
          message: 'First error',
          timestamp: '2024-01-01T00:00:00Z',
          severity: 'error',
        },
        {
          phase: 'enrichment',
          message: 'Second error',
          timestamp: '2024-01-01T00:00:01Z',
          severity: 'error',
        },
      ],
    });

    mockFetchWithAuth.mockResolvedValueOnce(
      mockJsonResponse({ data: errorStatus })
    );

    await expect(ontologyApi.pollStatus('proj-1')).rejects.toThrow(
      'First error; Second error'
    );
  });

  it('continues polling when errors only have warning/info severity', async () => {
    const warningStatus = makeStatus({
      is_complete: false,
      errors: [
        {
          phase: 'extraction',
          message: 'Non-critical warning',
          timestamp: '2024-01-01T00:00:00Z',
          severity: 'warning',
        },
      ],
    });
    const completeStatus = makeStatus({ is_complete: true });

    mockFetchWithAuth
      .mockResolvedValueOnce(mockJsonResponse({ data: warningStatus }))
      .mockResolvedValueOnce(mockJsonResponse({ data: completeStatus }));

    const pollPromise = ontologyApi.pollStatus('proj-1', { intervalMs: 500 });

    await vi.advanceTimersByTimeAsync(0);
    expect(mockFetchWithAuth).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(500);
    expect(mockFetchWithAuth).toHaveBeenCalledTimes(2);

    const result = await pollPromise;
    expect(result).toEqual(completeStatus);
  });

  it('rejects when getStatus throws a network error', async () => {
    mockFetchWithAuth.mockRejectedValueOnce(new Error('Network failure'));

    await expect(ontologyApi.pollStatus('proj-1')).rejects.toThrow(
      'Network failure'
    );
    expect(mockFetchWithAuth).toHaveBeenCalledTimes(1);
  });

  it('rejects when getStatus returns non-ok HTTP response', async () => {
    mockFetchWithAuth.mockResolvedValueOnce(
      mockJsonResponse({ error: 'Not Found' }, 404)
    );

    // makeRequest in ontologyApi throws on non-ok responses
    await expect(ontologyApi.pollStatus('proj-1')).rejects.toThrow();
    expect(mockFetchWithAuth).toHaveBeenCalledTimes(1);
  });

  it('stops polling after error — does not schedule further calls', async () => {
    mockFetchWithAuth.mockRejectedValueOnce(new Error('Server error'));

    const pollPromise = ontologyApi.pollStatus('proj-1', { intervalMs: 500 });

    // The promise rejects immediately (first poll fails without delay)
    await expect(pollPromise).rejects.toThrow('Server error');

    // Advancing timers further should not cause additional calls
    await vi.advanceTimersByTimeAsync(5000);
    expect(mockFetchWithAuth).toHaveBeenCalledTimes(1);
  });

  it('calls the correct endpoint for the given project ID', async () => {
    const completeStatus = makeStatus({ is_complete: true });
    mockFetchWithAuth.mockResolvedValueOnce(
      mockJsonResponse({ data: completeStatus })
    );

    await ontologyApi.pollStatus('my-project-123');

    expect(mockFetchWithAuth).toHaveBeenCalledWith(
      '/api/projects/my-project-123/ontology/workflow',
      expect.any(Object)
    );
  });
});
