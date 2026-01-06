import {
  ArrowLeft,
  ArrowRight,
  BookOpen,
  ChevronDown,
  ChevronRight,
  Plus,
  Edit3,
  Trash2,
} from "lucide-react";
import { useState, useEffect, useCallback } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { GlossaryTermEditor } from "../components/GlossaryTermEditor";
import { Button } from "../components/ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../components/ui/Card";
import { useToast } from "../hooks/useToast";
import engineApi from "../services/engineApi";
import ontologyService from "../services/ontologyService";
import type { GlossaryTerm, OntologyWorkflowStatus } from "../types";

/**
 * GlossaryPage - Display business glossary terms with technical mappings
 * Shows all glossary terms (discovered and user-defined) with their SQL details.
 * Glossary discovery is handled by the unified DAG workflow on the Ontology page.
 */
const GlossaryPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();

  // State for glossary terms
  const [terms, setTerms] = useState<GlossaryTerm[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // State for ontology status (to check if extraction has been run)
  const [ontologyStatus, setOntologyStatus] = useState<OntologyWorkflowStatus | null>(null);

  // Track which terms have expanded SQL details
  const [expandedTerms, setExpandedTerms] = useState<Set<string>>(new Set());

  // Editor state
  const [editorOpen, setEditorOpen] = useState(false);
  const [editingTerm, setEditingTerm] = useState<GlossaryTerm | null>(null);
  const [deletingTermId, setDeletingTermId] = useState<string | null>(null);

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
        const termsArray = response.data.terms ?? [];
        const sortedTerms = [...termsArray].sort((a, b) =>
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

  // Handle add term
  const handleAddTerm = (): void => {
    setEditingTerm(null);
    setEditorOpen(true);
  };

  // Handle edit term
  const handleEditTerm = (term: GlossaryTerm): void => {
    setEditingTerm(term);
    setEditorOpen(true);
  };

  // Handle delete term
  const handleDeleteTerm = async (term: GlossaryTerm): Promise<void> => {
    if (!pid) return;

    const confirmed = window.confirm(
      `Are you sure you want to delete the term "${term.term}"? This action cannot be undone.`
    );

    if (!confirmed) return;

    setDeletingTermId(term.id);

    try {
      const response = await engineApi.deleteGlossaryTerm(pid, term.id);

      if (response.success) {
        toast({
          title: 'Term deleted',
          description: `"${term.term}" has been removed from the glossary.`,
          variant: 'default',
        });
        fetchTerms();
      } else {
        toast({
          title: 'Failed to delete term',
          description: response.error || 'An error occurred while deleting the term.',
          variant: 'destructive',
        });
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to delete term';
      toast({
        title: 'Failed to delete term',
        description: errorMessage,
        variant: 'destructive',
      });
    } finally {
      setDeletingTermId(null);
    }
  };

  // Handle save from editor
  const handleEditorSave = (): void => {
    toast({
      title: editingTerm ? 'Term updated' : 'Term created',
      description: editingTerm
        ? 'The glossary term has been successfully updated.'
        : 'The new glossary term has been added.',
      variant: 'default',
    });
    fetchTerms();
  };

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
          <div className="flex items-center gap-2">
            <Button onClick={handleAddTerm}>
              <Plus className="mr-2 h-4 w-4" />
              Add Term
            </Button>
            <Button variant="outline" onClick={() => navigate(`/projects/${pid}/ontology`)}>
              Go to Ontology
              <ArrowRight className="ml-2 h-4 w-4" />
            </Button>
          </div>
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
              const hasSqlDetails = term.defining_sql || term.base_table || (term.output_columns && term.output_columns.length > 0) || (term.aliases && term.aliases.length > 0);

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
                            term.source === 'inferred'
                              ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
                              : term.source === 'manual'
                              ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
                              : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
                          }`}>
                            {term.source === 'inferred' ? 'Inferred' : term.source === 'manual' ? 'Manual' : 'Client'}
                          </span>
                        </div>
                        <p className="mt-1 text-sm text-text-secondary">
                          {term.definition}
                        </p>
                      </div>

                      <div className="flex items-center gap-2 ml-2">
                        {/* Action Buttons */}
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleEditTerm(term)}
                          disabled={deletingTermId === term.id}
                        >
                          <Edit3 className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleDeleteTerm(term)}
                          disabled={deletingTermId === term.id}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>

                        {/* Expand SQL Details Button */}
                        {hasSqlDetails && (
                          <button
                            onClick={() => toggleExpanded(term.id)}
                            className="flex items-center gap-1 px-2 py-1 rounded text-sm text-text-secondary hover:bg-surface-secondary/50 transition-colors"
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
                  </div>

                  {/* Expanded SQL Details */}
                  {expandedTerms.has(term.id) && hasSqlDetails && (
                    <div className="border-t border-border-light bg-surface-secondary/30">
                      <div className="p-4 space-y-3">
                        {term.defining_sql && (
                          <div>
                            <div className="text-xs font-medium text-text-tertiary mb-1">
                              Defining SQL
                            </div>
                            <pre className="text-sm font-mono bg-surface-primary border border-border-light rounded p-2 overflow-x-auto">
                              {term.defining_sql}
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

                        {term.output_columns && term.output_columns.length > 0 && (
                          <div>
                            <div className="text-xs font-medium text-text-tertiary mb-1">
                              Output Columns
                            </div>
                            <div className="space-y-1">
                              {term.output_columns.map((col, idx) => (
                                <div key={idx} className="text-sm">
                                  <span className="font-mono text-text-primary">{col.name}</span>
                                  <span className="text-text-tertiary"> ({col.type})</span>
                                  {col.description && (
                                    <span className="text-text-secondary"> - {col.description}</span>
                                  )}
                                </div>
                              ))}
                            </div>
                          </div>
                        )}

                        {term.aliases && term.aliases.length > 0 && (
                          <div>
                            <div className="text-xs font-medium text-text-tertiary mb-1">
                              Aliases
                            </div>
                            <div className="flex flex-wrap gap-1">
                              {term.aliases.map((alias, idx) => (
                                <span
                                  key={idx}
                                  className="px-2 py-0.5 rounded text-xs font-mono bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300"
                                >
                                  {alias}
                                </span>
                              ))}
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

      {/* Glossary Term Editor Modal */}
      {pid && (
        <GlossaryTermEditor
          projectId={pid}
          term={editingTerm}
          isOpen={editorOpen}
          onClose={() => setEditorOpen(false)}
          onSave={handleEditorSave}
        />
      )}
    </div>
  );
};

export default GlossaryPage;
