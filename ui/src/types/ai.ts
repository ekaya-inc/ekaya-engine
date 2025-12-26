/**
 * AI Configuration Types
 * Used by ProjectDashboard and AI configuration components
 */

export type AIOption = 'byok' | 'community' | 'embedded' | null;

export interface AIConfigForm {
  llmBaseUrl: string;
  llmApiKey: string;
  llmModel: string;
  embeddingBaseUrl: string;
  embeddingApiKey: string;
  embeddingModel: string;
}

export type AIErrorType = 'endpoint' | 'auth' | 'model' | 'unknown' | '';

export interface AITestResult {
  success: boolean;
  message: string;
  llm_success?: boolean;
  llm_message?: string;
  llm_error_type?: AIErrorType;
  llm_response_time_ms?: number;
  embedding_success?: boolean;
  embedding_message?: string;
  embedding_error_type?: AIErrorType;
}

export interface AIOptionConfig {
  llm_base_url: string;
  llm_model: string;
  embedding_url: string;
  embedding_model: string;
  available: boolean;
}

export interface AIOptionsResponse {
  community: AIOptionConfig;
  embedded: AIOptionConfig;
}
