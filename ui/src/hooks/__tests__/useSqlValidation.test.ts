import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import { useSqlValidation } from '../useSqlValidation';

// Mock the engineApi module
vi.mock('../../services/engineApi', () => ({
  default: {
    validateQuery: vi.fn(),
  },
}));

describe('useSqlValidation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllTimers();
  });

  it('starts with idle status', () => {
    const { result } = renderHook(() =>
      useSqlValidation({ projectId: 'proj-1', datasourceId: 'ds-1' })
    );

    expect(result.current.status).toBe('idle');
    expect(result.current.error).toBeNull();
  });

  it('stays idle for empty SQL', async () => {
    const { result } = renderHook(() =>
      useSqlValidation({ projectId: 'proj-1', datasourceId: 'ds-1', debounceMs: 10 })
    );

    act(() => {
      result.current.validate('   ');
    });

    // Wait a bit for any async operations
    await new Promise((r) => setTimeout(r, 50));

    expect(result.current.status).toBe('idle');
    expect(engineApi.validateQuery).not.toHaveBeenCalled();
  });

  it('validates after debounce period', async () => {
    vi.mocked(engineApi.validateQuery).mockResolvedValue({
      success: true,
      data: { valid: true },
    });

    const { result } = renderHook(() =>
      useSqlValidation({ projectId: 'proj-1', datasourceId: 'ds-1', debounceMs: 10 })
    );

    act(() => {
      result.current.validate('SELECT * FROM users');
    });

    // Before debounce completes, should still be idle
    expect(result.current.status).toBe('idle');

    // Wait for debounce and API call
    await waitFor(() => {
      expect(result.current.status).toBe('valid');
    });

    expect(engineApi.validateQuery).toHaveBeenCalledWith('proj-1', 'ds-1', {
      sql_query: 'SELECT * FROM users',
    });
  });

  it('shows error for invalid SQL', async () => {
    vi.mocked(engineApi.validateQuery).mockResolvedValue({
      success: true,
      data: { valid: false, message: 'syntax error at position 5' },
    });

    const { result } = renderHook(() =>
      useSqlValidation({ projectId: 'proj-1', datasourceId: 'ds-1', debounceMs: 10 })
    );

    act(() => {
      result.current.validate('SELEC * FROM users');
    });

    await waitFor(() => {
      expect(result.current.status).toBe('invalid');
      expect(result.current.error).toBe('syntax error at position 5');
    });
  });

  it('cancels pending validation on new input', async () => {
    vi.mocked(engineApi.validateQuery).mockResolvedValue({
      success: true,
      data: { valid: true },
    });

    const { result } = renderHook(() =>
      useSqlValidation({ projectId: 'proj-1', datasourceId: 'ds-1', debounceMs: 50 })
    );

    // First validation
    act(() => {
      result.current.validate('SELECT * FROM users');
    });

    // Quickly send new input before debounce completes
    await new Promise((r) => setTimeout(r, 10));

    act(() => {
      result.current.validate('SELECT * FROM orders');
    });

    // Wait for validation to complete
    await waitFor(() => {
      expect(result.current.status).toBe('valid');
    });

    // Should only be called once with the final value
    expect(engineApi.validateQuery).toHaveBeenCalledTimes(1);
    expect(engineApi.validateQuery).toHaveBeenCalledWith('proj-1', 'ds-1', {
      sql_query: 'SELECT * FROM orders',
    });
  });

  it('resets state on reset()', async () => {
    vi.mocked(engineApi.validateQuery).mockResolvedValue({
      success: true,
      data: { valid: true },
    });

    const { result } = renderHook(() =>
      useSqlValidation({ projectId: 'proj-1', datasourceId: 'ds-1', debounceMs: 10 })
    );

    // Validate
    act(() => {
      result.current.validate('SELECT * FROM users');
    });

    await waitFor(() => {
      expect(result.current.status).toBe('valid');
    });

    // Reset
    act(() => {
      result.current.reset();
    });

    expect(result.current.status).toBe('idle');
    expect(result.current.error).toBeNull();
  });

  it('handles API errors', async () => {
    vi.mocked(engineApi.validateQuery).mockRejectedValue(new Error('Network error'));

    const { result } = renderHook(() =>
      useSqlValidation({ projectId: 'proj-1', datasourceId: 'ds-1', debounceMs: 10 })
    );

    act(() => {
      result.current.validate('SELECT * FROM users');
    });

    await waitFor(() => {
      expect(result.current.status).toBe('invalid');
      expect(result.current.error).toBe('Network error');
    });
  });

  it('skips validation if SQL has not changed', async () => {
    vi.mocked(engineApi.validateQuery).mockResolvedValue({
      success: true,
      data: { valid: true },
    });

    const { result } = renderHook(() =>
      useSqlValidation({ projectId: 'proj-1', datasourceId: 'ds-1', debounceMs: 10 })
    );

    // First validation
    act(() => {
      result.current.validate('SELECT * FROM users');
    });

    await waitFor(() => {
      expect(result.current.status).toBe('valid');
    });

    // Same SQL again
    act(() => {
      result.current.validate('SELECT * FROM users');
    });

    // Wait a bit to ensure no new call is made
    await new Promise((r) => setTimeout(r, 50));

    // Should only have been called once
    expect(engineApi.validateQuery).toHaveBeenCalledTimes(1);
  });
});
