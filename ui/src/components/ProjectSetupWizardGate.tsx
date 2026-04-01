import { CheckCircle2, Circle, Database, Layers } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useParams } from 'react-router-dom';

import { useDatasourceConnection } from '../contexts/DatasourceConnectionContext';
import { useProject } from '../contexts/ProjectContext';
import { useInstalledApps } from '../hooks/useInstalledApps';
import {
  APP_ID_AI_AGENTS,
  APP_ID_AI_DATA_LIAISON,
  APP_ID_FILE_LOADER,
  APP_ID_MCP_SERVER,
  APP_ID_MCP_TUNNEL,
  APP_ID_ONTOLOGY_FORGE,
} from '../types';

import DatasourceSetupFlow from './DatasourceSetupFlow';
import { Button } from './ui/Button';

type SetupWizardMode = 'scratch' | 'provisioned';
type SetupStepId = 'mcp-server';

interface SetupStep {
  id: SetupStepId;
  label: string;
  title: string;
  description: string;
}

const APP_LABELS: Record<string, string> = {
  [APP_ID_MCP_SERVER]: 'MCP Server',
  [APP_ID_ONTOLOGY_FORGE]: 'Ontology Forge',
  [APP_ID_AI_DATA_LIAISON]: 'AI Data Liaison',
  [APP_ID_AI_AGENTS]: 'AI Agents',
  [APP_ID_FILE_LOADER]: 'Spreadsheet Loader',
  [APP_ID_MCP_TUNNEL]: 'MCP Tunnel',
};

const STEPS: SetupStep[] = [
  {
    id: 'mcp-server',
    label: 'MCP Server',
    title: 'Connect your datasource',
    description:
      'Choose your database provider, test the connection, and save it so the MCP Server can access your data.',
  },
];

const ProjectSetupWizardGate = () => {
  const { pid } = useParams<{ pid: string }>();
  const { provisioning, shouldShowSetupWizard, dismissSetupWizard } = useProject();
  const { datasources } = useDatasourceConnection();
  const { apps: installedApps } = useInstalledApps(pid);
  const [completedStepIds, setCompletedStepIds] = useState<SetupStepId[]>([]);
  const [embeddedBackHandler, setEmbeddedBackHandler] = useState<(() => void) | null>(null);

  const provisionedAppIds = useMemo(() => {
    const appIds = new Set<string>([
      ...provisioning.assignedAppIds,
      ...installedApps.map((app) => app.app_id),
    ]);

    return Array.from(appIds);
  }, [installedApps, provisioning.assignedAppIds]);

  const provisionedApplicationNames = useMemo(
    () =>
      provisionedAppIds
        .filter((appId) => appId !== APP_ID_MCP_SERVER)
        .map((appId) => APP_LABELS[appId] ?? appId),
    [provisionedAppIds]
  );

  const mode: SetupWizardMode =
    provisionedApplicationNames.length > 0 ? 'provisioned' : 'scratch';

  const hasUsableDatasource = useMemo(
    () => datasources.some((datasource) => datasource.decryption_failed !== true),
    [datasources]
  );

  useEffect(() => {
    setCompletedStepIds((current) => {
      const hasCompletedStep = current.includes('mcp-server');
      if (hasUsableDatasource) {
        return hasCompletedStep ? current : [...current, 'mcp-server'];
      }

      return hasCompletedStep ? current.filter((stepId) => stepId !== 'mcp-server') : current;
    });
  }, [hasUsableDatasource]);

  useEffect(() => {
    if (!shouldShowSetupWizard) {
      setCompletedStepIds([]);
      setEmbeddedBackHandler(null);
    }
  }, [shouldShowSetupWizard]);

  const handleEmbeddedBackNavigationChange = useCallback((handler: (() => void) | null) => {
    setEmbeddedBackHandler(() => handler);
  }, []);

  const isStepComplete = useCallback(
    (stepId: SetupStepId): boolean => {
      if (stepId === 'mcp-server') {
        return hasUsableDatasource;
      }

      return completedStepIds.includes(stepId);
    },
    [completedStepIds, hasUsableDatasource]
  );

  if (!shouldShowSetupWizard) {
    return null;
  }

  const currentStep = STEPS[0];
  if (!currentStep) {
    return null;
  }
  const isCurrentStepComplete = isStepComplete(currentStep.id);

  return (
    <div
      className="fixed inset-0 z-50 bg-slate-950/60 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
      aria-labelledby="setup-wizard-title"
    >
      <div className="absolute inset-4 overflow-hidden rounded-3xl border border-border-light bg-surface-primary shadow-2xl lg:inset-8">
        <div className="grid h-full lg:grid-cols-[320px,1fr]">
          <aside className="border-b border-border-light bg-surface-secondary/70 p-6 lg:border-b-0 lg:border-r">
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="text-sm font-semibold uppercase tracking-[0.2em] text-blue-600">
                  {mode === 'scratch' ? 'Scratch mode' : 'Provisioned mode'}
                </p>
                <h2 id="setup-wizard-title" className="mt-3 text-3xl font-semibold text-text-primary">
                  Set up your project
                </h2>
                <p className="mt-3 text-sm leading-6 text-text-secondary">
                  {mode === 'scratch'
                    ? 'Start with MCP Server setup so your project can reach the datasource immediately.'
                    : 'This project arrived with additional applications, so the wizard is using that provisioning context while it guides the initial setup.'}
                </p>
              </div>
              <Button variant="ghost" size="sm" onClick={dismissSetupWizard}>
                Skip setup
              </Button>
            </div>

            {provisionedApplicationNames.length > 0 && (
              <div className="mt-6 rounded-2xl border border-border-light bg-surface-primary p-4">
                <div className="flex items-center gap-2 text-sm font-medium text-text-primary">
                  <Layers className="h-4 w-4 text-blue-600" />
                  Provisioned applications
                </div>
                <div className="mt-3 flex flex-wrap gap-2">
                  {provisionedApplicationNames.map((appName) => (
                    <span
                      key={appName}
                      className="rounded-full bg-blue-500/10 px-3 py-1 text-xs font-medium text-blue-700"
                    >
                      {appName}
                    </span>
                  ))}
                </div>
              </div>
            )}

            <ol className="mt-8 space-y-3">
              {STEPS.map((step, index) => {
                const complete = isStepComplete(step.id);
                return (
                  <li
                    key={step.id}
                    className="rounded-2xl border border-border-light bg-surface-primary p-4"
                  >
                    <div className="flex items-start gap-3">
                      {complete ? (
                        <CheckCircle2 className="mt-0.5 h-5 w-5 text-green-600" />
                      ) : (
                        <Circle className="mt-0.5 h-5 w-5 text-text-secondary" />
                      )}
                      <div>
                        <p className="text-xs font-medium uppercase tracking-[0.18em] text-text-tertiary">
                          Step {index + 1}
                        </p>
                        <p className="mt-1 text-sm font-semibold text-text-primary">{step.label}</p>
                        <p className="mt-1 text-sm text-text-secondary">{step.description}</p>
                      </div>
                    </div>
                  </li>
                );
              })}
            </ol>
          </aside>

          <div className="flex min-h-0 flex-col">
            <div className="border-b border-border-light p-6">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <div className="flex items-center gap-2 text-sm text-text-secondary">
                    <Database className="h-4 w-4" />
                    <span>Step 1 of {STEPS.length}</span>
                  </div>
                  <h3 className="mt-2 text-2xl font-semibold text-text-primary">
                    {currentStep.title}
                  </h3>
                  <p className="mt-2 max-w-2xl text-sm leading-6 text-text-secondary">
                    {currentStep.description}
                  </p>
                </div>
                <Button variant="outline" onClick={dismissSetupWizard}>
                  Cancel setup
                </Button>
              </div>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto p-6">
              <DatasourceSetupFlow
                embedded
                onEmbeddedBackNavigationChange={handleEmbeddedBackNavigationChange}
                onSaveSuccess={() => {
                  setCompletedStepIds((current) =>
                    current.includes('mcp-server') ? current : [...current, 'mcp-server']
                  );
                }}
              />
            </div>

            <div className="border-t border-border-light p-6">
              <div className="flex items-center justify-between">
                <Button
                  variant="outline"
                  onClick={() => embeddedBackHandler?.()}
                  disabled={!embeddedBackHandler}
                >
                  Back
                </Button>
                <Button onClick={dismissSetupWizard} disabled={!isCurrentStepComplete}>
                  Finish
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default ProjectSetupWizardGate;
