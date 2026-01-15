import { useEffect } from 'react';

import { useConfig } from '../contexts/ConfigContext';

/**
 * ProjectsRedirect - Redirects /projects to auth server's projects page
 *
 * This component handles the case where a user lands at /projects without
 * a specific project ID. This can happen when:
 * 1. User clicks "Sign In" on the homepage and is redirected back after signing
 *    in at the auth server (us.ekaya.ai)
 * 2. User navigates directly to /projects
 *
 * Instead of showing a JSON 401 error, we redirect them to the auth server's
 * projects page where they can select which project to work with.
 */
export default function ProjectsRedirect() {
  const { config } = useConfig();

  useEffect(() => {
    if (config) {
      window.location.href = `${config.authServerUrl}/projects`;
    }
  }, [config]);

  return (
    <div className="flex min-h-screen items-center justify-center">
      <p className="text-gray-500">Redirecting to project selection...</p>
    </div>
  );
}
