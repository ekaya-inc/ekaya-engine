import { Link } from 'react-router-dom';

import type { EnabledToolInfo } from '../../types';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/Card';
import { Switch } from '../ui/Switch';

import MCPEnabledTools from './MCPEnabledTools';

interface UserToolsSectionProps {
  projectId: string;
  allowOntologyMaintenance: boolean;
  onAllowOntologyMaintenanceChange: (value: boolean) => void;
  enabledTools: EnabledToolInfo[];
  disabled?: boolean;
}

export default function UserToolsSection({
  projectId,
  allowOntologyMaintenance,
  onAllowOntologyMaintenanceChange,
  enabledTools,
  disabled = false,
}: UserToolsSectionProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>User Tools</CardTitle>
        <CardDescription>
          The MCP Client can use{' '}
          <Link to={`/projects/${projectId}/queries`} className="text-brand-purple hover:underline">
            Pre-Approved Queries
          </Link>{' '}
          and the Ontology to craft read-only SQL for ad-hoc requests. Any database-modifying SQL
          statements will need to be in the form of a Pre-Approved Query.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Ontology improvement toggle */}
        <div className="flex items-center justify-between gap-4">
          <div className="flex-1">
            <span className="text-sm font-medium text-text-primary">
              Allow Usage to Improve Ontology
            </span>
            <span className="ml-2 text-xs font-medium text-brand-purple">[RECOMMENDED]</span>
            <p className="text-sm text-text-secondary mt-1">
              Enable the MCP Client to update entities, relationships, and glossary terms as it
              learns from user interactions. This helps improve query accuracy over time.
            </p>
          </div>
          <Switch
            checked={allowOntologyMaintenance}
            onCheckedChange={onAllowOntologyMaintenanceChange}
            disabled={disabled}
          />
        </div>

        {/* Collapsible tools list */}
        <MCPEnabledTools tools={enabledTools} />
      </CardContent>
    </Card>
  );
}
