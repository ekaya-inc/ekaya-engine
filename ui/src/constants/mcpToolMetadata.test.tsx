import { describe, expect, it } from 'vitest';

import { TOOL_GROUP_IDS, TOOL_GROUP_SUB_OPTIONS } from './mcpToolMetadata';

describe('mcpToolMetadata', () => {
  describe('TOOL_GROUP_IDS', () => {
    it('contains all expected tool group IDs', () => {
      expect(TOOL_GROUP_IDS.USER).toBe('user');
      expect(TOOL_GROUP_IDS.DEVELOPER).toBe('developer');
      expect(TOOL_GROUP_IDS.AGENT).toBe('agent_tools');
    });

    it('has exactly 3 tool group IDs', () => {
      const ids = Object.keys(TOOL_GROUP_IDS);
      expect(ids).toHaveLength(3);
    });
  });

  describe('TOOL_GROUP_SUB_OPTIONS', () => {
    it('contains user tools sub-options', () => {
      expect(TOOL_GROUP_SUB_OPTIONS.ALLOW_ONTOLOGY_MAINTENANCE).toBe('allowOntologyMaintenance');
    });

    it('contains developer tools sub-options', () => {
      expect(TOOL_GROUP_SUB_OPTIONS.ADD_QUERY_TOOLS).toBe('addQueryTools');
      expect(TOOL_GROUP_SUB_OPTIONS.ADD_ONTOLOGY_MAINTENANCE).toBe('addOntologyMaintenance');
    });

    it('has exactly 3 sub-options', () => {
      const options = Object.keys(TOOL_GROUP_SUB_OPTIONS);
      expect(options).toHaveLength(3);
    });
  });
});
