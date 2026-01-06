/**
 * Queries Page
 * Entry point for query management, gets context and passes to QueriesView
 */

import { ArrowLeft, Database, AlertCircle } from 'lucide-react';
import { useNavigate, useParams } from 'react-router-dom';

import QueriesView from '../components/QueriesView';
import { Button } from '../components/ui/Button';
import { getProviderById, getAdapterInfo } from '../constants/adapters';
import { useDatasourceConnection } from '../contexts/DatasourceConnectionContext';
import { datasourceTypeToDialect } from '../types';

const QueriesPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { selectedDatasource, isConnected } = useDatasourceConnection();

  // Derive dialect from datasource type
  const dialect = selectedDatasource?.type
    ? datasourceTypeToDialect[selectedDatasource.type]
    : 'PostgreSQL';

  // Get display info for datasource (provider-specific if available, otherwise adapter info)
  const providerInfo = selectedDatasource?.provider
    ? getProviderById(selectedDatasource.provider)
    : undefined;
  const adapterInfo = getAdapterInfo(selectedDatasource?.type);
  const displayInfo = providerInfo ?? adapterInfo;

  // No datasource selected - show message
  if (!isConnected || !selectedDatasource?.datasourceId || !pid) {
    return (
      <div className="mx-auto max-w-7xl">
        {/* Header */}
        <div className="mb-6">
          <Button
            variant="ghost"
            onClick={() => navigate(`/projects/${pid}`)}
            className="mb-4"
          >
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-3xl font-bold text-text-primary flex items-center gap-2">
                <Database className="h-8 w-8 text-blue-500" />
                Pre-Approved Queries
              </h1>
              <p className="mt-2 text-text-secondary">
                Manage pre-approved natural language queries and their
                corresponding SQL
              </p>
            </div>
          </div>
        </div>

        {/* No Datasource Message */}
        <div className="flex items-center justify-center h-[calc(100vh-16rem)]">
          <div className="text-center">
            <AlertCircle className="h-12 w-12 text-amber-500 mx-auto mb-4" />
            <h2 className="text-xl font-medium text-text-primary mb-2">
              No Datasource Connected
            </h2>
            <p className="text-text-secondary mb-4 max-w-md">
              Please connect to a datasource before managing queries. Queries
              are scoped to a specific datasource.
            </p>
            <Button onClick={() => navigate(`/projects/${pid}/datasource`)}>
              Configure Datasource
            </Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-7xl">
      {/* Header */}
      <div className="mb-6">
        <Button
          variant="ghost"
          onClick={() => navigate(`/projects/${pid}`)}
          className="mb-4"
        >
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Dashboard
        </Button>
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-text-primary flex items-center gap-2">
              <Database className="h-8 w-8 text-blue-500" />
              Pre-Approved Queries
            </h1>
            <p className="mt-2 text-text-secondary">
              Manage pre-approved natural language queries and their
              corresponding SQL
            </p>
          </div>
          <div className="flex items-center gap-3">
            {displayInfo.icon && (
              <img
                src={displayInfo.icon}
                alt={displayInfo.name}
                className="h-10 w-10 object-contain"
              />
            )}
            <div className="text-right">
              <p className="text-sm text-text-tertiary">Datasource</p>
              <p className="text-sm font-medium text-text-primary">
                {selectedDatasource.displayName ?? selectedDatasource.name}
              </p>
              <p className="text-xs text-text-tertiary">
                {displayInfo.name}
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* Queries Management Interface */}
      <QueriesView
        projectId={pid}
        datasourceId={selectedDatasource.datasourceId}
        dialect={dialect}
      />
    </div>
  );
};

export default QueriesPage;
