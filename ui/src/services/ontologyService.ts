/**
 * Ontology Service
 * Communicates with the backend ontology extraction API
 * Transforms backend responses to UI types for the work queue display
 */

import type {
  EntityProgressResponse,
  EntityStatus,
  ExtractionQuestion,
  OntologyWorkflowStatus,
  TaskProgressResponse,
  TaskStatus,
  WorkflowProgress,
  WorkflowQuestion,
  WorkflowState,
  WorkflowStatusResponse,
  WorkItem,
  WorkQueueTaskItem,
} from '../types';

import ontologyApi from './ontologyApi';

type StatusUpdateCallback = (status: OntologyWorkflowStatus) => void;

/**
 * Transform backend entity_queue to UI WorkItem[] (LEGACY)
 */
function transformEntityQueue(entityQueue?: EntityProgressResponse[]): WorkItem[] {
  if (!entityQueue) return [];

  return entityQueue.map((entity) => {
    const item: WorkItem = {
      entityName: entity.entity_name,
      status: entity.status as EntityStatus,
    };
    if (entity.token_count !== undefined) {
      item.tokenCount = entity.token_count;
    }
    if (entity.last_updated !== undefined) {
      item.lastUpdated = entity.last_updated;
    }
    if (entity.error_message !== undefined) {
      item.errorMessage = entity.error_message;
    }
    return item;
  });
}

/**
 * Transform backend task_queue to UI WorkQueueTaskItem[] (NEW)
 */
function transformTaskQueue(taskQueue?: TaskProgressResponse[]): WorkQueueTaskItem[] {
  if (!taskQueue) return [];

  return taskQueue.map((task) => {
    const item: WorkQueueTaskItem = {
      id: task.id,
      name: task.name,
      status: task.status as TaskStatus,
      requiresLlm: task.requires_llm,
    };
    if (task.started_at !== undefined) {
      item.startedAt = task.started_at;
    }
    if (task.completed_at !== undefined) {
      item.completedAt = task.completed_at;
    }
    if (task.error_message !== undefined) {
      item.errorMessage = task.error_message;
    }
    return item;
  });
}

/**
 * Transform backend questions to UI ExtractionQuestion[]
 */
function transformQuestions(questions: WorkflowQuestion[]): ExtractionQuestion[] {
  return questions.map((q) => {
    const question: ExtractionQuestion = {
      id: q.id,
      text: q.text,
      affects: q.affects ?? [],
      isSubmitted: q.answered_at !== undefined && q.answered_at !== null,
    };
    // Note: answer is not set from backend - it's populated locally when user submits
    return question;
  });
}

/**
 * Derive UI workflow state from backend status
 */
function deriveWorkflowState(status: WorkflowStatusResponse): WorkflowState {
  // Check for awaiting_input state (questions need answers)
  if (status.current_phase === 'awaiting_input' || status.status_label === 'Awaiting Input') {
    return 'awaiting_input';
  }

  if (status.is_complete) {
    return 'complete';
  }

  // Ontology data exists without active workflow = complete state
  if (status.ontology_ready || status.has_result) {
    return 'complete';
  }

  switch (status.current_phase) {
    case 'initialized':
      return 'initializing';
    case 'schema_analysis':
    case 'data_profiling':
    case 'tier1_generation':
    case 'tiered_ontology_generation':
    case 'confidence_loop':
      return 'building';
    default:
      if (status.status_type === 'processing') {
        return 'building';
      }
      return 'idle';
  }
}

/**
 * Transform backend status response to UI status format
 */
function transformStatusToUI(
  statusResponse: WorkflowStatusResponse,
  questions: ExtractionQuestion[]
): OntologyWorkflowStatus {
  const workQueue = transformEntityQueue(statusResponse.entity_queue);
  const taskQueue = transformTaskQueue(statusResponse.task_queue);
  const pendingQuestions = questions.filter((q) => !q.isSubmitted);

  // Use server-provided entity counts (from workflow progress)
  // Never fall back to task counts - they measure different things
  const progress: WorkflowProgress = {
    state: deriveWorkflowState(statusResponse),
    current: statusResponse.current_entity ?? 0,
    total: statusResponse.total_entities ?? 0,
    // Note: tokensPerSecond and timeRemainingMs not provided by backend yet
  };

  const result: OntologyWorkflowStatus = {
    progress,
    workQueue,
    taskQueue,
    questions,
    pendingQuestionCount: pendingQuestions.length,
    // UX improvement fields
    ontologyReady: statusResponse.ontology_ready ?? false,
  };

  // Only set progressMessage if defined
  if (statusResponse.progress_message !== undefined) {
    result.progressMessage = statusResponse.progress_message;
  }

  if (statusResponse.errors !== undefined) {
    result.errors = statusResponse.errors;
  }
  if (statusResponse.last_error !== undefined) {
    result.lastError = statusResponse.last_error;
  }

  return result;
}

class OntologyService {
  private projectId: string | null = null;
  private status: OntologyWorkflowStatus;
  private onStatusUpdate: StatusUpdateCallback | null = null;
  private pollingInterval: ReturnType<typeof setInterval> | null = null;
  private pollIntervalMs: number = 2000; // Poll every 2 seconds

  constructor() {
    this.status = this.createIdleStatus();
  }

  private createIdleStatus(): OntologyWorkflowStatus {
    return {
      progress: {
        state: 'idle',
        current: 0,
        total: 0,
      },
      workQueue: [],
      taskQueue: [],
      questions: [],
      pendingQuestionCount: 0,
    };
  }

  /**
   * Set the project ID for API calls
   */
  setProjectId(projectId: string): void {
    this.projectId = projectId;
  }

  /**
   * Subscribe to status updates
   */
  subscribe(callback: StatusUpdateCallback): () => void {
    this.onStatusUpdate = callback;
    // Immediately emit current status
    callback(this.status);
    return () => {
      this.onStatusUpdate = null;
    };
  }

  private emitUpdate(): void {
    if (this.onStatusUpdate) {
      this.onStatusUpdate({ ...this.status });
    }
  }

  /**
   * Get current status (for one-time reads)
   */
  getStatus(): OntologyWorkflowStatus {
    return { ...this.status };
  }

  /**
   * Fetch status from backend and update UI
   */
  private async fetchAndUpdateStatus(): Promise<void> {
    if (!this.projectId) {
      console.warn('RealOntologyService: projectId not set');
      return;
    }

    try {
      // Fetch status
      const statusResponse = await ontologyApi.getStatus(this.projectId);

      // Fetch questions if workflow is active or has pending questions
      let questions: ExtractionQuestion[] = [];
      if (!statusResponse.is_complete || (statusResponse.pending_questions_count ?? 0) > 0) {
        try {
          const questionsResponse = await ontologyApi.getQuestions(this.projectId);
          // Flatten questions from priority groups
          const allQuestions: WorkflowQuestion[] = [
            ...(questionsResponse.questions.critical ?? []),
            ...(questionsResponse.questions.high ?? []),
            ...(questionsResponse.questions.medium ?? []),
            ...(questionsResponse.questions.low ?? []),
          ];
          questions = transformQuestions(allQuestions);
        } catch (e) {
          // Questions endpoint may fail if no active workflow
          console.debug('Could not fetch questions:', e);
        }
      }

      // Transform to UI format
      this.status = transformStatusToUI(statusResponse, questions);
      this.emitUpdate();

      // Stop polling if workflow is in a terminal state (not awaiting_input)
      // Keep polling for awaiting_input since answers may come in
      if (statusResponse.is_complete && this.status.progress.state !== 'awaiting_input') {
        this.stopPolling();
      }
    } catch (error) {
      console.error('Error fetching ontology status:', error);
      // On error, set error state so UI can show error screen
      const errorMessage = error instanceof Error && error.message.includes('Failed to fetch')
        ? 'Service is currently down.'
        : 'Unable to connect to the ontology service.';
      this.status = {
        ...this.status,
        progress: {
          ...this.status.progress,
          state: 'error',
        },
        lastError: errorMessage,
      };
      this.emitUpdate();
      // Stop polling on connection error - user will need to retry
      this.stopPolling();
    }
  }

  /**
   * Start polling for status updates
   */
  private startPolling(): void {
    if (this.pollingInterval) {
      clearInterval(this.pollingInterval);
    }

    // Fetch immediately
    void this.fetchAndUpdateStatus();

    // Then poll at interval
    this.pollingInterval = setInterval(() => {
      void this.fetchAndUpdateStatus();
    }, this.pollIntervalMs);
  }

  /**
   * Stop polling
   */
  private stopPolling(): void {
    if (this.pollingInterval) {
      clearInterval(this.pollingInterval);
      this.pollingInterval = null;
    }
  }

  /**
   * Start the extraction workflow
   * Note: Datasource is configured at project level, not passed per-request
   */
  async startExtraction(projectDescription?: string): Promise<void> {
    if (!this.projectId) {
      throw new Error('Project ID not set');
    }

    // Update status to initializing
    this.status = {
      ...this.createIdleStatus(),
      progress: {
        state: 'initializing',
        current: 0,
        total: 0,
        startedAt: new Date().toISOString(),
      },
    };
    this.emitUpdate();

    try {
      // Call backend to start extraction - datasource comes from project config
      await ontologyApi.extractOntology(
        this.projectId,
        projectDescription ? { projectDescription } : undefined
      );

      // Start polling for status updates
      this.startPolling();
    } catch (error) {
      console.error('Failed to start extraction:', error);
      this.status = this.createIdleStatus();
      this.emitUpdate();
      throw error;
    }
  }

  /**
   * Cancel the extraction (equivalent to pause/stop)
   */
  async cancel(): Promise<void> {
    if (!this.projectId) {
      throw new Error('Project ID not set');
    }

    try {
      await ontologyApi.cancelWorkflow(this.projectId);
      this.stopPolling();

      // Update status to idle
      this.status = this.createIdleStatus();
      this.emitUpdate();
    } catch (error) {
      console.error('Failed to cancel workflow:', error);
      throw error;
    }
  }

  /**
   * Refresh the extraction (cancel + start)
   * Used to continue building ontology from existing state
   */
  async refresh(): Promise<void> {
    if (!this.projectId) {
      throw new Error('Project ID not set');
    }

    try {
      // Try to cancel any existing workflow
      try {
        await ontologyApi.cancelWorkflow(this.projectId);
      } catch {
        // Ignore cancel errors (workflow may not exist)
      }

      // Start new extraction
      await this.startExtraction();
    } catch (error) {
      console.error('Failed to refresh extraction:', error);
      throw error;
    }
  }

  /**
   * Delete all ontology data for the project
   */
  async deleteOntology(): Promise<void> {
    if (!this.projectId) {
      throw new Error('Project ID not set');
    }

    try {
      await ontologyApi.deleteOntology(this.projectId);
      this.stopPolling();

      // Reset to idle state
      this.status = this.createIdleStatus();
      this.emitUpdate();
    } catch (error) {
      console.error('Failed to delete ontology:', error);
      throw error;
    }
  }

  /**
   * Submit an answer to a question
   */
  async submitAnswer(questionId: string, answer: string, _file?: File): Promise<void> {
    if (!this.projectId) {
      throw new Error('Project ID not set');
    }

    try {
      await ontologyApi.submitProjectAnswers(this.projectId, {
        answers: [{ question_id: questionId, answer }],
      });

      // Mark question as submitted locally
      const questionIndex = this.status.questions.findIndex((q) => q.id === questionId);
      if (questionIndex >= 0 && this.status.questions[questionIndex]) {
        this.status.questions[questionIndex].isSubmitted = true;
        this.status.questions[questionIndex].answer = answer;
        this.status.pendingQuestionCount = Math.max(0, this.status.pendingQuestionCount - 1);

        // Mark affected entities as updating
        const question = this.status.questions[questionIndex];
        if (question.affects) {
          for (const entityName of question.affects) {
            const entityIndex = this.status.workQueue.findIndex(
              (w) => w.entityName === entityName
            );
            if (entityIndex >= 0 && this.status.workQueue[entityIndex]) {
              const entity = this.status.workQueue[entityIndex];
              if (entity.status === 'complete') {
                entity.status = 'updating';
              }
            }
          }
        }

        this.emitUpdate();
      }

      // Continue polling to get updated status
      this.startPolling();
    } catch (error) {
      console.error('Failed to submit answer:', error);
      throw error;
    }
  }

  /**
   * Stop and clean up
   */
  stop(): void {
    this.stopPolling();
    this.status = this.createIdleStatus();
    this.emitUpdate();
  }

  /**
   * Initialize service for a project and check for existing workflow
   */
  async initialize(projectId: string): Promise<void> {
    this.setProjectId(projectId);

    try {
      // Check if there's an existing workflow
      const statusResponse = await ontologyApi.getStatus(projectId);

      // If workflow exists and is not complete, start polling
      if (statusResponse.workflow_id && !statusResponse.is_complete) {
        this.startPolling();
      } else if (statusResponse.workflow_id || statusResponse.ontology_ready || statusResponse.has_result) {
        // Workflow complete OR no workflow but ontology data exists - fetch status once
        await this.fetchAndUpdateStatus();
      }
    } catch {
      // No existing workflow, stay in idle state
      this.status = this.createIdleStatus();
      this.emitUpdate();
    }
  }

  /**
   * Get pending question count (for dashboard badge)
   */
  getPendingQuestionCount(): number {
    return this.status.pendingQuestionCount;
  }
}

// Export pure transform functions for testing
export { transformEntityQueue, transformTaskQueue, transformQuestions };

// Export singleton instance
export const ontologyService = new OntologyService();
export default ontologyService;
