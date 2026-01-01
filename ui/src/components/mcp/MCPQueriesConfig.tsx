import { AlertTriangle, ExternalLink } from 'lucide-react';
import { Link } from 'react-router-dom';

import { Card, CardContent } from '../ui/Card';
import { Switch } from '../ui/Switch';

interface MCPQueriesConfigProps {
  projectId: string;
  queryCount: number;
  forceMode: boolean;
  allowSuggestions: boolean;
  onToggleForceMode: (enabled: boolean) => void;
  onToggleSuggestions: (enabled: boolean) => void;
  disabled?: boolean;
}

/**
 * MCPQueriesConfig - Configuration section for Pre-Approved Queries in MCP Server
 *
 * When no queries exist: Shows disabled state with link to create queries
 * When queries exist: Shows FORCE mode and suggestions toggles
 */
export default function MCPQueriesConfig({
  projectId,
  queryCount,
  forceMode,
  allowSuggestions,
  onToggleForceMode,
  onToggleSuggestions,
  disabled = false,
}: MCPQueriesConfigProps) {
  const hasQueries = queryCount > 0;

  if (!hasQueries) {
    return (
      <Card className="opacity-60">
        <CardContent className="p-6">
          <div className="flex items-start gap-4">
            <div className="flex-1 space-y-2">
              <div className="flex items-center gap-3">
                <Switch checked={false} disabled />
                <span className="text-lg font-medium text-text-primary">Queries</span>
              </div>
              <p className="text-sm text-text-secondary">
                No Pre-Approved Queries have been created.
              </p>
              <Link
                to={`/projects/${projectId}/queries`}
                className="inline-flex items-center gap-1 text-sm text-brand-purple hover:underline"
              >
                <ExternalLink className="h-3 w-3" />
                Create queries to enable this feature
              </Link>
            </div>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardContent className="p-6 space-y-4">
        {/* FORCE Mode Toggle */}
        <div className="space-y-2">
          <div className="flex items-center gap-3">
            <Switch
              checked={forceMode}
              onCheckedChange={onToggleForceMode}
              disabled={disabled}
            />
            <span className="text-lg font-medium text-text-primary">
              FORCE all access through Pre-Approved Queries
            </span>
          </div>
          <p className="text-sm text-text-secondary">
            This is the safest way to enable AI access to the datasource. When enabled, MCP clients can only execute Pre-Approved Queries.
          </p>
          <div className="flex items-start gap-2 rounded-md bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-950 dark:text-amber-200">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
            <span>Enabling this will disable Developer Tools.</span>
          </div>
        </div>

        {/* Divider */}
        <div className="border-t border-border-light" />

        {/* Allow Suggestions Toggle */}
        <div className="space-y-2">
          <div className="flex items-center gap-3">
            <Switch
              checked={allowSuggestions}
              onCheckedChange={onToggleSuggestions}
              disabled={disabled}
            />
            <span className="text-sm font-medium text-text-primary">
              Allow MCP Client to suggest queries
            </span>
          </div>
          <p className="text-sm text-text-secondary">
            Allow the client using this interface to suggest queries that must be approved by an administrator. Enabling this will expose the ontology, schema and SQL of the Pre-Approved Queries.
          </p>
        </div>
      </CardContent>
    </Card>
  );
}
