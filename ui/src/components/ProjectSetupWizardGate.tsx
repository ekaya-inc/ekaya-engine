import { Check, ExternalLink, Loader2, RefreshCw } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';

import { useProject } from '../contexts/ProjectContext';
import { useSetupStatus } from '../hooks/useSetupStatus';
import { useToast } from '../hooks/useToast';
import engineApi from '../services/engineApi';
import type { SetupStatus } from '../types';

import DatasourceSetupFlow from './DatasourceSetupFlow';
import { Button } from './ui/Button';

type SetupStepId =
  | 'datasource_configured'
  | 'schema_selected'
  | 'ai_configured'
  | 'ontology_extracted'
  | 'questions_answered'
  | 'queries_created'
  | 'glossary_setup'
  | 'adl_activated'
  | 'agents_queries_created'
  | 'tunnel_activated'
  | 'tunnel_connected';

interface SetupStepDefinition {
  id: SetupStepId;
  label: string;
  appLabel: string;
  title: string;
  description: string;
  actionLabel?: string;
  destinationPath?: (projectId: string) => string;
  optional?: boolean;
  activationAppId?: string;
}

const SETUP_STEPS: SetupStepDefinition[] = [
  {
    id: 'datasource_configured',
    label: 'Datasource',
    appLabel: 'MCP Server',
    title: 'Connect a datasource',
    description:
      'Add a working datasource so the engine can read your schema and data. Datasources with failed credential decryption do not count as complete.',
  },
  {
    id: 'schema_selected',
    label: 'Schema',
    appLabel: 'Ontology Forge',
    title: 'Select the schema to model',
    description:
      'Choose the tables that belong in your business semantic layer before running ontology extraction.',
    actionLabel: 'Open schema',
    destinationPath: (projectId) => `/projects/${projectId}/schema`,
  },
  {
    id: 'ai_configured',
    label: 'AI config',
    appLabel: 'Ontology Forge',
    title: 'Configure AI',
    description:
      'Set an AI configuration with a real model so ontology extraction and semantic features can run.',
    actionLabel: 'Open AI config',
    destinationPath: (projectId) => `/projects/${projectId}/ai-config`,
  },
  {
    id: 'ontology_extracted',
    label: 'Ontology',
    appLabel: 'Ontology Forge',
    title: 'Run ontology extraction',
    description:
      'Start ontology extraction and wait for the latest run to complete successfully.',
    actionLabel: 'Open ontology',
    destinationPath: (projectId) => `/projects/${projectId}/ontology`,
  },
  {
    id: 'questions_answered',
    label: 'Questions',
    appLabel: 'Ontology Forge',
    title: 'Answer ontology questions',
    description:
      'Resolve the required ontology questions that remain after extraction so the ontology is ready for downstream setup.',
    actionLabel: 'Open questions',
    destinationPath: (projectId) => `/projects/${projectId}/ontology-questions`,
  },
  {
    id: 'queries_created',
    label: 'Queries',
    appLabel: 'Ontology Forge',
    title: 'Create approved queries',
    description:
      'Add at least one approved and enabled query so reusable query workflows are available. This step is optional for the dashboard badge.',
    actionLabel: 'Open queries',
    destinationPath: (projectId) => `/projects/${projectId}/queries`,
    optional: true,
  },
  {
    id: 'glossary_setup',
    label: 'Glossary',
    appLabel: 'AI Data Liaison',
    title: 'Set up the glossary',
    description:
      'Create at least one glossary term so shared business terminology is available to the AI Data Liaison workflow.',
    actionLabel: 'Open glossary',
    destinationPath: (projectId) => `/projects/${projectId}/glossary`,
  },
  {
    id: 'adl_activated',
    label: 'Activation',
    appLabel: 'AI Data Liaison',
    title: 'Activate AI Data Liaison',
    description:
      'Activate the application once its setup prerequisites are complete so the workflow is ready for business users.',
    actionLabel: 'Open AI Data Liaison',
    destinationPath: (projectId) => `/projects/${projectId}/ai-data-liaison`,
    activationAppId: 'ai-data-liaison',
  },
  {
    id: 'agents_queries_created',
    label: 'Agent queries',
    appLabel: 'AI Agents',
    title: 'Create agent-safe queries',
    description:
      'Add at least one approved and enabled query so AI Agents can be granted executable query access.',
    actionLabel: 'Open queries',
    destinationPath: (projectId) => `/projects/${projectId}/queries`,
  },
  {
    id: 'tunnel_activated',
    label: 'Activation',
    appLabel: 'MCP Tunnel',
    title: 'Activate MCP Tunnel',
    description:
      'Activate MCP Tunnel so the engine starts the outbound relay client for this project.',
    actionLabel: 'Open MCP Tunnel',
    destinationPath: (projectId) => `/projects/${projectId}/mcp-tunnel`,
    activationAppId: 'mcp-tunnel',
  },
  {
    id: 'tunnel_connected',
    label: 'Connection',
    appLabel: 'MCP Tunnel',
    title: 'Confirm the tunnel handshake',
    description:
      'This step completes after the relay connection succeeds at least once. Refresh after activation if the tunnel has already connected.',
    actionLabel: 'Open MCP Tunnel',
    destinationPath: (projectId) => `/projects/${projectId}/mcp-tunnel`,
  },
];

function getIncludedSteps(status: SetupStatus | null): SetupStepDefinition[] {
  if (!status) {
    return [];
  }

  return SETUP_STEPS.filter((step) =>
    Object.prototype.hasOwnProperty.call(status.steps, step.id)
  );
}

function getRecommendedStepId(
  status: SetupStatus | null,
  steps: SetupStepDefinition[]
): string | null {
  if (!status || steps.length === 0) {
    return null;
  }

  if (status.next_step && steps.some((step) => step.id === status.next_step)) {
    return status.next_step;
  }

  const firstStep = steps[0];
  if (!firstStep) {
    return null;
  }

  return steps.find((step) => !status.steps[step.id])?.id ?? firstStep.id;
}

const ProjectSetupWizardGate = () => {
  const { pid } = useParams<{ pid: string }>();
  const navigate = useNavigate();
  const { dismissSetupWizard } = useProject();
  const { toast } = useToast();
  const { status, isLoading, error, refetch } = useSetupStatus(pid);

  const [selectedStepId, setSelectedStepId] = useState<string | null>(null);
  const [embeddedBackHandler, setEmbeddedBackHandler] = useState<(() => void) | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [activatingStepId, setActivatingStepId] = useState<string | null>(null);

  const includedSteps = useMemo(() => getIncludedSteps(status), [status]);
  const recommendedStepId = useMemo(
    () => getRecommendedStepId(status, includedSteps),
    [includedSteps, status]
  );

  useEffect(() => {
    if (!selectedStepId || !includedSteps.some((step) => step.id === selectedStepId)) {
      setSelectedStepId(recommendedStepId);
    }
  }, [includedSteps, recommendedStepId, selectedStepId]);

  useEffect(() => {
    if (selectedStepId !== 'datasource_configured') {
      setEmbeddedBackHandler(null);
    }
  }, [selectedStepId]);

  const currentStep =
    includedSteps.find((step) => step.id === selectedStepId) ?? includedSteps[0] ?? null;
  const isCurrentStepComplete = currentStep ? Boolean(status?.steps[currentStep.id]) : false;

  const handleEmbeddedBackNavigationChange = useCallback(
    (handler: (() => void) | null) => {
      setEmbeddedBackHandler(() => handler);
    },
    []
  );

  const selectRecommendedStep = (nextStatus: SetupStatus | null) => {
    const nextIncludedSteps = getIncludedSteps(nextStatus);
    setSelectedStepId(getRecommendedStepId(nextStatus, nextIncludedSteps));
  };

  const handleRefresh = async (advance = false) => {
    setRefreshing(true);
    try {
      const nextStatus = await refetch();
      if (advance) {
        selectRecommendedStep(nextStatus);
      }
    } finally {
      setRefreshing(false);
    }
  };

  const handleActivate = async (step: SetupStepDefinition) => {
    if (!pid || !step.activationAppId) {
      return;
    }

    setActivatingStepId(step.id);
    try {
      const response = await engineApi.activateApp(pid, step.activationAppId);
      if (response.data?.redirectUrl) {
        window.location.href = response.data.redirectUrl;
        return;
      }

      await handleRefresh(true);
    } catch (err) {
      toast({
        title: 'Error',
        description: err instanceof Error ? err.message : 'Failed to activate application',
        variant: 'destructive',
      });
    } finally {
      setActivatingStepId(null);
    }
  };

  const exitSetupWizard = (): void => {
    dismissSetupWizard();
    if (pid) {
      navigate(`/projects/${pid}`);
    }
  };

  if (!pid) {
    return null;
  }

  if (isLoading && !status) {
    return (
      <div className="flex min-h-[50vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-text-secondary" />
      </div>
    );
  }

  if (!currentStep) {
    return (
      <div className="mx-auto max-w-3xl px-4 py-8">
        <div className="rounded-3xl border border-border-light bg-surface-primary p-8 shadow-sm">
          <h1 className="font-heading text-3xl font-semibold text-text-primary">Setup</h1>
          <p className="mt-3 text-sm text-text-secondary">
            No setup steps are currently required for this project.
          </p>
          <div className="mt-6">
            <Button onClick={exitSetupWizard}>Return to project</Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div
      className="min-h-screen bg-surface-primary px-4 py-4 lg:px-8 lg:py-8"
      aria-labelledby="setup-wizard-title"
    >
      <div className="mx-auto grid max-w-[1600px] overflow-hidden rounded-3xl border border-border-light bg-surface-primary shadow-2xl lg:min-h-[calc(100vh-4rem)] lg:grid-cols-[360px,1fr]">
        <aside className="bg-gradient-to-b from-[var(--wizard-sidebar-from)] to-[var(--wizard-sidebar-to)] p-8 text-white">
          <div>
            <h2 id="setup-wizard-title" className="font-heading text-3xl font-semibold text-white">
              Setup
            </h2>
            <p className="mt-2 text-sm text-white/70">
              {status && status.incomplete_count > 0
                ? `${status.incomplete_count} required step${status.incomplete_count === 1 ? '' : 's'} remaining`
                : 'Required setup is complete'}
            </p>
          </div>

          <ol className="mt-8 space-y-3">
            {includedSteps.map((step) => {
              const complete = Boolean(status?.steps[step.id]);
              const selected = step.id === currentStep.id;

              return (
                <li key={step.id}>
                  <button
                    type="button"
                    onClick={() => setSelectedStepId(step.id)}
                    className={`w-full rounded-2xl border px-4 py-4 text-left transition-colors ${
                      selected
                        ? 'border-white/25 bg-white/10'
                        : 'border-white/10 bg-white/5 hover:bg-white/8'
                    }`}
                  >
                    <div className="flex items-start gap-3">
                      {complete ? (
                        <div className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-green-500">
                          <Check className="h-3.5 w-3.5 text-white" />
                        </div>
                      ) : (
                        <div
                          className={`mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full ${
                            selected ? 'bg-[var(--wizard-accent)]' : 'bg-white/10'
                          }`}
                        >
                          <div
                            className={`rounded-full ${
                              selected ? 'h-2 w-2 bg-white' : 'h-1.5 w-1.5 bg-slate-400'
                            }`}
                          />
                        </div>
                      )}

                      <div className="min-w-0 flex-1">
                        <div className="flex items-center justify-between gap-3">
                          <p className="truncate text-sm font-semibold text-white">{step.label}</p>
                          <span
                            className={`whitespace-nowrap text-xs font-medium uppercase tracking-[0.15em] ${
                              complete
                                ? 'text-green-300'
                                : step.optional
                                  ? 'text-slate-300'
                                  : 'text-amber-200'
                            }`}
                          >
                            {complete ? 'Complete' : step.optional ? 'Optional' : 'Required'}
                          </span>
                        </div>
                        <p className="mt-1 text-xs text-white/65">{step.appLabel}</p>
                      </div>
                    </div>
                  </button>
                </li>
              );
            })}
          </ol>
        </aside>

        <div className="flex min-h-0 flex-col">
          <div className="border-b border-border-light px-8 py-6">
            <div className="flex flex-wrap items-start justify-between gap-4">
              <div>
                <div className="flex flex-wrap items-center gap-2">
                  <span className="rounded-full bg-surface-secondary px-3 py-1 text-xs font-medium uppercase tracking-[0.15em] text-text-secondary">
                    {currentStep.appLabel}
                  </span>
                  {currentStep.optional ? (
                    <span className="rounded-full bg-slate-100 px-3 py-1 text-xs font-medium uppercase tracking-[0.15em] text-slate-700 dark:bg-slate-800 dark:text-slate-300">
                      Optional
                    </span>
                  ) : null}
                </div>
                <h3 className="mt-4 font-heading text-2xl font-semibold text-text-primary">
                  {currentStep.title}
                </h3>
                <p className="mt-2 max-w-3xl text-sm leading-6 text-text-secondary">
                  {currentStep.description}
                </p>
              </div>

              <Button variant="outline" onClick={exitSetupWizard}>
                Close
              </Button>
            </div>
          </div>

          <div className="flex-1 overflow-y-auto px-8 py-6">
            {error ? (
              <div className="rounded-2xl border border-red-200 bg-red-50 px-5 py-4 text-sm text-red-800 dark:border-red-900 dark:bg-red-950 dark:text-red-200">
                {error}
              </div>
            ) : null}

            <div className="space-y-6">
              <div
                className={`rounded-2xl border px-5 py-4 ${
                  isCurrentStepComplete
                    ? 'border-green-200 bg-green-50 text-green-800 dark:border-green-900 dark:bg-green-950 dark:text-green-200'
                    : 'border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-100'
                }`}
              >
                {isCurrentStepComplete
                  ? 'This step is complete.'
                  : currentStep.optional
                    ? 'This optional step is still available if you want to complete it now.'
                    : 'This required step is still incomplete.'}
              </div>

              {currentStep.id === 'datasource_configured' ? (
                <DatasourceSetupFlow
                  embedded
                  onEmbeddedBackNavigationChange={handleEmbeddedBackNavigationChange}
                  onSaveSuccess={() => {
                    void handleRefresh(true);
                  }}
                />
              ) : (
                <div className="rounded-2xl border border-border-light bg-surface-primary p-6">
                  <div className="flex flex-wrap gap-3">
                    {!isCurrentStepComplete && currentStep.activationAppId ? (
                      <Button
                        onClick={() => void handleActivate(currentStep)}
                        disabled={activatingStepId === currentStep.id}
                      >
                        {activatingStepId === currentStep.id ? (
                          <>
                            <Loader2 className="h-4 w-4 animate-spin" />
                            Activating...
                          </>
                        ) : (
                          <>Activate</>
                        )}
                      </Button>
                    ) : null}

                    {currentStep.destinationPath ? (
                      <Button
                        asChild
                        variant={!isCurrentStepComplete && !currentStep.activationAppId ? 'default' : 'outline'}
                      >
                        <Link to={currentStep.destinationPath(pid)}>
                          {currentStep.actionLabel ?? 'Open page'}
                          <ExternalLink className="h-4 w-4" />
                        </Link>
                      </Button>
                    ) : null}
                  </div>

                  <div className="mt-6 rounded-2xl border border-border-light bg-surface-secondary/60 p-5">
                    <p className="text-sm leading-6 text-text-secondary">
                      Use the action above, then refresh this wizard to reconcile the persisted setup status and move to the next incomplete step.
                    </p>
                  </div>
                </div>
              )}
            </div>
          </div>

          <div className="border-t border-border-light px-8 py-5">
            <div className="flex items-center justify-between gap-4">
              <Button
                variant="ghost"
                onClick={() => embeddedBackHandler?.()}
                disabled={currentStep.id !== 'datasource_configured' || !embeddedBackHandler}
                className="text-text-secondary"
              >
                Back
              </Button>

              <Button
                variant="outline"
                onClick={() => void handleRefresh(true)}
                disabled={refreshing}
              >
                {refreshing ? (
                  <>
                    <Loader2 className="h-4 w-4 animate-spin" />
                    Refreshing...
                  </>
                ) : (
                  <>
                    <RefreshCw className="h-4 w-4" />
                    {isCurrentStepComplete ? 'Refresh status' : "I've completed this"}
                  </>
                )}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default ProjectSetupWizardGate;
