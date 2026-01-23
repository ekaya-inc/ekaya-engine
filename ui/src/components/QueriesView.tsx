/**
 * QueriesView Component
 * Manages saved queries for a datasource with full API integration
 */

import {
  Database,
  Plus,
  Edit3,
  Trash2,
  Play,
  Save,
  X,
  MessageSquare,
  FileText,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Copy,
  Search,
  Loader2,
  RefreshCw,
  Pencil,
} from 'lucide-react';
import { useState, useEffect, useCallback, useMemo } from 'react';

import { useSqlValidation, type ValidationStatus } from '../hooks/useSqlValidation';
import { useToast } from '../hooks/useToast';
import engineApi from '../services/engineApi';
import type { DatasourceSchema, Query, SqlDialect, CreateQueryRequest, UpdateQueryRequest, QueryParameter, OutputColumn, ExecuteQueryResponse } from '../types';
import { toCodeMirrorSchema } from '../utils/schemaUtils';

import { DeleteQueryDialog } from './DeleteQueryDialog';
import { OutputColumnEditor } from './OutputColumnEditor';
import { ParameterEditor } from './ParameterEditor';
import { ParameterInputForm } from './ParameterInputForm';
import { QueryResultsTable } from './QueryResultsTable';
import { SqlEditor } from './SqlEditor';
import { Button } from './ui/Button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from './ui/Card';
import { Input } from './ui/Input';

interface QueriesViewProps {
  projectId: string;
  datasourceId: string;
  dialect: SqlDialect;
}

interface EditingState {
  natural_language_prompt: string;
  additional_context: string;
  sql_query: string;
  is_enabled: boolean;
  allows_modification: boolean;
  parameters: QueryParameter[];
  output_columns: OutputColumn[];
  constraints: string;
}

type QueryFilterType = 'all' | 'read-only' | 'modifying';

const QueriesView = ({ projectId, datasourceId, dialect }: QueriesViewProps) => {
  const { toast } = useToast();

  // Data state
  const [queries, setQueries] = useState<Query[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [schema, setSchema] = useState<DatasourceSchema | null>(null);
  const [queryResults, setQueryResults] = useState<ExecuteQueryResponse | null>(null);

  // UI state
  const [selectedQuery, setSelectedQuery] = useState<Query | null>(null);
  const [isCreating, setIsCreating] = useState(false);
  const [editingQueryId, setEditingQueryId] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [queryFilter, setQueryFilter] = useState<QueryFilterType>('all');

  // Form state for creating
  const [newQuery, setNewQuery] = useState<EditingState>({
    natural_language_prompt: '',
    additional_context: '',
    sql_query: '',
    is_enabled: true,
    allows_modification: false,
    parameters: [],
    output_columns: [],
    constraints: '',
  });

  // Form state for editing
  const [editingState, setEditingState] = useState<EditingState | null>(null);

  // Parameter values for test/execute
  const [testParameterValues, setTestParameterValues] = useState<Record<string, unknown>>({});
  const [executeParameterValues, setExecuteParameterValues] = useState<Record<string, unknown>>({});

  // Action states
  const [isSaving, setIsSaving] = useState(false);
  const [isTesting, setIsTesting] = useState(false);
  const [testPassed, setTestPassed] = useState(false);
  const [editTestPassed, setEditTestPassed] = useState(false);

  // Delete dialog state
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [queryToDelete, setQueryToDelete] = useState<Query | null>(null);

  // SQL validation for create form
  const createValidation = useSqlValidation({
    projectId,
    datasourceId,
    debounceMs: 500,
  });

  // SQL validation for edit form
  const editValidation = useSqlValidation({
    projectId,
    datasourceId,
    debounceMs: 500,
  });

  /**
   * Load queries from API
   */
  const loadQueries = useCallback(async () => {
    setIsLoading(true);
    setLoadError(null);

    try {
      const response = await engineApi.listQueries(projectId, datasourceId);
      if (response.success && response.data) {
        setQueries(response.data.queries);
      } else {
        setLoadError(response.error ?? 'Failed to load queries');
      }
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : 'Failed to load queries');
    } finally {
      setIsLoading(false);
    }
  }, [projectId, datasourceId]);

  // Load queries on mount
  useEffect(() => {
    loadQueries();
  }, [loadQueries]);

  // Fetch schema for autocomplete (fire-and-forget, non-blocking)
  useEffect(() => {
    const fetchSchema = async () => {
      const response = await engineApi.getSchema(projectId, datasourceId);
      if (response.success && response.data) {
        setSchema(response.data);
      }
    };
    fetchSchema();
  }, [projectId, datasourceId]);

  // Transform schema to CodeMirror format for autocomplete
  const codeMirrorSchema = useMemo(
    () => toCodeMirrorSchema(schema),
    [schema]
  );

  // Check if SQL has undefined parameters (used in SQL but not defined)
  const hasUndefinedParams = useCallback(
    (sql: string, params: QueryParameter[]): boolean => {
      const regex = /\{\{([a-zA-Z_]\w*)\}\}/g;
      const sqlParams = new Set<string>();
      let match;
      while ((match = regex.exec(sql)) !== null) {
        const paramName = match[1];
        if (paramName !== undefined) {
          sqlParams.add(paramName);
        }
      }
      const definedNames = new Set(params.map((p) => p.name));
      for (const name of sqlParams) {
        if (!definedNames.has(name)) {
          return true;
        }
      }
      return false;
    },
    []
  );

  // Filter queries based on search and type filter
  const filteredQueries = queries.filter((query) => {
    // Text search filter
    const matchesSearch =
      query.natural_language_prompt
        .toLowerCase()
        .includes(searchTerm.toLowerCase()) ||
      query.sql_query.toLowerCase().includes(searchTerm.toLowerCase());

    // Type filter
    const matchesTypeFilter =
      queryFilter === 'all' ||
      (queryFilter === 'read-only' && !query.allows_modification) ||
      (queryFilter === 'modifying' && query.allows_modification);

    return matchesSearch && matchesTypeFilter;
  });

  /**
   * Reset create form
   */
  const resetCreateForm = () => {
    setNewQuery({
      natural_language_prompt: '',
      additional_context: '',
      sql_query: '',
      is_enabled: true,
      allows_modification: false,
      parameters: [],
      output_columns: [],
      constraints: '',
    });
    setTestPassed(false);
    setTestParameterValues({});
    createValidation.reset();
  };

  /**
   * Test query before saving
   */
  const handleTestQuery = async (
    sql: string,
    isCreateMode: boolean,
    parameters?: QueryParameter[],
    parameterValues?: Record<string, unknown>
  ) => {
    setIsTesting(true);

    try {
      const response = await engineApi.testQuery(projectId, datasourceId, {
        sql_query: sql,
        limit: 10,
        ...(parameters !== undefined && { parameter_definitions: parameters }),
        ...(parameterValues !== undefined && { parameter_values: parameterValues }),
      });

      if (response.success) {
        if (response.data) {
          setQueryResults(response.data);

          // Auto-populate output columns from test results
          const newOutputColumns: OutputColumn[] = response.data.columns.map((col) => ({
            name: col.name,
            type: col.type,
            description: '',
          }));

          if (isCreateMode) {
            setNewQuery((prev) => ({ ...prev, output_columns: newOutputColumns }));
          } else {
            setEditingState((prev) => prev ? { ...prev, output_columns: newOutputColumns } : prev);
          }
        }
        toast({
          title: 'Query executed successfully',
          description: `Returned ${response.data?.row_count ?? 0} rows`,
          variant: 'success',
        });
        if (isCreateMode) {
          setTestPassed(true);
        } else {
          setEditTestPassed(true);
        }
        return true;
      } else {
        setQueryResults(null);
        toast({
          title: 'Query execution failed',
          description: response.error ?? 'Unknown error',
          variant: 'destructive',
        });
        return false;
      }
    } catch (err) {
      setQueryResults(null);
      toast({
        title: 'Query execution failed',
        description: err instanceof Error ? err.message : 'Unknown error',
        variant: 'destructive',
      });
      return false;
    } finally {
      setIsTesting(false);
    }
  };

  /**
   * Create new query
   */
  const handleCreateQuery = async () => {
    if (!newQuery.natural_language_prompt.trim() || !newQuery.sql_query.trim()) {
      return;
    }

    // Require test to pass before saving
    if (!testPassed) {
      toast({
        title: 'Test required',
        description: 'Please test the query before saving',
        variant: 'destructive',
      });
      return;
    }

    setIsSaving(true);

    try {
      const request: CreateQueryRequest = {
        natural_language_prompt: newQuery.natural_language_prompt.trim(),
        sql_query: newQuery.sql_query.trim(),
        is_enabled: newQuery.is_enabled,
        allows_modification: newQuery.allows_modification,
      };

      if (newQuery.additional_context.trim()) {
        request.additional_context = newQuery.additional_context.trim();
      }

      if (newQuery.parameters.length > 0) {
        request.parameters = newQuery.parameters;
      }

      if (newQuery.output_columns.length > 0) {
        request.output_columns = newQuery.output_columns;
      }

      if (newQuery.constraints.trim()) {
        request.constraints = newQuery.constraints.trim();
      }

      const response = await engineApi.createQuery(projectId, datasourceId, request);

      if (response.success && response.data) {
        toast({
          title: 'Query created',
          description: 'Your query has been saved',
          variant: 'success',
        });
        setQueries((prev) => [...prev, response.data as Query]);
        setSelectedQuery(response.data as Query);
        setIsCreating(false);
        resetCreateForm();
      } else {
        toast({
          title: 'Failed to create query',
          description: response.error ?? 'Unknown error',
          variant: 'destructive',
        });
      }
    } catch (err) {
      toast({
        title: 'Failed to create query',
        description: err instanceof Error ? err.message : 'Unknown error',
        variant: 'destructive',
      });
    } finally {
      setIsSaving(false);
    }
  };

  /**
   * Start editing a query
   */
  const handleEditQuery = (query: Query) => {
    setEditingQueryId(query.query_id);
    setEditingState({
      natural_language_prompt: query.natural_language_prompt,
      additional_context: query.additional_context ?? '',
      sql_query: query.sql_query,
      is_enabled: query.is_enabled,
      allows_modification: query.allows_modification,
      parameters: query.parameters ?? [],
      output_columns: query.output_columns ?? [],
      constraints: query.constraints ?? '',
    });
    setQueryResults(null);
    editValidation.reset();
    setEditTestPassed(false);
    setTestParameterValues({});
  };

  /**
   * Save edited query
   */
  const handleSaveEdit = async () => {
    if (!editingQueryId || !editingState) return;

    if (
      !editingState.natural_language_prompt.trim() ||
      !editingState.sql_query.trim()
    ) {
      return;
    }

    setIsSaving(true);

    try {
      const updateRequest: UpdateQueryRequest = {
        natural_language_prompt: editingState.natural_language_prompt.trim(),
        sql_query: editingState.sql_query.trim(),
        is_enabled: editingState.is_enabled,
        allows_modification: editingState.allows_modification,
      };

      if (editingState.additional_context.trim()) {
        updateRequest.additional_context = editingState.additional_context.trim();
      }

      if (editingState.output_columns.length > 0) {
        updateRequest.output_columns = editingState.output_columns;
      }

      if (editingState.constraints.trim()) {
        updateRequest.constraints = editingState.constraints.trim();
      }

      const response = await engineApi.updateQuery(
        projectId,
        datasourceId,
        editingQueryId,
        updateRequest
      );

      if (response.success && response.data) {
        toast({
          title: 'Query updated',
          description: 'Your changes have been saved',
          variant: 'success',
        });
        setQueries((prev) =>
          prev.map((q) =>
            q.query_id === editingQueryId ? (response.data as Query) : q
          )
        );
        setSelectedQuery(response.data as Query);
        setEditingQueryId(null);
        setEditingState(null);
        editValidation.reset();
      } else {
        toast({
          title: 'Failed to update query',
          description: response.error ?? 'Unknown error',
          variant: 'destructive',
        });
      }
    } catch (err) {
      toast({
        title: 'Failed to update query',
        description: err instanceof Error ? err.message : 'Unknown error',
        variant: 'destructive',
      });
    } finally {
      setIsSaving(false);
    }
  };

  /**
   * Open delete confirmation dialog
   */
  const handleDeleteClick = (query: Query) => {
    setQueryToDelete(query);
    setDeleteDialogOpen(true);
  };

  /**
   * Handle query deletion
   */
  const handleQueryDeleted = (queryId: string) => {
    setQueries((prev) => prev.filter((q) => q.query_id !== queryId));
    if (selectedQuery?.query_id === queryId) {
      setSelectedQuery(null);
    }
    setDeleteDialogOpen(false);
    setQueryToDelete(null);
    toast({
      title: 'Query deleted',
      variant: 'success',
    });
  };

  /**
   * Toggle query enabled status
   */
  const handleToggleEnabled = async (query: Query) => {
    try {
      const response = await engineApi.updateQuery(
        projectId,
        datasourceId,
        query.query_id,
        { is_enabled: !query.is_enabled }
      );

      if (response.success && response.data) {
        setQueries((prev) =>
          prev.map((q) =>
            q.query_id === query.query_id ? (response.data as Query) : q
          )
        );
        if (selectedQuery?.query_id === query.query_id) {
          setSelectedQuery(response.data as Query);
        }
        toast({
          title: query.is_enabled ? 'Query disabled' : 'Query enabled',
          variant: 'success',
        });
      }
    } catch (err) {
      toast({
        title: 'Failed to update query',
        description: err instanceof Error ? err.message : 'Unknown error',
        variant: 'destructive',
      });
    }
  };

  /**
   * Execute a saved query
   */
  const handleExecuteQuery = async (query: Query) => {
    setIsTesting(true);

    try {
      const executeRequest =
        query.parameters && query.parameters.length > 0
          ? { limit: 100, parameters: executeParameterValues }
          : { limit: 100 };
      const response = await engineApi.executeQuery(
        projectId,
        datasourceId,
        query.query_id,
        executeRequest
      );

      if (response.success && response.data) {
        setQueryResults(response.data);
        toast({
          title: 'Query executed successfully',
          description: `Returned ${response.data.row_count} rows`,
          variant: 'success',
        });
        // Refresh to get updated usage count
        await loadQueries();
      } else {
        setQueryResults(null);
        toast({
          title: 'Query execution failed',
          description: response.error ?? 'Unknown error',
          variant: 'destructive',
        });
      }
    } catch (err) {
      setQueryResults(null);
      toast({
        title: 'Query execution failed',
        description: err instanceof Error ? err.message : 'Unknown error',
        variant: 'destructive',
      });
    } finally {
      setIsTesting(false);
    }
  };

  /**
   * Copy query to clipboard
   */
  const handleCopyQuery = (sqlQuery: string) => {
    navigator.clipboard.writeText(sqlQuery);
    toast({
      title: 'Copied to clipboard',
      variant: 'success',
      duration: 2000,
    });
  };

  /**
   * Get validation status for display
   */
  const getValidationStatus = (status: ValidationStatus): ValidationStatus => {
    return status;
  };

  // Loading state
  if (isLoading) {
    return (
      <div className="flex h-[calc(100vh-12rem)] items-center justify-center">
        <div className="text-center">
          <Loader2 className="h-8 w-8 animate-spin mx-auto mb-4 text-text-tertiary" />
          <p className="text-text-secondary">Loading queries...</p>
        </div>
      </div>
    );
  }

  // Error state
  if (loadError) {
    return (
      <div className="flex h-[calc(100vh-12rem)] items-center justify-center">
        <div className="text-center">
          <AlertCircle className="h-8 w-8 mx-auto mb-4 text-red-500" />
          <p className="text-text-primary font-medium mb-2">Failed to load queries</p>
          <p className="text-text-secondary mb-4">{loadError}</p>
          <Button onClick={loadQueries}>
            <RefreshCw className="mr-2 h-4 w-4" />
            Retry
          </Button>
        </div>
      </div>
    );
  }

  return (
    <>
      <div className="flex gap-6">
        {/* Left Sidebar - Query List */}
        <div className="w-80 flex flex-col h-[calc(100vh-12rem)] sticky top-4">
          <Card className="h-full flex flex-col">
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between mb-2">
                <CardTitle className="text-lg">Queries</CardTitle>
                <Button
                  size="sm"
                  onClick={() => {
                    setIsCreating(true);
                    setSelectedQuery(null);
                    setEditingQueryId(null);
                    setEditingState(null);
                    setQueryResults(null);
                    resetCreateForm();
                  }}
                  className="h-8 px-2"
                >
                  <Plus className="h-4 w-4" />
                </Button>
              </div>
              <div className="relative">
                <Search className="absolute left-2 top-2.5 h-4 w-4 text-text-tertiary" />
                <Input
                  placeholder="Search queries..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className="pl-8 h-9"
                />
              </div>
              <select
                value={queryFilter}
                onChange={(e) => setQueryFilter(e.target.value as QueryFilterType)}
                className="mt-2 h-8 w-full px-2 text-xs border border-border-medium rounded-md bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-brand-purple"
              >
                <option value="all">All queries</option>
                <option value="read-only">Read-only</option>
                <option value="modifying">Modifies data</option>
              </select>
            </CardHeader>
            <CardContent className="flex-1 overflow-y-auto p-2">
              {filteredQueries.length === 0 ? (
                <div className="text-center py-8">
                  <Database className="h-8 w-8 text-text-tertiary mx-auto mb-2" />
                  <p className="text-sm text-text-secondary">
                    {searchTerm ? 'No queries found' : 'No queries created yet'}
                  </p>
                </div>
              ) : (
                <div className="space-y-1">
                  {filteredQueries.map((query) => (
                    <button
                      key={query.query_id}
                      onClick={() => {
                        setSelectedQuery(query);
                        setIsCreating(false);
                        setEditingQueryId(null);
                        setEditingState(null);
                        setQueryResults(null);
                      }}
                      className={`w-full text-left p-2 rounded-lg transition-colors ${
                        selectedQuery?.query_id === query.query_id
                          ? 'bg-purple-500/10 border border-purple-500/30'
                          : 'hover:bg-surface-secondary/50'
                      } ${!query.is_enabled ? 'opacity-50' : ''}`}
                    >
                      <div className="flex items-center justify-between mb-0.5">
                        <div className="flex items-center gap-1.5">
                          <div
                            className={`h-1.5 w-1.5 rounded-full ${
                              query.is_enabled ? 'bg-green-500' : 'bg-gray-500'
                            }`}
                          />
                          <span className="text-xs text-text-tertiary">
                            {query.dialect}
                          </span>
                        </div>
                        {!query.is_enabled && (
                          <AlertCircle className="h-3 w-3 text-gray-500" />
                        )}
                      </div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-text-primary line-clamp-1 flex-1">
                          {query.natural_language_prompt}
                        </span>
                        {query.allows_modification && (
                          <span className="flex-shrink-0 inline-flex items-center gap-1 px-1.5 py-0.5 text-xs font-medium rounded bg-red-500/10 text-red-600 dark:text-red-400">
                            <Pencil className="h-3 w-3" />
                          </span>
                        )}
                      </div>
                      {query.usage_count > 0 && (
                        <div className="text-xs text-text-tertiary mt-0.5">
                          Used {query.usage_count} times
                        </div>
                      )}
                    </button>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        {/* Right Side - Query Editor/Viewer */}
        <div className="flex-1">
          <Card className="flex flex-col">
            {isCreating ? (
              // Create New Query Form
              <>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <CardTitle>Create New Query</CardTitle>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => {
                        setIsCreating(false);
                        resetCreateForm();
                      }}
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  </div>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div>
                    <label className="block text-sm font-medium text-text-primary mb-2">
                      Natural Language Prompt{' '}
                      <span className="text-red-500">*</span>
                    </label>
                    <textarea
                      value={newQuery.natural_language_prompt}
                      onChange={(e) => {
                        setNewQuery({
                          ...newQuery,
                          natural_language_prompt: e.target.value,
                        });
                        setTestPassed(false);
                      }}
                      placeholder="Describe what you want to query in natural language..."
                      className="w-full h-24 px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-purple-500"
                    />
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-text-primary mb-2">
                      Additional Context
                    </label>
                    <textarea
                      value={newQuery.additional_context}
                      onChange={(e) =>
                        setNewQuery({
                          ...newQuery,
                          additional_context: e.target.value,
                        })
                      }
                      placeholder="Any additional context or information..."
                      className="w-full h-20 px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-purple-500"
                    />
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-text-primary mb-2">
                      SQL Query <span className="text-red-500">*</span>
                    </label>
                    <SqlEditor
                      value={newQuery.sql_query}
                      onChange={(value) => {
                        setNewQuery({ ...newQuery, sql_query: value });
                        setTestPassed(false);
                        createValidation.validate(value);
                      }}
                      dialect={dialect}
                      schema={codeMirrorSchema}
                      validationStatus={getValidationStatus(createValidation.status)}
                      validationError={createValidation.error ?? undefined}
                      placeholder="SELECT * FROM users WHERE status = {{status}} LIMIT {{limit}}"
                      minHeight="200px"
                    />
                    {createValidation.warnings.length > 0 && (
                      <div className="mt-2 bg-amber-500/10 border border-amber-500/30 rounded-lg p-3">
                        <div className="flex items-start gap-2">
                          <AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400 mt-0.5 flex-shrink-0" />
                          <div className="flex-1">
                            <p className="text-sm font-medium text-amber-600 dark:text-amber-400">
                              Warning
                            </p>
                            {createValidation.warnings.map((warning, index) => (
                              <p
                                key={index}
                                className="text-xs text-amber-600/80 dark:text-amber-400/80 mt-1"
                              >
                                {warning}
                              </p>
                            ))}
                          </div>
                        </div>
                      </div>
                    )}
                    <div className="flex items-center gap-2 mt-4">
                      <input
                        type="checkbox"
                        id="allows-modification-create"
                        checked={newQuery.allows_modification}
                        onChange={(e) => {
                          setNewQuery({
                            ...newQuery,
                            allows_modification: e.target.checked,
                          });
                          setTestPassed(false);
                        }}
                        className="rounded border-border-medium"
                      />
                      <label
                        htmlFor="allows-modification-create"
                        className="text-sm text-text-primary cursor-pointer"
                      >
                        Allow data modification (INSERT, UPDATE, DELETE, CALL)
                      </label>
                    </div>
                    {newQuery.allows_modification && (
                      <div className="mt-2 bg-amber-500/10 border border-amber-500/30 rounded-lg p-3">
                        <div className="flex items-start gap-2">
                          <AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400 mt-0.5 flex-shrink-0" />
                          <div className="flex-1">
                            <p className="text-sm text-amber-600 dark:text-amber-400">
                              This query will be able to modify or delete data. Ensure the SQL
                              and parameters are thoroughly reviewed before enabling.
                            </p>
                          </div>
                        </div>
                      </div>
                    )}
                  </div>

                  <ParameterEditor
                    parameters={newQuery.parameters}
                    onChange={(parameters) => {
                      setNewQuery({ ...newQuery, parameters });
                      setTestPassed(false);
                    }}
                    sqlQuery={newQuery.sql_query}
                  />

                  {newQuery.parameters.length > 0 && (
                    <div className="border-t border-border-light pt-4">
                      <h3 className="text-sm font-medium text-text-primary mb-3">
                        Test
                      </h3>
                      <ParameterInputForm
                        parameters={newQuery.parameters}
                        values={testParameterValues}
                        onChange={setTestParameterValues}
                      />
                    </div>
                  )}

                  <div>
                    {newQuery.allows_modification && (
                      <div className="mb-4 bg-red-500/10 border border-red-500/30 rounded-lg p-3">
                        <div className="flex items-start gap-2">
                          <AlertTriangle className="h-4 w-4 text-red-600 dark:text-red-400 mt-0.5 flex-shrink-0" />
                          <div className="flex-1">
                            <p className="text-sm font-medium text-red-600 dark:text-red-400">
                              Warning: Testing modifies live data
                            </p>
                            <p className="text-xs text-red-600/80 dark:text-red-400/80 mt-1">
                              Testing this query will execute it against your live database and may
                              modify data. Use test parameters that won&apos;t cause harm.
                            </p>
                          </div>
                        </div>
                      </div>
                    )}
                    <Button
                      variant={newQuery.allows_modification ? 'destructive' : 'outline'}
                      onClick={() =>
                        handleTestQuery(
                          newQuery.sql_query,
                          true,
                          newQuery.parameters,
                          testParameterValues
                        )
                      }
                      disabled={
                        !newQuery.sql_query.trim() ||
                        isTesting ||
                        hasUndefinedParams(newQuery.sql_query, newQuery.parameters)
                      }
                    >
                      {isTesting ? (
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      ) : (
                        <Play className="mr-2 h-4 w-4" />
                      )}
                      {newQuery.allows_modification
                        ? 'Test Query (Modifies Data)'
                        : 'Test Query'}
                    </Button>
                  </div>

                  {queryResults && (
                    <QueryResultsTable
                      columns={queryResults.columns}
                      rows={queryResults.rows}
                      totalRowCount={queryResults.row_count}
                      maxRows={10}
                      maxColumns={20}
                    />
                  )}

                  <OutputColumnEditor
                    outputColumns={newQuery.output_columns}
                    onChange={(output_columns) =>
                      setNewQuery({ ...newQuery, output_columns })
                    }
                  />

                  <div>
                    <label className="block text-sm font-medium text-text-primary mb-2">
                      Constraints
                    </label>
                    <textarea
                      value={newQuery.constraints}
                      onChange={(e) =>
                        setNewQuery({
                          ...newQuery,
                          constraints: e.target.value,
                        })
                      }
                      placeholder="Describe limitations and assumptions (e.g., 'Only includes completed orders', 'Excludes refunded amounts')..."
                      className="w-full h-24 px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-purple-500"
                    />
                  </div>

                  <div className="pt-4 border-t border-border-light">
                    <div className="flex justify-end gap-2">
                      <Button
                        variant="outline"
                        onClick={() => {
                          setIsCreating(false);
                          resetCreateForm();
                        }}
                      >
                        Cancel
                      </Button>
                      <Button
                        onClick={handleCreateQuery}
                        disabled={
                          !newQuery.natural_language_prompt.trim() ||
                          !newQuery.sql_query.trim() ||
                          !testPassed ||
                          isSaving ||
                          hasUndefinedParams(newQuery.sql_query, newQuery.parameters)
                        }
                      >
                        {isSaving ? (
                          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        ) : (
                          <Save className="mr-2 h-4 w-4" />
                        )}
                        Save Query
                      </Button>
                    </div>
                    {!testPassed && newQuery.sql_query.trim() && (
                      <p className="text-sm text-amber-600 dark:text-amber-400 text-right mt-2">
                        Please test the query before saving
                      </p>
                    )}
                  </div>
                </CardContent>
              </>
            ) : editingQueryId && editingState ? (
              // Edit Query Form
              <>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <CardTitle>Edit Query</CardTitle>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => {
                        setEditingQueryId(null);
                        setEditingState(null);
                        editValidation.reset();
                      }}
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  </div>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div>
                    <label className="block text-sm font-medium text-text-primary mb-2">
                      Natural Language Prompt{' '}
                      <span className="text-red-500">*</span>
                    </label>
                    <textarea
                      value={editingState.natural_language_prompt}
                      onChange={(e) =>
                        setEditingState({
                          ...editingState,
                          natural_language_prompt: e.target.value,
                        })
                      }
                      placeholder="Describe what you want to query in natural language..."
                      className="w-full h-24 px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-purple-500"
                    />
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-text-primary mb-2">
                      Additional Context
                    </label>
                    <textarea
                      value={editingState.additional_context}
                      onChange={(e) =>
                        setEditingState({
                          ...editingState,
                          additional_context: e.target.value,
                        })
                      }
                      placeholder="Any additional context or information..."
                      className="w-full h-20 px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-purple-500"
                    />
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-text-primary mb-2">
                      SQL Query <span className="text-red-500">*</span>
                    </label>
                    <SqlEditor
                      value={editingState.sql_query}
                      onChange={(value) => {
                        setEditingState({ ...editingState, sql_query: value });
                        setEditTestPassed(false);
                        editValidation.validate(value);
                      }}
                      dialect={dialect}
                      schema={codeMirrorSchema}
                      validationStatus={getValidationStatus(editValidation.status)}
                      validationError={editValidation.error ?? undefined}
                      placeholder="SELECT * FROM users WHERE status = {{status}} LIMIT {{limit}}"
                      minHeight="200px"
                    />
                    {editValidation.warnings.length > 0 && (
                      <div className="mt-2 bg-amber-500/10 border border-amber-500/30 rounded-lg p-3">
                        <div className="flex items-start gap-2">
                          <AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400 mt-0.5 flex-shrink-0" />
                          <div className="flex-1">
                            <p className="text-sm font-medium text-amber-600 dark:text-amber-400">
                              Warning
                            </p>
                            {editValidation.warnings.map((warning, index) => (
                              <p
                                key={index}
                                className="text-xs text-amber-600/80 dark:text-amber-400/80 mt-1"
                              >
                                {warning}
                              </p>
                            ))}
                          </div>
                        </div>
                      </div>
                    )}
                    <div className="flex items-center gap-2 mt-4">
                      <input
                        type="checkbox"
                        id="allows-modification-edit"
                        checked={editingState.allows_modification}
                        onChange={(e) => {
                          setEditingState({
                            ...editingState,
                            allows_modification: e.target.checked,
                          });
                          setEditTestPassed(false);
                        }}
                        className="rounded border-border-medium"
                      />
                      <label
                        htmlFor="allows-modification-edit"
                        className="text-sm text-text-primary cursor-pointer"
                      >
                        Allow data modification (INSERT, UPDATE, DELETE, CALL)
                      </label>
                    </div>
                    {editingState.allows_modification && (
                      <div className="mt-2 bg-amber-500/10 border border-amber-500/30 rounded-lg p-3">
                        <div className="flex items-start gap-2">
                          <AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400 mt-0.5 flex-shrink-0" />
                          <div className="flex-1">
                            <p className="text-sm text-amber-600 dark:text-amber-400">
                              This query will be able to modify or delete data. Ensure the SQL
                              and parameters are thoroughly reviewed before enabling.
                            </p>
                          </div>
                        </div>
                      </div>
                    )}
                  </div>

                  <ParameterEditor
                    parameters={editingState.parameters}
                    onChange={(parameters) => {
                      setEditingState({ ...editingState, parameters });
                      setEditTestPassed(false);
                    }}
                    sqlQuery={editingState.sql_query}
                  />

                  {editingState.parameters.length > 0 && (
                    <div className="border-t border-border-light pt-4">
                      <h3 className="text-sm font-medium text-text-primary mb-3">
                        Test
                      </h3>
                      <ParameterInputForm
                        parameters={editingState.parameters}
                        values={testParameterValues}
                        onChange={setTestParameterValues}
                      />
                    </div>
                  )}

                  <div>
                    {editingState.allows_modification && (
                      <div className="mb-4 bg-red-500/10 border border-red-500/30 rounded-lg p-3">
                        <div className="flex items-start gap-2">
                          <AlertTriangle className="h-4 w-4 text-red-600 dark:text-red-400 mt-0.5 flex-shrink-0" />
                          <div className="flex-1">
                            <p className="text-sm font-medium text-red-600 dark:text-red-400">
                              Warning: Testing modifies live data
                            </p>
                            <p className="text-xs text-red-600/80 dark:text-red-400/80 mt-1">
                              Testing this query will execute it against your live database and may
                              modify data. Use test parameters that won&apos;t cause harm.
                            </p>
                          </div>
                        </div>
                      </div>
                    )}
                    <Button
                      variant={editingState.allows_modification ? 'destructive' : 'outline'}
                      onClick={() =>
                        handleTestQuery(
                          editingState.sql_query,
                          false,
                          editingState.parameters,
                          testParameterValues
                        )
                      }
                      disabled={
                        !editingState.sql_query.trim() ||
                        isTesting ||
                        hasUndefinedParams(editingState.sql_query, editingState.parameters)
                      }
                    >
                      {isTesting ? (
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      ) : (
                        <Play className="mr-2 h-4 w-4" />
                      )}
                      {editingState.allows_modification
                        ? 'Test Query (Modifies Data)'
                        : 'Test Query'}
                    </Button>
                  </div>

                  {queryResults && (
                    <QueryResultsTable
                      columns={queryResults.columns}
                      rows={queryResults.rows}
                      totalRowCount={queryResults.row_count}
                      maxRows={10}
                      maxColumns={20}
                    />
                  )}

                  <OutputColumnEditor
                    outputColumns={editingState.output_columns}
                    onChange={(output_columns) =>
                      setEditingState({ ...editingState, output_columns })
                    }
                  />

                  <div>
                    <label className="block text-sm font-medium text-text-primary mb-2">
                      Constraints
                    </label>
                    <textarea
                      value={editingState.constraints}
                      onChange={(e) =>
                        setEditingState({
                          ...editingState,
                          constraints: e.target.value,
                        })
                      }
                      placeholder="Describe limitations and assumptions (e.g., 'Only includes completed orders', 'Excludes refunded amounts')..."
                      className="w-full h-24 px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-purple-500"
                    />
                  </div>

                  <div className="pt-4 border-t border-border-light">
                    <div className="flex justify-end gap-2">
                      <Button
                        variant="outline"
                        onClick={() => {
                          setEditingQueryId(null);
                          setEditingState(null);
                          editValidation.reset();
                        }}
                      >
                        Cancel
                      </Button>
                      <Button
                        onClick={handleSaveEdit}
                        disabled={
                          !editingState.natural_language_prompt.trim() ||
                          !editingState.sql_query.trim() ||
                          !editTestPassed ||
                          isSaving ||
                          hasUndefinedParams(editingState.sql_query, editingState.parameters)
                        }
                      >
                        {isSaving ? (
                          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        ) : (
                          <Save className="mr-2 h-4 w-4" />
                        )}
                        Save Changes
                      </Button>
                    </div>
                    {!editTestPassed && editingState.sql_query.trim() && (
                      <p className="text-sm text-amber-600 dark:text-amber-400 text-right mt-2">
                        Please test the query before saving
                      </p>
                    )}
                  </div>
                </CardContent>
              </>
            ) : selectedQuery ? (
              // View Query Details
              <>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <div>
                      <div className="flex items-center gap-2 mb-1">
                        <CardTitle className="line-clamp-1">
                          {selectedQuery.natural_language_prompt}
                        </CardTitle>
                        {selectedQuery.allows_modification && (
                          <span className="flex-shrink-0 inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium rounded bg-red-500/10 text-red-600 dark:text-red-400">
                            <Pencil className="h-3 w-3" />
                            Modifies Data
                          </span>
                        )}
                      </div>
                      <CardDescription>
                        Created{' '}
                        {new Date(selectedQuery.created_at).toLocaleDateString()}{' '}
                        {selectedQuery.usage_count > 0 && (
                          <>
                            {' '}
                            • Used {selectedQuery.usage_count} time
                            {selectedQuery.usage_count !== 1 ? 's' : ''}
                          </>
                        )}
                      </CardDescription>
                    </div>
                    <div className="flex gap-2">
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleToggleEnabled(selectedQuery)}
                        title={
                          selectedQuery.is_enabled
                            ? 'Disable query'
                            : 'Enable query'
                        }
                      >
                        {selectedQuery.is_enabled ? (
                          <CheckCircle2 className="h-4 w-4 text-green-500" />
                        ) : (
                          <AlertCircle className="h-4 w-4 text-gray-500" />
                        )}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleEditQuery(selectedQuery)}
                      >
                        <Edit3 className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleDeleteClick(selectedQuery)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="space-y-6">
                  {selectedQuery.additional_context && (
                    <div>
                      <div className="flex items-center gap-2 mb-2">
                        <MessageSquare className="h-4 w-4 text-text-tertiary" />
                        <h3 className="text-sm font-medium text-text-primary">
                          Additional Context
                        </h3>
                      </div>
                      <p className="text-sm text-text-secondary bg-surface-secondary p-3 rounded-lg">
                        {selectedQuery.additional_context}
                      </p>
                    </div>
                  )}

                  <div>
                    <div className="flex items-center justify-between mb-2">
                      <div className="flex items-center gap-2">
                        <FileText className="h-4 w-4 text-text-tertiary" />
                        <h3 className="text-sm font-medium text-text-primary">
                          SQL Query
                        </h3>
                      </div>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleCopyQuery(selectedQuery.sql_query)}
                      >
                        <Copy className="h-3 w-3 mr-1" />
                        Copy
                      </Button>
                    </div>
                    <SqlEditor
                      value={selectedQuery.sql_query}
                      onChange={() => {}}
                      dialect={dialect}
                      schema={codeMirrorSchema}
                      readOnly
                      minHeight="150px"
                    />
                  </div>

                  {selectedQuery.parameters && selectedQuery.parameters.length > 0 && (
                    <div className="border-t border-border-light pt-4">
                      <ParameterInputForm
                        parameters={selectedQuery.parameters}
                        values={executeParameterValues}
                        onChange={setExecuteParameterValues}
                      />
                    </div>
                  )}

                  <div className="flex gap-2 pt-4 border-t border-border-light">
                    <Button
                      onClick={() => handleExecuteQuery(selectedQuery)}
                      disabled={!selectedQuery.is_enabled || isTesting}
                    >
                      {isTesting ? (
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      ) : (
                        <Play className="mr-2 h-4 w-4" />
                      )}
                      Execute Query
                    </Button>
                  </div>

                  {queryResults && (
                    <QueryResultsTable
                      columns={queryResults.columns}
                      rows={queryResults.rows}
                      totalRowCount={queryResults.row_count}
                      maxRows={10}
                      maxColumns={20}
                    />
                  )}
                </CardContent>
              </>
            ) : (
              // Empty State
              <CardContent className="flex-1 flex items-center justify-center">
                <div className="text-center">
                  <Database className="h-12 w-12 text-text-tertiary mx-auto mb-4" />
                  <h3 className="text-lg font-medium text-text-primary mb-2">
                    No Query Selected
                  </h3>
                  <p className="text-sm text-text-secondary mb-4">
                    Select a query from the list or create a new one
                  </p>
                  <Button
                    onClick={() => {
                      setIsCreating(true);
                      setSelectedQuery(null);
                      setQueryResults(null);
                      resetCreateForm();
                    }}
                  >
                    <Plus className="mr-2 h-4 w-4" />
                    Create New Query
                  </Button>
                </div>
              </CardContent>
            )}
          </Card>
        </div>
      </div>

      <DeleteQueryDialog
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
        projectId={projectId}
        datasourceId={datasourceId}
        query={queryToDelete}
        onQueryDeleted={handleQueryDeleted}
      />
    </>
  );
};

export default QueriesView;
