import type { DatasourceSchema, SchemaTable } from '../types';

/**
 * Builds a lookup map from table_name to schema-qualified name (schema.table).
 * This is required because the backend expects schema-qualified table names
 * (e.g., "public.users") when saving selections.
 */
export function buildTableNameToQualified(
  apiTables: SchemaTable[]
): Record<string, string> {
  const lookup: Record<string, string> = {};
  for (const table of apiTables) {
    lookup[table.table_name] = `${table.schema_name}.${table.table_name}`;
  }
  return lookup;
}

interface SelectionState {
  selected: boolean;
  columns: Record<string, boolean>;
}

interface TableWithColumns {
  name: string;
  columns: { name: string }[];
}

/**
 * Builds table and column selection payloads for the save API.
 * Returns schema-qualified table names as required by the backend.
 */
export function buildSelectionPayloads(
  tables: TableWithColumns[],
  selectionState: Record<string, SelectionState>,
  tableNameToQualified: Record<string, string>
): {
  tableSelections: Record<string, boolean>;
  columnSelections: Record<string, string[]>;
} {
  const tableSelections: Record<string, boolean> = {};
  const columnSelections: Record<string, string[]> = {};

  for (const table of tables) {
    const qualifiedName = tableNameToQualified[table.name] ?? table.name;
    const tableState = selectionState[table.name];

    tableSelections[qualifiedName] = tableState?.selected ?? false;

    if (tableState?.selected) {
      const selectedColumns = table.columns
        .filter((col) => tableState.columns[col.name])
        .map((col) => col.name);

      if (selectedColumns.length > 0) {
        columnSelections[qualifiedName] = selectedColumns;
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
