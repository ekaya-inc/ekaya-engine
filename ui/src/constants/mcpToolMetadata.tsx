// MCP tool group metadata - UI strings for the MCP Server configuration page.
// Backend returns only state (enabled, enableExecute, etc.); this file defines display text.

import type { ReactNode } from 'react';

export interface SubOptionMetadata {
  name: string;
  description?: ReactNode;
  warning?: string;
  tip?: ReactNode;
}

export interface ToolGroupMetadata {
  name: string;
  description: ReactNode;
  warning?: string;
  subOptions?: Record<string, SubOptionMetadata>;
}

export const TOOL_GROUP_IDS = {
  DEVELOPER: 'developer',
  APPROVED_QUERIES: 'approved_queries',
  AGENT_TOOLS: 'agent_tools',
  CUSTOM: 'custom',
} as const;

export const TOOL_GROUP_METADATA: Record<string, ToolGroupMetadata> = {
  [TOOL_GROUP_IDS.DEVELOPER]: {
    name: 'Developer Tools',
    description:
      'Enable raw access to the Datasource and Schema. This is intended for developers building applications or data engineers building ETL pipelines.',
    warning:
      'The MCP Client will have direct access to the Datasource using the supplied credentials -- this access includes potentially destructive operations. Back up the data before allowing AI to modify it.',
    subOptions: {
      addQueryTools: {
        name: 'Add Query Tools',
        description:
          'Include schema exploration, sampling, and ontology maintenance tools. Enables the MCP Client to explore and improve the data model.',
      },
      addOntologyQuestions: {
        name: 'Add Ontology Questions',
        description:
          'Include tools to review and answer questions generated during ontology extraction. Use this to refine the business ontology.',
        tip: (
          <div>
            <span className="font-semibold">Pro Tip:</span> Have AI answer questions about your Ontology{' '}
            <details className="inline">
              <summary className="inline cursor-pointer underline">(more info)</summary>
              <p className="mt-2 font-normal">
                After you have extracted your Ontology there might be questions that Ekaya cannot answer from the database schema and values alone. Connect your IDE to the MCP Server so that your LLM can answer questions by reviewing your codebase or other project documents saving you time.
              </p>
            </details>
          </div>
        ),
      },
    },
  },
  [TOOL_GROUP_IDS.APPROVED_QUERIES]: {
    name: 'Business User Tools',
    description:
      'Enable pre-approved SQL queries and ad-hoc query capabilities for business users. The MCP Client can use the ontology to craft SQL for ad-hoc requests.',
    subOptions: {
      allowOntologyMaintenance: {
        name: 'Allow Usage to Improve Ontology [RECOMMENDED]',
        description:
          'Enable the MCP Client to update entities, relationships, and glossary terms as it learns from user interactions. This helps improve query accuracy over time.',
      },
    },
  },
  [TOOL_GROUP_IDS.AGENT_TOOLS]: {
    name: 'Agent Tools',
    description:
      'Enable AI Agents to access the database safely and securely with logging and auditing capabilities. AI Agents can only use the enabled Pre-Approved Queries so that you have full control over access.',
  },
  [TOOL_GROUP_IDS.CUSTOM]: {
    name: 'Custom Tools',
    description:
      'Select individual tools to expose to the MCP Client. Use this for fine-grained control over tool access.',
  },
};

// Canonical tool order - tools are displayed in this order in the UI
// This must match AllToolsOrdered in the backend (pkg/services/mcp_tool_loadouts.go)
export const ALL_TOOLS_ORDERED = [
  // Default
  { name: 'health', description: 'Server health check' },

  // Developer Core
  { name: 'echo', description: 'Echo back input message for testing' },
  { name: 'execute', description: 'Execute DDL/DML statements' },

  // Query tools
  { name: 'validate', description: 'Check SQL syntax without executing' },
  { name: 'query', description: 'Execute read-only SQL SELECT statements' },
  { name: 'explain_query', description: 'Analyze SQL query performance using EXPLAIN ANALYZE' },
  { name: 'list_approved_queries', description: 'List pre-approved SQL queries' },
  { name: 'execute_approved_query', description: 'Execute a pre-approved query by ID' },
  { name: 'suggest_approved_query', description: 'Suggest a reusable parameterized query for approval' },
  { name: 'search_schema', description: 'Full-text search across tables, columns, and entities' },
  { name: 'get_schema', description: 'Get database schema with entity semantics' },
  { name: 'get_context', description: 'Get unified database context with progressive depth' },
  { name: 'get_entity', description: 'Retrieve full entity details including aliases and relationships' },
  { name: 'get_glossary_sql', description: 'Get SQL definition for a business term' },
  { name: 'get_ontology', description: 'Get business ontology for query generation' },
  { name: 'get_query_history', description: 'Get recent query execution history' },
  { name: 'list_glossary', description: 'List all business glossary terms' },
  { name: 'probe_column', description: 'Deep-dive into specific column with statistics and joinability' },
  { name: 'probe_columns', description: 'Batch variant of probe_column for multiple columns' },
  { name: 'probe_relationship', description: 'Deep-dive into relationships between entities' },
  { name: 'sample', description: 'Quick data preview from a table' },

  // Ontology Questions
  { name: 'list_ontology_questions', description: 'List ontology questions with filtering' },
  { name: 'dismiss_ontology_question', description: 'Mark a question as not worth pursuing' },
  { name: 'escalate_ontology_question', description: 'Mark a question as requiring human domain knowledge' },
  { name: 'resolve_ontology_question', description: 'Mark an ontology question as resolved' },
  { name: 'skip_ontology_question', description: 'Mark a question as skipped for revisiting later' },

  // Ontology Maintenance
  { name: 'update_column', description: 'Add or update semantic information about a column' },
  { name: 'update_entity', description: 'Create or update entity metadata' },
  { name: 'update_glossary_term', description: 'Create or update a business glossary term' },
  { name: 'update_project_knowledge', description: 'Create or update domain facts' },
  { name: 'update_relationship', description: 'Create or update relationship between entities' },
  { name: 'delete_column_metadata', description: 'Clear custom metadata for a column' },
  { name: 'delete_entity', description: 'Remove an entity that was incorrectly identified' },
  { name: 'delete_glossary_term', description: 'Delete a business glossary term' },
  { name: 'delete_project_knowledge', description: 'Remove incorrect or outdated domain facts' },
  { name: 'delete_relationship', description: 'Remove a relationship that was incorrectly identified' },
] as const;

// Tool order lookup for sorting
export const getToolOrder = (toolName: string): number => {
  const index = ALL_TOOLS_ORDERED.findIndex((t) => t.name === toolName);
  return index >= 0 ? index : ALL_TOOLS_ORDERED.length;
};

// Helper to get metadata for a tool group
export const getToolGroupMetadata = (groupId: string): ToolGroupMetadata | undefined => {
  return TOOL_GROUP_METADATA[groupId];
};
