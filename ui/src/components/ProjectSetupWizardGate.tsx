import { Check } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

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

type SetupStepId = 'mcp-server';

interface SetupStep {
  id: SetupStepId;
  label: string;
  title: string;
  description: string;
}

interface SetupApplication {
  id: string;
  label: string;
}

const APP_LABELS: Record<string, string> = {
  [APP_ID_MCP_SERVER]: 'MCP Server',
  [APP_ID_ONTOLOGY_FORGE]: 'Ontology Forge',
  [APP_ID_AI_DATA_LIAISON]: 'AI Data Liaison',
  [APP_ID_AI_AGENTS]: 'AI Agents',
  [APP_ID_FILE_LOADER]: 'Spreadsheet Loader',
  [APP_ID_MCP_TUNNEL]: 'MCP Tunnel',
};

const SETUP_APPLICATIONS: SetupApplication[] = [
  { id: APP_ID_MCP_SERVER, label: 'MCP Server' },
  { id: APP_ID_ONTOLOGY_FORGE, label: 'Ontology Forge' },
  { id: APP_ID_AI_DATA_LIAISON, label: 'AI Data Liaison' },
  { id: APP_ID_AI_AGENTS, label: 'AI Agents' },
  { id: APP_ID_FILE_LOADER, label: 'Spreadsheet Loader' },
  { id: APP_ID_MCP_TUNNEL, label: 'MCP Tunnel' },
];

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
  const navigate = useNavigate();
  const { provisioning, shouldShowSetupWizard, dismissSetupWizard } = useProject();
  const { datasources } = useDatasourceConnection();
  const { apps: installedApps } = useInstalledApps(pid);
  const [embeddedBackHandler, setEmbeddedBackHandler] = useState<(() => void) | null>(null);

  const provisionedAppIds = useMemo(() => {
    const appIds = new Set<string>([
      ...provisioning.assignedAppIds,
      ...installedApps.map((app) => app.app_id),
    ]);

    return Array.from(appIds);
  }, [installedApps, provisioning.assignedAppIds]);

  const orderedSetupApplications = useMemo(() => {
    const setupAppIds = new Set<string>([APP_ID_MCP_SERVER, ...provisionedAppIds]);
    const knownApplications = SETUP_APPLICATIONS.filter((application) =>
      setupAppIds.has(application.id)
    );
    const unknownApplications = Array.from(setupAppIds)
      .filter((appId) => !SETUP_APPLICATIONS.some((application) => application.id === appId))
      .map((appId) => ({
        id: appId,
        label: APP_LABELS[appId] ?? appId,
      }));

    return [...knownApplications, ...unknownApplications];
  }, [provisionedAppIds]);

  const hasUsableDatasource = useMemo(
    () => datasources.some((datasource) => datasource.decryption_failed !== true),
    [datasources]
  );

  useEffect(() => {
    if (!shouldShowSetupWizard) {
      setEmbeddedBackHandler(null);
    }
  }, [shouldShowSetupWizard]);

  const handleEmbeddedBackNavigationChange = useCallback((handler: (() => void) | null) => {
    setEmbeddedBackHandler(() => handler);
  }, []);

  if (!shouldShowSetupWizard) {
    return null;
  }

  const currentStep = STEPS[0];
  if (!currentStep) {
    return null;
  }
  const isCurrentStepComplete = hasUsableDatasource;
  const currentSetupApplicationId = isCurrentStepComplete ? null : currentStep.id;

  const exitSetupWizard = (): void => {
    dismissSetupWizard();
    if (pid) {
      navigate(`/projects/${pid}`);
    }
  };

  const getApplicationStatus = (appId: string): 'complete' | 'in-progress' | 'pending' => {
    if (appId === currentSetupApplicationId) {
      return 'in-progress';
    }

    if (appId === APP_ID_MCP_SERVER && hasUsableDatasource) {
      return 'complete';
    }

    return 'pending';
  };

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
          </div>

          <ol className="mt-8 space-y-3">
            {orderedSetupApplications.map((application) => {
              const status = getApplicationStatus(application.id);
              const isCurrent = status === 'in-progress';
              const isComplete = status === 'complete';

              return (
                <li
                  key={application.id}
                  className={`rounded-2xl border px-4 py-4 transition-colors ${
                    isCurrent
                      ? 'border-white/20 bg-white/8'
                      : 'border-white/10 bg-white/5'
                  }`}
                >
                  <div className="flex items-center gap-3">
                    {isComplete ? (
                      <div className="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-green-500">
                        <Check className="h-3.5 w-3.5 text-white" />
                      </div>
                    ) : (
                      <div
                        className={`flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full ${
                          isCurrent ? 'bg-[var(--wizard-accent)]' : 'bg-white/10'
                        }`}
                      >
                        <div
                          className={`rounded-full ${
                            isCurrent ? 'h-2 w-2 bg-white' : 'h-1.5 w-1.5 bg-slate-400'
                          }`}
                        />
                      </div>
                    )}

                    <div className="min-w-0 flex-1">
                      <div className="flex items-center justify-between gap-3">
                        <p className="truncate text-sm font-semibold text-white">
                          {application.label}
                        </p>
                        <span
                          className={`whitespace-nowrap text-xs font-medium uppercase tracking-[0.15em] ${
                            isComplete
                              ? 'text-green-300'
                              : isCurrent
                                ? 'text-blue-300'
                                : 'text-slate-400'
                          }`}
                        >
                          {isComplete ? 'Complete' : isCurrent ? 'In progress' : 'Pending'}
                        </span>
                      </div>
                    </div>
                  </div>
                </li>
              );
            })}
          </ol>
        </aside>

        <div className="flex flex-col">
          <div className="border-b border-border-light px-8 py-6">
            <div>
              <h3 className="font-heading text-2xl font-semibold text-text-primary">
                {currentStep.title}
              </h3>
              <p className="mt-2 max-w-2xl text-sm leading-6 text-text-secondary">
                {currentStep.description}
              </p>
            </div>
          </div>

          <div className="flex-1 px-8 py-6">
            <DatasourceSetupFlow
              embedded
              onEmbeddedBackNavigationChange={handleEmbeddedBackNavigationChange}
              onSaveSuccess={() => undefined}
            />
          </div>

          <div className="border-t border-border-light px-8 py-5">
            <div className="flex items-center justify-between gap-4">
              <Button
                variant="ghost"
                onClick={() => embeddedBackHandler?.()}
                disabled={!embeddedBackHandler}
                className="text-text-secondary"
              >
                Back
              </Button>
              <div className="flex items-center gap-3">
                <Button variant="outline" onClick={exitSetupWizard}>
                  Cancel
                </Button>
                <Button onClick={exitSetupWizard} disabled={!isCurrentStepComplete} className="px-8">
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
