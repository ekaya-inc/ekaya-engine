/**
 * Audit page type definitions
 */

// Common paginated response wrapper
export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  limit: number;
  offset: number;
}

// Query Executions tab
export interface QueryExecution {
  id: string;
  project_id: string;
  query_id: string;
  sql: string;
  executed_at: string;
  row_count: number;
  execution_time_ms: number;
  user_id?: string;
  source: string;
  is_modifying: boolean;
  success: boolean;
  error_message?: string;
  query_name?: string;
}

export interface QueryExecutionFilters {
  user_id?: string;
  since?: string;
  until?: string;
  success?: string;
  is_modifying?: string;
  source?: string;
  query_id?: string;
  limit?: number;
  offset?: number;
}

// Ontology Changes tab
export interface OntologyChange {
  id: string;
  project_id: string;
  entity_type: string;
  entity_id: string;
  action: string;
  source: string;
  user_id?: string;
  changed_fields?: Record<string, { old: unknown; new: unknown }>;
  created_at: string;
}

export interface OntologyChangeFilters {
  user_id?: string;
  since?: string;
  until?: string;
  entity_type?: string;
  action?: string;
  source?: string;
  limit?: number;
  offset?: number;
}

// Schema Changes tab
export interface SchemaChange {
  id: string;
  project_id: string;
  change_type: string;
  change_source: string;
  table_name?: string;
  column_name?: string;
  old_value?: Record<string, unknown>;
  new_value?: Record<string, unknown>;
  suggested_action?: string;
  suggested_payload?: Record<string, unknown>;
  status: string;
  reviewed_by?: string;
  reviewed_at?: string;
  created_at: string;
}

export interface SchemaChangeFilters {
  since?: string;
  until?: string;
  change_type?: string;
  status?: string;
  table_name?: string;
  limit?: number;
  offset?: number;
}

// Query Approvals tab
export interface QueryApproval {
  id: string;
  project_id: string;
  datasource_id: string;
  natural_language_prompt: string;
  sql_query: string;
  status: string;
  suggested_by?: string;
  reviewed_by?: string;
  reviewed_at?: string;
  rejection_reason?: string;
  created_at: string;
  updated_at: string;
}

export interface QueryApprovalFilters {
  since?: string;
  until?: string;
  status?: string;
  suggested_by?: string;
  reviewed_by?: string;
  limit?: number;
  offset?: number;
}

// MCP Events tab
export interface MCPAuditEvent {
  id: string;
  project_id: string;
  user_id: string;
  user_email?: string;
  session_id?: string;
  event_type: string;
  tool_name?: string;
  request_params?: Record<string, unknown>;
  natural_language?: string;
  sql_query?: string;
  was_successful: boolean;
  error_message?: string;
  result_summary?: Record<string, unknown>;
  duration_ms?: number;
  security_level: string;
  security_flags?: string[];
  client_info?: Record<string, unknown>;
  created_at: string;
}

export interface MCPAuditEventFilters {
  user_id?: string;
  since?: string;
  until?: string;
  event_type?: string;
  tool_name?: string;
  security_level?: string;
  limit?: number;
  offset?: number;
}

// Audit Alerts
export interface AuditAlert {
  id: string;
  project_id: string;
  alert_type: string;
  severity: string;
  title: string;
  description?: string;
  affected_user_id?: string;
  related_audit_ids?: string[];
  status: string;
  resolved_by?: string;
  resolved_at?: string;
  resolution_notes?: string;
  created_at: string;
  updated_at: string;
}

export interface ResolveAlertRequest {
  resolution: 'dismissed' | 'resolved';
  notes?: string;
}

// Audit Summary
export interface AuditSummary {
  total_query_executions: number;
  failed_query_count: number;
  destructive_query_count: number;
  ontology_changes_count: number;
  pending_schema_changes: number;
  pending_query_approvals: number;
  open_alerts_critical: number;
  open_alerts_warning: number;
  open_alerts_info: number;
}
