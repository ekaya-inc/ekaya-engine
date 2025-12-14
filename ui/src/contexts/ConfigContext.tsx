import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';

import type { AppConfig } from '../services/config';
import { fetchConfig } from '../services/config';

interface ConfigContextType {
  config: AppConfig | null;
  loading: boolean;
  error: Error | null;
}

const ConfigContext = createContext<ConfigContextType | undefined>(undefined);

interface ConfigProviderProps {
  children: ReactNode;
}

/**
 * ConfigProvider fetches application configuration from backend on mount
 * and provides it to all child components via context
 *
 * IMPORTANT: Blocks rendering of children until config is loaded to prevent
 * race conditions where components try to use config before it's available.
 * This ensures fetchWithAuth and other components can safely access config.
 */
export function ConfigProvider({ children }: ConfigProviderProps) {
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    async function loadConfig() {
      try {
        const cfg = await fetchConfig();
        setConfig(cfg);
        setError(null);
      } catch (err) {
        console.error('Failed to load config:', err);
        setError(err instanceof Error ? err : new Error('Failed to load config'));
      } finally {
        setLoading(false);
      }
    }

    loadConfig();
  }, []);

  // Block rendering until config is loaded to prevent race conditions
  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-screen bg-background">
        <div className="text-center">
          <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-solid border-current border-r-transparent motion-reduce:animate-[spin_1.5s_linear_infinite]" role="status">
            <span className="sr-only">Loading configuration...</span>
          </div>
          <p className="mt-4 text-sm text-muted-foreground">Loading configuration...</p>
        </div>
      </div>
    );
  }

  // Show error state if config failed to load
  if (error) {
    return (
      <div className="flex items-center justify-center min-h-screen bg-background">
        <div className="text-center max-w-md p-6">
          <div className="mb-4 text-destructive">
            <svg xmlns="http://www.w3.org/2000/svg" className="h-12 w-12 mx-auto" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
          </div>
          <h2 className="text-lg font-semibold mb-2">Configuration Error</h2>
          <p className="text-sm text-muted-foreground mb-4">
            Failed to load application configuration: {error.message}
          </p>
          <button
            onClick={() => window.location.reload()}
            className="px-4 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  // Only render children when config is successfully loaded
  return (
    <ConfigContext.Provider value={{ config, loading, error }}>
      {children}
    </ConfigContext.Provider>
  );
}

/**
 * Hook to access application configuration
 * Must be used within a ConfigProvider
 */
export function useConfig(): ConfigContextType {
  const context = useContext(ConfigContext);

  if (context === undefined) {
    throw new Error('useConfig must be used within a ConfigProvider');
  }

  return context;
}
