import type { ReactNode } from 'react';
import { createContext, useContext, useCallback, useState } from 'react';

import engineApi from '../services/engineApi';
import type {
  ApiResponse,
  ConnectionDetails,
  ConnectionStatus,
  CreateDatasourceResponse,
  DatasourceConfig,
  DatasourceType,
  DeleteDatasourceResponse,
  GetDatasourceResponse,
  TestConnectionRequest,
  TestConnectionResponse,
} from '../types';

interface DatasourceConnectionContextValue {
  isConnected: boolean;
  hasSelectedTables: boolean;
  datasources: ConnectionDetails[];
  selectedDatasource: ConnectionDetails | null;
  connectionDetails: ConnectionDetails | null;
  connectionStatus: ConnectionStatus | null;
  connect: (details: ConnectionDetails) => void;
  disconnect: (datasourceId: string) => void;
  testConnection: (
    projectId: string,
    details: TestConnectionRequest
  ) => Promise<ApiResponse<TestConnectionResponse>>;
  saveDataSource: (
    projectId: string,
    displayName: string,
    datasourceType: DatasourceType,
    config: DatasourceConfig
  ) => Promise<ApiResponse<CreateDatasourceResponse>>;
  loadDataSource: (
    projectId: string,
    datasourceId: string
  ) => Promise<GetDatasourceResponse>;
  loadDataSources: (projectId: string) => Promise<void>;
  refreshSchemaSelections: (projectId: string) => Promise<void>;
  updateDataSource: (
    projectId: string,
    datasourceId: string,
    displayName: string,
    datasourceType: DatasourceType,
    config: DatasourceConfig
  ) => Promise<ApiResponse<CreateDatasourceResponse>>;
  deleteDataSource: (
    projectId: string,
    datasourceId: string
  ) => Promise<ApiResponse<DeleteDatasourceResponse>>;
  selectDatasource: (datasourceId: string) => void;
  clearError: () => void;
  isLoading: boolean;
  error: string | null;
}

const DatasourceConnectionContext = createContext<
  DatasourceConnectionContextValue | undefined
>(undefined);

export const useDatasourceConnection = (): DatasourceConnectionContextValue => {
  const context = useContext(DatasourceConnectionContext);
  if (!context) {
    throw new Error(
      'useDatasourceConnection must be used within a DatasourceConnectionProvider'
    );
  }
  return context;
};

interface DatasourceConnectionProviderProps {
  children: ReactNode;
}

export const DatasourceConnectionProvider = ({
  children,
}: DatasourceConnectionProviderProps) => {
  const [datasources, setDatasources] = useState<ConnectionDetails[]>([]);
  const [selectedDatasource, setSelectedDatasource] =
    useState<ConnectionDetails | null>(null);
  const [connectionStatus, setConnectionStatus] =
    useState<ConnectionStatus | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [hasSelectedTables, setHasSelectedTables] = useState<boolean>(false);

  // Computed values for backward compatibility
  const isConnected = datasources.length > 0;
  const connectionDetails = selectedDatasource;

  /**
   * Load datasources for a specific project
   * Called explicitly by ProjectGuard after JWT validation
   * Memoized to prevent unnecessary re-renders and infinite loops in dependent components
   */
  const loadDataSources = useCallback(
    async (projectId: string): Promise<void> => {
      if (!projectId) {
        console.warn('loadDataSources called without projectId');
        return;
      }

      try {
        const result = await engineApi.listDataSources(projectId);

        if (result.success && result.data?.datasources) {
          const loadedDatasources: ConnectionDetails[] =
            result.data.datasources.map((ds) => ({
              datasourceId: ds.datasource_id,
              projectId: ds.project_id,
              type: ds.type,
              displayName: ds.name,
              ...ds.config,
            }));
          setDatasources(loadedDatasources);

          // Select first datasource if available
          if (loadedDatasources.length > 0) {
            setSelectedDatasource(loadedDatasources[0] ?? null);
          }

          // Load schema to check if tables are selected (via is_selected field)
          try {
            const firstDatasource = loadedDatasources[0];
            if (firstDatasource?.datasourceId) {
              const schemaResult = await engineApi.getSchema(projectId, firstDatasource.datasourceId);
              if (schemaResult.data) {
                const hasSelections = schemaResult.data.tables.some((t) => t.is_selected === true);
                setHasSelectedTables(hasSelections);
              } else {
                setHasSelectedTables(false);
              }
            }
          } catch {
            // No schema yet, this is normal
            console.log('No schema found for project');
            setHasSelectedTables(false);
          }
        }
      } catch {
        // No existing datasources found, this is normal
        console.log('No datasources found for project');
      }
    },
    []
  );

  /**
   * Refresh schema selections state
   * Called after saving schema selections to update hasSelectedTables
   * Memoized to prevent unnecessary re-renders
   */
  const refreshSchemaSelections = useCallback(
    async (projectId: string): Promise<void> => {
      if (!projectId) {
        console.warn('refreshSchemaSelections called without projectId');
        return;
      }

      try {
        // Use the currently selected datasource
        const datasourceId = selectedDatasource?.datasourceId;
        if (!datasourceId) {
          console.log('No datasource selected, cannot refresh schema selections');
          setHasSelectedTables(false);
          return;
        }

        const schemaResult = await engineApi.getSchema(projectId, datasourceId);
        if (schemaResult.data) {
          const hasSelections = schemaResult.data.tables.some((t) => t.is_selected === true);
          setHasSelectedTables(hasSelections);
        } else {
          setHasSelectedTables(false);
        }
      } catch {
        // No schema yet, this is normal
        console.log('No schema found for project');
        setHasSelectedTables(false);
      }
    },
    [selectedDatasource?.datasourceId]
  );

  const connect = (details: ConnectionDetails): void => {
    // Add or update datasource in array
    const existingIndex = datasources.findIndex(
      (ds) => ds.datasourceId === details.datasourceId
    );

    if (existingIndex >= 0) {
      // Update existing
      const updated = [...datasources];
      updated[existingIndex] = details;
      setDatasources(updated);
    } else {
      // Add new
      setDatasources([...datasources, details]);
    }

    setSelectedDatasource(details);
    setError(null);
  };

  const disconnect = (datasourceId: string): void => {
    // Remove specific datasource from array
    const updated = datasources.filter(
      (ds) => ds.datasourceId !== datasourceId
    );
    setDatasources(updated);

    // Clear selection if deleted datasource was selected
    if (selectedDatasource?.datasourceId === datasourceId) {
      setSelectedDatasource(updated.length > 0 ? updated[0] ?? null : null);
    }

    setConnectionStatus(null);
    setError(null);
  };

  const selectDatasource = (datasourceId: string): void => {
    const datasource = datasources.find(
      (ds) => ds.datasourceId === datasourceId
    );
    setSelectedDatasource(datasource ?? null);
  };

  const testConnection = async (
    projectId: string,
    details: TestConnectionRequest
  ): Promise<ApiResponse<TestConnectionResponse>> => {
    setIsLoading(true);
    setError(null);

    try {
      engineApi.validateConnectionDetails(details);
      const result = await engineApi.testDatasourceConnection(projectId, details);

      setConnectionStatus({
        success: result.success,
        message: result.message ?? 'Connection test completed',
        timestamp: new Date().toISOString(),
      });

      return result;
    } catch (err) {
      const errorMessage =
        err instanceof Error ? err.message : 'Connection test failed';
      setError(errorMessage);
      setConnectionStatus({
        success: false,
        message: errorMessage,
        timestamp: new Date().toISOString(),
      });
      throw err;
    } finally {
      setIsLoading(false);
    }
  };

  const saveDataSource = async (
    projectId: string,
    displayName: string,
    datasourceType: DatasourceType,
    config: DatasourceConfig
  ): Promise<ApiResponse<CreateDatasourceResponse>> => {
    if (!projectId) {
      throw new Error('projectId is required');
    }

    setIsLoading(true);
    setError(null);

    try {
      const result = await engineApi.createDataSource({
        name: displayName,
        datasourceType,
        config,
        projectId,
      });

      if (result.success && result.data) {
        const connectionDetails: ConnectionDetails = {
          datasourceId: result.data.datasource_id,
          projectId: result.data.project_id,
          type: datasourceType,
          displayName: result.data.name,
          ...config,
        };
        connect(connectionDetails);
      }
      return result;
    } catch (err) {
      const errorMessage =
        err instanceof Error ? err.message : 'Failed to save datasource';
      setError(errorMessage);
      throw err;
    } finally {
      setIsLoading(false);
    }
  };

  const loadDataSource = async (
    projectId: string,
    datasourceId: string
  ): Promise<GetDatasourceResponse> => {
    setIsLoading(true);
    setError(null);

    try {
      const result = await engineApi.getDataSource(projectId, datasourceId);

      if (result.success && result.data) {
        const { datasource_id, project_id, name, type, config } = result.data;
        const connectionDetails: ConnectionDetails = {
          datasourceId: datasource_id,
          projectId: project_id,
          type: type,
          displayName: name,
          ...config,
        };
        connect(connectionDetails);
        return result.data;
      }

      throw new Error('No datasource found');
    } catch (err) {
      const errorMessage =
        err instanceof Error ? err.message : 'Failed to load datasource';
      setError(errorMessage);
      throw err;
    } finally {
      setIsLoading(false);
    }
  };

  const updateDataSource = async (
    projectId: string,
    datasourceId: string,
    displayName: string,
    datasourceType: DatasourceType,
    config: DatasourceConfig
  ): Promise<ApiResponse<CreateDatasourceResponse>> => {
    setIsLoading(true);
    setError(null);

    try {
      const result = await engineApi.updateDataSource(
        projectId,
        datasourceId,
        displayName,
        datasourceType,
        config
      );

      // Update connection details if update was successful
      if (result.success) {
        const connectionDetails: ConnectionDetails = {
          datasourceId,
          projectId,
          type: datasourceType,
          displayName,
          ...config,
        };
        connect(connectionDetails);
      }

      return result;
    } catch (err) {
      const errorMessage =
        err instanceof Error ? err.message : 'Failed to update datasource';
      setError(errorMessage);
      throw err;
    } finally {
      setIsLoading(false);
    }
  };

  const deleteDataSource = async (
    projectId: string,
    datasourceId: string
  ): Promise<ApiResponse<DeleteDatasourceResponse>> => {
    setIsLoading(true);
    setError(null);

    try {
      const result = await engineApi.deleteDataSource(projectId, datasourceId);
      if (result.success) {
        disconnect(datasourceId);
      }
      return result;
    } catch (err) {
      const errorMessage =
        err instanceof Error ? err.message : 'Failed to delete datasource';
      setError(errorMessage);
      throw err;
    } finally {
      setIsLoading(false);
    }
  };

  const clearError = (): void => {
    setError(null);
  };

  return (
    <DatasourceConnectionContext.Provider
      value={{
        datasources,
        selectedDatasource,
        isConnected,
        hasSelectedTables,
        connectionDetails,
        connectionStatus,
        connect,
        disconnect,
        testConnection,
        saveDataSource,
        loadDataSource,
        loadDataSources,
        refreshSchemaSelections,
        updateDataSource,
        deleteDataSource,
        selectDatasource,
        isLoading,
        error,
        clearError,
      }}
    >
      {children}
    </DatasourceConnectionContext.Provider>
  );
};
