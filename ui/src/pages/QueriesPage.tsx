/**
 * Queries Page
 * Entry point for approved query management, gets context and passes to QueriesView
 *
 * URL Structure:
 * - /queries - Default (approved queries)
 * - /queries/view/:queryId - View specific query
 * - /queries/edit/:queryId - Edit specific query
 * - /queries/approved/new - Create new query
 */

import { ArrowLeft, Search, AlertCircle } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useNavigate, useParams, useLocation } from 'react-router-dom';

import QueriesView from '../components/QueriesView';
import { Button } from '../components/ui/Button';
import { getProviderById, getAdapterInfo } from '../constants/adapters';
import { useDatasourceConnection } from '../contexts/DatasourceConnectionContext';
import { datasourceTypeToDialect } from '../types';

type ViewMode = 'list' | 'view' | 'edit' | 'new';

interface ParsedUrl {
  queryId: string | null;
  mode: ViewMode;
}

/**
 * Parse the URL path to extract queryId and mode
 */
function parseUrl(pathname: string, basePath: string): ParsedUrl {
  // Remove base path and get the queries-specific part
  const queriesPath = pathname.replace(basePath, '').replace(/^\//, '');
  const segments = queriesPath.split('/').filter(Boolean);

  if (segments.length === 0) {
    return { queryId: null, mode: 'list' };
  }

  const first = segments[0];

  // /queries/approved or /queries/approved/new
  if (first === 'approved') {
    if (segments[1] === 'new') {
      return { queryId: null, mode: 'new' };
    }
    return { queryId: null, mode: 'list' };
  }

  // /queries/view/:queryId
  if (first === 'view' && segments[1]) {
    return { queryId: segments[1], mode: 'view' };
  }

  // /queries/edit/:queryId
  if (first === 'edit' && segments[1]) {
    return { queryId: segments[1], mode: 'edit' };
  }

  return { queryId: null, mode: 'list' };
}

const QueriesPage = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { pid } = useParams<{ pid: string }>();
  const { selectedDatasource, isConnected } = useDatasourceConnection();

  // Parse URL to get initial state
  const basePath = `/projects/${pid}/queries`;
  const parsed = useMemo(() => parseUrl(location.pathname, basePath), [location.pathname, basePath]);

  // Navigation handlers
  const handleQuerySelect = useCallback((queryId: string) => {
    navigate(`${basePath}/view/${queryId}`);
  }, [navigate, basePath]);

  const handleEditQuery = useCallback((queryId: string) => {
    navigate(`${basePath}/edit/${queryId}`);
  }, [navigate, basePath]);

  const handleCreateQuery = useCallback(() => {
    navigate(`${basePath}/approved/new`);
  }, [navigate, basePath]);

  const handleCancelEdit = useCallback(() => {
    navigate(basePath);
  }, [navigate, basePath]);

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
                <Search className="h-8 w-8 text-blue-500" />
                Approved Queries
              </h1>
              <p className="mt-2 text-text-secondary">
                Manage approved natural language queries and their
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
              <Search className="h-8 w-8 text-blue-500" />
              Approved Queries
            </h1>
            <p className="mt-2 text-text-secondary">
              Manage approved natural language queries and their
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
        filter="approved"
        initialQueryId={parsed.queryId}
        initialMode={parsed.mode}
        onQuerySelect={handleQuerySelect}
        onEditQuery={handleEditQuery}
        onCreateQuery={handleCreateQuery}
        onCancelEdit={handleCancelEdit}
      />
    </div>
  );
};

export default QueriesPage;
