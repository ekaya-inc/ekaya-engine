import type { ReactNode } from 'react';
import { createContext, useContext, useState, useCallback } from 'react';

interface ProjectURLs {
  projectsPageUrl: string | null;
  projectPageUrl: string | null;
}

interface ProjectContextValue {
  projectId: string | null;
  projectName: string | null;
  urls: ProjectURLs;
  setProjectInfo: (
    projectId: string,
    name: string | null,
    urls: { projectsPageUrl?: string; projectPageUrl?: string }
  ) => void;
  clearProjectInfo: () => void;
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

  const setProjectInfo = useCallback(
    (
      id: string,
      name: string | null,
      urlInfo: { projectsPageUrl?: string; projectPageUrl?: string }
    ) => {
      setProjectId(id);
      setProjectName(name);
      setUrls({
        projectsPageUrl: urlInfo.projectsPageUrl ?? null,
        projectPageUrl: urlInfo.projectPageUrl ?? null,
      });
    },
    []
  );

  const clearProjectInfo = useCallback(() => {
    setProjectId(null);
    setProjectName(null);
    setUrls({ projectsPageUrl: null, projectPageUrl: null });
  }, []);

  const value: ProjectContextValue = {
    projectId,
    projectName,
    urls,
    setProjectInfo,
    clearProjectInfo,
  };

  return (
    <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>
  );
};
