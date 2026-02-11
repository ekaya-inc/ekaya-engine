import {
  ArrowLeft,
  ArrowRight,
  BookOpen,
  ChevronDown,
  ChevronRight,
  Loader2,
  MessageSquareWarning,
  Plus,
  Edit3,
  RefreshCw,
  Sparkles,
  Trash2,
} from "lucide-react";
import { useState, useEffect, useCallback, useRef } from "react";
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
import { ConfirmDialog } from "../components/ui/ConfirmDialog";
import { useToast } from "../hooks/useToast";
import engineApi from "../services/engineApi";
import ontologyApi from "../services/ontologyApi";
import type { GlossaryGenerationStatus, GlossaryTerm } from "../types";

const POLL_INTERVAL_MS = 3000;

/**
 * GlossaryPage - Display business glossary terms with technical mappings
 * Shows all glossary terms (discovered and user-defined) with their SQL details.
 * Supports auto-generation gated on ontology questions being answered.
 */
const GlossaryPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();

  // State for glossary terms
  const [terms, setTerms] = useState<GlossaryTerm[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Generation status from the list endpoint
  const [generationStatus, setGenerationStatus] = useState<GlossaryGenerationStatus | null>(null);

  // Question-gating state
  const [pendingRequiredQuestions, setPendingRequiredQuestions] = useState<number | null>(null);
  const [checkingQuestions, setCheckingQuestions] = useState(false);

  // Auto-generate in-flight
  const [generating, setGenerating] = useState(false);

  // Track which terms have expanded SQL details
  const [expandedTerms, setExpandedTerms] = useState<Set<string>>(new Set());

  // Editor state
  const [editorOpen, setEditorOpen] = useState(false);
  const [editingTerm, setEditingTerm] = useState<GlossaryTerm | null>(null);
  const [deletingTermId, setDeletingTermId] = useState<string | null>(null);

  // Confirmation dialog state
  const [confirmRegenerate, setConfirmRegenerate] = useState(false);
  const [termToDelete, setTermToDelete] = useState<GlossaryTerm | null>(null);

  // Polling ref
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

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

  // Fetch glossary terms (and generation_status)
  const fetchTerms = useCallback(async (silent = false): Promise<void> => {
    if (!pid) return;
    try {
      if (!silent) {
        setLoading(true);
      }
      setError(null);

      const response = await engineApi.listGlossaryTerms(pid);

      if (response.data) {
        const termsArray = response.data.terms ?? [];
        const sortedTerms = [...termsArray].sort((a, b) =>
          a.term.localeCompare(b.term)
        );
        setTerms(sortedTerms);
        if (response.data.generation_status) {
          setGenerationStatus(response.data.generation_status);
        }
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : "Failed to fetch glossary terms";
      if (!silent) {
        console.error("Failed to fetch glossary terms:", errorMessage);
        setError(errorMessage);
      }
    } finally {
      if (!silent) {
        setLoading(false);
      }
    }
  }, [pid]);

  // Fetch on mount
  useEffect(() => {
    fetchTerms();
  }, [fetchTerms]);

  // Check for pending required questions via the questions endpoint (read-only, no side effects)
  const checkPendingQuestions = useCallback(async (): Promise<void> => {
    if (!pid) return;
    setCheckingQuestions(true);
    try {
      const response = await ontologyApi.getNextQuestion(pid);
      const required = response.counts?.required ?? 0;
      setPendingRequiredQuestions(required);
    } catch {
      // If we can't check questions, assume none pending so the button is available
      setPendingRequiredQuestions(0);
    } finally {
      setCheckingQuestions(false);
    }
  }, [pid]);

  // When in empty state with idle generation, check for pending questions on mount
  useEffect(() => {
    if (
      !loading &&
      terms.length === 0 &&
      generationStatus?.status === 'idle' &&
      pendingRequiredQuestions === null &&
      !checkingQuestions
    ) {
      checkPendingQuestions();
    }
  }, [loading, terms.length, generationStatus?.status, pendingRequiredQuestions, checkingQuestions, checkPendingQuestions]);

  // Poll while generation is in progress
  const isGenerating = generationStatus?.status === 'discovering' || generationStatus?.status === 'enriching';

  useEffect(() => {
    if (isGenerating) {
      pollRef.current = setInterval(() => {
        fetchTerms(true);
      }, POLL_INTERVAL_MS);
    } else {
      if (pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
      // If generation just completed, refresh once more
      if (generating && generationStatus?.status === 'completed') {
        setGenerating(false);
        fetchTerms(true);
        toast({
          title: 'Glossary generated',
          description: generationStatus.message,
          variant: 'default',
        });
      } else if (generating && generationStatus?.status === 'failed') {
        setGenerating(false);
        toast({
          title: 'Glossary generation failed',
          description: generationStatus.error ?? generationStatus.message,
          variant: 'destructive',
        });
      }
    }

    return () => {
      if (pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
    };
  }, [isGenerating, generating, generationStatus?.status, generationStatus?.message, generationStatus?.error, fetchTerms, toast]);

  // Handle auto-generate
  const handleAutoGenerate = async (): Promise<void> => {
    if (!pid) return;
    setGenerating(true);
    try {
      await engineApi.autoGenerateGlossary(pid);
      // Poll will pick up status changes
      fetchTerms(true);
    } catch (err) {
      setGenerating(false);
      const errorMessage = err instanceof Error ? err.message : 'Failed to start glossary generation';
      toast({
        title: 'Failed to generate glossary',
        description: errorMessage,
        variant: 'destructive',
      });
    }
  };

  // Handle regenerate (with confirmation)
  const handleRegenerate = (): void => {
    setConfirmRegenerate(true);
  };

  const confirmRegenerateAction = async (): Promise<void> => {
    if (!pid) return;
    setConfirmRegenerate(false);
    setGenerating(true);
    try {
      await engineApi.autoGenerateGlossary(pid);
      fetchTerms(true);
    } catch (err) {
      setGenerating(false);
      toast({
        title: 'Failed to regenerate glossary',
        description: err instanceof Error ? err.message : 'An unexpected error occurred',
        variant: 'destructive',
      });
    }
  };

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
  const handleDeleteTerm = (term: GlossaryTerm): void => {
    setTermToDelete(term);
  };

  const confirmDeleteAction = async (): Promise<void> => {
    if (!pid || !termToDelete) return;
    const term = termToDelete;
    setTermToDelete(null);
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
          description: response.error ?? 'An error occurred while deleting the term.',
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

  // Empty state - with generation status awareness
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
            {/* Generation in progress */}
            {isGenerating ? (
              <>
                <div className="mb-4">
                  <Loader2 className="h-16 w-16 mx-auto text-cyan-500 animate-spin" />
                </div>
                <h2 className="text-xl font-semibold mb-2">Generating Glossary Terms</h2>
                <p className="text-sm text-muted-foreground mb-2">
                  {generationStatus?.message ?? 'Working...'}
                </p>
                <p className="text-xs text-text-tertiary">
                  This may take a few minutes depending on the size of your schema.
                </p>
              </>
            ) : generationStatus?.status === 'failed' ? (
              <>
                <div className="mb-4 text-destructive">
                  <svg xmlns="http://www.w3.org/2000/svg" className="h-12 w-12 mx-auto" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                  </svg>
                </div>
                <h2 className="text-xl font-semibold mb-2">Generation Failed</h2>
                <p className="text-sm text-muted-foreground mb-4">
                  {generationStatus.error ?? generationStatus.message}
                </p>
                <Button onClick={handleAutoGenerate} disabled={generating}>
                  <RefreshCw className="mr-2 h-4 w-4" />
                  Retry
                </Button>
              </>
            ) : checkingQuestions ? (
              <>
                <div className="mb-4">
                  <Loader2 className="h-16 w-16 mx-auto text-muted-foreground animate-spin" />
                </div>
                <h2 className="text-xl font-semibold mb-2">Checking readiness...</h2>
              </>
            ) : pendingRequiredQuestions !== null && pendingRequiredQuestions !== 0 ? (
              /* Required questions need answering */
              <>
                <div className="mb-4">
                  <MessageSquareWarning className="h-16 w-16 mx-auto text-amber-500" />
                </div>
                <h2 className="text-xl font-semibold mb-2">Answer Required Questions First</h2>
                <p className="text-sm text-muted-foreground mb-6">
                  {pendingRequiredQuestions > 0
                    ? `${pendingRequiredQuestions} required question${pendingRequiredQuestions === 1 ? '' : 's'} must be answered before glossary terms can be generated.`
                    : 'Required ontology questions must be answered before glossary terms can be generated.'}
                </p>
                <Button onClick={() => navigate(`/projects/${pid}/ontology-questions`)}>
                  Answer Questions
                  <ArrowRight className="ml-2 h-4 w-4" />
                </Button>
              </>
            ) : pendingRequiredQuestions === 0 && !generating ? (
              /* Ready to auto-generate */
              <>
                <div className="mb-4">
                  <BookOpen className="h-16 w-16 mx-auto text-muted-foreground" />
                </div>
                <h2 className="text-xl font-semibold mb-2">No Glossary Terms Yet</h2>
                <p className="text-sm text-muted-foreground mb-6">
                  Generate business glossary terms from your ontology. Terms will include SQL definitions and technical mappings.
                </p>
                <Button onClick={handleAutoGenerate}>
                  <Sparkles className="mr-2 h-4 w-4" />
                  Auto-Generate Terms
                </Button>
              </>
            ) : (
              /* Fallback: still loading / no info yet */
              <>
                <div className="mb-4">
                  <BookOpen className="h-16 w-16 mx-auto text-muted-foreground" />
                </div>
                <h2 className="text-xl font-semibold mb-2">No Glossary Terms Yet</h2>
                <p className="text-sm text-muted-foreground mb-6">
                  Run the ontology extraction workflow first, then generate glossary terms.
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
            <Button onClick={handleAddTerm} disabled={isGenerating}>
              <Plus className="mr-2 h-4 w-4" />
              Add Term
            </Button>
            <Button
              variant="outline"
              onClick={handleRegenerate}
              disabled={isGenerating || generating}
            >
              {isGenerating ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <RefreshCw className="mr-2 h-4 w-4" />
              )}
              Regenerate
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

      {/* Generation in-progress banner */}
      {isGenerating && (
        <Card className="mb-6 border-cyan-500/30 bg-cyan-500/5">
          <CardContent className="py-4">
            <div className="flex items-center gap-3">
              <Loader2 className="h-5 w-5 text-cyan-500 animate-spin flex-shrink-0" />
              <div>
                <p className="text-sm font-medium text-text-primary">
                  Generating glossary terms...
                </p>
                <p className="text-xs text-text-secondary">
                  {generationStatus?.message ?? 'Working...'}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

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
              const hasSqlDetails = !!term.defining_sql || !!term.base_table || (term.output_columns != null && term.output_columns.length > 0) || (term.aliases != null && term.aliases.length > 0);

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

      {/* Regenerate Confirmation */}
      <ConfirmDialog
        open={confirmRegenerate}
        onConfirm={confirmRegenerateAction}
        onCancel={() => setConfirmRegenerate(false)}
        title="Regenerate Glossary"
        description="This will regenerate all inferred glossary terms. Manually created terms will be preserved."
        confirmLabel="Regenerate"
      />

      {/* Delete Confirmation */}
      <ConfirmDialog
        open={!!termToDelete}
        onConfirm={confirmDeleteAction}
        onCancel={() => setTermToDelete(null)}
        title="Delete Term"
        description={`Are you sure you want to delete "${termToDelete?.term ?? ''}"? This action cannot be undone.`}
        confirmLabel="Delete"
        variant="destructive"
      />
    </div>
  );
};

export default GlossaryPage;
