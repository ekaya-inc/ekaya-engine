/**
 * Ontology API Service
 * Handles communication with the Ekaya Ontology REST API
 */

import { fetchWithAuth } from '../lib/api';
import type {
  AnswerQuestionResponse,
  ExtractOntologyResponse,
  GetNextQuestionResponse,
  SkipDeleteResponse,
  SubmitAnswersRequest,
  SubmitAnswersResponse,
  WorkflowQuestionsResponse,
  WorkflowResultResponse,
  WorkflowStatusResponse,
} from '../types';

const ONTOLOGY_BASE_URL = '/ontology/v1';

class OntologyApiService {
  private async makeRequest<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const url = `${ONTOLOGY_BASE_URL}${endpoint}`;
    const config: RequestInit = {
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
      ...options,
    };

    try {
      const response = await fetchWithAuth(url, config);
      const data = (await response.json()) as T;

      if (!response.ok) {
        const error = new Error(
          `HTTP ${response.status}: ${response.statusText}`
        ) as Error & { status?: number; data?: T };
        error.status = response.status;
        error.data = data;
        throw error;
      }

      return data;
    } catch (error) {
      console.error(`Ontology API Error (${endpoint}):`, error);
      throw error;
    }
  }

  /**
   * Start ontology extraction workflow
   * POST /ontology/v1/{project_id}/extract
   */
  async extractOntology(
    projectId: string,
    selectedTables?: string[]
  ): Promise<ExtractOntologyResponse> {
    const options: RequestInit = {
      method: 'POST',
    };

    if (selectedTables && selectedTables.length > 0) {
      options.body = JSON.stringify({ selected_tables: selectedTables });
    }

    return this.makeRequest<ExtractOntologyResponse>(
      `/${projectId}/extract`,
      options
    );
  }

  /**
   * Get workflow result (tiered ontology) for the latest workflow
   * GET /ontology/v1/{project_id}/result
   */
  async getWorkflowResult(projectId: string): Promise<WorkflowResultResponse> {
    return this.makeRequest<WorkflowResultResponse>(`/${projectId}/result`);
  }

  /**
   * Get business rules for a project
   * GET /ontology/v1/{project_id}/business-rules
   */
  async getBusinessRules(
    projectId: string,
    entityId?: string
  ): Promise<any> {
    const params = entityId ? `?entity_id=${entityId}` : '';
    return this.makeRequest<any>(
      `/${projectId}/business-rules${params}`
    );
  }

  /**
   * Get status of active workflow for project
   * GET /ontology/v1/{project_id}/status
   */
  async getStatus(projectId: string): Promise<WorkflowStatusResponse> {
    return this.makeRequest<WorkflowStatusResponse>(`/${projectId}/status`);
  }

  /**
   * Get questions for active workflow
   * GET /ontology/v1/{project_id}/questions
   */
  async getQuestions(projectId: string): Promise<WorkflowQuestionsResponse> {
    return this.makeRequest<WorkflowQuestionsResponse>(`/${projectId}/questions`);
  }

  /**
   * Submit answers to active workflow
   * POST /ontology/v1/{project_id}/answers
   */
  async submitProjectAnswers(
    projectId: string,
    request: SubmitAnswersRequest
  ): Promise<SubmitAnswersResponse> {
    return this.makeRequest<SubmitAnswersResponse>(`/${projectId}/answers`, {
      method: 'POST',
      body: JSON.stringify(request),
    });
  }

  /**
   * Cancel active workflow
   * POST /ontology/v1/{project_id}/cancel
   */
  async cancelWorkflow(
    projectId: string
  ): Promise<{ workflow_id: string; status: string; message: string; error?: string }> {
    return this.makeRequest<{ workflow_id: string; status: string; message: string; error?: string }>(
      `/${projectId}/cancel`,
      { method: 'POST' }
    );
  }

  /**
   * Poll active workflow status until completion
   */
  async pollStatus(
    projectId: string,
    options: {
      intervalMs?: number;
      timeoutMs?: number;
      onStatusUpdate?: (status: WorkflowStatusResponse) => void;
    } = {}
  ): Promise<WorkflowStatusResponse> {
    const intervalMs = options.intervalMs || 2000; // Default: 2 seconds

    return new Promise((resolve, reject) => {
      const poll = async () => {
        try {
          const status = await this.getStatus(projectId);

          if (options.onStatusUpdate) {
            options.onStatusUpdate(status);
          }

          // Check for errors first - any error should stop polling and surface to UI
          if (status.errors && status.errors.length > 0) {
            const errorMessages = status.errors
              .filter(e => e.severity === 'error')
              .map(e => e.message);
            if (errorMessages.length > 0) {
              console.error('Workflow errors:', errorMessages);
              reject(new Error(errorMessages.join('; ')));
              return;
            }
          }

          if (status.is_complete) {
            resolve(status);
            return;
          }

          setTimeout(poll, intervalMs);
        } catch (error) {
          reject(error);
        }
      };

      poll();
    });
  }

  // ============================================================================
  // Question-by-Question API Methods (Application-Controlled State Machine)
  // ============================================================================

  /**
   * Get the next pending question for a project
   * GET /ontology/v1/{project_id}/questions/next
   */
  async getNextQuestion(
    projectId: string,
    includeSkipped: boolean = false
  ): Promise<GetNextQuestionResponse> {
    const params = includeSkipped ? '?include_skipped=true' : '';
    return this.makeRequest<GetNextQuestionResponse>(
      `/${projectId}/questions/next${params}`
    );
  }

  /**
   * Submit an answer to a specific question
   * POST /ontology/v1/{project_id}/questions/{question_id}/answer
   */
  async answerQuestion(
    projectId: string,
    questionId: string,
    answer: string
  ): Promise<AnswerQuestionResponse> {
    return this.makeRequest<AnswerQuestionResponse>(
      `/${projectId}/questions/${questionId}/answer`,
      {
        method: 'POST',
        body: JSON.stringify({ answer }),
      }
    );
  }

  /**
   * Skip a question (may resurface later)
   * POST /ontology/v1/{project_id}/questions/{question_id}/skip
   */
  async skipQuestion(
    projectId: string,
    questionId: string
  ): Promise<SkipDeleteResponse> {
    return this.makeRequest<SkipDeleteResponse>(
      `/${projectId}/questions/${questionId}/skip`,
      { method: 'POST' }
    );
  }

  /**
   * Delete a question (soft delete - won't be asked again)
   * DELETE /ontology/v1/{project_id}/questions/{question_id}
   */
  async deleteQuestion(
    projectId: string,
    questionId: string
  ): Promise<SkipDeleteResponse> {
    return this.makeRequest<SkipDeleteResponse>(
      `/${projectId}/questions/${questionId}`,
      { method: 'DELETE' }
    );
  }

  // ============================================================================
  // Chat API Methods
  // ============================================================================

  /**
   * Initialize chat session
   * POST /ontology/v1/{project_id}/chat/initialize
   */
  async initializeChat(
    projectId: string
  ): Promise<{
    status: string;
    has_pending_questions: boolean;
    pending_count?: number;
    opening_message?: string;
    has_messages?: boolean;
  }> {
    return this.makeRequest(`/${projectId}/chat/initialize`, {
      method: 'POST',
    });
  }

  /**
   * Get chat history
   * GET /ontology/v1/{project_id}/chat/history
   */
  async getChatHistory(
    projectId: string,
    limit?: number
  ): Promise<{
    messages: ChatMessage[];
    total: number;
  }> {
    const params = limit ? `?limit=${limit}` : '';
    return this.makeRequest(`/${projectId}/chat/history${params}`);
  }

  /**
   * Clear chat history
   * DELETE /ontology/v1/{project_id}/chat/history
   */
  async clearChatHistory(projectId: string): Promise<{ success: boolean; message: string }> {
    return this.makeRequest(`/${projectId}/chat/history`, {
      method: 'DELETE',
    });
  }

  /**
   * Get project knowledge
   * GET /ontology/v1/{project_id}/knowledge
   */
  async getProjectKnowledge(
    projectId: string,
    factType?: string
  ): Promise<{
    knowledge: KnowledgeFact[];
    total: number;
  }> {
    const params = factType ? `?fact_type=${factType}` : '';
    return this.makeRequest(`/${projectId}/knowledge${params}`);
  }

  /**
   * Send chat message with SSE streaming response
   * POST /ontology/v1/{project_id}/chat/message
   * Returns an EventSource for SSE events
   */
  sendChatMessage(
    projectId: string,
    content: string,
    onEvent: (event: ChatEvent) => void,
    onError: (error: Error) => void,
    onComplete: () => void
  ): AbortController {
    const controller = new AbortController();
    const url = `${ONTOLOGY_BASE_URL}/${projectId}/chat/message`;

    // Send via fetch with SSE handling
    fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ content }),
      credentials: 'include',
      signal: controller.signal,
    })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        const reader = response.body?.getReader();
        if (!reader) {
          throw new Error('No response body');
        }

        const decoder = new TextDecoder();
        let buffer = '';

        let done = false;
        while (!done) {
          const result = await reader.read();
          done = result.done;
          if (done) break;
          const value = result.value;

          buffer += decoder.decode(value, { stream: true });

          // Process complete SSE events (data: ...\n\n)
          const events = buffer.split('\n\n');
          buffer = events.pop() || ''; // Keep incomplete event in buffer

          for (const event of events) {
            if (event.startsWith('data: ')) {
              try {
                const data = JSON.parse(event.slice(6)) as ChatEvent;
                onEvent(data);
                if (data.type === 'done') {
                  onComplete();
                  return;
                }
              } catch {
                console.warn('Failed to parse SSE event:', event);
              }
            }
          }
        }

        onComplete();
      })
      .catch((error) => {
        if (error.name !== 'AbortError') {
          onError(error);
        }
      });

    return controller;
  }
}

// Chat types
export interface ChatMessage {
  id: string;
  role: 'user' | 'assistant' | 'system' | 'tool';
  content: string;
  tool_call_id?: string;
  tool_calls?: ToolCall[];
  created_at: string;
}

export interface ToolCall {
  id: string;
  type: string;
  function: {
    name: string;
    arguments: string;
  };
}

export interface KnowledgeFact {
  id: string;
  project_id: string;
  fact_type: string;
  key: string;
  value: string;
  context?: string;
  created_at: string;
  updated_at: string;
}

export interface ChatEvent {
  type: 'text' | 'tool_call' | 'tool_result' | 'ontology_update' | 'knowledge_stored' | 'done' | 'error';
  content?: string;
  data?: Record<string, unknown>;
}

// Create and export singleton instance
const ontologyApi = new OntologyApiService();
export default ontologyApi;
