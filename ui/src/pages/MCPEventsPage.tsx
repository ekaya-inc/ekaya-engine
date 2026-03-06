/**
 * MCP Events Page
 * Standalone page for monitoring MCP tool calls, security events, and API activity.
 */

import {
  ArrowLeft,
  CheckCircle,
  ChevronDown,
  ChevronRight,
  Clock,
  Loader2,
  RefreshCw,
  Terminal,
  XCircle,
} from 'lucide-react';
import { Fragment, useState, useEffect, useCallback, useRef } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import { Button } from '../components/ui/Button';
import { Card, CardContent } from '../components/ui/Card';
import engineApi from '../services/engineApi';
import type { MCPAuditEvent, PaginatedResponse } from '../types';

// Time range presets
type TimeRange = '24h' | '7d' | '30d' | 'all';

function getTimeRangeSince(range: TimeRange): string | undefined {
  if (range === 'all') return undefined;
  const now = new Date();
  const hours = range === '24h' ? 24 : range === '7d' ? 168 : 720;
  return new Date(now.getTime() - hours * 60 * 60 * 1000).toISOString();
}

function formatDate(dateStr: string): string {
  const d = new Date(dateStr);
  return d.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

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

const MCPEventsPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();

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
  const [refreshKey, setRefreshKey] = useState(0);
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
    if (!pid) return;
    setLoading(true);
    try {
      const params: Record<string, string> = { limit: '50', offset: String(offset) };
      const since = getTimeRangeSince(timeRange);
      if (since) params.since = since;
      if (eventTypeFilter) params.event_type = eventTypeFilter;
      if (toolNameFilter) params.tool_name = toolNameFilter;
      if (securityLevelFilter) params.security_level = securityLevelFilter;
      if (userFilter) params.user_id = userFilter;

      const response = await engineApi.listAuditMCPEvents(pid, params);
      if (response.success && response.data) {
        setData(response.data);
      }
    } catch (error) {
      console.error('Failed to fetch MCP events:', error);
    } finally {
      setLoading(false);
    }
  }, [pid, timeRange, eventTypeFilter, toolNameFilter, securityLevelFilter, userFilter, offset, refreshKey]);

  useEffect(() => { fetchData(); }, [fetchData]);
  useEffect(() => { setOffset(0); }, [timeRange, eventTypeFilter, toolNameFilter, securityLevelFilter, userFilter]);

  const securityColor = (level: string) => {
    switch (level) {
      case 'elevated': return 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300';
      case 'critical': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300';
      default: return 'bg-surface-secondary text-text-secondary';
    }
  };

  if (!pid) return null;

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
              <Terminal className="h-8 w-8 text-blue-500" />
              MCP Events
            </h1>
            <p className="mt-2 text-text-secondary">
              Monitor MCP tool calls, security events, and API activity.
            </p>
          </div>
          <Button variant="outline" onClick={() => setRefreshKey(k => k + 1)}>
            <RefreshCw className="mr-2 h-4 w-4" />
            Refresh
          </Button>
        </div>
      </div>

      <Card>
        <CardContent className="p-6">
          {/* Filters */}
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

          {/* Table */}
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
                      const hasDetails = row.request_params ?? row.result_summary ?? row.error_message ?? row.sql_query;
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
        </CardContent>
      </Card>
    </div>
  );
};

export default MCPEventsPage;
