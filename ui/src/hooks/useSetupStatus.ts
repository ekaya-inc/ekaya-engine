import { useCallback, useEffect, useState } from 'react';

import engineApi from '../services/engineApi';
import type { SetupStatus } from '../types';

interface UseSetupStatusResult {
  status: SetupStatus | null;
  isLoading: boolean;
  error: string | null;
  refetch: () => Promise<SetupStatus | null>;
}

export function useSetupStatus(projectId: string | undefined): UseSetupStatusResult {
  const [status, setStatus] = useState<SetupStatus | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchStatus = useCallback(async (): Promise<SetupStatus | null> => {
    if (!projectId) {
      setStatus(null);
      setError(null);
      setIsLoading(false);
      return null;
    }

    setIsLoading(true);
    setError(null);

    try {
      const response = await engineApi.getSetupStatus(projectId);
      const nextStatus = response.data ?? null;
      setStatus(nextStatus);
      return nextStatus;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to fetch setup status';
      setError(message);
      setStatus(null);
      return null;
    } finally {
      setIsLoading(false);
    }
  }, [projectId]);

  useEffect(() => {
    void fetchStatus();
  }, [fetchStatus]);

  return {
    status,
    isLoading,
    error,
    refetch: fetchStatus,
  };
}
