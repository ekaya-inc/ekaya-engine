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
          'Include schema exploration, sampling, and query tools. Enables the MCP Client to explore the database and run queries.',
      },
      addOntologyMaintenance: {
        name: 'Add Ontology Maintenance',
        description:
          'Include tools to manage the ontology: create/update entities, relationships, refresh schema, and review pending changes.',
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

// Helper to get metadata for a tool group
export const getToolGroupMetadata = (groupId: string): ToolGroupMetadata | undefined => {
  return TOOL_GROUP_METADATA[groupId];
};
