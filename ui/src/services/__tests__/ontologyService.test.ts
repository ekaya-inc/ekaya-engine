import { describe, it, expect } from 'vitest';
import { transformEntityQueue } from '../ontologyService';
import type { EntityProgressResponse } from '../../types';

describe('transformEntityQueue', () => {
  it('returns empty array when input is undefined', () => {
    expect(transformEntityQueue(undefined)).toEqual([]);
  });

  it('returns empty array when input is an empty array', () => {
    expect(transformEntityQueue([])).toEqual([]);
  });

  it('maps entity_name and status to camelCase WorkItem fields', () => {
    const input: EntityProgressResponse[] = [
      { entity_name: 'users', status: 'complete' },
      { entity_name: 'orders', status: 'processing' },
    ];

    const result = transformEntityQueue(input);

    expect(result).toEqual([
      { entityName: 'users', status: 'complete' },
      { entityName: 'orders', status: 'processing' },
    ]);
  });

  it('includes optional fields only when present in input', () => {
    const input: EntityProgressResponse[] = [
      {
        entity_name: 'products',
        status: 'complete',
        token_count: 1500,
        last_updated: '2025-01-15T10:30:00Z',
      },
    ];

    const result = transformEntityQueue(input);

    expect(result).toEqual([
      {
        entityName: 'products',
        status: 'complete',
        tokenCount: 1500,
        lastUpdated: '2025-01-15T10:30:00Z',
      },
    ]);
  });

  it('includes error_message as errorMessage when present', () => {
    const input: EntityProgressResponse[] = [
      {
        entity_name: 'payments',
        status: 'failed',
        error_message: 'Connection timeout',
      },
    ];

    const result = transformEntityQueue(input);

    expect(result).toHaveLength(1);
    expect(result[0]).toEqual({
      entityName: 'payments',
      status: 'failed',
      errorMessage: 'Connection timeout',
    });
  });

  it('omits optional fields when they are undefined in input', () => {
    const input: EntityProgressResponse[] = [
      { entity_name: 'sessions', status: 'queued' },
    ];

    const result = transformEntityQueue(input);

    expect(result[0]).not.toHaveProperty('tokenCount');
    expect(result[0]).not.toHaveProperty('lastUpdated');
    expect(result[0]).not.toHaveProperty('errorMessage');
  });

  it('handles all possible entity statuses', () => {
    const statuses: EntityProgressResponse['status'][] = [
      'queued', 'processing', 'complete', 'updating', 'schema-changed', 'outdated', 'failed',
    ];

    const input: EntityProgressResponse[] = statuses.map((status, i) => ({
      entity_name: `table_${i}`,
      status,
    }));

    const result = transformEntityQueue(input);

    expect(result).toHaveLength(statuses.length);
    statuses.forEach((status, i) => {
      expect(result[i]?.status).toBe(status);
    });
  });

  it('transforms a full entity with all optional fields', () => {
    const input: EntityProgressResponse[] = [
      {
        entity_name: 'invoices',
        status: 'processing',
        token_count: 3200,
        last_updated: '2025-06-01T12:00:00Z',
        error_message: 'Rate limit exceeded',
      },
    ];

    const result = transformEntityQueue(input);

    expect(result).toEqual([
      {
        entityName: 'invoices',
        status: 'processing',
        tokenCount: 3200,
        lastUpdated: '2025-06-01T12:00:00Z',
        errorMessage: 'Rate limit exceeded',
      },
    ]);
  });
});
