// MCP tool group metadata - UI strings for the MCP Server configuration page.
// Backend returns only state (enabled, enableExecute, etc.); this file defines display text.

import type { ReactNode } from 'react';

export interface SubOptionMetadata {
  name: string;
  description?: ReactNode;
  warning?: string;
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
} as const;

export const TOOL_GROUP_METADATA: Record<string, ToolGroupMetadata> = {
  [TOOL_GROUP_IDS.DEVELOPER]: {
    name: 'Developer Tools',
    description:
      'Enable raw access to the Datasource and Schema. This is intended for developers building applications or data engineers building ETL pipelines.',
    warning: 'This setting is NOT recommended for business end users doing analytics.',
    subOptions: {
      enableExecute: {
        name: 'Enable Execute',
        warning:
          'The MCP Client will have direct access to the Datasource using the supplied credentials -- this access includes potentially destructive operations. Back up the data before allowing AI to modify it.',
      },
    },
  },
  [TOOL_GROUP_IDS.APPROVED_QUERIES]: {
    name: 'Pre-Approved Queries',
    description:
      'Enable pre-approved SQL queries that can be executed by the MCP client. Queries must be created and enabled in the Pre-Approved Queries section.',
    subOptions: {
      forceMode: {
        name: 'FORCE all access through Pre-Approved Queries',
        description:
          'When enabled, MCP clients can only execute Pre-Approved Queries. This is the safest way to enable AI access to data but it is also the least flexible. Enable this option if you want restricted scope access to set UI features, reports or processes. This will not support ad-hoc requests from business end users.',
        warning: 'Enabling this will disable Developer Tools.',
      },
      allowClientSuggestions: {
        name: 'Allow MCP Client to suggest Queries for the Pre-Approved List',
        description:
          'Allow the MCP Client to suggest new Queries to be added to the Pre-Approved list after your review.',
      },
    },
  },
};

// Helper to get metadata for a tool group
export const getToolGroupMetadata = (groupId: string): ToolGroupMetadata | undefined => {
  return TOOL_GROUP_METADATA[groupId];
};
