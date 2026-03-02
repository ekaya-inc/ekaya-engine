import {
  ArrowLeft,
  ChevronDown,
  ChevronRight,
  Info,
  ListTree,
  RefreshCw,
  Table2,
  Columns,
} from "lucide-react";
import { useState, useCallback, useMemo, useEffect } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { Button } from "../components/ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../components/ui/Card";
import { useDatasourceConnection } from "../contexts/DatasourceConnectionContext";
import { useToast } from "../hooks/useToast";
import engineApi from "../services/engineApi";
import type { SchemaTable as ApiSchemaTable, PendingChangeInfo } from "../types";
import { buildSelectionPayloads } from "../utils/schemaUtils";

interface Column {
  name: string;
  type: string;
  notNull: boolean;
}

interface Table {
  name: string;
  columns: Column[];
}

interface SchemaData {
  catalog: string;
  schema: string;
  tables: Table[];
}

interface TableSelectionState {
  selected: boolean;
  columns: Record<string, boolean>;
}

type SelectionState = Record<string, TableSelectionState>;
type ExpandedTables = Record<string, boolean>;

interface SelectedCount {
  tables: number;
  columns: number;
}

const SchemaPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();
  const { refreshSchemaSelections, selectedDatasource } = useDatasourceConnection();

  // State for schema data from API
  const [schemaData, setSchemaData] = useState<SchemaData | null>(null);
  const [apiTables, setApiTables] = useState<ApiSchemaTable[]>([]); // Raw API response for is_selected
  const [pendingChanges, setPendingChanges] = useState<Record<string, PendingChangeInfo>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isStartingExtraction, setIsStartingExtraction] = useState(false);
  const [isRefreshingSchema, setIsRefreshingSchema] = useState(false);

  // Fetch schema data from API
  useEffect(() => {
    if (!pid || !selectedDatasource?.datasourceId) return;

    async function fetchSchema() {
      try {
        setLoading(true);
        setError(null);
        const response = await engineApi.getSchema(pid as string, selectedDatasource?.datasourceId as string);

        // Transform API response to match SchemaData interface
        if (!response.data) {
          throw new Error("No schema data returned from API");
        }
        const transformedData: SchemaData = {
          catalog: "Datasource Schema", // API doesn't provide catalog name
          schema: "public", // API doesn't provide schema name
          tables: response.data.tables.map(table => ({
            name: table.table_name,
            columns: table.columns.map(col => ({
              name: col.column_name,
              type: col.data_type,
              notNull: col.is_nullable === 'NO' || col.is_nullable === false,
            })),
          })),
        };

        setSchemaData(transformedData);
        setApiTables(response.data.tables); // Store raw API tables for is_selected
        setPendingChanges(response.data.pending_changes ?? {});
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : "Failed to fetch schema";
        console.error("Failed to fetch schema:", errorMessage);
        setError(errorMessage);
      } finally {
        setLoading(false);
      }
    }

    fetchSchema();
  }, [pid, selectedDatasource?.datasourceId]);

  const [selectionState, setSelectionState] = useState<SelectionState>({});
  const [expandedTables, setExpandedTables] = useState<ExpandedTables>({});
  const [initialSelectionState, setInitialSelectionState] = useState<SelectionState | null>(null);
  const [isFirstTimeSetup, setIsFirstTimeSetup] = useState(false);

  // Initialize selection state when schema data is loaded
  // Selection state is now embedded in the schema response (is_selected fields)
  useEffect(() => {
    if (!schemaData || !pid || apiTables.length === 0) return;

    // Check if any table has is_selected=true (indicates previous selections saved)
    const hasAnySelection = apiTables.some((table) => table.is_selected === true);

    // First time setup: no tables have is_selected=true yet
    const isFirstTime = !hasAnySelection;
    setIsFirstTimeSetup(isFirstTime);

    // Build selection state from API response
    const state: SelectionState = {};

    schemaData.tables.forEach((table) => {
      // Find corresponding API table for is_selected state
      const apiTable = apiTables.find((t) => t.table_name === table.name);

      // If first time, default all tables to selected
      // Otherwise, use the is_selected state from API
      const isTableSelected = isFirstTime ? true : (apiTable?.is_selected ?? false);

      state[table.name] = {
        selected: isTableSelected,
        columns: {},
      };

      const tableState = state[table.name];
      if (!tableState) return;

      table.columns.forEach((column) => {
        // Find corresponding API column for is_selected state
        const apiColumn = apiTable?.columns.find((c) => c.column_name === column.name);

        if (isFirstTime) {
          // First time: all columns selected
          tableState.columns[column.name] = true;
        } else {
          // Use saved selection state
          tableState.columns[column.name] = apiColumn?.is_selected ?? false;
        }
      });
    });

    setSelectionState(state);
    setInitialSelectionState(JSON.parse(JSON.stringify(state)));
  }, [schemaData, pid, apiTables]);

  // Calculate if all items are selected
  const allSelected = useMemo(() => {
    return Object.values(selectionState).every(
      (table) =>
        table.selected && Object.values(table.columns).every((col) => col)
    );
  }, [selectionState]);

  // Calculate if some items are selected (for indeterminate state)
  const someSelected = useMemo(() => {
    const selections = Object.values(selectionState).flatMap((table) => [
      table.selected,
      ...Object.values(table.columns),
    ]);
    return selections.some((s) => s) && !selections.every((s) => s);
  }, [selectionState]);

  // Toggle expansion of a table
  const toggleTableExpansion = useCallback((tableName: string): void => {
    setExpandedTables((prev) => ({
      ...prev,
      [tableName]: !prev[tableName],
    }));
  }, []);

  // Handle select/unselect all
  const handleSelectAll = useCallback((checked: boolean): void => {
    if (!schemaData) return;
    const newState: SelectionState = {};
    schemaData.tables.forEach((table) => {
      newState[table.name] = {
        selected: checked,
        columns: {},
      };
      const tableState = newState[table.name];
      if (!tableState) return;
      table.columns.forEach((column) => {
        tableState.columns[column.name] = checked;
      });
    });
    setSelectionState(newState);
  }, [schemaData]);

  // Handle table checkbox change
  const handleTableChange = useCallback(
    (tableName: string, checked: boolean): void => {
      if (!schemaData) return;
      setSelectionState((prev) => {
        const newState = { ...prev };
        newState[tableName] = {
          selected: checked,
          columns: {},
        };
        const tableState = newState[tableName];
        if (!tableState) return newState;
        // When checking/unchecking a table, update all its columns
        const table = schemaData.tables.find((t) => t.name === tableName);
        if (table) {
          table.columns.forEach((column) => {
            tableState.columns[column.name] = checked;
          });
        }
        return newState;
      });
    },
    [schemaData]
  );

  // Handle column checkbox change
  const handleColumnChange = useCallback(
    (tableName: string, columnName: string, checked: boolean): void => {
      setSelectionState((prev) => {
        const newState = { ...prev };
        const tableState = newState[tableName];
        if (!tableState) return newState;

        tableState.columns[columnName] = checked;

        // If checking a column and table is unchecked, check the table
        if (checked && !tableState.selected) {
          tableState.selected = true;
        }

        // If unchecking a column and all columns are unchecked, uncheck the table
        const allColumnsUnchecked = Object.values(
          tableState.columns
        ).every((col) => !col);
        if (!checked && allColumnsUnchecked) {
          tableState.selected = false;
        }

        return newState;
      });
    },
    []
  );

  // Compute auto-applied removals for the info banner
  const autoAppliedRemovals = useMemo(() => {
    const removals: string[] = [];
    for (const [key, change] of Object.entries(pendingChanges)) {
      if (change.status === 'auto_applied' && (change.change_type === 'dropped_table' || change.change_type === 'dropped_column')) {
        removals.push(key);
      }
    }
    return removals;
  }, [pendingChanges]);

  // Whether there are actionable pending changes (not auto_applied)
  const hasPendingChanges = useMemo(() => {
    return Object.values(pendingChanges).some(change => change.status === 'pending');
  }, [pendingChanges]);

  // Handle reject all pending changes and navigate away
  const handleRejectChanges = useCallback(async (): Promise<void> => {
    if (!pid || !selectedDatasource?.datasourceId) return;

    try {
      await engineApi.rejectPendingChanges(pid, selectedDatasource.datasourceId);
      toast({
        title: "Changes Rejected",
        description: "All pending schema changes have been rejected.",
        variant: "default",
      });
      navigate(`/projects/${pid}`);
    } catch (err) {
      console.error('Failed to reject pending changes:', err);
      toast({
        title: "Error",
        description: "Failed to reject pending changes. Please try again.",
        variant: "destructive",
      });
    }
  }, [pid, selectedDatasource?.datasourceId, toast, navigate]);

  // Handle save schema
  const handleSaveSchema = useCallback(async (): Promise<void> => {
    if (!schemaData || !pid || !selectedDatasource?.datasourceId) return;

    try {
      setIsStartingExtraction(true);

      // Build selection payloads using table and column IDs
      const { tableSelections, columnSelections } = buildSelectionPayloads(
        apiTables,
        selectionState
      );

      // Save schema selections to database
      const saveResponse = await engineApi.saveSchemaSelections(pid, selectedDatasource.datasourceId, tableSelections, columnSelections);

      // Refresh schema selections state to enable ontology tile
      await refreshSchemaSelections(pid);

      // Update initial state after successful save (no longer dirty)
      setInitialSelectionState(JSON.parse(JSON.stringify(selectionState)));
      setIsFirstTimeSetup(false);
      setPendingChanges({}); // Clear pending changes after save

      // Build toast description with resolved change counts
      const approved = saveResponse.data?.approved_count ?? 0;
      const rejected = saveResponse.data?.rejected_count ?? 0;
      let description = "Schema selections saved successfully!";
      if (approved > 0 || rejected > 0) {
        const parts: string[] = [];
        if (approved > 0) parts.push(`${approved} changes approved`);
        if (rejected > 0) parts.push(`${rejected} changes rejected`);
        description = `Schema saved. ${parts.join(', ')}.`;
      }
      if (autoAppliedRemovals.length > 0) {
        description += ` ${autoAppliedRemovals.length} items were automatically removed (no longer in datasource).`;
      }

      // Show success toast
      toast({
        title: "Success",
        description,
        variant: "success",
      });

      // Navigate back to dashboard after successful save
      navigate(`/projects/${pid}`);
    } catch (err) {
      console.error('Failed to save schema selections:', err);
      toast({
        title: "Error",
        description: "Failed to save schema selections. Please try again.",
        variant: "destructive",
      });
    } finally {
      setIsStartingExtraction(false);
    }
  }, [schemaData, pid, selectedDatasource?.datasourceId, selectionState, apiTables, refreshSchemaSelections, toast, navigate, autoAppliedRemovals]);

  // Handle refresh schema from datasource
  const handleRefreshSchema = useCallback(async (): Promise<void> => {
    if (!pid || !selectedDatasource?.datasourceId) {
      toast({
        title: "Error",
        description: "No datasource available for refresh",
        variant: "destructive",
      });
      return;
    }

    try {
      setIsRefreshingSchema(true);
      setLoading(true);

      // Call the refresh schema API
      await engineApi.refreshSchema(pid, selectedDatasource.datasourceId);

      // Re-fetch the schema data to show updated tables/columns
      const response = await engineApi.getSchema(pid, selectedDatasource.datasourceId);

      // Transform API response to match SchemaData interface
      if (!response.data) {
        throw new Error("No schema data returned from API");
      }
      const transformedData: SchemaData = {
        catalog: "Datasource Schema",
        schema: "public",
        tables: response.data.tables.map(table => ({
          name: table.table_name,
          columns: table.columns.map(col => ({
            name: col.column_name,
            type: col.data_type,
            notNull: col.is_nullable === 'NO' || col.is_nullable === false,
          })),
        })),
      };

      setSchemaData(transformedData);
      setApiTables(response.data.tables); // Update raw API tables
      setPendingChanges(response.data.pending_changes ?? {});
      setError(null);

      toast({
        title: "Success",
        description: "Schema refreshed from datasource",
        variant: "success",
      });
    } catch (err) {
      console.error('Failed to refresh schema:', err);
      const errorMessage = err instanceof Error ? err.message : "Failed to refresh schema";
      setError(errorMessage);
      toast({
        title: "Error",
        description: errorMessage,
        variant: "destructive",
      });
    } finally {
      setIsRefreshingSchema(false);
      setLoading(false);
    }
  }, [pid, selectedDatasource, toast]);

  // Count selected items
  const selectedCount: SelectedCount = useMemo(() => {
    let tables = 0;
    let columns = 0;
    Object.values(selectionState).forEach((table) => {
      if (table.selected) tables++;
      Object.values(table.columns).forEach((col) => {
        if (col) columns++;
      });
    });
    return { tables, columns };
  }, [selectionState]);

  // Check if selection state has changed from initial state
  const isDirty = useMemo(() => {
    if (!initialSelectionState) return false;
    return JSON.stringify(selectionState) !== JSON.stringify(initialSelectionState);
  }, [selectionState, initialSelectionState]);

  // Helper to get pending change for a table
  const getTablePendingChange = useCallback((schemaName: string, tableName: string): PendingChangeInfo | undefined => {
    return pendingChanges[`${schemaName}.${tableName}`];
  }, [pendingChanges]);

  // Helper to get pending change for a column
  const getColumnPendingChange = useCallback((schemaName: string, tableName: string, columnName: string): PendingChangeInfo | undefined => {
    return pendingChanges[`${schemaName}.${tableName}.${columnName}`];
  }, [pendingChanges]);

  // Loading state
  if (loading) {
    return (
      <div className="mx-auto max-w-6xl">
        <div className="mb-6">
          <Button variant="ghost" onClick={() => navigate(`/projects/${pid}`)}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
        </div>
        <div className="flex items-center justify-center min-h-[400px]">
          <div className="text-center">
            <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-solid border-current border-r-transparent motion-reduce:animate-[spin_1.5s_linear_infinite]" role="status">
              <span className="sr-only">Loading schema...</span>
            </div>
            <p className="mt-4 text-sm text-muted-foreground">Loading database schema...</p>
          </div>
        </div>
      </div>
    );
  }

  // Error state
  if (error || !schemaData) {
    return (
      <div className="mx-auto max-w-6xl">
        <div className="mb-6">
          <Button variant="ghost" onClick={() => navigate(`/projects/${pid}`)}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
        </div>
        <div className="flex items-center justify-center min-h-[400px]">
          <div className="text-center max-w-md p-6">
            <div className="mb-4 text-destructive">
              <svg xmlns="http://www.w3.org/2000/svg" className="h-12 w-12 mx-auto" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
            </div>
            <h2 className="text-lg font-semibold mb-2">Failed to Load Schema</h2>
            <p className="text-sm text-muted-foreground mb-4">
              {error ?? "No schema data available"}
            </p>
            <button
              onClick={() => window.location.reload()}
              className="px-4 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90"
            >
              Retry
            </button>
          </div>
        </div>
      </div>
    );
  }

  // Empty schema state
  if (schemaData.tables.length === 0) {
    return (
      <div className="mx-auto max-w-6xl">
        <div className="mb-6">
          <Button variant="ghost" onClick={() => navigate(`/projects/${pid}`)}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
        </div>
        <div className="flex items-center justify-center min-h-[400px]">
          <div className="text-center max-w-md p-6">
            <div className="mb-4">
              <ListTree className="h-16 w-16 mx-auto text-muted-foreground" />
            </div>
            <h2 className="text-xl font-semibold mb-4">Schema is empty</h2>
            <Button
              onClick={handleRefreshSchema}
              disabled={isRefreshingSchema || !selectedDatasource?.datasourceId}
            >
              {isRefreshingSchema ? 'Importing...' : 'Import'}
            </Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-6xl">
      <div className="mb-6">
        <div className="flex items-center justify-between mb-4">
          <Button
            variant="ghost"
            onClick={() => navigate(`/projects/${pid}`)}
            disabled={isFirstTimeSetup || isDirty}
            className={isFirstTimeSetup || isDirty ? "opacity-50 cursor-not-allowed" : ""}
          >
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
          <div className="flex gap-2">
            <Button
              variant="outline"
              onClick={hasPendingChanges ? handleRejectChanges : () => navigate(`/projects/${pid}`)}
              disabled={!(isFirstTimeSetup || isDirty || hasPendingChanges)}
            >
              {hasPendingChanges ? 'Reject Changes' : 'Cancel'}
            </Button>
            <Button
              onClick={handleSaveSchema}
              disabled={isStartingExtraction || !(isFirstTimeSetup || isDirty || hasPendingChanges)}
              className={hasPendingChanges ? "bg-green-600 hover:bg-green-700 text-white font-medium" : "bg-blue-600 hover:bg-blue-700 text-white font-medium"}
            >
              {isStartingExtraction ? 'Saving...' : (hasPendingChanges ? 'Approve Changes' : 'Save Schema')}
            </Button>
          </div>
        </div>
        <h1 className="text-3xl font-bold text-text-primary">
          Schema Selection
        </h1>
        <p className="mt-2 text-text-secondary">
          Select the tables and columns to include in your ontology
        </p>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-green-500/10">
                <ListTree className="h-5 w-5 text-green-500" />
              </div>
              <div>
                <CardTitle>{schemaData.catalog}</CardTitle>
                <CardDescription>
                  Schema: {schemaData.schema} • {selectedCount.tables} tables,{" "}
                  {selectedCount.columns} columns selected
                </CardDescription>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={handleRefreshSchema}
                disabled={isRefreshingSchema || !selectedDatasource?.datasourceId}
              >
                <RefreshCw className={`h-4 w-4 mr-1 ${isRefreshingSchema ? 'animate-spin' : ''}`} />
                {isRefreshingSchema ? 'Refreshing...' : 'Refresh'}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setExpandedTables({})}
              >
                Collapse All
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  const expanded: ExpandedTables = {};
                  schemaData.tables.forEach((table) => {
                    expanded[table.name] = true;
                  });
                  setExpandedTables(expanded);
                }}
              >
                Expand All
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {/* Auto-applied removals banner */}
          {autoAppliedRemovals.length > 0 && (
            <div className="mb-4 flex items-start gap-2 rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm text-blue-800 dark:border-blue-800 dark:bg-blue-950/30 dark:text-blue-200">
              <Info className="mt-0.5 h-4 w-4 shrink-0" />
              <div>
                <span className="font-medium">Last refresh removed: </span>
                {autoAppliedRemovals.join(', ')}
              </div>
            </div>
          )}

          <div className="space-y-2">
            {/* Select/Unselect All */}
            <div className="flex items-center gap-2 border-b border-border-light pb-3 mb-3">
              <input
                type="checkbox"
                id="select-all"
                checked={allSelected}
                ref={(input) => {
                  if (input) input.indeterminate = someSelected;
                }}
                onChange={(e) => handleSelectAll(e.target.checked)}
                className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              <label
                htmlFor="select-all"
                className="font-medium text-text-primary cursor-pointer"
              >
                Select All Tables and Columns
              </label>
            </div>

            {/* Tables and Columns */}
            {schemaData.tables.map((table) => {
              // Safety check: ensure selection state is initialized for this table
              const tableState = selectionState[table.name];
              if (!tableState) return null;

              const isExpanded = expandedTables[table.name];
              const tableSelected = tableState.selected;
              const columnSelections = Object.values(tableState.columns);
              const allColumnsSelected = columnSelections.every((col) => col);
              const someColumnsSelected =
                columnSelections.some((col) => col) && !allColumnsSelected;

              // Look up pending change for this table
              const apiTable = apiTables.find((t) => t.table_name === table.name);
              const schemaName = apiTable?.schema_name ?? "public";
              const tablePendingChange = getTablePendingChange(schemaName, table.name);

              return (
                <div
                  key={table.name}
                  className="border border-border-light rounded-lg"
                >
                  {/* Table Header */}
                  <div className="flex items-center gap-2 p-3 hover:bg-surface-secondary">
                    <button
                      onClick={() => toggleTableExpansion(table.name)}
                      className="p-0.5 hover:bg-surface-tertiary rounded"
                    >
                      {isExpanded ? (
                        <ChevronDown className="h-4 w-4 text-text-secondary" />
                      ) : (
                        <ChevronRight className="h-4 w-4 text-text-secondary" />
                      )}
                    </button>
                    <input
                      type="checkbox"
                      id={`table-${table.name}`}
                      checked={tableSelected && allColumnsSelected}
                      ref={(input) => {
                        if (input)
                          input.indeterminate =
                            tableSelected && someColumnsSelected;
                      }}
                      onChange={(e) =>
                        handleTableChange(table.name, e.target.checked)
                      }
                      className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                    />
                    <Table2 className="h-4 w-4 text-text-secondary" />
                    <label
                      htmlFor={`table-${table.name}`}
                      className="font-medium text-text-primary cursor-pointer flex-1"
                    >
                      {table.name}
                    </label>
                    {tablePendingChange?.change_type === 'new_table' && (
                      <span className="inline-flex items-center rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900/40 dark:text-blue-300">
                        New
                      </span>
                    )}
                    <span className="text-sm text-text-secondary">
                      {table.columns.length} columns
                    </span>
                  </div>

                  {/* Columns (when expanded) */}
                  {isExpanded && (
                    <div className="border-t border-border-light bg-surface-secondary/50">
                      {table.columns.map((column) => {
                        const colPendingChange = getColumnPendingChange(schemaName, table.name, column.name);
                        return (
                        <div
                          key={column.name}
                          className="flex items-center gap-2 px-3 py-2 pl-11 hover:bg-surface-secondary"
                        >
                          <input
                            type="checkbox"
                            id={`column-${table.name}-${column.name}`}
                            checked={
                              selectionState[table.name]?.columns[
                                column.name
                              ] ?? false
                            }
                            onChange={(e) =>
                              handleColumnChange(
                                table.name,
                                column.name,
                                e.target.checked
                              )
                            }
                            className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                          />
                          <Columns className="h-3.5 w-3.5 text-text-tertiary" />
                          <label
                            htmlFor={`column-${table.name}-${column.name}`}
                            className="text-text-primary cursor-pointer flex-1"
                          >
                            {column.name}
                          </label>
                          {colPendingChange?.change_type === 'new_column' && (
                            <span className="inline-flex items-center rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900/40 dark:text-blue-300">
                              New
                            </span>
                          )}
                          {colPendingChange?.change_type === 'modified_column' && (
                            <span className="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">
                              Modified
                            </span>
                          )}
                          <span className="text-xs text-text-tertiary">
                            {column.type}
                          </span>
                          {column.notNull && (
                            <span className="text-xs font-medium text-orange-500">
                              NOT NULL
                            </span>
                          )}
                        </div>
                        );
                      })}
                    </div>
                  )}
                </div>
              );
            })}
          </div>

        </CardContent>
      </Card>
    </div>
  );
};

export default SchemaPage;
