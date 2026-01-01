import { Check, Copy, ExternalLink } from 'lucide-react';
import { useState } from 'react';

import { Button } from '../ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '../ui/Card';

interface MCPServerURLProps {
  serverUrl: string;
  docsUrl?: string;
}

export default function MCPServerURL({ serverUrl, docsUrl }: MCPServerURLProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(serverUrl);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy URL:', err);
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

        {docsUrl && (
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
        )}
      </CardContent>
    </Card>
  );
}
