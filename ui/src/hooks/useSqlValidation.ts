/**
 * SQL Validation Hook
 * Provides debounced SQL validation against the backend
 */

import { useCallback, useEffect, useRef, useState } from 'react';

import engineApi from '../services/engineApi';

export type ValidationStatus = 'idle' | 'validating' | 'valid' | 'invalid';

interface UseSqlValidationOptions {
  projectId: string;
  datasourceId: string;
  debounceMs?: number;
}

interface UseSqlValidationResult {
  status: ValidationStatus;
  error: string | null;
  validate: (sql: string) => void;
  reset: () => void;
}

/**
 * Hook for validating SQL queries with debounced API calls
 *
 * @param options - Configuration options
 * @returns Validation state and control functions
 */
export function useSqlValidation({
  projectId,
  datasourceId,
  debounceMs = 500,
}: UseSqlValidationOptions): UseSqlValidationResult {
  const [status, setStatus] = useState<ValidationStatus>('idle');
  const [error, setError] = useState<string | null>(null);

  // Use refs to track the latest values and avoid stale closures
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const lastSqlRef = useRef<string>('');

  /**
   * Reset validation state
   */
  const reset = useCallback(() => {
    // Clear any pending timeout
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    }

    // Abort any in-flight request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }

    setStatus('idle');
    setError(null);
    lastSqlRef.current = '';
  }, []);

  /**
   * Validate SQL with debouncing
   */
  const validate = useCallback(
    (sql: string) => {
      // Clear any pending timeout
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
        timeoutRef.current = null;
      }

      // Abort any in-flight request
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
        abortControllerRef.current = null;
      }

      // Don't validate empty queries
      const trimmedSql = sql.trim();
      if (!trimmedSql) {
        setStatus('idle');
        setError(null);
        lastSqlRef.current = '';
        return;
      }

      // Skip if SQL hasn't changed
      if (trimmedSql === lastSqlRef.current) {
        return;
      }

      lastSqlRef.current = trimmedSql;

      // Set debounced validation
      timeoutRef.current = setTimeout(async () => {
        setStatus('validating');
        setError(null);

        try {
          const response = await engineApi.validateQuery(projectId, datasourceId, {
            sql_query: trimmedSql,
          });

          if (response.success && response.data) {
            if (response.data.valid) {
              setStatus('valid');
              setError(null);
            } else {
              setStatus('invalid');
              setError(response.data.message ?? 'SQL validation failed');
            }
          } else {
            setStatus('invalid');
            setError(response.error ?? 'Validation failed');
          }
        } catch (err) {
          // Ignore aborted requests
          if (err instanceof Error && err.name === 'AbortError') {
            return;
          }

          setStatus('invalid');
          setError(err instanceof Error ? err.message : 'Validation failed');
        }
      }, debounceMs);
    },
    [projectId, datasourceId, debounceMs]
  );

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, []);

  return {
    status,
    error,
    validate,
    reset,
  };
}
