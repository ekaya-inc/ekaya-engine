import {
  ArrowLeft,
  ArrowRight,
  Boxes,
  ChevronDown,
  ChevronRight,
  MapPin,
  Tag,
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
import engineApi from "../services/engineApi";
import type { EntityDetail } from "../types";

/**
 * EntitiesPage - Display domain entities discovered in the schema
 * Shows all entities with their aliases and occurrences (read-only).
 * Entity discovery is now handled by the unified DAG workflow on the Ontology page.
 */
const EntitiesPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();

  // State for entities data
  const [entities, setEntities] = useState<EntityDetail[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Track which entities have expanded occurrences
  const [expandedEntities, setExpandedEntities] = useState<Set<string>>(new Set());

  // Toggle expanded state for an entity
  const toggleExpanded = (entityId: string): void => {
    setExpandedEntities(prev => {
      const next = new Set(prev);
      if (next.has(entityId)) {
        next.delete(entityId);
      } else {
        next.add(entityId);
      }
      return next;
    });
  };

  // Fetch entities data
  const fetchEntities = useCallback(async (): Promise<void> => {
    if (!pid) return;
    try {
      setLoading(true);
      setError(null);

      const response = await engineApi.listEntities(pid);

      if (response.data) {
        // Filter out deleted entities for display
        const activeEntities = response.data.entities.filter(e => !e.is_deleted);
        setEntities(activeEntities);
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : "Failed to fetch data";
      console.error("Failed to fetch entities data:", errorMessage);
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  }, [pid]);

  // Fetch on mount
  useEffect(() => {
    fetchEntities();
  }, [fetchEntities]);

  // Calculate totals
  const totalOccurrences = entities.reduce((sum, e) => sum + e.occurrence_count, 0);

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
              <span className="sr-only">Loading entities...</span>
            </div>
            <p className="mt-4 text-sm text-muted-foreground">Loading entities...</p>
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
            <h2 className="text-lg font-semibold mb-2">Failed to Load Entities</h2>
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
  if (entities.length === 0) {
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
              <Boxes className="h-16 w-16 mx-auto text-muted-foreground" />
            </div>
            <h2 className="text-xl font-semibold mb-2">No Entities Discovered</h2>
            <p className="text-sm text-muted-foreground mb-6">
              No domain entities have been discovered yet. Run the ontology extraction workflow to identify domain concepts in your database schema.
            </p>
            <Button onClick={() => navigate(`/projects/${pid}/ontology`)}>
              Go to Ontology
              <ArrowRight className="ml-2 h-4 w-4" />
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
          Entities
        </h1>
        <p className="mt-2 text-text-secondary">
          Domain concepts discovered in your database schema
        </p>
      </div>

      {/* Summary Card */}
      <Card className="mb-6">
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-green-500/10">
              <Boxes className="h-5 w-5 text-green-500" />
            </div>
            <div>
              <CardTitle>Summary</CardTitle>
              <CardDescription>
                {entities.length} {entities.length === 1 ? 'entity' : 'entities'} with {totalOccurrences} total {totalOccurrences === 1 ? 'occurrence' : 'occurrences'}
              </CardDescription>
            </div>
          </div>
        </CardHeader>
      </Card>

      {/* Entities List */}
      <Card>
        <CardHeader>
          <CardTitle>Entities</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {entities.map((entity) => (
              <div key={entity.id} className="border border-border-light rounded-lg">
                {/* Entity Header */}
                <div className="p-4">
                  <div className="flex items-start justify-between">
                    <div className="flex-1 min-w-0">
                      <h3 className="font-semibold text-text-primary text-lg">
                        {entity.name}
                      </h3>
                      {entity.description && (
                        <p className="mt-1 text-sm text-text-secondary">
                          {entity.description}
                        </p>
                      )}

                      {/* Primary Location */}
                      <div className="mt-2 flex items-center gap-2 text-sm text-text-tertiary">
                        <MapPin className="h-4 w-4" />
                        <span className="font-mono">
                          {entity.primary_schema}.{entity.primary_table}.{entity.primary_column}
                        </span>
                      </div>

                      {/* Aliases */}
                      {entity.aliases.length > 0 && (
                        <div className="mt-2 flex items-center gap-2 flex-wrap">
                          <Tag className="h-4 w-4 text-text-tertiary" />
                          {entity.aliases.map((alias) => (
                            <span
                              key={alias.id}
                              className="px-2 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300"
                            >
                              {alias.alias}
                            </span>
                          ))}
                        </div>
                      )}
                    </div>

                    {/* Occurrence Count & Expand Button */}
                    {entity.occurrence_count > 1 && (
                      <button
                        onClick={() => toggleExpanded(entity.id)}
                        className="flex items-center gap-1 px-2 py-1 rounded text-sm text-text-secondary hover:bg-surface-secondary/50 transition-colors"
                      >
                        <span>{entity.occurrence_count} occurrences</span>
                        {expandedEntities.has(entity.id) ? (
                          <ChevronDown className="h-4 w-4" />
                        ) : (
                          <ChevronRight className="h-4 w-4" />
                        )}
                      </button>
                    )}
                    {entity.occurrence_count === 1 && (
                      <span className="text-sm text-text-tertiary">
                        1 occurrence
                      </span>
                    )}
                  </div>
                </div>

                {/* Expanded Occurrences */}
                {expandedEntities.has(entity.id) && entity.occurrences.length > 0 && (
                  <div className="border-t border-border-light bg-surface-secondary/30">
                    <div className="p-3">
                      <div className="text-xs font-medium text-text-tertiary mb-2">
                        Occurrences
                      </div>
                      <div className="space-y-2">
                        {entity.occurrences.map((occ) => (
                          <div
                            key={occ.id}
                            className="flex items-center justify-between text-sm"
                          >
                            <span className="font-mono text-text-primary">
                              {occ.schema_name}.{occ.table_name}.{occ.column_name}
                            </span>
                            <div className="flex items-center gap-2">
                              {occ.role && (
                                <span className="px-2 py-0.5 rounded-full text-xs bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400">
                                  {occ.role}
                                </span>
                              )}
                              <span className="text-xs text-text-tertiary">
                                {Math.round(occ.confidence * 100)}% confidence
                              </span>
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
};

export default EntitiesPage;
