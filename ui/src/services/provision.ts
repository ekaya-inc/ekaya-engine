import { fetchWithAuth } from '../lib/api';

// Shared response structure for project info
export interface ProjectInfoResponse {
  status: string;
  pid: string;
  name?: string;
  papi_url?: string;
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
 * Provisions a project in region_projects, region_users, and ekaya-forge
 *
 * This endpoint is idempotent - safe to call multiple times for the same project.
 * It will:
 * 1. Create/update project in region_projects table
 * 2. Add user to region_users table with role from JWT
 * 3. Call ekaya-forge provision endpoint
 *
 * Rate limited: 10 requests per user per minute
 *
 * @throws {Error} If provisioning fails
 */
export async function provisionProject(): Promise<ProvisionResponse> {
  const response = await fetchWithAuth('/cloud/project_provision', {
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
