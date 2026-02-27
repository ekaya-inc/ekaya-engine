import {
  AlertCircle,
  ArrowLeft,
  ArrowRight,
  Brain,
  Loader2,
  Plus,
  Edit3,
  Trash2,
} from "lucide-react";
import { useState, useEffect, useCallback } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { ProjectKnowledgeEditor } from "../components/ProjectKnowledgeEditor";
import { Button } from "../components/ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../components/ui/Card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../components/ui/Dialog";
import { Input } from "../components/ui/Input";
import { useToast } from "../hooks/useToast";
import engineApi from "../services/engineApi";
import ontologyService from "../services/ontologyService";
import type { ProjectKnowledge, OntologyWorkflowStatus } from "../types";

/**
 * Badge colors for fact types
 */
const FACT_TYPE_COLORS: Record<string, string> = {
  business_rule: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
  convention: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
  domain_term: 'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-300',
  relationship: 'bg-pink-100 text-pink-700 dark:bg-pink-900/30 dark:text-pink-300',
};

/**
 * Badge colors for source types
 */
const SOURCE_COLORS: Record<string, string> = {
  inference: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  inferred: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  manual: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
  mcp: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
};

/**
 * Get display name for source
 */
function getSourceDisplayName(source: string): string {
  switch (source) {
    case 'inference':
    case 'inferred':
      return 'Inferred';
    case 'manual':
      return 'Manual';
    case 'mcp':
      return 'Client';
    default:
      return source;
  }
}

/**
 * Get display name for fact type
 */
function getFactTypeDisplayName(factType: string): string {
  return factType
    .split('_')
    .map(word => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');
}

/**
 * ProjectKnowledgePage - Display project knowledge facts
 * Shows domain facts and business rules learned during ontology refinement.
 */
const ProjectKnowledgePage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();

  // State for knowledge facts
  const [facts, setFacts] = useState<ProjectKnowledge[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // State for ontology status (to check if extraction has been run)
  const [ontologyStatus, setOntologyStatus] = useState<OntologyWorkflowStatus | null>(null);

  // Editor state
  const [editorOpen, setEditorOpen] = useState(false);
  const [editingFact, setEditingFact] = useState<ProjectKnowledge | null>(null);
  const [deletingFactId, setDeletingFactId] = useState<string | null>(null);

  // Delete all state
  const [showDeleteAllDialog, setShowDeleteAllDialog] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');
  const [isDeletingAll, setIsDeletingAll] = useState(false);

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

  // Fetch project knowledge facts
  const fetchFacts = useCallback(async (): Promise<void> => {
    if (!pid) return;
    try {
      setLoading(true);
      setError(null);

      const response = await engineApi.listProjectKnowledge(pid);

      if (response.data) {
        // Sort facts alphabetically by value
        const factsArray = response.data.facts ?? [];
        const sortedFacts = [...factsArray].sort((a, b) =>
          a.value.localeCompare(b.value)
        );
        setFacts(sortedFacts);
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : "Failed to fetch project knowledge";
      console.error("Failed to fetch project knowledge:", errorMessage);
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  }, [pid]);

  // Fetch on mount
  useEffect(() => {
    fetchFacts();
  }, [fetchFacts]);

  // Handle add fact
  const handleAddFact = (): void => {
    setEditingFact(null);
    setEditorOpen(true);
  };

  // Handle edit fact
  const handleEditFact = (fact: ProjectKnowledge): void => {
    setEditingFact(fact);
    setEditorOpen(true);
  };

  // Handle delete fact
  const handleDeleteFact = async (fact: ProjectKnowledge): Promise<void> => {
    if (!pid) return;

    setDeletingFactId(fact.id);

    try {
      const response = await engineApi.deleteProjectKnowledge(pid, fact.id);

      if (response.success) {
        toast({
          title: 'Fact deleted',
          description: 'The fact has been removed.',
          variant: 'default',
        });
        fetchFacts();
      } else {
        toast({
          title: 'Failed to delete fact',
          description: response.error ?? 'An error occurred while deleting the fact.',
          variant: 'destructive',
        });
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to delete fact';
      toast({
        title: 'Failed to delete fact',
        description: errorMessage,
        variant: 'destructive',
      });
    } finally {
      setDeletingFactId(null);
    }
  };

  // Handle save from editor
  const handleEditorSave = (): void => {
    toast({
      title: editingFact ? 'Fact updated' : 'Fact created',
      description: editingFact
        ? 'The project knowledge fact has been successfully updated.'
        : 'The new project knowledge fact has been added.',
      variant: 'default',
    });
    fetchFacts();
  };

  // Handle delete all
  const handleDeleteAll = async (): Promise<void> => {
    if (!pid) return;

    setIsDeletingAll(true);

    try {
      const response = await engineApi.deleteAllProjectKnowledge(pid);

      if (response.success) {
        toast({
          title: 'All facts deleted',
          description: 'All project knowledge facts have been removed.',
          variant: 'default',
        });
        setShowDeleteAllDialog(false);
        setDeleteConfirmText('');
        fetchFacts();
      } else {
        toast({
          title: 'Failed to delete facts',
          description: response.error ?? 'An error occurred while deleting the facts.',
          variant: 'destructive',
        });
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to delete facts';
      toast({
        title: 'Failed to delete facts',
        description: errorMessage,
        variant: 'destructive',
      });
    } finally {
      setIsDeletingAll(false);
    }
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
              <span className="sr-only">Loading project knowledge...</span>
            </div>
            <p className="mt-4 text-sm text-muted-foreground">Loading project knowledge...</p>
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
            <h2 className="text-lg font-semibold mb-2">Failed to Load Project Knowledge</h2>
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
  if (facts.length === 0) {
    return (
      <div className="mx-auto max-w-6xl">
        <div className="mb-6 flex items-center justify-between">
          <Button variant="ghost" onClick={() => navigate(`/projects/${pid}`)}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
          <Button onClick={handleAddFact}>
            <Plus className="mr-2 h-4 w-4" />
            Add Fact
          </Button>
        </div>
        <div className="flex items-center justify-center min-h-[400px]">
          <div className="text-center max-w-md p-6">
            <div className="mb-4">
              <Brain className="h-16 w-16 mx-auto text-muted-foreground" />
            </div>
            {!isOntologyComplete ? (
              <>
                <h2 className="text-xl font-semibold mb-2">Run Ontology Extraction First</h2>
                <p className="text-sm text-muted-foreground mb-6">
                  No project knowledge has been discovered yet. Run the ontology extraction workflow to identify domain facts in your database schema.
                </p>
                <Button onClick={() => navigate(`/projects/${pid}/ontology`)}>
                  Go to Ontology
                  <ArrowRight className="ml-2 h-4 w-4" />
                </Button>
              </>
            ) : (
              <>
                <h2 className="text-xl font-semibold mb-2">No Project Knowledge Yet</h2>
                <p className="text-sm text-muted-foreground mb-6">
                  No domain facts have been discovered or added yet. You can add facts manually or they will be learned during ontology refinement.
                </p>
                <Button onClick={handleAddFact}>
                  <Plus className="mr-2 h-4 w-4" />
                  Add Your First Fact
                </Button>
              </>
            )}
          </div>
        </div>

        {/* Editor Modal */}
        {pid && (
          <ProjectKnowledgeEditor
            projectId={pid}
            fact={editingFact}
            isOpen={editorOpen}
            onClose={() => setEditorOpen(false)}
            onSave={handleEditorSave}
            onProcessing={() => toast({ title: 'Processing fact...', description: 'The fact is being analyzed and will appear shortly.' })}
            onError={(message) => toast({ title: 'Failed to add fact', description: message, variant: 'destructive' })}
          />
        )}
      </div>
    );
  }

  // Group facts by fact_type for display
  const factsByType = facts.reduce((acc, fact) => {
    const type = fact.fact_type ?? 'other';
    acc[type] ??= [];
    acc[type].push(fact);
    return acc;
  }, {} as Record<string, ProjectKnowledge[]>);

  // Sort the types for consistent display
  const sortedTypes = Object.keys(factsByType).sort();

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
            <Button onClick={handleAddFact}>
              <Plus className="mr-2 h-4 w-4" />
              Add Fact
            </Button>
            {facts.length > 0 && (
              <Button
                variant="outline"
                className="text-red-600 hover:text-red-700 hover:bg-red-50"
                onClick={() => setShowDeleteAllDialog(true)}
              >
                <Trash2 className="mr-2 h-4 w-4" />
                Delete All
              </Button>
            )}
            <Button variant="outline" onClick={() => navigate(`/projects/${pid}/ontology`)}>
              Go to Ontology
              <ArrowRight className="ml-2 h-4 w-4" />
            </Button>
          </div>
        </div>
        <h1 className="text-3xl font-bold text-text-primary">
          Project Knowledge
        </h1>
        <p className="mt-2 text-text-secondary">
          Domain facts and business rules learned during ontology refinement
        </p>
      </div>

      {/* Summary Card */}
      <Card className="mb-6">
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-cyan-500/10">
              <Brain className="h-5 w-5 text-cyan-500" />
            </div>
            <div>
              <CardTitle>Summary</CardTitle>
              <CardDescription>
                {facts.length} {facts.length === 1 ? 'fact' : 'facts'} across {sortedTypes.length} {sortedTypes.length === 1 ? 'category' : 'categories'}
              </CardDescription>
            </div>
          </div>
        </CardHeader>
      </Card>

      {/* Facts List - Grouped by Type */}
      {sortedTypes.map((factType) => {
        const typeFacts = factsByType[factType] ?? [];
        return (
        <Card key={factType} className="mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${FACT_TYPE_COLORS[factType] ?? 'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-300'}`}>
                {getFactTypeDisplayName(factType)}
              </span>
              <span className="text-sm font-normal text-text-tertiary">
                ({typeFacts.length})
              </span>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {typeFacts.map((fact) => (
                <div key={fact.id} className="border border-border-light rounded-lg p-4">
                  <div className="flex items-start justify-between">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-2">
                        {/* Source Badge */}
                        <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${SOURCE_COLORS[fact.source] ?? 'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-300'}`}>
                          {getSourceDisplayName(fact.source)}
                        </span>
                      </div>

                      {/* Fact Value */}
                      <p className="text-sm text-text-primary mb-2">
                        {fact.value}
                      </p>

                      {/* Context (if present) */}
                      {fact.context && (
                        <p className="text-xs text-text-tertiary italic">
                          Context: {fact.context}
                        </p>
                      )}
                    </div>

                    <div className="flex items-center gap-2 ml-4">
                      {/* Action Buttons */}
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleEditFact(fact)}
                        disabled={deletingFactId === fact.id}
                      >
                        <Edit3 className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleDeleteFact(fact)}
                        disabled={deletingFactId === fact.id}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
        );
      })}

      {/* Project Knowledge Editor Modal */}
      {pid && (
        <ProjectKnowledgeEditor
          projectId={pid}
          fact={editingFact}
          isOpen={editorOpen}
          onClose={() => setEditorOpen(false)}
          onSave={handleEditorSave}
          onProcessing={() => toast({ title: 'Processing fact...', description: 'The fact is being analyzed and will appear shortly.' })}
          onError={(message) => toast({ title: 'Failed to add fact', description: message, variant: 'destructive' })}
        />
      )}

      {/* Delete All Confirmation Dialog */}
      <Dialog open={showDeleteAllDialog} onOpenChange={setShowDeleteAllDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Project Knowledge?</DialogTitle>
            <DialogDescription>
              This will permanently delete all project knowledge facts. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>

          <div className="flex items-start gap-3 rounded-lg border border-red-200 bg-red-50 p-3 dark:border-red-900/50 dark:bg-red-900/20">
            <AlertCircle className="h-5 w-5 text-red-600 dark:text-red-400 mt-0.5" />
            <div className="text-sm text-red-700 dark:text-red-300">
              <p className="font-medium">Warning</p>
              <p>All {facts.length} knowledge {facts.length === 1 ? 'fact' : 'facts'} will be permanently deleted.</p>
            </div>
          </div>

          <div className="space-y-2">
            <label className="text-sm text-muted-foreground">
              Type <span className="font-mono font-semibold">delete project knowledge</span> to confirm:
            </label>
            <Input
              value={deleteConfirmText}
              onChange={(e) => setDeleteConfirmText(e.target.value)}
              placeholder="delete project knowledge"
              disabled={isDeletingAll}
            />
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setShowDeleteAllDialog(false);
                setDeleteConfirmText('');
              }}
              disabled={isDeletingAll}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteAll}
              disabled={deleteConfirmText !== 'delete project knowledge' || isDeletingAll}
            >
              {isDeletingAll ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Deleting...
                </>
              ) : (
                'Delete All Facts'
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default ProjectKnowledgePage;
