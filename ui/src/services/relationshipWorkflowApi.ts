/**
 * Relationship Workflow API Service
 * Handles communication with the relationship detection workflow API
 */

import { fetchWithAuth } from '../lib/api';
import type {
  CancelWorkflowResponse,
  CandidateResponse,
  CandidatesResponse,
  EntitiesResponse,
  RelationshipWorkflowStatusResponse,
  SaveRelationshipsResponse,
  StartDetectionResponse,
} from '../types';

const BASE_URL = '/api/projects';

class RelationshipWorkflowApiService {
  private async makeRequest<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const url = `${BASE_URL}${endpoint}`;
    const config: RequestInit = {
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
      ...options,
    };

    try {
      const response = await fetchWithAuth(url, config);
      const json = (await response.json()) as { data?: T } | T;

      if (!response.ok) {
        // Extract error message from response body if available
        const errorJson = json as { message?: string; error?: string };
        const errorMessage = errorJson.message || errorJson.error || response.statusText;
        const error = new Error(errorMessage) as Error & { status?: number; data?: unknown };
        error.status = response.status;
        error.data = json;
        throw error;
      }

      // Unwrap ApiResponse if present (backend wraps in {success, data}), otherwise return as-is
      const data =
        json !== null &&
        typeof json === 'object' &&
        'data' in json &&
        json.data !== undefined
          ? json.data
          : (json as T);
      return data;
    } catch (error) {
      console.error(`Relationship Workflow API Error (${endpoint}):`, error);
      throw error;
    }
  }

  /**
   * Start relationship detection workflow
   * POST /api/projects/{pid}/datasources/{dsid}/relationships/detect
   */
  async startDetection(
    projectId: string,
    datasourceId: string
  ): Promise<StartDetectionResponse> {
    return this.makeRequest<StartDetectionResponse>(
      `/${projectId}/datasources/${datasourceId}/relationships/detect`,
      { method: 'POST' }
    );
  }

  /**
   * Get workflow status with candidate counts
   * GET /api/projects/{pid}/datasources/{dsid}/relationships/status
   */
  async getStatus(
    projectId: string,
    datasourceId: string
  ): Promise<RelationshipWorkflowStatusResponse> {
    return this.makeRequest<RelationshipWorkflowStatusResponse>(
      `/${projectId}/datasources/${datasourceId}/relationships/status`
    );
  }

  /**
   * Get all candidates grouped by status
   * GET /api/projects/{pid}/datasources/{dsid}/relationships/candidates
   */
  async getCandidates(
    projectId: string,
    datasourceId: string
  ): Promise<CandidatesResponse> {
    return this.makeRequest<CandidatesResponse>(
      `/${projectId}/datasources/${datasourceId}/relationships/candidates`
    );
  }

  /**
   * Update a candidate's decision (accept or reject)
   * PUT /api/projects/{pid}/datasources/{dsid}/relationships/candidates/{cid}
   */
  async updateCandidate(
    projectId: string,
    datasourceId: string,
    candidateId: string,
    decision: 'accepted' | 'rejected'
  ): Promise<CandidateResponse> {
    return this.makeRequest<CandidateResponse>(
      `/${projectId}/datasources/${datasourceId}/relationships/candidates/${candidateId}`,
      {
        method: 'PUT',
        body: JSON.stringify({ decision }),
      }
    );
  }

  /**
   * Cancel the current workflow
   * POST /api/projects/{pid}/datasources/{dsid}/relationships/cancel
   */
  async cancel(
    projectId: string,
    datasourceId: string
  ): Promise<CancelWorkflowResponse> {
    return this.makeRequest<CancelWorkflowResponse>(
      `/${projectId}/datasources/${datasourceId}/relationships/cancel`,
      { method: 'POST' }
    );
  }

  /**
   * Save accepted relationships
   * POST /api/projects/{pid}/datasources/{dsid}/relationships/save
   */
  async save(
    projectId: string,
    datasourceId: string
  ): Promise<SaveRelationshipsResponse> {
    return this.makeRequest<SaveRelationshipsResponse>(
      `/${projectId}/datasources/${datasourceId}/relationships/save`,
      { method: 'POST' }
    );
  }

  /**
   * Get discovered entities with their occurrences
   * GET /api/projects/{pid}/datasources/{dsid}/relationships/entities
   */
  async getEntities(
    projectId: string,
    datasourceId: string
  ): Promise<EntitiesResponse> {
    return this.makeRequest<EntitiesResponse>(
      `/${projectId}/datasources/${datasourceId}/relationships/entities`
    );
  }
}

// Create and export singleton instance
const relationshipWorkflowApi = new RelationshipWorkflowApiService();
export default relationshipWorkflowApi;
