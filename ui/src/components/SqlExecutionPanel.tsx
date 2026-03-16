import { Loader2, Play } from 'lucide-react';
import { useState } from 'react';

import { useToast } from '../hooks/useToast';
import engineApi from '../services/engineApi';
import type { ExecuteQueryResponse } from '../types';

import { QueryResultsTable } from './QueryResultsTable';
import { Button } from './ui/Button';

interface SqlExecutionPanelProps {
  projectId: string;
  datasourceId?: string | undefined;
  sql: string;
  buttonLabel?: string;
}

export function SqlExecutionPanel({
  projectId,
  datasourceId,
  sql,
  buttonLabel = 'Execute Query',
}: SqlExecutionPanelProps) {
  const { toast } = useToast();
  const [isExecuting, setIsExecuting] = useState(false);
  const [results, setResults] = useState<ExecuteQueryResponse | null>(null);

  const hasExecutableSql = sql.trim().length > 0;
  const canExecute = hasExecutableSql && !!datasourceId;

  const handleExecute = async (): Promise<void> => {
    if (!datasourceId || !hasExecutableSql) {
      return;
    }

    setIsExecuting(true);

    try {
      const response = await engineApi.testQuery(projectId, datasourceId, {
        sql_query: sql,
        limit: 100,
      });

      if (response.success && response.data) {
        setResults(response.data);
        toast({
          title: 'Query executed successfully',
          description: `Returned ${response.data.row_count} rows`,
          variant: 'success',
        });
      } else {
        setResults(null);
        toast({
          title: 'Query execution failed',
          description: response.error ?? 'Unknown error',
          variant: 'destructive',
        });
      }
    } catch (err) {
      setResults(null);
      toast({
        title: 'Query execution failed',
        description: err instanceof Error ? err.message : 'Unknown error',
        variant: 'destructive',
      });
    } finally {
      setIsExecuting(false);
    }
  };

  if (!hasExecutableSql) {
    return null;
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-3 pt-1">
        <Button
          onClick={handleExecute}
          disabled={!canExecute || isExecuting}
        >
          {isExecuting ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Play className="h-4 w-4" />
          )}
          {buttonLabel}
        </Button>

        {!datasourceId && (
          <span className="text-sm text-text-tertiary">
            Select a datasource to execute this SQL.
          </span>
        )}
      </div>

      {results && (
        <QueryResultsTable
          columns={results.columns}
          rows={results.rows}
          totalRowCount={results.row_count}
          maxRows={10}
          maxColumns={20}
        />
      )}
    </div>
  );
}
