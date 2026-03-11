import { ChevronDown, ChevronRight, ExternalLink } from 'lucide-react';
import { useState } from 'react';
import { Link } from 'react-router-dom';

import type { EnabledToolInfo } from '../../types';
import { Card, CardContent, CardHeader, CardTitle } from '../ui/Card';

// App page routes keyed by app ID
const APP_ROUTES: Record<string, string> = {
  'mcp-server': 'mcp-server',
  'ontology-forge': 'ontology-forge',
  'ai-data-liaison': 'ai-data-liaison',
};

// Canonical app ordering
const APP_ORDER = ['mcp-server', 'ontology-forge', 'ai-data-liaison'];

interface ToolInventoryProps {
  developerTools: EnabledToolInfo[];
  userTools: EnabledToolInfo[];
  appNames: Record<string, string>;
  projectId: string;
}

function ToolsByApp({
  tools,
  appNames,
  projectId,
}: {
  tools: EnabledToolInfo[];
  appNames: Record<string, string>;
  projectId: string;
}) {
  // Group tools by appId
  const grouped = new Map<string, EnabledToolInfo[]>();
  for (const tool of tools) {
    const existing = grouped.get(tool.appId) ?? [];
    existing.push(tool);
    grouped.set(tool.appId, existing);
  }

  // Sort app groups in canonical order
  const sortedAppIds = [...grouped.keys()].sort((a, b) => {
    const ai = APP_ORDER.indexOf(a);
    const bi = APP_ORDER.indexOf(b);
    return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi);
  });

  return (
    <div className="space-y-3">
      {sortedAppIds.map((appId) => {
        const appTools = grouped.get(appId) ?? [];
        const displayName = appNames[appId] ?? appId;
        const route = APP_ROUTES[appId];

        return (
          <div key={appId}>
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm font-medium text-text-primary">{displayName}</span>
              {route && (
                <Link
                  to={`/projects/${projectId}/${route}`}
                  className="inline-flex items-center gap-1 text-xs text-brand-purple hover:underline"
                >
                  Configure
                  <ExternalLink className="h-3 w-3" />
                </Link>
              )}
            </div>
            <table className="w-full text-sm">
              <tbody>
                {appTools.map((tool) => (
                  <tr key={tool.name} className="border-b border-border-light last:border-0">
                    <td className="py-1.5 font-mono text-text-primary">{tool.name}</td>
                    <td className="py-1.5 text-text-secondary">{tool.description}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        );
      })}
    </div>
  );
}

function CollapsibleSection({
  title,
  tools,
  appNames,
  projectId,
}: {
  title: string;
  tools: EnabledToolInfo[];
  appNames: Record<string, string>;
  projectId: string;
}) {
  const [isExpanded, setIsExpanded] = useState(false);

  return (
    <div>
      <button
        type="button"
        onClick={() => setIsExpanded(!isExpanded)}
        className="flex items-center gap-2 text-base font-semibold text-text-primary hover:text-brand-purple transition-colors"
      >
        {isExpanded ? (
          <ChevronDown className="h-5 w-5" />
        ) : (
          <ChevronRight className="h-5 w-5" />
        )}
        {title} ({tools.length})
      </button>
      {isExpanded && (
        <div className="mt-3 ml-7">
          {tools.length === 0 ? (
            <p className="text-sm text-text-secondary italic">No tools enabled</p>
          ) : (
            <ToolsByApp tools={tools} appNames={appNames} projectId={projectId} />
          )}
        </div>
      )}
    </div>
  );
}

export default function ToolInventory({
  developerTools,
  userTools,
  appNames,
  projectId,
}: ToolInventoryProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Tool Inventory</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <CollapsibleSection
          title="Developer Tools"
          tools={developerTools}
          appNames={appNames}
          projectId={projectId}
        />
        <CollapsibleSection
          title="User Tools"
          tools={userTools}
          appNames={appNames}
          projectId={projectId}
        />
      </CardContent>
    </Card>
  );
}
