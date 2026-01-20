import { AlertTriangle, XCircle } from 'lucide-react';
import { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';

import DatasourceAdapterSelection from '../components/DatasourceAdapterSelection';
import DatasourceConfiguration from '../components/DatasourceConfiguration';
import { Button } from '../components/ui/Button';
import type { ProviderInfo } from '../constants/adapters';
import { useDatasourceConnection } from '../contexts/DatasourceConnectionContext';

const DatasourcePage = () => {
  const { pid } = useParams<{ pid: string }>();
  const { datasources, selectedDatasource, deleteDataSource } = useDatasourceConnection();
  const [selectedAdapter, setSelectedAdapter] = useState<string | null>(null);
  const [selectedProvider, setSelectedProvider] = useState<ProviderInfo | undefined>(undefined);
  const [showSetup, setShowSetup] = useState<boolean>(false);
  const [isDisconnecting, setIsDisconnecting] = useState<boolean>(false);

  // Set selected adapter and show config when selectedDatasource exists
  useEffect(() => {
    if (selectedDatasource) {
      setSelectedAdapter(selectedDatasource.type);
      setShowSetup(true);
    }
  }, [selectedDatasource]);

  const handleAdapterSelect = (adapterId: string, provider?: ProviderInfo): void => {
    setSelectedAdapter(adapterId);
    setSelectedProvider(provider);
    setShowSetup(true);
  };

  const handleBackToSelection = (): void => {
    setShowSetup(false);
    setSelectedAdapter(null);
    setSelectedProvider(undefined);
  };

  const handleDisconnectDatasource = async (): Promise<void> => {
    if (!pid || !selectedDatasource?.datasourceId) return;

    setIsDisconnecting(true);
    try {
      await deleteDataSource(pid, selectedDatasource.datasourceId);
    } catch (error) {
      console.error('Failed to disconnect datasource:', error);
    } finally {
      setIsDisconnecting(false);
    }
  };

  // Check if any datasource has decryption failure
  const failedDatasource = datasources.find((ds) => ds.decryption_failed === true);

  if (showSetup) {
    return (
      <DatasourceConfiguration
        selectedAdapter={selectedAdapter}
        selectedProvider={selectedProvider}
        onBackToSelection={handleBackToSelection}
      />
    );
  }

  return (
    <div className="mx-auto max-w-6xl">
      {failedDatasource && (
        <div className="mb-6 p-4 bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-800 rounded-lg">
          <div className="flex items-start gap-3">
            <AlertTriangle className="w-5 h-5 text-red-600 dark:text-red-400 mt-0.5 flex-shrink-0" />
            <div className="flex-1">
              <h3 className="text-sm font-semibold text-red-900 dark:text-red-100 mb-1">
                Credential Key Mismatch
              </h3>
              <p className="text-sm text-red-800 dark:text-red-200 mb-3">
                The datasource credentials were encrypted with a different <code className="px-1 py-0.5 bg-red-100 dark:bg-red-900 rounded text-xs">PROJECT_CREDENTIALS_KEY</code> than the one currently configured.
                This typically happens in multi-environment setups where different configuration files are used.
              </p>
              {failedDatasource.error_message && (
                <p className="text-xs text-red-700 dark:text-red-300 mb-3 font-mono bg-red-100 dark:bg-red-900 p-2 rounded">
                  {failedDatasource.error_message}
                </p>
              )}
              <div className="flex gap-3">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleDisconnectDatasource}
                  disabled={isDisconnecting}
                  className="text-red-600 border-red-300 hover:bg-red-100 dark:text-red-400 dark:border-red-700 dark:hover:bg-red-900"
                >
                  {isDisconnecting ? (
                    <>
                      <XCircle className="w-4 h-4 mr-2 animate-spin" />
                      Disconnecting...
                    </>
                  ) : (
                    <>
                      <XCircle className="w-4 h-4 mr-2" />
                      Disconnect Datasource
                    </>
                  )}
                </Button>
                <div className="flex-1 text-xs text-red-700 dark:text-red-300">
                  <strong>Alternative:</strong> Restart the server with the correct <code className="px-1 py-0.5 bg-red-100 dark:bg-red-900 rounded">PROJECT_CREDENTIALS_KEY</code> that was used to encrypt these credentials.
                </div>
              </div>
            </div>
          </div>
        </div>
      )}
      <DatasourceAdapterSelection
        selectedAdapter={selectedAdapter}
        onAdapterSelect={handleAdapterSelect}
        datasources={datasources}
      />
    </div>
  );
};

export default DatasourcePage;
