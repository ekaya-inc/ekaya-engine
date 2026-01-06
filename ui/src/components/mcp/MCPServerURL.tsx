import { Check, Copy, ExternalLink } from 'lucide-react';
import { useState } from 'react';

import { Button } from '../ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '../ui/Card';

import MCPEnabledTools from './MCPEnabledTools';

interface EnabledTool {
  name: string;
  description: string;
}

interface MCPServerURLProps {
  serverUrl: string;
  docsUrl?: string;
  agentMode?: boolean;
  agentApiKey?: string;
  enabledTools?: EnabledTool[];
}

export default function MCPServerURL({
  serverUrl,
  docsUrl,
  agentMode = false,
  agentApiKey,
  enabledTools = [],
}: MCPServerURLProps) {
  const [copied, setCopied] = useState(false);
  const [configCopied, setConfigCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(serverUrl);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy URL:', err);
    }
  };

  // Generate the .mcp.json configuration for Claude Code
  const mcpConfig = JSON.stringify(
    {
      mcpServers: {
        ekaya: {
          type: 'http',
          url: serverUrl,
          headers: {
            Authorization: `Bearer ${agentApiKey ?? '<your-api-key>'}`,
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
        <CardTitle className="text-lg">Your MCP Server URL</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center gap-2">
          <div className="flex-1 rounded-lg border border-border-light bg-surface-secondary px-4 py-3 font-mono text-sm text-text-primary">
            {serverUrl}
          </div>
          <Button
            variant="outline"
            size="icon"
            onClick={handleCopy}
            className="shrink-0"
            title={copied ? 'Copied!' : 'Copy to clipboard'}
          >
            {copied ? (
              <Check className="h-4 w-4 text-green-500" />
            ) : (
              <Copy className="h-4 w-4" />
            )}
          </Button>
        </div>

        {agentMode ? (
          <div className="space-y-3">
            <p className="text-sm font-medium text-text-primary">Agent Setup Example:</p>
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
            <p className="text-sm text-text-secondary font-medium">
              Note: Changes to configuration will take effect after restarting the MCP Client.
            </p>
          </div>
        ) : (
          docsUrl && (
            <div className="space-y-3">
              <a
                href={docsUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-2 text-sm text-brand-purple hover:underline"
              >
                <ExternalLink className="h-4 w-4" />
                MCP Setup Instructions
              </a>
              <p className="text-sm text-text-secondary font-medium">
                Note: Changes to configuration will take effect after restarting the MCP Client.
              </p>
            </div>
          )
        )}

        <MCPEnabledTools tools={enabledTools} />
      </CardContent>
    </Card>
  );
}
