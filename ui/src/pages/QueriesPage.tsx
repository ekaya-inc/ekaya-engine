/**
 * Queries Page
 * Entry point for query management, gets context and passes to QueriesView
 *
 * URL Structure:
 * - /queries - Default (approved tab)
 * - /queries/approved - Approved tab
 * - /queries/pending - Pending tab
 * - /queries/rejected - Rejected tab
 * - /queries/view/:queryId - View specific query (auto-selects tab based on query status)
 * - /queries/edit/:queryId - Edit specific query
 * - /queries/:status/new - Create new query
 */

import { ArrowLeft, Database, AlertCircle } from 'lucide-react';
import { useState, useEffect, useCallback, useMemo } from 'react';
import { useNavigate, useParams, useLocation } from 'react-router-dom';

import QueriesView, { type QueryFilterType } from '../components/QueriesView';
import { Button } from '../components/ui/Button';
import { getProviderById, getAdapterInfo } from '../constants/adapters';
import { useDatasourceConnection } from '../contexts/DatasourceConnectionContext';
import engineApi from '../services/engineApi';
import { datasourceTypeToDialect } from '../types';

type ViewMode = 'list' | 'view' | 'edit' | 'new';

interface ParsedUrl {
  status: QueryFilterType;
  queryId: string | null;
  mode: ViewMode;
}

/**
 * Parse the URL path to extract status, queryId, and mode
 */
function parseUrl(pathname: string, basePath: string): ParsedUrl {
  // Remove base path and get the queries-specific part
  const queriesPath = pathname.replace(basePath, '').replace(/^\//, '');
  const segments = queriesPath.split('/').filter(Boolean);

  // Default values
  let status: QueryFilterType = 'approved';
  let queryId: string | null = null;
  let mode: ViewMode = 'list';

  if (segments.length === 0) {
    // /queries - default to approved
    return { status: 'approved', queryId: null, mode: 'list' };
  }

  const first = segments[0];

  // Check for status-only paths: /queries/approved, /queries/pending, /queries/rejected
  if (first === 'approved' || first === 'pending' || first === 'rejected') {
    status = first;

    // Check for /queries/:status/new
    if (segments[1] === 'new') {
      mode = 'new';
    }
    return { status, queryId, mode };
  }

  // Check for /queries/view/:queryId
  if (first === 'view' && segments[1]) {
    queryId = segments[1];
    mode = 'view';
    return { status, queryId, mode };
  }

  // Check for /queries/edit/:queryId
  if (first === 'edit' && segments[1]) {
    queryId = segments[1];
    mode = 'edit';
    return { status, queryId, mode };
  }

  // Unknown path, default to approved
  return { status: 'approved', queryId: null, mode: 'list' };
}

const QueriesPage = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { pid } = useParams<{ pid: string }>();
  const { selectedDatasource, isConnected } = useDatasourceConnection();
  const [pendingCount, setPendingCount] = useState<number>(0);

  // Parse URL to get initial state
  const basePath = `/projects/${pid}/queries`;
  const parsed = useMemo(() => parseUrl(location.pathname, basePath), [location.pathname, basePath]);

  // Track active filter from URL (can be overridden when viewing a query with different status)
  const [activeFilter, setActiveFilter] = useState<QueryFilterType>(parsed.status);

  // Sync activeFilter when URL changes (but not when viewing a specific query)
  useEffect(() => {
    if (parsed.mode === 'list' || parsed.mode === 'new') {
      setActiveFilter(parsed.status);
    }
  }, [parsed.status, parsed.mode]);

  // Fetch pending query count
  const fetchPendingCount = useCallback(async () => {
    if (!pid) return;
    try {
      const response = await engineApi.listPendingQueries(pid);
      if (response.success && response.data) {
        setPendingCount(response.data.count);
      }
    } catch {
      // Silently fail - badge just won't show
    }
  }, [pid]);

  useEffect(() => {
    fetchPendingCount();
  }, [fetchPendingCount]);

  // Navigation handlers
  const handleTabChange = useCallback((status: QueryFilterType) => {
    setActiveFilter(status);
    navigate(`${basePath}/${status}`);
  }, [navigate, basePath]);

  const handleQuerySelect = useCallback((queryId: string) => {
    navigate(`${basePath}/view/${queryId}`);
  }, [navigate, basePath]);

  const handleEditQuery = useCallback((queryId: string) => {
    navigate(`${basePath}/edit/${queryId}`);
  }, [navigate, basePath]);

  const handleCreateQuery = useCallback(() => {
    navigate(`${basePath}/${activeFilter}/new`);
  }, [navigate, basePath, activeFilter]);

  const handleCancelEdit = useCallback(() => {
    // Go back to the current tab
    navigate(`${basePath}/${activeFilter}`);
  }, [navigate, basePath, activeFilter]);

  // Used for auto-switching tabs when viewing a query via deep link (no navigation)
  const handleFilterChange = useCallback((newStatus: QueryFilterType) => {
    setActiveFilter(newStatus);
  }, []);

  // Used after status change actions (approve/reject/move) - updates filter AND navigates
  const handleQueryStatusChangeComplete = useCallback((newStatus: QueryFilterType) => {
    setActiveFilter(newStatus);
    navigate(`${basePath}/${newStatus}`);
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

      {/* Tab Menu */}
      <div className="border-b border-border-light mb-6">
        <nav className="flex gap-6" aria-label="Query filters">
          <button
            onClick={() => handleTabChange('approved')}
            className={`pb-3 text-sm font-medium border-b-2 transition-colors ${
              activeFilter === 'approved'
                ? 'border-brand-purple text-brand-purple'
                : 'border-transparent text-text-secondary hover:text-text-primary hover:border-border-medium'
            }`}
          >
            Approved
          </button>
          <button
            onClick={() => handleTabChange('pending')}
            className={`pb-3 text-sm font-medium border-b-2 transition-colors flex items-center gap-2 ${
              activeFilter === 'pending'
                ? 'border-brand-purple text-brand-purple'
                : 'border-transparent text-text-secondary hover:text-text-primary hover:border-border-medium'
            }`}
          >
            Pending Approval
            {pendingCount > 0 && (
              <span className={`inline-flex items-center justify-center px-2 py-0.5 text-xs font-medium rounded-full ${
                activeFilter === 'pending'
                  ? 'bg-brand-purple/10 text-brand-purple'
                  : 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300'
              }`}>
                {pendingCount}
              </span>
            )}
          </button>
          <button
            onClick={() => handleTabChange('rejected')}
            className={`pb-3 text-sm font-medium border-b-2 transition-colors ${
              activeFilter === 'rejected'
                ? 'border-brand-purple text-brand-purple'
                : 'border-transparent text-text-secondary hover:text-text-primary hover:border-border-medium'
            }`}
          >
            Rejected
          </button>
        </nav>
      </div>

      {/* Queries Management Interface */}
      <QueriesView
        projectId={pid}
        datasourceId={selectedDatasource.datasourceId}
        dialect={dialect}
        filter={activeFilter}
        initialQueryId={parsed.queryId}
        initialMode={parsed.mode}
        onPendingCountChange={fetchPendingCount}
        onQuerySelect={handleQuerySelect}
        onEditQuery={handleEditQuery}
        onCreateQuery={handleCreateQuery}
        onCancelEdit={handleCancelEdit}
        onFilterChange={handleFilterChange}
        onQueryStatusChangeComplete={handleQueryStatusChangeComplete}
      />
    </div>
  );
};

export default QueriesPage;
