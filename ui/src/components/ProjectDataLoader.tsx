import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";

import { useDatasourceConnection } from "../contexts/DatasourceConnectionContext";
import { useProject } from "../contexts/ProjectContext";
import { getProject, provisionProject } from "../services/provision";
import type { ProjectInfoResponse } from "../services/provision";

interface ProjectDataLoaderProps {
  children: React.ReactNode;
}

/**
 * ProjectDataLoader - Loads project data for /projects/:pid routes
 *
 * Responsibilities:
 * - Extract project ID (pid) from URL
 * - Load datasources for the project
 * - Detect project switching and clear stale JWT cookies
 * - Provide project context to child components
 *
 * Authentication:
 * - NO client-side auth checks (HttpOnly cookies can't be read by JavaScript)
 * - Backend validates JWT on API calls and returns 401/403 if invalid
 * - fetchWithAuth handles 401/403 by clearing cookie and initiating OAuth flow
 * - This pattern keeps cookies secure (HttpOnly) while handling auth correctly
 *
 * Project Switching:
 * - When pid changes, clears old JWT cookie proactively
 * - Prevents API calls with wrong project JWT
 * - Next API call triggers fresh OAuth flow with correct project
 */
export default function ProjectDataLoader({
  children,
}: ProjectDataLoaderProps) {
  const { pid } = useParams<{ pid: string }>();
  const { loadDataSources } = useDatasourceConnection();
  const { setProjectInfo, clearProjectInfo } = useProject();
  const lastProjectIdRef = useRef<string | undefined>(undefined);
  const [provisioning, setProvisioning] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Detect project switching and clear stale JWT cookie
  useEffect(() => {
    if (pid && lastProjectIdRef.current && pid !== lastProjectIdRef.current) {
      console.log(`Project switched from ${lastProjectIdRef.current} to ${pid}`);
      console.log("Clearing stale JWT cookie to trigger re-authentication");

      // Clear JWT cookie by setting it to expire immediately
      // This ensures the next API call will get 401 and trigger OAuth
      document.cookie = 'ekaya_jwt=; Max-Age=0; path=/; SameSite=Strict';

      // Clear project info from context
      clearProjectInfo();
    }

    lastProjectIdRef.current = pid;
  }, [pid, clearProjectInfo]);

  // Load project info and datasources
  useEffect(() => {
    if (!pid) return;

    async function loadProject() {
      try {
        setProvisioning(true);
        setError(null);

        // Step 1: Try to get existing project info
        let projectInfo: ProjectInfoResponse;
        try {
          console.log("Getting project info:", pid);
          projectInfo = await getProject();
          console.log("Project found:", projectInfo.name);
        } catch (getErr) {
          // Project not found - provision it
          if (getErr instanceof Error && getErr.message === 'Project not found') {
            console.log("Project not found, provisioning:", pid);
            projectInfo = await provisionProject();
            console.log("Project provisioned:", projectInfo.name);
          } else {
            throw getErr;
          }
        }

        // Step 2: Store project info in context
        const urlInfo: { projectsPageUrl?: string; projectPageUrl?: string } = {};
        if (projectInfo.projects_page_url) {
          urlInfo.projectsPageUrl = projectInfo.projects_page_url;
        }
        if (projectInfo.project_page_url) {
          urlInfo.projectPageUrl = projectInfo.project_page_url;
        }
        setProjectInfo(pid as string, projectInfo.name ?? null, urlInfo);

        // Step 3: Load datasources
        console.log("Loading datasources for project:", pid);
        await loadDataSources(pid as string);
        console.log("Datasources loaded successfully");
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : "Unknown error";
        console.error("Failed to load project:", errorMessage);
        setError(errorMessage);
      } finally {
        setProvisioning(false);
      }
    }

    loadProject();
  }, [pid, loadDataSources, setProjectInfo]);

  // Show loading state during provisioning
  if (provisioning) {
    return (
      <div className="flex items-center justify-center min-h-screen bg-background">
        <div className="text-center">
          <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-solid border-current border-r-transparent motion-reduce:animate-[spin_1.5s_linear_infinite]" role="status">
            <span className="sr-only">Setting up project...</span>
          </div>
          <p className="mt-4 text-sm text-muted-foreground">Setting up project...</p>
        </div>
      </div>
    );
  }

  // Show error state if provisioning failed
  if (error) {
    return (
      <div className="flex items-center justify-center min-h-screen bg-background">
        <div className="text-center max-w-md p-6">
          <div className="mb-4 text-destructive">
            <svg xmlns="http://www.w3.org/2000/svg" className="h-12 w-12 mx-auto" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
          </div>
          <h2 className="text-lg font-semibold mb-2">Project Setup Error</h2>
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
    );
  }

  return <>{children}</>;
}
