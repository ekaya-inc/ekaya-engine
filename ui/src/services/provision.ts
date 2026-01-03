import { fetchWithAuth } from '../lib/api';

// Shared response structure for project info
export interface ProjectInfoResponse {
  status: string;
  pid: string;
  name?: string;
  papi_url?: string;
  projects_page_url?: string;
  project_page_url?: string;
}

export interface ProvisionResponse extends ProjectInfoResponse {
  forge_result?: Record<string, unknown>;
  error?: string;
}

/**
 * Gets project info from the database
 *
 * @throws {Error} If project not found or request fails
 */
export async function getProject(): Promise<ProjectInfoResponse> {
  const response = await fetchWithAuth('/projects', {
    method: 'GET',
  });

  if (!response.ok) {
    if (response.status === 404) {
      throw new Error('Project not found');
    }
    const errorText = await response.text();
    throw new Error(`Failed to get project: ${errorText}`);
  }

  return response.json();
}

/**
 * Provisions a project and user from JWT claims.
 *
 * This endpoint is idempotent - safe to call multiple times for the same project.
 * It will:
 * 1. Create project in projects table (if not exists)
 * 2. Add user to project_users table with admin role
 *
 * @throws {Error} If provisioning fails
 */
export async function provisionProject(): Promise<ProvisionResponse> {
  const response = await fetchWithAuth('/projects', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
  });

  if (!response.ok) {
    const errorText = await response.text();

    // Check for rate limiting
    if (response.status === 429) {
      throw new Error('Rate limit exceeded. Please try again in a minute.');
    }

    throw new Error(`Failed to provision project: ${errorText}`);
  }

  return response.json();
}
