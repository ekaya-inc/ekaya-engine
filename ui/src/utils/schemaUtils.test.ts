import { describe, expect, it } from 'vitest';

import type { SchemaTable } from '../types';

import { buildSelectionPayloads } from './schemaUtils';

describe('schemaUtils', () => {
  describe('buildSelectionPayloads', () => {
    it('builds selection payloads using table and column IDs', () => {
      const apiTables: SchemaTable[] = [
        {
          id: 'table-uuid-1',
          table_name: 'users',
          schema_name: 'public',
          is_selected: false,
          row_count: 100,
          columns: [
            { id: 'col-uuid-1', column_name: 'id', data_type: 'uuid' },
            { id: 'col-uuid-2', column_name: 'email', data_type: 'text' },
          ],
        },
        {
          id: 'table-uuid-2',
          table_name: 'orders',
          schema_name: 'public',
          is_selected: false,
          row_count: 50,
          columns: [
            { id: 'col-uuid-3', column_name: 'id', data_type: 'uuid' },
            { id: 'col-uuid-4', column_name: 'total', data_type: 'numeric' },
          ],
        },
      ];

      const selectionState = {
        users: { selected: true, columns: { id: true, email: true } },
        orders: { selected: false, columns: { id: false, total: false } },
      };

      const { tableSelections, columnSelections } = buildSelectionPayloads(
        apiTables,
        selectionState
      );

      // Table keys should be UUIDs
      expect(tableSelections).toEqual({
        'table-uuid-1': true,
        'table-uuid-2': false,
      });

      // Column selections should use column UUIDs for selected tables only
      expect(columnSelections).toEqual({
        'table-uuid-1': ['col-uuid-1', 'col-uuid-2'],
      });
    });

    it('excludes unselected columns from columnSelections', () => {
      const apiTables: SchemaTable[] = [
        {
          id: 'table-uuid-1',
          table_name: 'users',
          schema_name: 'public',
          is_selected: false,
          row_count: 100,
          columns: [
            { id: 'col-uuid-1', column_name: 'id', data_type: 'uuid' },
            { id: 'col-uuid-2', column_name: 'email', data_type: 'text' },
            { id: 'col-uuid-3', column_name: 'password_hash', data_type: 'text' },
          ],
        },
      ];

      const selectionState = {
        users: {
          selected: true,
          columns: { id: true, email: true, password_hash: false },
        },
      };

      const { columnSelections } = buildSelectionPayloads(
        apiTables,
        selectionState
      );

      // password_hash should not be included (only selected columns)
      expect(columnSelections['table-uuid-1']).toEqual(['col-uuid-1', 'col-uuid-2']);
      expect(columnSelections['table-uuid-1']).not.toContain('col-uuid-3');
    });

    it('handles missing selection state for table', () => {
      const apiTables: SchemaTable[] = [
        {
          id: 'table-uuid-1',
          table_name: 'users',
          schema_name: 'public',
          is_selected: false,
          row_count: 100,
          columns: [
            { id: 'col-uuid-1', column_name: 'id', data_type: 'uuid' },
          ],
        },
      ];

      const selectionState = {}; // No selection state

      const { tableSelections, columnSelections } = buildSelectionPayloads(
        apiTables,
        selectionState
      );

      // Table should be marked as not selected
      expect(tableSelections).toEqual({ 'table-uuid-1': false });
      // No columns for unselected table
      expect(columnSelections).toEqual({});
    });

    it('skips tables without IDs', () => {
      const apiTables: SchemaTable[] = [
        {
          // No id field
          table_name: 'users',
          schema_name: 'public',
          is_selected: false,
          row_count: 100,
          columns: [],
        },
      ];

      const selectionState = {
        users: { selected: true, columns: {} },
      };

      const { tableSelections, columnSelections } = buildSelectionPayloads(
        apiTables,
        selectionState
      );

      // Table without ID should be skipped
      expect(tableSelections).toEqual({});
      expect(columnSelections).toEqual({});
    });

    it('skips columns without IDs', () => {
      const apiTables: SchemaTable[] = [
        {
          id: 'table-uuid-1',
          table_name: 'users',
          schema_name: 'public',
          is_selected: false,
          row_count: 100,
          columns: [
            { id: 'col-uuid-1', column_name: 'id', data_type: 'uuid' },
            { column_name: 'email', data_type: 'text' }, // No id
          ],
        },
      ];

      const selectionState = {
        users: { selected: true, columns: { id: true, email: true } },
      };

      const { columnSelections } = buildSelectionPayloads(
        apiTables,
        selectionState
      );

      // Only column with ID should be included
      expect(columnSelections['table-uuid-1']).toEqual(['col-uuid-1']);
    });

    it('handles empty apiTables', () => {
      const { tableSelections, columnSelections } = buildSelectionPayloads(
        [],
        {}
      );

      expect(tableSelections).toEqual({});
      expect(columnSelections).toEqual({});
    });
  });
});
