import type { DatasourceSchema, SchemaTable } from '../types';

interface SelectionState {
  selected: boolean;
  columns: Record<string, boolean>;
}

/**
 * Builds table and column selection payloads for the save API using IDs.
 * Maps from table/column names (used in UI state) to their UUIDs (required by API).
 *
 * @param apiTables - Tables from the API response (contains IDs)
 * @param selectionState - UI selection state keyed by table name
 * @returns Payloads with table IDs and column IDs
 */
export function buildSelectionPayloads(
  apiTables: SchemaTable[],
  selectionState: Record<string, SelectionState>
): {
  tableSelections: Record<string, boolean>;
  columnSelections: Record<string, string[]>;
} {
  const tableSelections: Record<string, boolean> = {};
  const columnSelections: Record<string, string[]> = {};

  for (const apiTable of apiTables) {
    const tableId = apiTable.id;
    if (!tableId) {
      console.warn(`Table ${apiTable.table_name} has no ID, skipping`);
      continue;
    }

    const tableState = selectionState[apiTable.table_name];
    const isTableSelected = tableState?.selected ?? false;

    tableSelections[tableId] = isTableSelected;

    if (isTableSelected && tableState) {
      const selectedColumnIds: string[] = [];

      for (const apiColumn of apiTable.columns) {
        const columnId = apiColumn.id;
        if (!columnId) {
          console.warn(`Column ${apiColumn.column_name} in table ${apiTable.table_name} has no ID, skipping`);
          continue;
        }

        if (tableState.columns[apiColumn.column_name]) {
          selectedColumnIds.push(columnId);
        }
      }

      if (selectedColumnIds.length > 0) {
        columnSelections[tableId] = selectedColumnIds;
      }
    }
  }

  return { tableSelections, columnSelections };
}

/**
 * Transform DatasourceSchema to CodeMirror SQLNamespace format for autocomplete.
 * Includes both schema-qualified and unqualified table names for flexibility.
 *
 * @example
 * // Input
 * { tables: [{ table_name: "users", schema_name: "public", columns: [{ column_name: "id" }] }] }
 * // Output
 * { "public.users": ["id"], "users": ["id"] }
 */
export function toCodeMirrorSchema(
  schema: DatasourceSchema | null | undefined
): Record<string, readonly string[]> {
  if (!schema?.tables) return {};

  const result: Record<string, string[]> = {};

  for (const table of schema.tables) {
    const columns = table.columns.map((c) => c.column_name);

    // Always add unqualified table name
    result[table.table_name] = columns;

    // Add schema-qualified name if schema_name exists
    if (table.schema_name) {
      result[`${table.schema_name}.${table.table_name}`] = columns;
    }
  }

  return result;
}
