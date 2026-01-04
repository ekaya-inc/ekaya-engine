import { describe, expect, it } from 'vitest';

import {
  TOOL_GROUP_IDS,
  TOOL_GROUP_METADATA,
  getToolGroupMetadata,
} from './mcpToolMetadata';

describe('mcpToolMetadata', () => {
  describe('TOOL_GROUP_IDS', () => {
    it('contains all expected tool group IDs', () => {
      expect(TOOL_GROUP_IDS.DEVELOPER).toBe('developer');
      expect(TOOL_GROUP_IDS.APPROVED_QUERIES).toBe('approved_queries');
      expect(TOOL_GROUP_IDS.AGENT_TOOLS).toBe('agent_tools');
    });

    it('has exactly 3 tool group IDs', () => {
      const ids = Object.keys(TOOL_GROUP_IDS);
      expect(ids).toHaveLength(3);
    });
  });

  describe('TOOL_GROUP_METADATA', () => {
    it('has metadata for developer tools', () => {
      const metadata = TOOL_GROUP_METADATA[TOOL_GROUP_IDS.DEVELOPER];
      expect(metadata).toBeDefined();
      expect(metadata.name).toBe('Developer Tools');
      expect(metadata.description).toBeTruthy();
      expect(metadata.warning).toBeTruthy();
      expect(metadata.subOptions).toBeDefined();
      expect(metadata.subOptions?.enableExecute).toBeDefined();
    });

    it('has metadata for approved queries', () => {
      const metadata = TOOL_GROUP_METADATA[TOOL_GROUP_IDS.APPROVED_QUERIES];
      expect(metadata).toBeDefined();
      expect(metadata.name).toBe('Pre-Approved Queries');
      expect(metadata.description).toBeTruthy();
      expect(metadata.subOptions).toBeDefined();
      expect(metadata.subOptions?.forceMode).toBeDefined();
      expect(metadata.subOptions?.allowClientSuggestions).toBeDefined();
    });

    it('has metadata for agent tools', () => {
      const metadata = TOOL_GROUP_METADATA[TOOL_GROUP_IDS.AGENT_TOOLS];
      expect(metadata).toBeDefined();
      expect(metadata.name).toBe('Agent Tools');
      expect(metadata.description).toContain('AI Agents');
      expect(metadata.description).toContain('Pre-Approved Queries');
      expect(metadata.warning).toContain('API key authentication');
    });

    it('agent tools has no subOptions', () => {
      const metadata = TOOL_GROUP_METADATA[TOOL_GROUP_IDS.AGENT_TOOLS];
      expect(metadata.subOptions).toBeUndefined();
    });

    it('all tool group IDs have corresponding metadata', () => {
      for (const id of Object.values(TOOL_GROUP_IDS)) {
        expect(TOOL_GROUP_METADATA[id]).toBeDefined();
        expect(TOOL_GROUP_METADATA[id].name).toBeTruthy();
        expect(TOOL_GROUP_METADATA[id].description).toBeTruthy();
      }
    });
  });

  describe('getToolGroupMetadata', () => {
    it('returns metadata for valid group ID', () => {
      const metadata = getToolGroupMetadata(TOOL_GROUP_IDS.DEVELOPER);
      expect(metadata).toBe(TOOL_GROUP_METADATA[TOOL_GROUP_IDS.DEVELOPER]);
    });

    it('returns metadata for agent_tools', () => {
      const metadata = getToolGroupMetadata('agent_tools');
      expect(metadata).toBeDefined();
      expect(metadata?.name).toBe('Agent Tools');
    });

    it('returns undefined for unknown group ID', () => {
      const metadata = getToolGroupMetadata('unknown_group');
      expect(metadata).toBeUndefined();
    });

    it('returns undefined for empty string', () => {
      const metadata = getToolGroupMetadata('');
      expect(metadata).toBeUndefined();
    });
  });
});
