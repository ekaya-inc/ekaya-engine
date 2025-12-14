import type { ReactNode } from 'react';
import { createContext, useContext, useState, useCallback } from 'react';

interface ProjectContextValue {
  projectId: string | null;
  projectName: string | null;
  papiUrl: string | null;
  setProjectInfo: (projectId: string, name: string | null, papiUrl: string | null) => void;
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
  const [papiUrl, setPapiUrl] = useState<string | null>(null);

  const setProjectInfo = useCallback(
    (id: string, name: string | null, papi: string | null) => {
      setProjectId(id);
      setProjectName(name);
      setPapiUrl(papi);
    },
    []
  );

  const clearProjectInfo = useCallback(() => {
    setProjectId(null);
    setProjectName(null);
    setPapiUrl(null);
  }, []);

  const value: ProjectContextValue = {
    projectId,
    projectName,
    papiUrl,
    setProjectInfo,
    clearProjectInfo,
  };

  return (
    <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>
  );
};
