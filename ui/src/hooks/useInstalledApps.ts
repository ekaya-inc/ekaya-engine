/**
 * useInstalledApps Hook
 * Provides state management for installed applications per project
 */

import { useCallback, useEffect, useState } from 'react';

import engineApi from '../services/engineApi';
import type { InstalledApp } from '../types';

interface UseInstalledAppsResult {
  /** List of installed apps */
  apps: InstalledApp[];
  /** Loading state for initial fetch */
  isLoading: boolean;
  /** Error message if fetch failed */
  error: string | null;
  /** Refetch the installed apps list */
  refetch: () => Promise<void>;
  /** Check if a specific app is installed */
  isInstalled: (appId: string) => boolean;
}

/**
 * Hook for fetching and managing installed apps for a project
 */
export function useInstalledApps(projectId: string | undefined): UseInstalledAppsResult {
  const [apps, setApps] = useState<InstalledApp[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchApps = useCallback(async () => {
    if (!projectId) {
      setApps([]);
      setIsLoading(false);
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      const response = await engineApi.listInstalledApps(projectId);
      if (response.data?.apps) {
        setApps(response.data.apps);
      } else {
        setApps([]);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch installed apps');
      setApps([]);
    } finally {
      setIsLoading(false);
    }
  }, [projectId]);

  useEffect(() => {
    fetchApps();
  }, [fetchApps]);

  const isInstalled = useCallback(
    (appId: string): boolean => {
      return apps.some((app) => app.app_id === appId);
    },
    [apps]
  );

  return {
    apps,
    isLoading,
    error,
    refetch: fetchApps,
    isInstalled,
  };
}

interface UseInstallAppResult {
  /** Install an app */
  install: (appId: string) => Promise<InstalledApp | null>;
  /** Uninstall an app */
  uninstall: (appId: string) => Promise<boolean>;
  /** Loading state for install/uninstall operations */
  isLoading: boolean;
  /** Error message if operation failed */
  error: string | null;
}

/**
 * Hook for installing and uninstalling apps
 */
export function useInstallApp(projectId: string | undefined): UseInstallAppResult {
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const install = useCallback(
    async (appId: string): Promise<InstalledApp | null> => {
      if (!projectId) {
        setError('Project ID is required');
        return null;
      }

      setIsLoading(true);
      setError(null);

      try {
        const response = await engineApi.installApp(projectId, appId);
        return response.data ?? null;
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Failed to install app';
        setError(message);
        return null;
      } finally {
        setIsLoading(false);
      }
    },
    [projectId]
  );

  const uninstall = useCallback(
    async (appId: string): Promise<boolean> => {
      if (!projectId) {
        setError('Project ID is required');
        return false;
      }

      setIsLoading(true);
      setError(null);

      try {
        await engineApi.uninstallApp(projectId, appId);
        return true;
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Failed to uninstall app';
        setError(message);
        return false;
      } finally {
        setIsLoading(false);
      }
    },
    [projectId]
  );

  return {
    install,
    uninstall,
    isLoading,
    error,
  };
}
