import { AlertCircle, ArrowRight, Loader2 } from 'lucide-react';
import { useState, useEffect, useMemo } from 'react';

import engineApi from '../services/engineApi';
import type { DatasourceSchema, SchemaTable, RelationshipDetail } from '../types';

import { Button } from './ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog';

interface AddRelationshipDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectId: string;
  datasourceId: string;
  schema: DatasourceSchema | null;
  onRelationshipAdded: (relationship: RelationshipDetail) => void;
}

interface ColumnOption {
  tableName: string;
  columnName: string;
  dataType: string;
}

/**
 * Dialog for manually adding a relationship between two columns.
 * Allows selecting source and target table/column from the selected schema.
 */
export const AddRelationshipDialog = ({
  open,
  onOpenChange,
  projectId,
  datasourceId,
  schema,
  onRelationshipAdded,
}: AddRelationshipDialogProps) => {
  // Form state
  const [sourceTable, setSourceTable] = useState<string>('');
  const [sourceColumn, setSourceColumn] = useState<string>('');
  const [targetTable, setTargetTable] = useState<string>('');
  const [targetColumn, setTargetColumn] = useState<string>('');

  // UI state
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset form when dialog opens/closes
  useEffect(() => {
    if (!open) {
      setSourceTable('');
      setSourceColumn('');
      setTargetTable('');
      setTargetColumn('');
      setError(null);
    }
  }, [open]);

  // Reset column selection when table changes
  useEffect(() => {
    setSourceColumn('');
  }, [sourceTable]);

  useEffect(() => {
    setTargetColumn('');
  }, [targetTable]);

  // Get tables from schema (only selected tables with selected columns)
  const availableTables = useMemo((): SchemaTable[] => {
    if (!schema?.tables) return [];
    // Filter to tables that have columns (implicitly selected)
    return schema.tables.filter(t => t.columns && t.columns.length > 0);
  }, [schema]);

  // Get columns for source table
  const sourceColumns = useMemo((): ColumnOption[] => {
    if (!sourceTable || !schema?.tables) return [];
    const table = schema.tables.find(t => t.table_name === sourceTable);
    if (!table?.columns) return [];
    return table.columns.map(c => ({
      tableName: table.table_name,
      columnName: c.column_name,
      dataType: c.data_type,
    }));
  }, [sourceTable, schema]);

  // Get columns for target table
  const targetColumns = useMemo((): ColumnOption[] => {
    if (!targetTable || !schema?.tables) return [];
    const table = schema.tables.find(t => t.table_name === targetTable);
    if (!table?.columns) return [];
    return table.columns.map(c => ({
      tableName: table.table_name,
      columnName: c.column_name,
      dataType: c.data_type,
    }));
  }, [targetTable, schema]);

  // Validation
  const isValid = sourceTable && sourceColumn && targetTable && targetColumn;

  const handleSubmit = async (): Promise<void> => {
    if (!isValid) return;

    setIsSubmitting(true);
    setError(null);

    try {
      const response = await engineApi.createRelationship(projectId, datasourceId, {
        source_table: sourceTable,
        source_column: sourceColumn,
        target_table: targetTable,
        target_column: targetColumn,
      });

      if (response.error) {
        setError(response.error);
        return;
      }

      if (response.data) {
        onRelationshipAdded(response.data);
        onOpenChange(false);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to create relationship';
      setError(message);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>Add Relationship</DialogTitle>
          <DialogDescription>
            Create a manual relationship between two columns. The relationship will be analyzed to determine cardinality.
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 py-4">
          {/* Source Selection */}
          <div className="space-y-3">
            <h4 className="text-sm font-medium text-text-primary">Source</h4>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label htmlFor="source-table" className="block text-xs font-medium mb-1 text-text-secondary">
                  Table
                </label>
                <select
                  id="source-table"
                  value={sourceTable}
                  onChange={(e) => setSourceTable(e.target.value)}
                  className="w-full rounded-md border border-border-medium bg-surface-secondary text-text-primary px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-brand-purple focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed"
                  disabled={isSubmitting}
                >
                  <option value="">Select table...</option>
                  {availableTables.map(table => (
                    <option key={table.table_name} value={table.table_name}>
                      {table.table_name}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label htmlFor="source-column" className="block text-xs font-medium mb-1 text-text-secondary">
                  Column
                </label>
                <select
                  id="source-column"
                  value={sourceColumn}
                  onChange={(e) => setSourceColumn(e.target.value)}
                  className="w-full rounded-md border border-border-medium bg-surface-secondary text-text-primary px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-brand-purple focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed"
                  disabled={!sourceTable || isSubmitting}
                >
                  <option value="">Select column...</option>
                  {sourceColumns.map(col => (
                    <option key={col.columnName} value={col.columnName}>
                      {col.columnName} ({col.dataType})
                    </option>
                  ))}
                </select>
              </div>
            </div>
          </div>

          {/* Arrow */}
          <div className="flex justify-center">
            <ArrowRight className="h-5 w-5 text-text-tertiary" />
          </div>

          {/* Target Selection */}
          <div className="space-y-3">
            <h4 className="text-sm font-medium text-text-primary">Target</h4>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label htmlFor="target-table" className="block text-xs font-medium mb-1 text-text-secondary">
                  Table
                </label>
                <select
                  id="target-table"
                  value={targetTable}
                  onChange={(e) => setTargetTable(e.target.value)}
                  className="w-full rounded-md border border-border-medium bg-surface-secondary text-text-primary px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-brand-purple focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed"
                  disabled={isSubmitting}
                >
                  <option value="">Select table...</option>
                  {availableTables.map(table => (
                    <option key={table.table_name} value={table.table_name}>
                      {table.table_name}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label htmlFor="target-column" className="block text-xs font-medium mb-1 text-text-secondary">
                  Column
                </label>
                <select
                  id="target-column"
                  value={targetColumn}
                  onChange={(e) => setTargetColumn(e.target.value)}
                  className="w-full rounded-md border border-border-medium bg-surface-secondary text-text-primary px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-brand-purple focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed"
                  disabled={!targetTable || isSubmitting}
                >
                  <option value="">Select column...</option>
                  {targetColumns.map(col => (
                    <option key={col.columnName} value={col.columnName}>
                      {col.columnName} ({col.dataType})
                    </option>
                  ))}
                </select>
              </div>
            </div>
          </div>

          {/* Error Display */}
          {error && (
            <div className="flex items-center gap-2 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/20 dark:text-red-400">
              <AlertCircle className="h-4 w-4 flex-shrink-0" />
              <span>{error}</span>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={isSubmitting}
          >
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!isValid || isSubmitting}
          >
            {isSubmitting ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Creating...
              </>
            ) : (
              'Add Relationship'
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default AddRelationshipDialog;
