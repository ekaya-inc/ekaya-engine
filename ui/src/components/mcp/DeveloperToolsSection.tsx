import { AlertTriangle, Lightbulb } from 'lucide-react';

import type { EnabledToolInfo } from '../../types';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/Card';
import { Switch } from '../ui/Switch';

import MCPEnabledTools from './MCPEnabledTools';

interface DeveloperToolsSectionProps {
  addQueryTools: boolean;
  onAddQueryToolsChange: (value: boolean) => void;
  addOntologyMaintenance: boolean;
  onAddOntologyMaintenanceChange: (value: boolean) => void;
  enabledTools: EnabledToolInfo[];
  disabled?: boolean;
}

export default function DeveloperToolsSection({
  addQueryTools,
  onAddQueryToolsChange,
  addOntologyMaintenance,
  onAddOntologyMaintenanceChange,
  enabledTools,
  disabled = false,
}: DeveloperToolsSectionProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Developer Tools</CardTitle>
        <CardDescription>
          Full access to schema exploration, query management, and ontology maintenance. Available
          to users with admin, data, or developer roles.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Warning about direct access */}
        <div className="flex items-start gap-2 rounded-md bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-950 dark:text-amber-200">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>
            The MCP Client will have direct access to the Datasource using the supplied credentials
            -- this access includes potentially destructive operations. Back up the data before
            allowing AI to modify it.
          </span>
        </div>

        {/* Add Query Tools toggle */}
        <div className="flex items-center justify-between gap-4">
          <div className="flex-1">
            <span className="text-sm font-medium text-text-primary">Add Query Tools</span>
            <p className="text-sm text-text-secondary mt-1">
              Include schema exploration, sampling, and query tools. Enables the MCP Client to
              explore the database and run queries.
            </p>
          </div>
          <Switch
            checked={addQueryTools}
            onCheckedChange={onAddQueryToolsChange}
            disabled={disabled}
          />
        </div>

        {/* Add Ontology Maintenance toggle */}
        <div className="flex items-center justify-between gap-4">
          <div className="flex-1">
            <span className="text-sm font-medium text-text-primary">Add Ontology Maintenance</span>
            <p className="text-sm text-text-secondary mt-1">
              Include tools to manage the ontology: update columns, relationships, refresh
              schema, and review pending changes.
            </p>
          </div>
          <Switch
            checked={addOntologyMaintenance}
            onCheckedChange={onAddOntologyMaintenanceChange}
            disabled={disabled}
          />
        </div>

        {/* Pro tip */}
        <div className="flex items-start gap-2 rounded-md bg-brand-purple/10 p-3 text-sm text-brand-purple dark:bg-brand-purple/20">
          <Lightbulb className="mt-0.5 h-4 w-4 shrink-0" />
          <div>
            <span className="font-semibold">Pro Tip:</span> Have AI answer questions about your
            Ontology{' '}
            <details className="inline">
              <summary className="inline cursor-pointer underline">(more info)</summary>
              <p className="mt-2 font-normal">
                After you have extracted your Ontology there might be questions that Ekaya cannot
                answer from the database schema and values alone. Connect your IDE to the MCP Server
                so that your LLM can answer questions by reviewing your codebase or other project
                documents saving you time.
              </p>
            </details>
          </div>
        </div>

        {/* Collapsible tools list */}
        <MCPEnabledTools tools={enabledTools} />
      </CardContent>
    </Card>
  );
}
