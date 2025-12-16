import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  Circle,
  Key,
  Lightbulb,
  Network,
  Pencil,
  Plus,
  Sparkles,
  Trash2,
} from "lucide-react";
import { useState, useEffect } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { AddRelationshipDialog } from "../components/AddRelationshipDialog";
import { RelationshipDiscoveryProgress } from "../components/RelationshipDiscoveryProgress";
import { RemoveRelationshipDialog } from "../components/RemoveRelationshipDialog";
import { Button } from "../components/ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../components/ui/Card";
import { useDatasourceConnection } from "../contexts/DatasourceConnectionContext";
import engineApi from "../services/engineApi";
import type { RelationshipDetail, RelationshipType, DatasourceSchema } from "../types";

/**
 * RelationshipsPage - Display and manage data relationships
 * Shows all relationships between tables with filtering and grouping options.
 */
const RelationshipsPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { selectedDatasource } = useDatasourceConnection();

  // State for relationships data
  const [relationships, setRelationships] = useState<RelationshipDetail[]>([]);
  const [emptyTables, setEmptyTables] = useState<string[]>([]);
  const [orphanTables, setOrphanTables] = useState<string[]>([]);
  const [schema, setSchema] = useState<DatasourceSchema | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filter state - tableFilter can be empty (all), a table name, or special values
  const [typeFilter, setTypeFilter] = useState<RelationshipType | 'all'>('all');
  const [tableFilter, setTableFilter] = useState<string>('');
  const TABLE_FILTER_EMPTY = '__empty__';
  const TABLE_FILTER_NO_RELATIONSHIPS = '__no_relationships__';

  // Dialog state
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [discoveryOpen, setDiscoveryOpen] = useState(false);
  const [removeDialogOpen, setRemoveDialogOpen] = useState(false);
  const [relationshipToRemove, setRelationshipToRemove] = useState<RelationshipDetail | null>(null);

  // Handler for "Find Relationships" button - opens discovery progress dialog
  const handleFindRelationships = (): void => {
    setDiscoveryOpen(true);
  };

  // Handler for when discovery completes - refresh relationships
  const handleDiscoveryComplete = async (): Promise<void> => {
    if (!pid || !selectedDatasource?.datasourceId) return;
    try {
      const response = await engineApi.getRelationships(pid, selectedDatasource.datasourceId);
      if (response.data) {
        setRelationships(response.data.relationships);
        setEmptyTables(response.data.empty_tables ?? []);
        setOrphanTables(response.data.orphan_tables ?? []);
      }
    } catch (err) {
      console.error("Failed to refresh relationships after discovery:", err);
    }
  };

  // Handler for when a new relationship is added
  const handleRelationshipAdded = (newRelationship: RelationshipDetail): void => {
    setRelationships(prev => [...prev, newRelationship]);
  };

  // Handler to open remove confirmation dialog
  const handleRemoveClick = (rel: RelationshipDetail): void => {
    setRelationshipToRemove(rel);
    setRemoveDialogOpen(true);
  };

  // Handler for when a relationship is removed
  const handleRelationshipRemoved = (relationshipId: string): void => {
    setRelationships(prev => prev.filter(r => r.id !== relationshipId));
    setRelationshipToRemove(null);
    setRemoveDialogOpen(false);
  };

  // Fetch relationships and schema data
  useEffect(() => {
    if (!pid || !selectedDatasource?.datasourceId) return;

    async function fetchData(): Promise<void> {
      try {
        setLoading(true);
        setError(null);

        const datasourceId = selectedDatasource?.datasourceId as string;

        // Fetch both relationships and schema in parallel
        const [relationshipsResponse, schemaResponse] = await Promise.all([
          engineApi.getRelationships(pid as string, datasourceId),
          engineApi.getSchema(pid as string, datasourceId),
        ]);

        if (relationshipsResponse.data) {
          setRelationships(relationshipsResponse.data.relationships);
          setEmptyTables(relationshipsResponse.data.empty_tables ?? []);
          setOrphanTables(relationshipsResponse.data.orphan_tables ?? []);
        }
        if (schemaResponse.data) {
          setSchema(schemaResponse.data);
        }
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : "Failed to fetch data";
        console.error("Failed to fetch relationships data:", errorMessage);
        setError(errorMessage);
      } finally {
        setLoading(false);
      }
    }

    fetchData();
  }, [pid, selectedDatasource?.datasourceId]);

  // Get unique table names for filter dropdown
  const tableNames = [...new Set([
    ...relationships.map(r => r.source_table_name),
    ...relationships.map(r => r.target_table_name),
  ])].sort();

  // Check if special filter is active (empty or no relationships)
  const isSpecialFilter = tableFilter === TABLE_FILTER_EMPTY || tableFilter === TABLE_FILTER_NO_RELATIONSHIPS;

  // Filter relationships (only when not using special filter)
  const filteredRelationships = isSpecialFilter ? [] : relationships.filter(rel => {
    if (typeFilter !== 'all' && rel.relationship_type !== typeFilter) {
      return false;
    }
    if (tableFilter && rel.source_table_name !== tableFilter && rel.target_table_name !== tableFilter) {
      return false;
    }
    return true;
  });

  // Get tables to display when special filter is active
  const specialFilterTables = tableFilter === TABLE_FILTER_EMPTY
    ? emptyTables
    : tableFilter === TABLE_FILTER_NO_RELATIONSHIPS
    ? orphanTables
    : [];

  // Group by source table for display
  const groupedBySourceTable = filteredRelationships.reduce<Record<string, RelationshipDetail[]>>((acc, rel) => {
    const key = rel.source_table_name;
    acc[key] ??= [];
    acc[key].push(rel);
    return acc;
  }, {});

  // Count by type
  const countByType = {
    fk: relationships.filter(r => r.relationship_type === 'fk').length,
    inferred: relationships.filter(r => r.relationship_type === 'inferred').length,
    manual: relationships.filter(r => r.relationship_type === 'manual').length,
  };

  // Calculate tables without relationships (using API data or fallback to computed)
  const totalTablesInSchema = schema?.total_tables ?? 0;
  const tablesWithRelationships = new Set([
    ...relationships.map(r => r.source_table_name),
    ...relationships.map(r => r.target_table_name),
  ]);
  // Use API-provided counts if available, otherwise compute
  const emptyTableCount = emptyTables.length;
  const orphanTableCount = orphanTables.length;
  const tablesWithoutRelationships = (emptyTableCount + orphanTableCount) || (totalTablesInSchema - tablesWithRelationships.size);

  // Get type icon
  const getTypeIcon = (type: RelationshipType): React.ReactNode => {
    switch (type) {
      case 'fk':
        return <Key className="h-4 w-4 text-blue-500" />;
      case 'inferred':
        return <Lightbulb className="h-4 w-4 text-amber-500" />;
      case 'manual':
        return <Pencil className="h-4 w-4 text-green-500" />;
    }
  };

  // Get type label
  const getTypeLabel = (type: RelationshipType): string => {
    switch (type) {
      case 'fk':
        return 'Foreign Key';
      case 'inferred':
        return 'Inferred';
      case 'manual':
        return 'Manual';
    }
  };

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
              <span className="sr-only">Loading relationships...</span>
            </div>
            <p className="mt-4 text-sm text-muted-foreground">Loading relationships...</p>
          </div>
        </div>
      </div>
    );
  }

  // Error state
  if (error) {
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
            <h2 className="text-lg font-semibold mb-2">Failed to Load Relationships</h2>
            <p className="text-sm text-muted-foreground mb-4">
              {error}
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

  // Empty state
  if (relationships.length === 0) {
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
              <Network className="h-16 w-16 mx-auto text-muted-foreground" />
            </div>
            <h2 className="text-xl font-semibold mb-2">No Relationships Found</h2>
            <p className="text-sm text-muted-foreground mb-6">
              No foreign key or inferred relationships have been discovered yet.
              {totalTablesInSchema > 0 && (
                <span className="block mt-2 text-amber-600 dark:text-amber-400 font-medium">
                  {totalTablesInSchema} table{totalTablesInSchema !== 1 ? 's' : ''} in the schema could have relationships.
                </span>
              )}
            </p>
            <div className="flex flex-col sm:flex-row gap-3 justify-center">
              {totalTablesInSchema > 0 && (
                <Button
                  variant="default"
                  onClick={handleFindRelationships}
                  className="bg-amber-600 hover:bg-amber-700 text-white"
                >
                  <Sparkles className="mr-2 h-4 w-4" />
                  Find Relationships
                </Button>
              )}
              <Button variant="outline" onClick={() => setAddDialogOpen(true)}>
                <Plus className="mr-2 h-4 w-4" />
                Add Relationship
              </Button>
            </div>
          </div>
        </div>

        {/* Add Relationship Dialog (for empty state) */}
        <AddRelationshipDialog
          open={addDialogOpen}
          onOpenChange={setAddDialogOpen}
          projectId={pid ?? ''}
          datasourceId={selectedDatasource?.datasourceId ?? ''}
          schema={schema}
          onRelationshipAdded={handleRelationshipAdded}
        />

        {/* Relationship Discovery Progress (for empty state) */}
        <RelationshipDiscoveryProgress
          projectId={pid ?? ''}
          datasourceId={selectedDatasource?.datasourceId ?? ''}
          isOpen={discoveryOpen}
          onClose={() => setDiscoveryOpen(false)}
          onComplete={handleDiscoveryComplete}
        />
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
          >
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
        </div>
        <h1 className="text-3xl font-bold text-text-primary">
          Relationship Manager
        </h1>
        <p className="mt-2 text-text-secondary">
          View and manage relationships between tables in your datasource
        </p>
      </div>

      {/* Summary Card */}
      <Card className="mb-6">
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-indigo-500/10">
              <Network className="h-5 w-5 text-indigo-500" />
            </div>
            <div>
              <CardTitle>Summary</CardTitle>
              <CardDescription>
                {relationships.length} total relationships across {tableNames.length} tables
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {/* Warning alert for tables without relationships */}
          {tablesWithoutRelationships > 0 && (
            <div className="mb-4 flex items-center justify-between rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-900/50 dark:bg-amber-950/20">
              <div className="flex items-center gap-3">
                <AlertTriangle className="h-5 w-5 text-amber-600 dark:text-amber-500" />
                <div>
                  <p className="text-sm font-medium text-amber-800 dark:text-amber-200">
                    {tablesWithoutRelationships} table{tablesWithoutRelationships !== 1 ? 's' : ''} without relationships
                  </p>
                  <p className="text-xs text-amber-600 dark:text-amber-400">
                    {emptyTableCount > 0 && `${emptyTableCount} empty`}
                    {emptyTableCount > 0 && orphanTableCount > 0 && ', '}
                    {orphanTableCount > 0 && `${orphanTableCount} with no relationships`}
                  </p>
                </div>
              </div>
              <Button
                variant="default"
                size="sm"
                onClick={handleFindRelationships}
                className="bg-amber-600 hover:bg-amber-700 text-white"
              >
                <Sparkles className="mr-2 h-4 w-4" />
                Find Relationships
              </Button>
            </div>
          )}

          <div className="flex flex-wrap gap-4">
            <button
              onClick={() => { setTypeFilter('fk'); setTableFilter(''); }}
              className={`flex items-center gap-2 px-3 py-1.5 rounded-md transition-colors ${
                typeFilter === 'fk' && !tableFilter
                  ? 'bg-blue-100 dark:bg-blue-900/30'
                  : 'hover:bg-gray-100 dark:hover:bg-gray-800'
              }`}
            >
              <Key className="h-4 w-4 text-blue-500" />
              <span className="text-sm">
                <span className="font-medium">{countByType.fk}</span> Foreign Keys
              </span>
            </button>
            <button
              onClick={() => { setTypeFilter('inferred'); setTableFilter(''); }}
              className={`flex items-center gap-2 px-3 py-1.5 rounded-md transition-colors ${
                typeFilter === 'inferred' && !tableFilter
                  ? 'bg-amber-100 dark:bg-amber-900/30'
                  : 'hover:bg-gray-100 dark:hover:bg-gray-800'
              }`}
            >
              <Lightbulb className="h-4 w-4 text-amber-500" />
              <span className="text-sm">
                <span className="font-medium">{countByType.inferred}</span> Inferred
              </span>
            </button>
            <button
              onClick={() => { setTypeFilter('manual'); setTableFilter(''); }}
              className={`flex items-center gap-2 px-3 py-1.5 rounded-md transition-colors ${
                typeFilter === 'manual' && !tableFilter
                  ? 'bg-green-100 dark:bg-green-900/30'
                  : 'hover:bg-gray-100 dark:hover:bg-gray-800'
              }`}
            >
              <Pencil className="h-4 w-4 text-green-500" />
              <span className="text-sm">
                <span className="font-medium">{countByType.manual}</span> Manual
              </span>
            </button>
            {emptyTableCount > 0 && (
              <button
                onClick={() => { setTypeFilter('all'); setTableFilter(TABLE_FILTER_EMPTY); }}
                className={`flex items-center gap-2 px-3 py-1.5 rounded-md transition-colors ${
                  tableFilter === TABLE_FILTER_EMPTY
                    ? 'bg-gray-200 dark:bg-gray-700'
                    : 'hover:bg-gray-100 dark:hover:bg-gray-800'
                }`}
              >
                <Circle className="h-4 w-4 text-gray-400" />
                <span className="text-sm">
                  <span className="font-medium">{emptyTableCount}</span> Empty Tables
                </span>
              </button>
            )}
            {orphanTableCount > 0 && (
              <button
                onClick={() => { setTypeFilter('all'); setTableFilter(TABLE_FILTER_NO_RELATIONSHIPS); }}
                className={`flex items-center gap-2 px-3 py-1.5 rounded-md transition-colors ${
                  tableFilter === TABLE_FILTER_NO_RELATIONSHIPS
                    ? 'bg-amber-100 dark:bg-amber-900/30'
                    : 'hover:bg-gray-100 dark:hover:bg-gray-800'
                }`}
              >
                <AlertTriangle className="h-4 w-4 text-amber-500" />
                <span className="text-sm">
                  <span className="font-medium">{orphanTableCount}</span> No Relationships
                </span>
              </button>
            )}
            {(typeFilter !== 'all' || tableFilter) && (
              <button
                onClick={() => { setTypeFilter('all'); setTableFilter(''); }}
                className="flex items-center gap-2 px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
              >
                Clear filters
              </button>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Relationships List */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-4">
              <CardTitle>Relationships</CardTitle>
              {(typeFilter !== 'all' || tableFilter) && (
                <span className="text-sm text-muted-foreground">
                  Showing {isSpecialFilter ? specialFilterTables.length : filteredRelationships.length} of {relationships.length}
                </span>
              )}
            </div>
            <Button variant="outline" size="sm" onClick={() => setAddDialogOpen(true)}>
              <Plus className="mr-2 h-4 w-4" />
              Add Relationship
            </Button>
          </div>
          {/* Filters */}
          <div className="flex gap-4 mt-4">
            <div className="w-48">
              <label htmlFor="type-filter" className="block text-xs font-medium mb-1 text-muted-foreground">
                Type
              </label>
              <select
                id="type-filter"
                value={typeFilter}
                onChange={(e) => setTypeFilter(e.target.value as RelationshipType | 'all')}
                className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Types</option>
                <option value="fk">Foreign Key</option>
                <option value="inferred">Inferred</option>
                <option value="manual">Manual</option>
              </select>
            </div>
            <div className="w-48">
              <label htmlFor="table-filter" className="block text-xs font-medium mb-1 text-muted-foreground">
                Table
              </label>
              <select
                id="table-filter"
                value={tableFilter}
                onChange={(e) => setTableFilter(e.target.value)}
                className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="">All Tables</option>
                {emptyTableCount > 0 && (
                  <option value={TABLE_FILTER_EMPTY}>Empty Tables ({emptyTableCount})</option>
                )}
                {orphanTableCount > 0 && (
                  <option value={TABLE_FILTER_NO_RELATIONSHIPS}>No Relationships ({orphanTableCount})</option>
                )}
                {(emptyTableCount > 0 || orphanTableCount > 0) && (
                  <option disabled>──────────</option>
                )}
                {tableNames.map(name => (
                  <option key={name} value={name}>{name}</option>
                ))}
              </select>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {/* Display special filter tables (empty or no relationships) */}
          {isSpecialFilter && specialFilterTables.length > 0 && (
            <div className="space-y-2">
              <div className="text-sm text-muted-foreground mb-3">
                {tableFilter === TABLE_FILTER_EMPTY
                  ? "Tables with 0 rows - cannot discover relationships without data"
                  : "Tables with data but no discovered or defined relationships"}
              </div>
              {specialFilterTables.map(tableName => (
                <div
                  key={tableName}
                  className="flex items-center gap-3 p-3 border border-border-light rounded-lg hover:bg-surface-secondary/30"
                >
                  {tableFilter === TABLE_FILTER_EMPTY ? (
                    <Circle className="h-4 w-4 text-gray-400" />
                  ) : (
                    <AlertTriangle className="h-4 w-4 text-amber-500" />
                  )}
                  <span className="font-medium text-text-primary">{tableName}</span>
                  <span className="px-2 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-600">
                    {tableFilter === TABLE_FILTER_EMPTY ? 'Empty' : 'No Relationships'}
                  </span>
                </div>
              ))}
            </div>
          )}

          {/* Display relationships grouped by table */}
          {!isSpecialFilter && (
            <div className="space-y-4">
              {Object.entries(groupedBySourceTable).map(([tableName, tableRels]) => (
                <div key={tableName} className="border border-border-light rounded-lg">
                  {/* Table Header */}
                  <div className="flex items-center gap-2 p-3 bg-surface-secondary/50 border-b border-border-light rounded-t-lg">
                    <span className="font-medium text-text-primary">{tableName}</span>
                    <span className="text-sm text-text-secondary">
                      ({tableRels.length} relationship{tableRels.length !== 1 ? 's' : ''})
                    </span>
                  </div>

                  {/* Relationships */}
                  <div className="divide-y divide-border-light">
                    {tableRels.map((rel) => (
                      <div
                        key={rel.id}
                        className="group flex items-center gap-4 p-3 hover:bg-surface-secondary/30"
                      >
                        {/* Type Icon */}
                        <div className="flex-shrink-0" title={getTypeLabel(rel.relationship_type)}>
                          {getTypeIcon(rel.relationship_type)}
                        </div>

                        {/* Relationship Details */}
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 text-sm">
                            <span className="font-mono text-text-primary">
                              {rel.source_column_name}
                            </span>
                            <span className="text-text-tertiary text-xs">
                              ({rel.source_column_type})
                            </span>
                            <ArrowRight className="h-3 w-3 text-text-tertiary flex-shrink-0" />
                            <span className="font-medium text-text-primary">
                              {rel.target_table_name}
                            </span>
                            <span className="text-text-tertiary">.</span>
                            <span className="font-mono text-text-primary">
                              {rel.target_column_name}
                            </span>
                            <span className="text-text-tertiary text-xs">
                              ({rel.target_column_type})
                            </span>
                          </div>
                          {rel.cardinality && (
                            <div className="mt-1 text-xs text-text-secondary">
                              Cardinality: {rel.cardinality}
                            </div>
                          )}
                        </div>

                        {/* Type Badge */}
                        <span className="px-2 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-600">
                          {getTypeLabel(rel.relationship_type)}
                        </span>

                        {/* Remove Button - appears on hover */}
                        <button
                          onClick={() => handleRemoveClick(rel)}
                          className="flex-shrink-0 p-1 rounded opacity-0 group-hover:opacity-100 hover:bg-red-100 text-gray-400 hover:text-red-600 transition-opacity"
                          title="Remove relationship"
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          )}

          {/* Empty state messages */}
          {isSpecialFilter && specialFilterTables.length === 0 && (
            <div className="text-center py-8 text-muted-foreground">
              {tableFilter === TABLE_FILTER_EMPTY
                ? "No empty tables found"
                : "No tables without relationships"}
            </div>
          )}
          {!isSpecialFilter && filteredRelationships.length === 0 && (
            <div className="text-center py-8 text-muted-foreground">
              No relationships match the current filters
            </div>
          )}
        </CardContent>
      </Card>

      {/* Add Relationship Dialog */}
      <AddRelationshipDialog
        open={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        projectId={pid ?? ''}
        datasourceId={selectedDatasource?.datasourceId ?? ''}
        schema={schema}
        onRelationshipAdded={handleRelationshipAdded}
      />

      {/* Relationship Discovery Progress */}
      <RelationshipDiscoveryProgress
        projectId={pid ?? ''}
        datasourceId={selectedDatasource?.datasourceId ?? ''}
        isOpen={discoveryOpen}
        onClose={() => setDiscoveryOpen(false)}
        onComplete={handleDiscoveryComplete}
      />

      {/* Remove Relationship Confirmation */}
      <RemoveRelationshipDialog
        open={removeDialogOpen}
        onOpenChange={setRemoveDialogOpen}
        projectId={pid ?? ''}
        datasourceId={selectedDatasource?.datasourceId ?? ''}
        relationship={relationshipToRemove}
        onRelationshipRemoved={handleRelationshipRemoved}
      />
    </div>
  );
};

export default RelationshipsPage;
