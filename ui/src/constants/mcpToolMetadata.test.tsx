import { describe, expect, it } from 'vitest';

import { TOOL_GROUP_IDS, TOOL_GROUP_SUB_OPTIONS } from './mcpToolMetadata';

describe('mcpToolMetadata', () => {
  describe('TOOL_GROUP_IDS', () => {
    it('contains all expected tool group IDs', () => {
      expect(TOOL_GROUP_IDS.AGENT).toBe('agent_tools');
      expect(TOOL_GROUP_IDS.TOOLS).toBe('tools');
    });

    it('has exactly 2 tool group IDs', () => {
      const ids = Object.keys(TOOL_GROUP_IDS);
      expect(ids).toHaveLength(2);
    });
  });

  describe('TOOL_GROUP_SUB_OPTIONS', () => {
    it('contains per-app tool toggle sub-options', () => {
      expect(TOOL_GROUP_SUB_OPTIONS.ADD_DIRECT_DATABASE_ACCESS).toBe('addDirectDatabaseAccess');
      expect(TOOL_GROUP_SUB_OPTIONS.ADD_ONTOLOGY_MAINTENANCE_TOOLS).toBe('addOntologyMaintenanceTools');
      expect(TOOL_GROUP_SUB_OPTIONS.ADD_ONTOLOGY_SUGGESTIONS).toBe('addOntologySuggestions');
      expect(TOOL_GROUP_SUB_OPTIONS.ADD_APPROVAL_TOOLS).toBe('addApprovalTools');
      expect(TOOL_GROUP_SUB_OPTIONS.ADD_REQUEST_TOOLS).toBe('addRequestTools');
    });

    it('has exactly 5 sub-options', () => {
      const options = Object.keys(TOOL_GROUP_SUB_OPTIONS);
      expect(options).toHaveLength(5);
    });
  });
});
