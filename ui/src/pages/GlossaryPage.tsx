import {
  ArrowLeft,
  ArrowRight,
  BookOpen,
  ChevronDown,
  ChevronRight,
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
import ontologyService from "../services/ontologyService";
import type { BusinessGlossaryTerm, OntologyWorkflowStatus } from "../types";

/**
 * GlossaryPage - Display business glossary terms with technical mappings
 * Shows all glossary terms (discovered and user-defined) with their SQL details.
 * Glossary discovery is handled by the unified DAG workflow on the Ontology page.
 */
const GlossaryPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();

  // State for glossary terms
  const [terms, setTerms] = useState<BusinessGlossaryTerm[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // State for ontology status (to check if extraction has been run)
  const [ontologyStatus, setOntologyStatus] = useState<OntologyWorkflowStatus | null>(null);

  // Track which terms have expanded SQL details
  const [expandedTerms, setExpandedTerms] = useState<Set<string>>(new Set());

  // Toggle expanded state for a term
  const toggleExpanded = (termId: string): void => {
    setExpandedTerms(prev => {
      const next = new Set(prev);
      if (next.has(termId)) {
        next.delete(termId);
      } else {
        next.add(termId);
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

  // Fetch glossary terms
  const fetchTerms = useCallback(async (): Promise<void> => {
    if (!pid) return;
    try {
      setLoading(true);
      setError(null);

      const response = await engineApi.listGlossaryTerms(pid);

      if (response.data) {
        // Sort terms alphabetically by term field
        const sortedTerms = [...response.data.terms].sort((a, b) =>
          a.term.localeCompare(b.term)
        );
        setTerms(sortedTerms);
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : "Failed to fetch glossary terms";
      console.error("Failed to fetch glossary terms:", errorMessage);
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  }, [pid]);

  // Fetch on mount
  useEffect(() => {
    fetchTerms();
  }, [fetchTerms]);

  // Check if ontology is complete
  const isOntologyComplete = ontologyStatus?.progress.state === 'complete'
    || ontologyStatus?.ontologyReady === true;

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
              <span className="sr-only">Loading glossary terms...</span>
            </div>
            <p className="mt-4 text-sm text-muted-foreground">Loading glossary terms...</p>
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
            <h2 className="text-lg font-semibold mb-2">Failed to Load Glossary</h2>
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
  if (terms.length === 0) {
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
              <BookOpen className="h-16 w-16 mx-auto text-muted-foreground" />
            </div>
            {!isOntologyComplete ? (
              <>
                <h2 className="text-xl font-semibold mb-2">Run Ontology Extraction First</h2>
                <p className="text-sm text-muted-foreground mb-6">
                  No glossary terms have been discovered yet. Run the ontology extraction workflow to identify business terms in your database schema.
                </p>
                <Button onClick={() => navigate(`/projects/${pid}/ontology`)}>
                  Go to Ontology
                  <ArrowRight className="ml-2 h-4 w-4" />
                </Button>
              </>
            ) : (
              <>
                <h2 className="text-xl font-semibold mb-2">No Glossary Terms Discovered Yet</h2>
                <p className="text-sm text-muted-foreground mb-6">
                  The ontology extraction has completed, but no business glossary terms were discovered in your database schema.
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
          Glossary
        </h1>
        <p className="mt-2 text-text-secondary">
          Business terms with their technical mappings
        </p>
      </div>

      {/* Summary Card */}
      <Card className="mb-6">
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-cyan-500/10">
              <BookOpen className="h-5 w-5 text-cyan-500" />
            </div>
            <div>
              <CardTitle>Summary</CardTitle>
              <CardDescription>
                {terms.length} {terms.length === 1 ? 'term' : 'terms'}
              </CardDescription>
            </div>
          </div>
        </CardHeader>
      </Card>

      {/* Terms List */}
      <Card>
        <CardHeader>
          <CardTitle>Terms</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {terms.map((term) => {
              const hasSqlDetails = term.sql_pattern || term.base_table || term.columns_used || term.filters || term.aggregation;

              return (
                <div key={term.id} className="border border-border-light rounded-lg">
                  {/* Term Header */}
                  <div className="p-4">
                    <div className="flex items-start justify-between">
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 mb-1">
                          <h3 className="font-semibold text-text-primary text-lg">
                            {term.term}
                          </h3>
                          {/* Source Badge */}
                          <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${
                            term.source === 'suggested'
                              ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
                              : 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
                          }`}>
                            {term.source === 'suggested' ? 'Suggested' : 'User'}
                          </span>
                        </div>
                        <p className="mt-1 text-sm text-text-secondary">
                          {term.definition}
                        </p>
                      </div>

                      {/* Expand SQL Details Button */}
                      {hasSqlDetails && (
                        <button
                          onClick={() => toggleExpanded(term.id)}
                          className="flex items-center gap-1 px-2 py-1 rounded text-sm text-text-secondary hover:bg-surface-secondary/50 transition-colors ml-2"
                        >
                          <span>SQL Details</span>
                          {expandedTerms.has(term.id) ? (
                            <ChevronDown className="h-4 w-4" />
                          ) : (
                            <ChevronRight className="h-4 w-4" />
                          )}
                        </button>
                      )}
                    </div>
                  </div>

                  {/* Expanded SQL Details */}
                  {expandedTerms.has(term.id) && hasSqlDetails && (
                    <div className="border-t border-border-light bg-surface-secondary/30">
                      <div className="p-4 space-y-3">
                        {term.sql_pattern && (
                          <div>
                            <div className="text-xs font-medium text-text-tertiary mb-1">
                              SQL Pattern
                            </div>
                            <pre className="text-sm font-mono bg-surface-primary border border-border-light rounded p-2 overflow-x-auto">
                              {term.sql_pattern}
                            </pre>
                          </div>
                        )}

                        {term.base_table && (
                          <div>
                            <div className="text-xs font-medium text-text-tertiary mb-1">
                              Base Table
                            </div>
                            <div className="text-sm font-mono text-text-primary">
                              {term.base_table}
                            </div>
                          </div>
                        )}

                        {term.columns_used && term.columns_used.length > 0 && (
                          <div>
                            <div className="text-xs font-medium text-text-tertiary mb-1">
                              Columns Used
                            </div>
                            <div className="flex flex-wrap gap-1">
                              {term.columns_used.map((col, idx) => (
                                <span
                                  key={idx}
                                  className="px-2 py-0.5 rounded text-xs font-mono bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300"
                                >
                                  {col}
                                </span>
                              ))}
                            </div>
                          </div>
                        )}

                        {term.filters && term.filters.length > 0 && (
                          <div>
                            <div className="text-xs font-medium text-text-tertiary mb-1">
                              Filters
                            </div>
                            <div className="space-y-1">
                              {term.filters.map((filter, idx) => (
                                <div key={idx} className="text-sm font-mono text-text-primary">
                                  {filter.column} {filter.operator} {filter.values.join(', ')}
                                </div>
                              ))}
                            </div>
                          </div>
                        )}

                        {term.aggregation && (
                          <div>
                            <div className="text-xs font-medium text-text-tertiary mb-1">
                              Aggregation
                            </div>
                            <div className="text-sm font-mono text-text-primary">
                              {term.aggregation}
                            </div>
                          </div>
                        )}
                      </div>
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

export default GlossaryPage;
