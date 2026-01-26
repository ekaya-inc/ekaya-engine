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
      expect(TOOL_GROUP_IDS.CUSTOM).toBe('custom');
    });

    it('has exactly 4 tool group IDs', () => {
      const ids = Object.keys(TOOL_GROUP_IDS);
      expect(ids).toHaveLength(4);
    });
  });

  describe('TOOL_GROUP_METADATA', () => {
    it('has metadata for developer tools with sub-options', () => {
      const metadata = TOOL_GROUP_METADATA[TOOL_GROUP_IDS.DEVELOPER];
      if (metadata === undefined) {
        throw new Error('Expected metadata to be defined');
      }
      expect(metadata.name).toBe('Developer Tools');
      expect(metadata.description).toBeTruthy();
      // Warning is at top level (execute is now always included when developer is enabled)
      expect(metadata.warning).toBeDefined();
      expect(metadata.warning).toContain('destructive operations');
      // Sub-options for adding query tools and ontology maintenance
      expect(metadata.subOptions).toBeDefined();
      expect(metadata.subOptions?.addQueryTools).toBeDefined();
      expect(metadata.subOptions?.addOntologyMaintenance).toBeDefined();
    });

    it('has metadata for business user tools', () => {
      const metadata = TOOL_GROUP_METADATA[TOOL_GROUP_IDS.APPROVED_QUERIES];
      if (metadata === undefined) {
        throw new Error('Expected metadata to be defined');
      }
      expect(metadata.name).toBe('Business User Tools');
      expect(metadata.description).toBeTruthy();
      expect(metadata.subOptions).toBeDefined();
      expect(metadata.subOptions?.allowOntologyMaintenance).toBeDefined();
    });

    it('has metadata for agent tools', () => {
      const metadata = TOOL_GROUP_METADATA[TOOL_GROUP_IDS.AGENT_TOOLS];
      if (metadata === undefined) {
        throw new Error('Expected metadata to be defined');
      }
      expect(metadata.name).toBe('Agent Tools');
      expect(metadata.description).toContain('AI Agents');
      expect(metadata.description).toContain('Pre-Approved Queries');
      // Warning is now rendered inline in the component, not in metadata
      expect(metadata.warning).toBeUndefined();
    });

    it('agent tools has no subOptions', () => {
      const metadata = TOOL_GROUP_METADATA[TOOL_GROUP_IDS.AGENT_TOOLS];
      if (metadata === undefined) {
        throw new Error('Expected metadata to be defined');
      }
      expect(metadata.subOptions).toBeUndefined();
    });

    it('has metadata for custom tools', () => {
      const metadata = TOOL_GROUP_METADATA[TOOL_GROUP_IDS.CUSTOM];
      if (metadata === undefined) {
        throw new Error('Expected metadata to be defined');
      }
      expect(metadata.name).toBe('Custom Tools');
      expect(metadata.description).toBeTruthy();
      expect(metadata.subOptions).toBeUndefined();
    });

    it('all tool group IDs have corresponding metadata', () => {
      for (const id of Object.values(TOOL_GROUP_IDS)) {
        const metadata = TOOL_GROUP_METADATA[id];
        expect(metadata).toBeDefined();
        if (metadata !== undefined) {
          expect(metadata.name).toBeTruthy();
          expect(metadata.description).toBeTruthy();
        }
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

    it('returns metadata for custom', () => {
      const metadata = getToolGroupMetadata('custom');
      expect(metadata).toBeDefined();
      expect(metadata?.name).toBe('Custom Tools');
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
