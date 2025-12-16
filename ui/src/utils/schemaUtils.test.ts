import { describe, expect, it } from 'vitest';

import type { SchemaTable } from '../types';

import { buildSelectionPayloads, buildTableNameToQualified } from './schemaUtils';

describe('schemaUtils', () => {
  describe('buildTableNameToQualified', () => {
    it('builds lookup from table_name to schema-qualified name', () => {
      const apiTables: SchemaTable[] = [
        {
          id: '1',
          table_name: 'users',
          schema_name: 'public',
          is_selected: false,
          row_count: 100,
          columns: [],
        },
        {
          id: '2',
          table_name: 'orders',
          schema_name: 'public',
          is_selected: false,
          row_count: 50,
          columns: [],
        },
        {
          id: '3',
          table_name: 'products',
          schema_name: 'inventory',
          is_selected: false,
          row_count: 25,
          columns: [],
        },
      ];

      const result = buildTableNameToQualified(apiTables);

      expect(result).toEqual({
        users: 'public.users',
        orders: 'public.orders',
        products: 'inventory.products',
      });
    });

    it('handles empty array', () => {
      const result = buildTableNameToQualified([]);
      expect(result).toEqual({});
    });
  });

  describe('buildSelectionPayloads', () => {
    it('builds schema-qualified table selections', () => {
      const tables = [
        { name: 'users', columns: [{ name: 'id' }, { name: 'email' }] },
        { name: 'orders', columns: [{ name: 'id' }, { name: 'total' }] },
      ];

      const selectionState = {
        users: { selected: true, columns: { id: true, email: true } },
        orders: { selected: false, columns: { id: false, total: false } },
      };

      const tableNameToQualified = {
        users: 'public.users',
        orders: 'public.orders',
      };

      const { tableSelections, columnSelections } = buildSelectionPayloads(
        tables,
        selectionState,
        tableNameToQualified
      );

      // Table keys should be schema-qualified
      expect(tableSelections).toEqual({
        'public.users': true,
        'public.orders': false,
      });

      // Column selections should only include selected tables
      expect(columnSelections).toEqual({
        'public.users': ['id', 'email'],
      });
    });

    it('excludes unselected columns from columnSelections', () => {
      const tables = [
        {
          name: 'users',
          columns: [
            { name: 'id' },
            { name: 'email' },
            { name: 'password_hash' },
          ],
        },
      ];

      const selectionState = {
        users: {
          selected: true,
          columns: { id: true, email: true, password_hash: false },
        },
      };

      const tableNameToQualified = { users: 'public.users' };

      const { columnSelections } = buildSelectionPayloads(
        tables,
        selectionState,
        tableNameToQualified
      );

      // password_hash should not be included
      expect(columnSelections['public.users']).toEqual(['id', 'email']);
      expect(columnSelections['public.users']).not.toContain('password_hash');
    });

    it('falls back to unqualified name if not in lookup', () => {
      const tables = [{ name: 'unknown_table', columns: [{ name: 'id' }] }];

      const selectionState = {
        unknown_table: { selected: true, columns: { id: true } },
      };

      const tableNameToQualified = {}; // Empty lookup

      const { tableSelections, columnSelections } = buildSelectionPayloads(
        tables,
        selectionState,
        tableNameToQualified
      );

      // Should fall back to unqualified name
      expect(tableSelections).toEqual({ unknown_table: true });
      expect(columnSelections).toEqual({ unknown_table: ['id'] });
    });

    it('handles missing selection state for table', () => {
      const tables = [{ name: 'users', columns: [{ name: 'id' }] }];
      const selectionState = {}; // No selection state
      const tableNameToQualified = { users: 'public.users' };

      const { tableSelections, columnSelections } = buildSelectionPayloads(
        tables,
        selectionState,
        tableNameToQualified
      );

      expect(tableSelections).toEqual({ 'public.users': false });
      expect(columnSelections).toEqual({}); // No columns for unselected table
    });
  });
});
