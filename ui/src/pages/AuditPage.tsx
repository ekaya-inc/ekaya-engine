/**
 * Audit Page
 * Tabbed interface showing query executions, ontology changes, schema changes,
 * and query approvals. Manual refresh only (no polling).
 */

import {
  ArrowLeft,
  ScrollText,
  RefreshCw,
  Loader2,
  AlertTriangle,
  XCircle,
  CheckCircle,
  ChevronDown,
  ChevronRight,
  Clock,
  Database,
  Shield,
  FileEdit,
  GitPullRequest,
  Terminal,
  Bell,
} from 'lucide-react';
import { Fragment, useState, useEffect, useCallback, useRef } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import { Button } from '../components/ui/Button';
import { Card, CardContent } from '../components/ui/Card';
import { useToast } from '../hooks/useToast';
import engineApi from '../services/engineApi';
import type {
  AuditAlert,
  AuditSummary,
  MCPAuditEvent,
  OntologyChange,
  PaginatedResponse,
  QueryApproval,
  QueryExecution,
  ResolveAlertRequest,
  SchemaChange,
} from '../types';

type AuditTab = 'query-executions' | 'ontology-changes' | 'schema-changes' | 'query-approvals' | 'mcp-events' | 'alerts';

// Each summary card maps to a tab (and optionally a filter preset)
type SummaryCardAction =
  | { tab: AuditTab; filter?: undefined }
  | { tab: 'query-executions'; filter: 'failed' };

// Time range presets
type TimeRange = '24h' | '7d' | '30d' | 'all';

function getTimeRangeSince(range: TimeRange): string | undefined {
  if (range === 'all') return undefined;
  const now = new Date();
  const hours = range === '24h' ? 24 : range === '7d' ? 168 : 720;
  return new Date(now.getTime() - hours * 60 * 60 * 1000).toISOString();
}

// Format date for display
function formatDate(dateStr: string): string {
  const d = new Date(dateStr);
  return d.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

// Format duration in ms
function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

// Truncate SQL for display
function truncateSQL(sql: string, maxLen = 80): string {
  if (sql.length <= maxLen) return sql;
  return sql.substring(0, maxLen) + '...';
}

// ============================================================================
// Audit Summary Header
// ============================================================================

function AuditSummaryHeader({
  summary,
  activeTab,
  activeFilter,
  onCardClick,
}: {
  summary: AuditSummary | null;
  activeTab: AuditTab;
  activeFilter?: 'failed';
  onCardClick: (action: SummaryCardAction) => void;
}) {
  if (!summary) return null;

  const totalOpenAlerts = summary.open_alerts_critical + summary.open_alerts_warning + summary.open_alerts_info;

  const stats: {
    label: string;
    value: number;
    icon: typeof Database;
    warn?: boolean;
    critical?: boolean;
    action: SummaryCardAction;
  }[] = [
    { label: 'Query Executions (30d)', value: summary.total_query_executions, icon: Database, action: { tab: 'query-executions' } },
    { label: 'Failed Queries', value: summary.failed_query_count, icon: XCircle, warn: summary.failed_query_count > 0, action: { tab: 'query-executions', filter: 'failed' } },
    { label: 'Ontology Changes', value: summary.ontology_changes_count, icon: FileEdit, action: { tab: 'ontology-changes' } },
    { label: 'Pending Schema', value: summary.pending_schema_changes, icon: GitPullRequest, warn: summary.pending_schema_changes > 0, action: { tab: 'schema-changes' } },
    { label: 'Pending Approvals', value: summary.pending_query_approvals, icon: Shield, warn: summary.pending_query_approvals > 0, action: { tab: 'query-approvals' } },
    { label: 'MCP Events', value: summary.mcp_events_count, icon: Terminal, action: { tab: 'mcp-events' } },
    { label: 'Open Alerts', value: totalOpenAlerts, icon: Bell, warn: summary.open_alerts_critical > 0, critical: summary.open_alerts_critical > 0, action: { tab: 'alerts' } },
  ];

  const isCardActive = (action: SummaryCardAction): boolean => {
    if (action.tab !== activeTab) return false;
    // For query-executions tab, distinguish between "all" and "failed" cards
    if (action.tab === 'query-executions') {
      return (action.filter ?? undefined) === activeFilter;
    }
    return true;
  };

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-7 mb-6">
      {stats.map(({ label, value, icon: Icon, warn, critical, action }) => {
        const isActive = isCardActive(action);
        return (
          <Card
            key={label}
            className={`cursor-pointer transition-all hover:shadow-md ${
              isActive ? 'ring-2 ring-brand-purple shadow-md' : ''
            }`}
            onClick={() => onCardClick(action)}
          >
            <CardContent className="p-3">
              <div className="flex items-center gap-2 mb-1">
                <Icon className={`h-4 w-4 ${critical ? 'text-red-500' : warn ? 'text-amber-500' : 'text-text-tertiary'}`} />
                <span className="text-xs text-text-secondary truncate">{label}</span>
              </div>
              <p className={`text-xl font-semibold ${critical ? 'text-red-600 dark:text-red-400' : warn ? 'text-amber-600 dark:text-amber-400' : 'text-text-primary'}`}>
                {value.toLocaleString()}
              </p>
              {label === 'Open Alerts' && totalOpenAlerts > 0 && (
                <div className="flex gap-1 mt-1">
                  {summary.open_alerts_critical > 0 && (
                    <span className="text-[10px] px-1 py-0.5 rounded bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300">
                      {summary.open_alerts_critical} critical
                    </span>
                  )}
                  {summary.open_alerts_warning > 0 && (
                    <span className="text-[10px] px-1 py-0.5 rounded bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300">
                      {summary.open_alerts_warning} warn
                    </span>
                  )}
                  {summary.open_alerts_info > 0 && (
                    <span className="text-[10px] px-1 py-0.5 rounded bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300">
                      {summary.open_alerts_info} info
                    </span>
                  )}
                </div>
              )}
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}

// ============================================================================
// Pagination
// ============================================================================

function Pagination({
  total,
  limit,
  offset,
  onPageChange,
}: {
  total: number;
  limit: number;
  offset: number;
  onPageChange: (newOffset: number) => void;
}) {
  if (total <= limit) return null;

  const currentPage = Math.floor(offset / limit) + 1;
  const totalPages = Math.ceil(total / limit);

  return (
    <div className="flex items-center justify-between mt-4 text-sm text-text-secondary">
      <span>
        Showing {offset + 1}–{Math.min(offset + limit, total)} of {total}
      </span>
      <div className="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={currentPage <= 1}
          onClick={() => onPageChange(Math.max(0, offset - limit))}
        >
          Previous
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={currentPage >= totalPages}
          onClick={() => onPageChange(offset + limit)}
        >
          Next
        </Button>
      </div>
    </div>
  );
}

// ============================================================================
// Time Range Filter
// ============================================================================

function TimeRangeFilter({ value, onChange }: { value: TimeRange; onChange: (v: TimeRange) => void }) {
  return (
    <div className="flex gap-1">
      {([
        ['24h', 'Last 24h'],
        ['7d', 'Last 7d'],
        ['30d', 'Last 30d'],
        ['all', 'All'],
      ] as [TimeRange, string][]).map(([key, label]) => (
        <button
          key={key}
          onClick={() => onChange(key)}
          className={`px-2 py-1 text-xs rounded ${
            value === key
              ? 'bg-brand-purple text-white'
              : 'bg-surface-secondary text-text-secondary hover:text-text-primary'
          }`}
        >
          {label}
        </button>
      ))}
    </div>
  );
}

// ============================================================================
// Query Executions Tab
// ============================================================================

function QueryExecutionsTab({ projectId, initialFilter }: { projectId: string; initialFilter?: 'failed' }) {
  const [data, setData] = useState<PaginatedResponse<QueryExecution> | null>(null);
  const [loading, setLoading] = useState(true);
  const [timeRange, setTimeRange] = useState<TimeRange>('7d');
  const [sourceFilter, setSourceFilter] = useState('');
  const [successFilter, setSuccessFilter] = useState(initialFilter === 'failed' ? 'false' : '');
  const [destructiveFilter, setDestructiveFilter] = useState(false);
  const [userInput, setUserInput] = useState('');
  const [userFilter, setUserFilter] = useState('');
  const [queryIdInput, setQueryIdInput] = useState('');
  const [queryIdFilter, setQueryIdFilter] = useState('');
  const [offset, setOffset] = useState(0);
  const userDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const queryIdDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Debounce user input
  useEffect(() => {
    if (userDebounceRef.current) clearTimeout(userDebounceRef.current);
    userDebounceRef.current = setTimeout(() => { setUserFilter(userInput); }, 300);
    return () => { if (userDebounceRef.current) clearTimeout(userDebounceRef.current); };
  }, [userInput]);

  // Debounce query ID input
  useEffect(() => {
    if (queryIdDebounceRef.current) clearTimeout(queryIdDebounceRef.current);
    queryIdDebounceRef.current = setTimeout(() => { setQueryIdFilter(queryIdInput); }, 300);
    return () => { if (queryIdDebounceRef.current) clearTimeout(queryIdDebounceRef.current); };
  }, [queryIdInput]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, string> = { limit: '50', offset: String(offset) };
      const since = getTimeRangeSince(timeRange);
      if (since) params.since = since;
      if (sourceFilter) params.source = sourceFilter;
      if (successFilter) params.success = successFilter;
      if (destructiveFilter) params.is_modifying = 'true';
      if (userFilter) params.user_id = userFilter;
      if (queryIdFilter) params.query_id = queryIdFilter;

      const response = await engineApi.listAuditQueryExecutions(projectId, params);
      if (response.success && response.data) {
        setData(response.data);
      }
    } catch (error) {
      console.error('Failed to fetch query executions:', error);
    } finally {
      setLoading(false);
    }
  }, [projectId, timeRange, sourceFilter, successFilter, destructiveFilter, userFilter, queryIdFilter, offset]);

  useEffect(() => { fetchData(); }, [fetchData]);

  // Reset offset when filters change
  useEffect(() => { setOffset(0); }, [timeRange, sourceFilter, successFilter, destructiveFilter, userFilter, queryIdFilter]);

  return (
    <div>
      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3 mb-4">
        <TimeRangeFilter value={timeRange} onChange={setTimeRange} />
        <select
          value={sourceFilter}
          onChange={e => setSourceFilter(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary"
        >
          <option value="">All Sources</option>
          <option value="mcp">MCP</option>
          <option value="api">API</option>
          <option value="ui">UI</option>
        </select>
        <select
          value={successFilter}
          onChange={e => setSuccessFilter(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary"
        >
          <option value="">All Results</option>
          <option value="true">Success</option>
          <option value="false">Failed</option>
        </select>
        <label className="flex items-center gap-1 text-xs text-text-secondary">
          <input
            type="checkbox"
            checked={destructiveFilter}
            onChange={e => setDestructiveFilter(e.target.checked)}
            className="rounded"
          />
          Destructive only
        </label>
        <input
          type="text"
          placeholder="Filter by user..."
          value={userInput}
          onChange={e => setUserInput(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary w-36"
        />
        <input
          type="text"
          placeholder="Query ID..."
          value={queryIdInput}
          onChange={e => setQueryIdInput(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary w-36"
        />
      </div>

      {loading ? (
        <div className="flex justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-text-secondary" />
        </div>
      ) : !data || data.items.length === 0 ? (
        <div className="text-center py-12 text-text-secondary">
          <Database className="h-8 w-8 mx-auto mb-2 text-text-tertiary" />
          <p>No query executions found</p>
        </div>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border-light text-left text-text-secondary">
                  <th className="pb-2 pr-3 font-medium">Time</th>
                  <th className="pb-2 pr-3 font-medium">User</th>
                  <th className="pb-2 pr-3 font-medium">Query Name</th>
                  <th className="pb-2 pr-3 font-medium">SQL</th>
                  <th className="pb-2 pr-3 font-medium text-right">Duration</th>
                  <th className="pb-2 pr-3 font-medium text-right">Rows</th>
                  <th className="pb-2 pr-3 font-medium">Status</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map(row => (
                  <tr
                    key={row.id}
                    className={`border-b border-border-light/50 ${
                      !row.success ? 'bg-red-50/50 dark:bg-red-900/10' :
                      row.is_modifying ? 'bg-amber-50/50 dark:bg-amber-900/10' : ''
                    }`}
                  >
                    <td className="py-2 pr-3 whitespace-nowrap text-text-secondary">
                      <div className="flex items-center gap-1">
                        <Clock className="h-3 w-3" />
                        {formatDate(row.executed_at)}
                      </div>
                    </td>
                    <td className="py-2 pr-3 text-text-primary truncate max-w-[120px]">
                      {row.user_id ?? '–'}
                    </td>
                    <td className="py-2 pr-3 text-text-primary truncate max-w-[160px]">
                      {row.query_name ?? '–'}
                    </td>
                    <td className="py-2 pr-3 font-mono text-xs text-text-secondary truncate max-w-[200px]" title={row.sql}>
                      {truncateSQL(row.sql)}
                    </td>
                    <td className="py-2 pr-3 text-right text-text-secondary whitespace-nowrap">
                      {formatDuration(row.execution_time_ms)}
                    </td>
                    <td className="py-2 pr-3 text-right text-text-secondary">
                      {row.row_count.toLocaleString()}
                    </td>
                    <td className="py-2 pr-3">
                      <div className="flex items-center gap-1">
                        {!row.success ? (
                          <span className="flex items-center gap-1 text-red-600 dark:text-red-400">
                            <XCircle className="h-3.5 w-3.5" /> Failed
                          </span>
                        ) : row.is_modifying ? (
                          <span className="flex items-center gap-1 text-amber-600 dark:text-amber-400">
                            <AlertTriangle className="h-3.5 w-3.5" /> Destructive
                          </span>
                        ) : (
                          <span className="flex items-center gap-1 text-green-600 dark:text-green-400">
                            <CheckCircle className="h-3.5 w-3.5" /> OK
                          </span>
                        )}
                        <span className="ml-1 text-xs px-1.5 py-0.5 rounded bg-surface-secondary text-text-tertiary">
                          {row.source}
                        </span>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination total={data.total} limit={data.limit} offset={data.offset} onPageChange={setOffset} />
        </>
      )}
    </div>
  );
}

// ============================================================================
// Ontology Changes Tab
// ============================================================================

function OntologyChangesTab({ projectId }: { projectId: string }) {
  const [data, setData] = useState<PaginatedResponse<OntologyChange> | null>(null);
  const [loading, setLoading] = useState(true);
  const [timeRange, setTimeRange] = useState<TimeRange>('7d');
  const [entityTypeFilter, setEntityTypeFilter] = useState('');
  const [actionFilter, setActionFilter] = useState('');
  const [sourceFilter, setSourceFilter] = useState('');
  const [userInput, setUserInput] = useState('');
  const [userFilter, setUserFilter] = useState('');
  const [offset, setOffset] = useState(0);
  const [expandedRow, setExpandedRow] = useState<string | null>(null);
  const userDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Debounce user input
  useEffect(() => {
    if (userDebounceRef.current) clearTimeout(userDebounceRef.current);
    userDebounceRef.current = setTimeout(() => { setUserFilter(userInput); }, 300);
    return () => { if (userDebounceRef.current) clearTimeout(userDebounceRef.current); };
  }, [userInput]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, string> = { limit: '50', offset: String(offset) };
      const since = getTimeRangeSince(timeRange);
      if (since) params.since = since;
      if (entityTypeFilter) params.entity_type = entityTypeFilter;
      if (actionFilter) params.action = actionFilter;
      if (sourceFilter) params.source = sourceFilter;
      if (userFilter) params.user_id = userFilter;

      const response = await engineApi.listAuditOntologyChanges(projectId, params);
      if (response.success && response.data) {
        setData(response.data);
      }
    } catch (error) {
      console.error('Failed to fetch ontology changes:', error);
    } finally {
      setLoading(false);
    }
  }, [projectId, timeRange, entityTypeFilter, actionFilter, sourceFilter, userFilter, offset]);

  useEffect(() => { fetchData(); }, [fetchData]);
  useEffect(() => { setOffset(0); }, [timeRange, entityTypeFilter, actionFilter, sourceFilter, userFilter]);

  return (
    <div>
      <div className="flex flex-wrap items-center gap-3 mb-4">
        <TimeRangeFilter value={timeRange} onChange={setTimeRange} />
        <select
          value={entityTypeFilter}
          onChange={e => setEntityTypeFilter(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary"
        >
          <option value="">All Entity Types</option>
          <option value="entity">Entity</option>
          <option value="relationship">Relationship</option>
          <option value="glossary_term">Glossary Term</option>
          <option value="project_knowledge">Project Knowledge</option>
        </select>
        <select
          value={actionFilter}
          onChange={e => setActionFilter(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary"
        >
          <option value="">All Actions</option>
          <option value="create">Create</option>
          <option value="update">Update</option>
          <option value="delete">Delete</option>
        </select>
        <select
          value={sourceFilter}
          onChange={e => setSourceFilter(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary"
        >
          <option value="">All Sources</option>
          <option value="inferred">Inferred</option>
          <option value="mcp">MCP</option>
          <option value="manual">Manual</option>
        </select>
        <input
          type="text"
          placeholder="Filter by user..."
          value={userInput}
          onChange={e => setUserInput(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary w-36"
        />
      </div>

      {loading ? (
        <div className="flex justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-text-secondary" />
        </div>
      ) : !data || data.items.length === 0 ? (
        <div className="text-center py-12 text-text-secondary">
          <FileEdit className="h-8 w-8 mx-auto mb-2 text-text-tertiary" />
          <p>No ontology changes found</p>
        </div>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border-light text-left text-text-secondary">
                  <th className="pb-2 pr-3 font-medium w-6"></th>
                  <th className="pb-2 pr-3 font-medium">Time</th>
                  <th className="pb-2 pr-3 font-medium">User</th>
                  <th className="pb-2 pr-3 font-medium">Entity Type</th>
                  <th className="pb-2 pr-3 font-medium">Action</th>
                  <th className="pb-2 pr-3 font-medium">Source</th>
                  <th className="pb-2 pr-3 font-medium">Changed Fields</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map(row => {
                  const isExpanded = expandedRow === row.id;
                  const hasChanges = row.changed_fields && Object.keys(row.changed_fields).length > 0;
                  return (
                    <Fragment key={row.id}>
                      <tr
                        className={`border-b border-border-light/50 ${hasChanges ? 'cursor-pointer hover:bg-surface-secondary/50' : ''}`}
                        onClick={() => hasChanges && setExpandedRow(isExpanded ? null : row.id)}
                      >
                        <td className="py-2 pr-1">
                          {hasChanges && (
                            isExpanded ? <ChevronDown className="h-4 w-4 text-text-tertiary" /> : <ChevronRight className="h-4 w-4 text-text-tertiary" />
                          )}
                        </td>
                        <td className="py-2 pr-3 whitespace-nowrap text-text-secondary">{formatDate(row.created_at)}</td>
                        <td className="py-2 pr-3 text-text-primary truncate max-w-[120px]">{row.user_id ?? '–'}</td>
                        <td className="py-2 pr-3">
                          <span className="px-1.5 py-0.5 text-xs rounded bg-surface-secondary text-text-primary">{row.entity_type}</span>
                        </td>
                        <td className="py-2 pr-3">
                          <span className={`px-1.5 py-0.5 text-xs rounded ${
                            row.action === 'create' ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300' :
                            row.action === 'delete' ? 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300' :
                            'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300'
                          }`}>
                            {row.action}
                          </span>
                        </td>
                        <td className="py-2 pr-3 text-text-secondary">{row.source}</td>
                        <td className="py-2 pr-3 text-text-secondary text-xs">
                          {hasChanges ? `${Object.keys(row.changed_fields!).length} field(s)` : '–'}
                        </td>
                      </tr>
                      {isExpanded && hasChanges && (
                        <tr>
                          <td colSpan={7} className="py-2 px-4 bg-surface-secondary/30">
                            <div className="font-mono text-xs space-y-1 max-h-60 overflow-y-auto">
                              {Object.entries(row.changed_fields!).map(([field, change]) => (
                                <div key={field} className="flex gap-2">
                                  <span className="font-semibold text-text-primary min-w-[120px]">{field}:</span>
                                  <span className="text-red-600 dark:text-red-400 line-through">{JSON.stringify(change.old)}</span>
                                  <span className="text-text-tertiary">&rarr;</span>
                                  <span className="text-green-600 dark:text-green-400">{JSON.stringify(change.new)}</span>
                                </div>
                              ))}
                            </div>
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  );
                })}
              </tbody>
            </table>
          </div>
          <Pagination total={data.total} limit={data.limit} offset={data.offset} onPageChange={setOffset} />
        </>
      )}
    </div>
  );
}

// ============================================================================
// Schema Changes Tab
// ============================================================================

function SchemaChangesTab({ projectId }: { projectId: string }) {
  const [data, setData] = useState<PaginatedResponse<SchemaChange> | null>(null);
  const [loading, setLoading] = useState(true);
  const [timeRange, setTimeRange] = useState<TimeRange>('30d');
  const [changeTypeFilter, setChangeTypeFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [tableNameInput, setTableNameInput] = useState('');
  const [tableNameFilter, setTableNameFilter] = useState('');
  const [offset, setOffset] = useState(0);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Debounce table name input
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setTableNameFilter(tableNameInput);
    }, 300);
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [tableNameInput]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, string> = { limit: '50', offset: String(offset) };
      const since = getTimeRangeSince(timeRange);
      if (since) params.since = since;
      if (changeTypeFilter) params.change_type = changeTypeFilter;
      if (statusFilter) params.status = statusFilter;
      if (tableNameFilter) params.table_name = tableNameFilter;

      const response = await engineApi.listAuditSchemaChanges(projectId, params);
      if (response.success && response.data) {
        setData(response.data);
      }
    } catch (error) {
      console.error('Failed to fetch schema changes:', error);
    } finally {
      setLoading(false);
    }
  }, [projectId, timeRange, changeTypeFilter, statusFilter, tableNameFilter, offset]);

  useEffect(() => { fetchData(); }, [fetchData]);
  useEffect(() => { setOffset(0); }, [timeRange, changeTypeFilter, statusFilter, tableNameFilter]);

  const statusColor = (status: string) => {
    switch (status) {
      case 'pending': return 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300';
      case 'approved': return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300';
      case 'rejected': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300';
      case 'auto_applied': return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300';
      default: return 'bg-surface-secondary text-text-secondary';
    }
  };

  return (
    <div>
      <div className="flex flex-wrap items-center gap-3 mb-4">
        <TimeRangeFilter value={timeRange} onChange={setTimeRange} />
        <select
          value={changeTypeFilter}
          onChange={e => setChangeTypeFilter(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary"
        >
          <option value="">All Change Types</option>
          <option value="new_table">New Table</option>
          <option value="dropped_table">Dropped Table</option>
          <option value="new_column">New Column</option>
          <option value="dropped_column">Dropped Column</option>
          <option value="modified_column">Modified Column</option>
          <option value="new_enum_value">New Enum Value</option>
          <option value="cardinality_change">Cardinality Change</option>
          <option value="new_fk_pattern">New FK Pattern</option>
        </select>
        <select
          value={statusFilter}
          onChange={e => setStatusFilter(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary"
        >
          <option value="">All Statuses</option>
          <option value="pending">Pending</option>
          <option value="approved">Approved</option>
          <option value="rejected">Rejected</option>
          <option value="auto_applied">Auto Applied</option>
        </select>
        <input
          type="text"
          placeholder="Filter by table name..."
          value={tableNameInput}
          onChange={e => setTableNameInput(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary w-40"
        />
      </div>

      {loading ? (
        <div className="flex justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-text-secondary" />
        </div>
      ) : !data || data.items.length === 0 ? (
        <div className="text-center py-12 text-text-secondary">
          <GitPullRequest className="h-8 w-8 mx-auto mb-2 text-text-tertiary" />
          <p>No schema changes found</p>
        </div>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border-light text-left text-text-secondary">
                  <th className="pb-2 pr-3 font-medium">Time</th>
                  <th className="pb-2 pr-3 font-medium">Change Type</th>
                  <th className="pb-2 pr-3 font-medium">Table</th>
                  <th className="pb-2 pr-3 font-medium">Column</th>
                  <th className="pb-2 pr-3 font-medium">Status</th>
                  <th className="pb-2 pr-3 font-medium">Reviewed By</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map(row => (
                  <tr key={row.id} className="border-b border-border-light/50">
                    <td className="py-2 pr-3 whitespace-nowrap text-text-secondary">{formatDate(row.created_at)}</td>
                    <td className="py-2 pr-3">
                      <span className="px-1.5 py-0.5 text-xs rounded bg-surface-secondary text-text-primary">
                        {row.change_type.replace(/_/g, ' ')}
                      </span>
                    </td>
                    <td className="py-2 pr-3 text-text-primary font-mono text-xs">{row.table_name ?? '–'}</td>
                    <td className="py-2 pr-3 text-text-primary font-mono text-xs">{row.column_name ?? '–'}</td>
                    <td className="py-2 pr-3">
                      <span className={`px-1.5 py-0.5 text-xs rounded ${statusColor(row.status)}`}>
                        {row.status.replace(/_/g, ' ')}
                      </span>
                    </td>
                    <td className="py-2 pr-3 text-text-secondary">{row.reviewed_by ?? '–'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination total={data.total} limit={data.limit} offset={data.offset} onPageChange={setOffset} />
        </>
      )}
    </div>
  );
}

// ============================================================================
// Query Approvals Tab
// ============================================================================

function QueryApprovalsTab({ projectId }: { projectId: string }) {
  const [data, setData] = useState<PaginatedResponse<QueryApproval> | null>(null);
  const [loading, setLoading] = useState(true);
  const [timeRange, setTimeRange] = useState<TimeRange>('30d');
  const [statusFilter, setStatusFilter] = useState('');
  const [suggestedByInput, setSuggestedByInput] = useState('');
  const [suggestedByFilter, setSuggestedByFilter] = useState('');
  const [reviewedByInput, setReviewedByInput] = useState('');
  const [reviewedByFilter, setReviewedByFilter] = useState('');
  const [offset, setOffset] = useState(0);
  const suggestedByDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reviewedByDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Debounce suggested by input
  useEffect(() => {
    if (suggestedByDebounceRef.current) clearTimeout(suggestedByDebounceRef.current);
    suggestedByDebounceRef.current = setTimeout(() => { setSuggestedByFilter(suggestedByInput); }, 300);
    return () => { if (suggestedByDebounceRef.current) clearTimeout(suggestedByDebounceRef.current); };
  }, [suggestedByInput]);

  // Debounce reviewed by input
  useEffect(() => {
    if (reviewedByDebounceRef.current) clearTimeout(reviewedByDebounceRef.current);
    reviewedByDebounceRef.current = setTimeout(() => { setReviewedByFilter(reviewedByInput); }, 300);
    return () => { if (reviewedByDebounceRef.current) clearTimeout(reviewedByDebounceRef.current); };
  }, [reviewedByInput]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, string> = { limit: '50', offset: String(offset) };
      const since = getTimeRangeSince(timeRange);
      if (since) params.since = since;
      if (statusFilter) params.status = statusFilter;
      if (suggestedByFilter) params.suggested_by = suggestedByFilter;
      if (reviewedByFilter) params.reviewed_by = reviewedByFilter;

      const response = await engineApi.listAuditQueryApprovals(projectId, params);
      if (response.success && response.data) {
        setData(response.data);
      }
    } catch (error) {
      console.error('Failed to fetch query approvals:', error);
    } finally {
      setLoading(false);
    }
  }, [projectId, timeRange, statusFilter, suggestedByFilter, reviewedByFilter, offset]);

  useEffect(() => { fetchData(); }, [fetchData]);
  useEffect(() => { setOffset(0); }, [timeRange, statusFilter, suggestedByFilter, reviewedByFilter]);

  const statusColor = (status: string) => {
    switch (status) {
      case 'pending': return 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300';
      case 'approved': return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300';
      case 'rejected': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300';
      default: return 'bg-surface-secondary text-text-secondary';
    }
  };

  return (
    <div>
      <div className="flex flex-wrap items-center gap-3 mb-4">
        <TimeRangeFilter value={timeRange} onChange={setTimeRange} />
        <select
          value={statusFilter}
          onChange={e => setStatusFilter(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary"
        >
          <option value="">All Statuses</option>
          <option value="pending">Pending</option>
          <option value="approved">Approved</option>
          <option value="rejected">Rejected</option>
        </select>
        <input
          type="text"
          placeholder="Suggested by..."
          value={suggestedByInput}
          onChange={e => setSuggestedByInput(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary w-36"
        />
        <input
          type="text"
          placeholder="Reviewed by..."
          value={reviewedByInput}
          onChange={e => setReviewedByInput(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary w-36"
        />
      </div>

      {loading ? (
        <div className="flex justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-text-secondary" />
        </div>
      ) : !data || data.items.length === 0 ? (
        <div className="text-center py-12 text-text-secondary">
          <Shield className="h-8 w-8 mx-auto mb-2 text-text-tertiary" />
          <p>No query approvals found</p>
        </div>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border-light text-left text-text-secondary">
                  <th className="pb-2 pr-3 font-medium">Time</th>
                  <th className="pb-2 pr-3 font-medium">Suggested By</th>
                  <th className="pb-2 pr-3 font-medium">Query Name</th>
                  <th className="pb-2 pr-3 font-medium">SQL</th>
                  <th className="pb-2 pr-3 font-medium">Status</th>
                  <th className="pb-2 pr-3 font-medium">Reviewed By</th>
                  <th className="pb-2 pr-3 font-medium">Reviewed At</th>
                  <th className="pb-2 pr-3 font-medium">Rejection Reason</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map(row => (
                  <tr key={row.id} className="border-b border-border-light/50">
                    <td className="py-2 pr-3 whitespace-nowrap text-text-secondary">{formatDate(row.created_at)}</td>
                    <td className="py-2 pr-3 text-text-primary">{row.suggested_by ?? '–'}</td>
                    <td className="py-2 pr-3 text-text-primary truncate max-w-[180px]">{row.natural_language_prompt}</td>
                    <td className="py-2 pr-3 font-mono text-xs text-text-secondary truncate max-w-[200px]" title={row.sql_query}>
                      {truncateSQL(row.sql_query)}
                    </td>
                    <td className="py-2 pr-3">
                      <span className={`px-1.5 py-0.5 text-xs rounded ${statusColor(row.status)}`}>
                        {row.status}
                      </span>
                    </td>
                    <td className="py-2 pr-3 text-text-secondary">{row.reviewed_by ?? '–'}</td>
                    <td className="py-2 pr-3 text-text-secondary whitespace-nowrap">
                      {row.reviewed_at ? formatDate(row.reviewed_at) : '–'}
                    </td>
                    <td className="py-2 pr-3 text-text-secondary text-xs truncate max-w-[160px]">
                      {row.rejection_reason ?? '–'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination total={data.total} limit={data.limit} offset={data.offset} onPageChange={setOffset} />
        </>
      )}
    </div>
  );
}

// ============================================================================
// MCP Events Tab
// ============================================================================

function MCPEventsTab({ projectId }: { projectId: string }) {
  const [data, setData] = useState<PaginatedResponse<MCPAuditEvent> | null>(null);
  const [loading, setLoading] = useState(true);
  const [timeRange, setTimeRange] = useState<TimeRange>('7d');
  const [eventTypeFilter, setEventTypeFilter] = useState('');
  const [toolNameInput, setToolNameInput] = useState('');
  const [toolNameFilter, setToolNameFilter] = useState('');
  const [securityLevelFilter, setSecurityLevelFilter] = useState('');
  const [userInput, setUserInput] = useState('');
  const [userFilter, setUserFilter] = useState('');
  const [offset, setOffset] = useState(0);
  const [expandedRow, setExpandedRow] = useState<string | null>(null);
  const toolNameDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const userDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Debounce tool name input
  useEffect(() => {
    if (toolNameDebounceRef.current) clearTimeout(toolNameDebounceRef.current);
    toolNameDebounceRef.current = setTimeout(() => { setToolNameFilter(toolNameInput); }, 300);
    return () => { if (toolNameDebounceRef.current) clearTimeout(toolNameDebounceRef.current); };
  }, [toolNameInput]);

  // Debounce user input
  useEffect(() => {
    if (userDebounceRef.current) clearTimeout(userDebounceRef.current);
    userDebounceRef.current = setTimeout(() => { setUserFilter(userInput); }, 300);
    return () => { if (userDebounceRef.current) clearTimeout(userDebounceRef.current); };
  }, [userInput]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, string> = { limit: '50', offset: String(offset) };
      const since = getTimeRangeSince(timeRange);
      if (since) params.since = since;
      if (eventTypeFilter) params.event_type = eventTypeFilter;
      if (toolNameFilter) params.tool_name = toolNameFilter;
      if (securityLevelFilter) params.security_level = securityLevelFilter;
      if (userFilter) params.user_id = userFilter;

      const response = await engineApi.listAuditMCPEvents(projectId, params);
      if (response.success && response.data) {
        setData(response.data);
      }
    } catch (error) {
      console.error('Failed to fetch MCP events:', error);
    } finally {
      setLoading(false);
    }
  }, [projectId, timeRange, eventTypeFilter, toolNameFilter, securityLevelFilter, userFilter, offset]);

  useEffect(() => { fetchData(); }, [fetchData]);
  useEffect(() => { setOffset(0); }, [timeRange, eventTypeFilter, toolNameFilter, securityLevelFilter, userFilter]);

  const securityColor = (level: string) => {
    switch (level) {
      case 'elevated': return 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300';
      case 'critical': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300';
      default: return 'bg-surface-secondary text-text-secondary';
    }
  };

  return (
    <div>
      <div className="flex flex-wrap items-center gap-3 mb-4">
        <TimeRangeFilter value={timeRange} onChange={setTimeRange} />
        <select
          value={eventTypeFilter}
          onChange={e => setEventTypeFilter(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary"
        >
          <option value="">All Event Types</option>
          <option value="tool_call">Tool Call</option>
          <option value="tool_error">Tool Error</option>
        </select>
        <select
          value={securityLevelFilter}
          onChange={e => setSecurityLevelFilter(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary"
        >
          <option value="">All Security Levels</option>
          <option value="normal">Normal</option>
          <option value="elevated">Elevated</option>
          <option value="critical">Critical</option>
        </select>
        <input
          type="text"
          placeholder="Filter by tool..."
          value={toolNameInput}
          onChange={e => setToolNameInput(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary w-36"
        />
        <input
          type="text"
          placeholder="Filter by user..."
          value={userInput}
          onChange={e => setUserInput(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary w-36"
        />
      </div>

      {loading ? (
        <div className="flex justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-text-secondary" />
        </div>
      ) : !data || data.items.length === 0 ? (
        <div className="text-center py-12 text-text-secondary">
          <Terminal className="h-8 w-8 mx-auto mb-2 text-text-tertiary" />
          <p>No MCP events found</p>
        </div>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border-light text-left text-text-secondary">
                  <th className="pb-2 pr-3 font-medium w-6"></th>
                  <th className="pb-2 pr-3 font-medium">Time</th>
                  <th className="pb-2 pr-3 font-medium">User</th>
                  <th className="pb-2 pr-3 font-medium">Tool</th>
                  <th className="pb-2 pr-3 font-medium">Event</th>
                  <th className="pb-2 pr-3 font-medium text-right">Duration</th>
                  <th className="pb-2 pr-3 font-medium">Status</th>
                  <th className="pb-2 pr-3 font-medium">Security</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map(row => {
                  const isExpanded = expandedRow === row.id;
                  const hasDetails = row.request_params || row.result_summary || row.error_message || row.sql_query;
                  return (
                    <Fragment key={row.id}>
                      <tr
                        className={`border-b border-border-light/50 ${
                          !row.was_successful ? 'bg-red-50/50 dark:bg-red-900/10' :
                          row.security_level !== 'normal' ? 'bg-amber-50/50 dark:bg-amber-900/10' : ''
                        } ${hasDetails ? 'cursor-pointer hover:bg-surface-secondary/50' : ''}`}
                        onClick={() => hasDetails && setExpandedRow(isExpanded ? null : row.id)}
                      >
                        <td className="py-2 pr-1">
                          {hasDetails && (
                            isExpanded ? <ChevronDown className="h-4 w-4 text-text-tertiary" /> : <ChevronRight className="h-4 w-4 text-text-tertiary" />
                          )}
                        </td>
                        <td className="py-2 pr-3 whitespace-nowrap text-text-secondary">
                          <div className="flex items-center gap-1">
                            <Clock className="h-3 w-3" />
                            {formatDate(row.created_at)}
                          </div>
                        </td>
                        <td className="py-2 pr-3 text-text-primary truncate max-w-[140px]" title={row.user_email ?? row.user_id}>
                          {row.user_email ?? row.user_id}
                        </td>
                        <td className="py-2 pr-3">
                          <span className="px-1.5 py-0.5 text-xs rounded bg-surface-secondary text-text-primary font-mono">
                            {row.tool_name ?? '–'}
                          </span>
                        </td>
                        <td className="py-2 pr-3">
                          <span className={`px-1.5 py-0.5 text-xs rounded ${
                            row.event_type === 'tool_error'
                              ? 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300'
                              : 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300'
                          }`}>
                            {row.event_type.replace(/_/g, ' ')}
                          </span>
                        </td>
                        <td className="py-2 pr-3 text-right text-text-secondary whitespace-nowrap">
                          {row.duration_ms != null ? formatDuration(row.duration_ms) : '–'}
                        </td>
                        <td className="py-2 pr-3">
                          {row.was_successful ? (
                            <span className="flex items-center gap-1 text-green-600 dark:text-green-400">
                              <CheckCircle className="h-3.5 w-3.5" /> OK
                            </span>
                          ) : (
                            <span className="flex items-center gap-1 text-red-600 dark:text-red-400">
                              <XCircle className="h-3.5 w-3.5" /> Failed
                            </span>
                          )}
                        </td>
                        <td className="py-2 pr-3">
                          <span className={`px-1.5 py-0.5 text-xs rounded ${securityColor(row.security_level)}`}>
                            {row.security_level}
                          </span>
                        </td>
                      </tr>
                      {isExpanded && hasDetails && (
                        <tr>
                          <td colSpan={8} className="py-3 px-4 bg-surface-secondary/30">
                            <div className="space-y-2 text-xs">
                              {row.error_message && (
                                <div>
                                  <span className="font-semibold text-red-600 dark:text-red-400">Error:</span>
                                  <span className="ml-2 text-text-primary">{row.error_message}</span>
                                </div>
                              )}
                              {row.sql_query && (
                                <div>
                                  <span className="font-semibold text-text-secondary">SQL:</span>
                                  <pre className="mt-1 p-2 rounded bg-surface-primary font-mono text-text-primary overflow-x-auto">{row.sql_query}</pre>
                                </div>
                              )}
                              {row.request_params && Object.keys(row.request_params).length > 0 && (
                                <div>
                                  <span className="font-semibold text-text-secondary">Request Params:</span>
                                  <pre className="mt-1 p-2 rounded bg-surface-primary font-mono text-text-primary overflow-x-auto max-h-40">
                                    {JSON.stringify(row.request_params, null, 2)}
                                  </pre>
                                </div>
                              )}
                              {row.result_summary && Object.keys(row.result_summary).length > 0 && (
                                <div>
                                  <span className="font-semibold text-text-secondary">Result Summary:</span>
                                  <pre className="mt-1 p-2 rounded bg-surface-primary font-mono text-text-primary overflow-x-auto max-h-40">
                                    {JSON.stringify(row.result_summary, null, 2)}
                                  </pre>
                                </div>
                              )}
                              {row.security_flags && row.security_flags.length > 0 && (
                                <div>
                                  <span className="font-semibold text-text-secondary">Security Flags:</span>
                                  <span className="ml-2 text-text-primary">{row.security_flags.join(', ')}</span>
                                </div>
                              )}
                            </div>
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  );
                })}
              </tbody>
            </table>
          </div>
          <Pagination total={data.total} limit={data.limit} offset={data.offset} onPageChange={setOffset} />
        </>
      )}
    </div>
  );
}

// ============================================================================
// Alerts Tab
// ============================================================================

function AlertsTab({ projectId }: { projectId: string }) {
  const { toast } = useToast();
  const [data, setData] = useState<PaginatedResponse<AuditAlert> | null>(null);
  const [loading, setLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState('open');
  const [severityFilter, setSeverityFilter] = useState('');
  const [offset, setOffset] = useState(0);
  const [resolvingId, setResolvingId] = useState<string | null>(null);
  const [resolveForm, setResolveForm] = useState<{ resolution: ResolveAlertRequest['resolution']; notes: string }>({
    resolution: 'resolved',
    notes: '',
  });
  const [submitting, setSubmitting] = useState(false);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, string> = { limit: '50', offset: String(offset) };
      if (statusFilter) params.status = statusFilter;
      if (severityFilter) params.severity = severityFilter;

      const response = await engineApi.getAuditAlerts(projectId, params);
      if (response.success && response.data) {
        setData(response.data);
      }
    } catch (error) {
      console.error('Failed to fetch alerts:', error);
    } finally {
      setLoading(false);
    }
  }, [projectId, statusFilter, severityFilter, offset]);

  useEffect(() => { fetchData(); }, [fetchData]);
  useEffect(() => { setOffset(0); }, [statusFilter, severityFilter]);

  const handleResolve = async (alertId: string) => {
    setSubmitting(true);
    try {
      const body: ResolveAlertRequest = { resolution: resolveForm.resolution };
      if (resolveForm.notes) body.notes = resolveForm.notes;
      const response = await engineApi.resolveAuditAlert(projectId, alertId, body);
      if (response.success) {
        toast({ title: 'Alert resolved', description: response.message ?? 'Alert resolved successfully.' });
        setResolvingId(null);
        setResolveForm({ resolution: 'resolved', notes: '' });
        fetchData();
      }
    } catch (error) {
      console.error('Failed to resolve alert:', error);
      toast({ title: 'Error', description: 'Failed to resolve alert.', variant: 'destructive' });
    } finally {
      setSubmitting(false);
    }
  };

  const severityBadge = (severity: string) => {
    switch (severity) {
      case 'critical': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300';
      case 'warning': return 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300';
      case 'info': return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300';
      default: return 'bg-surface-secondary text-text-secondary';
    }
  };

  const statusBadge = (status: string) => {
    switch (status) {
      case 'open': return 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300';
      case 'resolved': return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300';
      case 'dismissed': return 'bg-surface-secondary text-text-secondary';
      default: return 'bg-surface-secondary text-text-secondary';
    }
  };

  return (
    <div>
      <div className="flex flex-wrap items-center gap-3 mb-4">
        <select
          value={statusFilter}
          onChange={e => setStatusFilter(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary"
        >
          <option value="open">Open</option>
          <option value="resolved">Resolved</option>
          <option value="dismissed">Dismissed</option>
          <option value="">All</option>
        </select>
        <select
          value={severityFilter}
          onChange={e => setSeverityFilter(e.target.value)}
          className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary"
        >
          <option value="">All Severities</option>
          <option value="critical">Critical</option>
          <option value="warning">Warning</option>
          <option value="info">Info</option>
        </select>
      </div>

      {loading ? (
        <div className="flex justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-text-secondary" />
        </div>
      ) : !data || data.items.length === 0 ? (
        <div className="text-center py-12 text-text-secondary">
          <Bell className="h-8 w-8 mx-auto mb-2 text-text-tertiary" />
          <p>No alerts found</p>
        </div>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border-light text-left text-text-secondary">
                  <th className="pb-2 pr-3 font-medium">Time</th>
                  <th className="pb-2 pr-3 font-medium">Alert Type</th>
                  <th className="pb-2 pr-3 font-medium">Title</th>
                  <th className="pb-2 pr-3 font-medium">Affected User</th>
                  <th className="pb-2 pr-3 font-medium">Status</th>
                  <th className="pb-2 pr-3 font-medium">Resolved By</th>
                  <th className="pb-2 pr-3 font-medium">Resolution Notes</th>
                  <th className="pb-2 pr-3 font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map(alert => (
                  <Fragment key={alert.id}>
                    <tr className={`border-b border-border-light/50 ${
                      alert.severity === 'critical' ? 'bg-red-50/50 dark:bg-red-900/10' :
                      alert.severity === 'warning' ? 'bg-amber-50/50 dark:bg-amber-900/10' : ''
                    }`}>
                      <td className="py-2 pr-3 whitespace-nowrap text-text-secondary">
                        <div className="flex items-center gap-1">
                          <Clock className="h-3 w-3" />
                          {formatDate(alert.created_at)}
                        </div>
                      </td>
                      <td className="py-2 pr-3">
                        <span className={`px-1.5 py-0.5 text-xs rounded ${severityBadge(alert.severity)}`}>
                          {alert.alert_type.replace(/_/g, ' ')}
                        </span>
                      </td>
                      <td className="py-2 pr-3 text-text-primary max-w-[220px]" title={alert.description ?? undefined}>
                        {alert.title}
                      </td>
                      <td className="py-2 pr-3 text-text-secondary truncate max-w-[140px]">
                        {alert.affected_user_id ?? '–'}
                      </td>
                      <td className="py-2 pr-3">
                        <span className={`px-1.5 py-0.5 text-xs rounded ${statusBadge(alert.status)}`}>
                          {alert.status}
                        </span>
                      </td>
                      <td className="py-2 pr-3 text-text-secondary truncate max-w-[120px]">
                        {alert.resolved_by ?? '–'}
                      </td>
                      <td className="py-2 pr-3 text-text-secondary text-xs truncate max-w-[160px]" title={alert.resolution_notes ?? undefined}>
                        {alert.resolution_notes ?? '–'}
                      </td>
                      <td className="py-2 pr-3">
                        {alert.status === 'open' && (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => setResolvingId(resolvingId === alert.id ? null : alert.id)}
                          >
                            Resolve
                          </Button>
                        )}
                      </td>
                    </tr>
                    {resolvingId === alert.id && (
                      <tr>
                        <td colSpan={8} className="py-3 px-4 bg-surface-secondary/30">
                          <div className="flex items-center gap-3">
                            <label className="flex items-center gap-1 text-xs text-text-secondary">
                              <input
                                type="radio"
                                name={`resolution-${alert.id}`}
                                checked={resolveForm.resolution === 'resolved'}
                                onChange={() => setResolveForm(f => ({ ...f, resolution: 'resolved' }))}
                                className="rounded"
                              />
                              Resolve
                            </label>
                            <label className="flex items-center gap-1 text-xs text-text-secondary">
                              <input
                                type="radio"
                                name={`resolution-${alert.id}`}
                                checked={resolveForm.resolution === 'dismissed'}
                                onChange={() => setResolveForm(f => ({ ...f, resolution: 'dismissed' }))}
                                className="rounded"
                              />
                              Dismiss
                            </label>
                            <input
                              type="text"
                              placeholder="Notes (optional)..."
                              value={resolveForm.notes}
                              onChange={e => setResolveForm(f => ({ ...f, notes: e.target.value }))}
                              className="text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary flex-1 max-w-xs"
                            />
                            <Button
                              size="sm"
                              disabled={submitting}
                              onClick={() => handleResolve(alert.id)}
                            >
                              {submitting ? <Loader2 className="h-3 w-3 animate-spin" /> : 'Confirm'}
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => { setResolvingId(null); setResolveForm({ resolution: 'resolved', notes: '' }); }}
                            >
                              Cancel
                            </Button>
                          </div>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination total={data.total} limit={data.limit} offset={data.offset} onPageChange={setOffset} />
        </>
      )}
    </div>
  );
}

// ============================================================================
// Main Audit Page
// ============================================================================

const AuditPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();

  const [activeTab, setActiveTab] = useState<AuditTab>('query-executions');
  const [queryFilter, setQueryFilter] = useState<'failed' | undefined>(undefined);
  const [summary, setSummary] = useState<AuditSummary | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  const fetchSummary = useCallback(async () => {
    if (!pid) return;
    try {
      const response = await engineApi.getAuditSummary(pid);
      if (response.success && response.data) {
        setSummary(response.data);
      }
    } catch (error) {
      console.error('Failed to fetch audit summary:', error);
    }
  }, [pid]);

  useEffect(() => {
    fetchSummary();
  }, [fetchSummary, refreshKey]);

  const handleRefresh = useCallback(() => {
    setRefreshKey(k => k + 1);
    toast({
      title: 'Refreshing',
      description: 'Audit data is being refreshed.',
    });
  }, [toast]);

  const handleCardClick = useCallback((action: SummaryCardAction) => {
    setActiveTab(action.tab);
    // When clicking "Failed Queries", pre-set the filter; otherwise clear it
    setQueryFilter(action.filter);
    // Force re-mount of the tab component so initialFilter takes effect
    setRefreshKey(k => k + 1);
  }, []);

  if (!pid) return null;

  return (
    <div className="mx-auto max-w-7xl">
      {/* Header */}
      <div className="mb-6">
        <Button
          variant="ghost"
          onClick={() => navigate(`/projects/${pid}/ai-data-liaison`)}
          className="mb-4"
        >
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to AI Data Liaison
        </Button>
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-text-primary flex items-center gap-2">
              <ScrollText className="h-8 w-8 text-brand-purple" />
              Audit Log
            </h1>
            <p className="mt-2 text-text-secondary">
              Review query executions, ontology changes, schema changes, and query approvals
            </p>
          </div>
          <Button variant="outline" onClick={handleRefresh}>
            <RefreshCw className="mr-2 h-4 w-4" />
            Refresh
          </Button>
        </div>
      </div>

      {/* Summary cards (also serve as tab navigation) */}
      <AuditSummaryHeader summary={summary} activeTab={activeTab} {...(queryFilter ? { activeFilter: queryFilter } : {})} onCardClick={handleCardClick} />

      {/* Tab Content */}
      <Card>
        <CardContent className="p-6">
          {activeTab === 'query-executions' && <QueryExecutionsTab key={refreshKey} projectId={pid} {...(queryFilter ? { initialFilter: queryFilter } : {})} />}
          {activeTab === 'ontology-changes' && <OntologyChangesTab key={refreshKey} projectId={pid} />}
          {activeTab === 'schema-changes' && <SchemaChangesTab key={refreshKey} projectId={pid} />}
          {activeTab === 'query-approvals' && <QueryApprovalsTab key={refreshKey} projectId={pid} />}
          {activeTab === 'mcp-events' && <MCPEventsTab key={refreshKey} projectId={pid} />}
          {activeTab === 'alerts' && <AlertsTab key={refreshKey} projectId={pid} />}
        </CardContent>
      </Card>
    </div>
  );
};

export default AuditPage;
