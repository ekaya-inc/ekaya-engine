import { AlertTriangle, Check, Copy } from 'lucide-react';
import { useState } from 'react';

import type { EnabledToolInfo } from '../../types';
import { Button } from '../ui/Button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/Card';

import AgentAPIKeyDisplay from './AgentAPIKeyDisplay';
import MCPEnabledTools from './MCPEnabledTools';

interface AgentToolsSectionProps {
  projectId: string;
  serverUrl: string;
  agentApiKey: string;
  onAgentApiKeyChange: (key: string) => void;
  enabledTools: EnabledToolInfo[];
}

export default function AgentToolsSection({
  projectId,
  serverUrl,
  agentApiKey,
  onAgentApiKeyChange,
  enabledTools,
}: AgentToolsSectionProps) {
  const [configCopied, setConfigCopied] = useState(false);

  // Generate the .mcp.json configuration for Claude Code
  const mcpConfig = JSON.stringify(
    {
      mcpServers: {
        ekaya: {
          type: 'http',
          url: serverUrl,
          headers: {
            Authorization: `Bearer ${agentApiKey || '<your-api-key>'}`,
          },
        },
      },
    },
    null,
    2
  );

  const handleCopyConfig = async () => {
    try {
      await navigator.clipboard.writeText(mcpConfig);
      setConfigCopied(true);
      setTimeout(() => setConfigCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy config:', err);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Agent Tools</CardTitle>
        <CardDescription>
          Limited tool access for AI agents using API key authentication. AI Agents can only use
          the enabled Pre-Approved Queries so that you have full control over access.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* API Key Management */}
        <AgentAPIKeyDisplay projectId={projectId} onKeyChange={onAgentApiKeyChange} />

        {/* Setup Example */}
        <div className="space-y-2">
          <span className="text-sm font-medium text-text-primary">Agent Setup Example:</span>
          <div className="relative">
            <pre className="rounded-lg border border-border-light bg-surface-secondary p-4 font-mono text-xs text-text-primary overflow-x-auto">
              {mcpConfig}
            </pre>
            <Button
              variant="outline"
              size="icon"
              onClick={handleCopyConfig}
              className="absolute top-2 right-2"
              title={configCopied ? 'Copied!' : 'Copy configuration'}
            >
              {configCopied ? (
                <Check className="h-4 w-4 text-green-500" />
              ) : (
                <Copy className="h-4 w-4" />
              )}
            </Button>
          </div>
        </div>

        {/* Warning */}
        <div className="flex items-start gap-2 rounded-md bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-950 dark:text-amber-200">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>Distribute keys carefully and rotate them periodically.</span>
        </div>

        {/* Collapsible tools list */}
        <MCPEnabledTools tools={enabledTools} />
      </CardContent>
    </Card>
  );
}
