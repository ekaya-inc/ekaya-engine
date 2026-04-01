import type { ReactNode } from 'react';
import { createContext, useContext, useState, useCallback } from 'react';

interface ProjectURLs {
  projectsPageUrl: string | null;
  projectPageUrl: string | null;
}

interface ProjectProvisioningState {
  justProvisioned: boolean;
  assignedAppIds: string[];
}

interface ProjectContextValue {
  projectId: string | null;
  projectName: string | null;
  urls: ProjectURLs;
  provisioning: ProjectProvisioningState;
  shouldShowSetupWizard: boolean;
  setProjectInfo: (
    projectId: string,
    name: string | null,
    urls: { projectsPageUrl?: string; projectPageUrl?: string },
    provisioning?: Partial<ProjectProvisioningState>
  ) => void;
  clearProjectInfo: () => void;
  dismissSetupWizard: () => void;
}

const ProjectContext = createContext<ProjectContextValue | undefined>(undefined);

export const useProject = (): ProjectContextValue => {
  const context = useContext(ProjectContext);
  if (!context) {
    throw new Error('useProject must be used within a ProjectProvider');
  }
  return context;
};

interface ProjectProviderProps {
  children: ReactNode;
}

export const ProjectProvider = ({ children }: ProjectProviderProps) => {
  const [projectId, setProjectId] = useState<string | null>(null);
  const [projectName, setProjectName] = useState<string | null>(null);
  const [urls, setUrls] = useState<ProjectURLs>({
    projectsPageUrl: null,
    projectPageUrl: null,
  });
  const [provisioning, setProvisioning] = useState<ProjectProvisioningState>({
    justProvisioned: false,
    assignedAppIds: [],
  });
  const [setupWizardDismissed, setSetupWizardDismissed] = useState(false);

  const setProjectInfo = useCallback(
    (
      id: string,
      name: string | null,
      urlInfo: { projectsPageUrl?: string; projectPageUrl?: string },
      provisioningInfo?: Partial<ProjectProvisioningState>
    ) => {
      setProjectId(id);
      setProjectName(name);
      setUrls({
        projectsPageUrl: urlInfo.projectsPageUrl ?? null,
        projectPageUrl: urlInfo.projectPageUrl ?? null,
      });
      setProvisioning({
        justProvisioned: provisioningInfo?.justProvisioned ?? false,
        assignedAppIds: provisioningInfo?.assignedAppIds ?? [],
      });
      setSetupWizardDismissed(false);
    },
    []
  );

  const clearProjectInfo = useCallback(() => {
    setProjectId(null);
    setProjectName(null);
    setUrls({ projectsPageUrl: null, projectPageUrl: null });
    setProvisioning({ justProvisioned: false, assignedAppIds: [] });
    setSetupWizardDismissed(false);
  }, []);

  const dismissSetupWizard = useCallback(() => {
    setSetupWizardDismissed(true);
  }, []);

  const value: ProjectContextValue = {
    projectId,
    projectName,
    urls,
    provisioning,
    shouldShowSetupWizard: provisioning.justProvisioned && !setupWizardDismissed,
    setProjectInfo,
    clearProjectInfo,
    dismissSetupWizard,
  };

  return (
    <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>
  );
};
