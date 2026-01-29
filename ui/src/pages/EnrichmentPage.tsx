import {
  ArrowLeft,
  ArrowRight,
  ChevronDown,
  ChevronRight,
  Key,
  Link2,
  Sparkles,
} from "lucide-react";
import { useState, useEffect, useCallback } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { Button } from "../components/ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../components/ui/Card";
import ontologyApi from "../services/ontologyApi";
import ontologyService from "../services/ontologyService";
import type {
  ColumnDetail,
  EnrichmentResponse,
  EntitySummary,
  OntologyWorkflowStatus,
} from "../types";

/**
 * EnrichmentPage - Display semantic enrichment for tables and columns
 * Shows column and table enrichment data from the TieredOntology.
 * Provides visibility into what semantic information has been extracted
 * (descriptions, types, roles, enums, FK associations).
 */
const EnrichmentPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();

  // State for enrichment data
  const [enrichment, setEnrichment] = useState<EnrichmentResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // State for ontology status (to check if extraction has been run)
  const [ontologyStatus, setOntologyStatus] = useState<OntologyWorkflowStatus | null>(null);

  // Track which tables are expanded
  const [expandedTables, setExpandedTables] = useState<Set<string>>(new Set());

  // Toggle expanded state for a table
  const toggleExpanded = (tableName: string): void => {
    setExpandedTables(prev => {
      const next = new Set(prev);
      if (next.has(tableName)) {
        next.delete(tableName);
      } else {
        next.add(tableName);
      }
      return next;
    });
  };

  // Subscribe to ontology status updates
  useEffect(() => {
    if (!pid) return;

    ontologyService.setProjectId(pid);
    const unsubscribe = ontologyService.subscribe((status) => {
      setOntologyStatus(status);
    });

    return () => {
      unsubscribe();
    };
  }, [pid]);

  // Fetch enrichment data
  const fetchEnrichment = useCallback(async (): Promise<void> => {
    if (!pid) return;
    try {
      setLoading(true);
      setError(null);

      const response = await ontologyApi.getEnrichment(pid);
      setEnrichment(response);
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : "Failed to fetch enrichment data";
      console.error("Failed to fetch enrichment data:", errorMessage);
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  }, [pid]);

  // Fetch on mount
  useEffect(() => {
    fetchEnrichment();
  }, [fetchEnrichment]);

  // Helper: Get badge color for semantic type
  const getSemanticTypeBadgeColor = (semanticType: string): string => {
    const typeColors: Record<string, string> = {
      identifier: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
      timestamp: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
      date: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
      datetime: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
      currency: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
      money: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
      percentage: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
      percent: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
      email: 'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-300',
      url: 'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-300',
      phone: 'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-300',
      boolean: 'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-300',
      status: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300',
      enum: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300',
    };
    return typeColors[semanticType.toLowerCase()] ?? 'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-300';
  };

  // Helper: Get badge color for role
  const getRoleBadgeColor = (role: string): string => {
    const roleColors: Record<string, string> = {
      dimension: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
      measure: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
      identifier: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
      attribute: 'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-300',
      key: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
      foreign_key: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300',
    };
    return roleColors[role.toLowerCase()] ?? 'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-300';
  };

  // Check if ontology is complete
  const isOntologyComplete = ontologyStatus?.progress.state === 'complete'
    || ontologyStatus?.ontologyReady === true;

  // Get entity summaries and column details
  const entitySummaries = enrichment?.entity_summaries ?? [];
  const columnDetailsMap = new Map<string, ColumnDetail[]>();
  for (const ec of enrichment?.column_details ?? []) {
    columnDetailsMap.set(ec.table_name, ec.columns);
  }

  // Calculate summary statistics
  const tableCount = entitySummaries.length;
  const columnCount = (enrichment?.column_details ?? []).reduce(
    (acc, ec) => acc + ec.columns.length,
    0
  );

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
              <span className="sr-only">Loading enrichment data...</span>
            </div>
            <p className="mt-4 text-sm text-muted-foreground">Loading enrichment data...</p>
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
            <h2 className="text-lg font-semibold mb-2">Failed to Load Enrichment Data</h2>
            <p className="text-sm text-muted-foreground mb-4">
              {error}
            </p>
            <button
              onClick={() => fetchEnrichment()}
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
  if (!enrichment || entitySummaries.length === 0) {
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
              <Sparkles className="h-16 w-16 mx-auto text-muted-foreground" />
            </div>
            {!isOntologyComplete ? (
              <>
                <h2 className="text-xl font-semibold mb-2">Run Ontology Extraction First</h2>
                <p className="text-sm text-muted-foreground mb-6">
                  No enrichment data available yet. Run the ontology extraction workflow to generate semantic information about your tables and columns.
                </p>
                <Button onClick={() => navigate(`/projects/${pid}/ontology`)}>
                  Go to Ontology
                  <ArrowRight className="ml-2 h-4 w-4" />
                </Button>
              </>
            ) : (
              <>
                <h2 className="text-xl font-semibold mb-2">No Enrichment Data Available</h2>
                <p className="text-sm text-muted-foreground mb-6">
                  The ontology extraction has completed, but no enrichment data was generated. This may indicate an issue with the extraction process.
                </p>
                <Button onClick={() => navigate(`/projects/${pid}/ontology`)}>
                  Go to Ontology
                  <ArrowRight className="ml-2 h-4 w-4" />
                </Button>
              </>
            )}
          </div>
        </div>
      </div>
    );
  }

  // Render column detail
  const renderColumnDetail = (column: ColumnDetail) => (
    <div key={column.name} className="border-b border-border-light last:border-b-0 py-3 px-4">
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          {/* Column name and badges */}
          <div className="flex items-center gap-2 flex-wrap mb-1">
            <span className="font-mono text-sm font-medium text-text-primary">
              {column.name}
            </span>
            {column.semantic_type && (
              <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${getSemanticTypeBadgeColor(column.semantic_type)}`}>
                {column.semantic_type}
              </span>
            )}
            {column.role && (
              <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${getRoleBadgeColor(column.role)}`}>
                {column.role}
              </span>
            )}
            {column.is_primary_key && (
              <span className="flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
                <Key className="h-3 w-3" />
                PK
              </span>
            )}
            {column.is_foreign_key && (
              <span className="flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300">
                <Link2 className="h-3 w-3" />
                FK
              </span>
            )}
          </div>

          {/* Description */}
          {column.description && (
            <p className="text-sm text-text-secondary mb-2">
              {column.description}
            </p>
          )}

          {/* Additional details */}
          <div className="space-y-1">
            {/* Foreign key reference */}
            {column.is_foreign_key && column.foreign_table && (
              <div className="text-xs text-text-tertiary">
                <span className="font-medium">References:</span>{' '}
                <span className="font-mono">{column.foreign_table}</span>
              </div>
            )}

            {/* Synonyms */}
            {column.synonyms && column.synonyms.length > 0 && (
              <div className="flex items-center gap-1 flex-wrap">
                <span className="text-xs font-medium text-text-tertiary">Synonyms:</span>
                {column.synonyms.map((synonym, idx) => (
                  <span
                    key={idx}
                    className="px-1.5 py-0.5 rounded text-xs bg-surface-secondary text-text-secondary"
                  >
                    {synonym}
                  </span>
                ))}
              </div>
            )}

            {/* Enum values */}
            {column.enum_values && column.enum_values.length > 0 && (
              <div className="mt-2">
                <span className="text-xs font-medium text-text-tertiary">Values:</span>
                <div className="flex items-center gap-1 flex-wrap mt-1">
                  {column.enum_values.map((ev, idx) => (
                    <span
                      key={idx}
                      className="px-2 py-0.5 rounded text-xs bg-indigo-50 text-indigo-700 dark:bg-indigo-900/20 dark:text-indigo-300"
                      title={ev.meaning}
                    >
                      {ev.value}
                      {ev.meaning && <span className="text-text-tertiary ml-1">({ev.meaning})</span>}
                    </span>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );

  // Render table row
  const renderTableRow = (entity: EntitySummary) => {
    const columns = columnDetailsMap.get(entity.table_name) ?? [];
    const isExpanded = expandedTables.has(entity.table_name);

    return (
      <div key={entity.table_name} className="border border-border-light rounded-lg mb-3 last:mb-0">
        {/* Table Header */}
        <button
          onClick={() => toggleExpanded(entity.table_name)}
          className="w-full p-4 flex items-center justify-between hover:bg-surface-secondary/30 transition-colors rounded-t-lg"
        >
          <div className="flex items-center gap-3">
            {isExpanded ? (
              <ChevronDown className="h-5 w-5 text-text-tertiary" />
            ) : (
              <ChevronRight className="h-5 w-5 text-text-tertiary" />
            )}
            <div className="text-left">
              <div className="flex items-center gap-2">
                <span className="font-mono font-medium text-text-primary">
                  {entity.table_name}
                </span>
                {entity.domain && (
                  <span className="px-2 py-0.5 rounded-full text-xs font-medium bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300">
                    {entity.domain}
                  </span>
                )}
              </div>
              {entity.business_name && entity.business_name !== entity.table_name && (
                <div className="text-sm text-text-secondary mt-0.5">
                  {entity.business_name}
                </div>
              )}
            </div>
          </div>
          <div className="text-sm text-text-tertiary">
            {columns.length} {columns.length === 1 ? 'column' : 'columns'}
          </div>
        </button>

        {/* Table Description (if expanded) */}
        {isExpanded && entity.description && (
          <div className="px-4 pb-3 border-b border-border-light bg-surface-secondary/20">
            <p className="text-sm text-text-secondary pl-8">
              {entity.description}
            </p>
            {entity.key_columns && entity.key_columns.length > 0 && (
              <div className="mt-2 pl-8">
                <span className="text-xs font-medium text-text-tertiary">Key Columns: </span>
                {entity.key_columns.map((kc, idx) => (
                  <span key={idx} className="text-xs text-text-secondary">
                    {kc.name}
                    {idx < entity.key_columns.length - 1 && ', '}
                  </span>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Column Details (if expanded) */}
        {isExpanded && columns.length > 0 && (
          <div className="bg-surface-secondary/10">
            {columns.map(renderColumnDetail)}
          </div>
        )}

        {/* Empty columns message */}
        {isExpanded && columns.length === 0 && (
          <div className="px-4 py-6 text-center text-sm text-text-tertiary bg-surface-secondary/10">
            No column enrichment data available for this table.
          </div>
        )}
      </div>
    );
  };

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
          <Button variant="outline" onClick={() => navigate(`/projects/${pid}/ontology`)}>
            Go to Ontology
            <ArrowRight className="ml-2 h-4 w-4" />
          </Button>
        </div>
        <h1 className="text-3xl font-bold text-text-primary">
          Enrichment
        </h1>
        <p className="mt-2 text-text-secondary">
          View semantic enrichment for tables and columns
        </p>
      </div>

      {/* Summary Card */}
      <Card className="mb-6">
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-orange-500/10">
              <Sparkles className="h-5 w-5 text-orange-500" />
            </div>
            <div>
              <CardTitle>Summary</CardTitle>
              <CardDescription>
                {tableCount} {tableCount === 1 ? 'table' : 'tables'}, {columnCount} {columnCount === 1 ? 'column' : 'columns'} enriched
              </CardDescription>
            </div>
          </div>
        </CardHeader>
      </Card>

      {/* Tables List */}
      <Card>
        <CardHeader>
          <CardTitle>Tables</CardTitle>
          <CardDescription>
            Click a table to view column enrichment details
          </CardDescription>
        </CardHeader>
        <CardContent>
          {entitySummaries.map(renderTableRow)}
        </CardContent>
      </Card>
    </div>
  );
};

export default EnrichmentPage;
