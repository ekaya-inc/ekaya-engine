import { AlertTriangle, ExternalLink, Loader2, Check, Copy } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import AppPageHeader from '../components/AppPageHeader';
import MCPLogo from '../components/icons/MCPLogo';
import ToolInventory from '../components/mcp/ToolInventory';
import { Button } from '../components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '../components/ui/Card';
import { Switch } from '../components/ui/Switch';
import { useConfig } from '../contexts/ConfigContext';
import { useToast } from '../hooks/useToast';
import { getUserRoles } from '../lib/auth-token';
import engineApi from '../services/engineApi';
import type {
  MCPConfigResponse,
  ServerStatusResponse,
  TunnelStatusResponse,
} from '../types';

const MCPServerPage = () => {
  const { pid } = useParams<{ pid: string }>();
  const { config: appConfig } = useConfig();
  const { toast } = useToast();

  const [config, setConfig] = useState<MCPConfigResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [updating, setUpdating] = useState(false);
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null);

  const [serverStatus, setServerStatus] = useState<ServerStatusResponse | null>(null);
  const [tunnelStatus, setTunnelStatus] = useState<TunnelStatusResponse | null>(null);

  // Read tool group config from backend
  const addDirectDatabaseAccess = config?.toolGroups['tools']?.addDirectDatabaseAccess ?? true;

  const fetchConfig = useCallback(async () => {
    if (!pid) return;

    try {
      setLoading(true);
      const [mcpRes, serverStatusRes, tunnelStatusRes] = await Promise.all([
        engineApi.getMCPConfig(pid),
        engineApi.getServerStatus(),
        engineApi.getTunnelStatus(pid).catch(() => null),
      ]);

      if (mcpRes.success && mcpRes.data) {
        setConfig(mcpRes.data);
      } else {
        throw new Error(mcpRes.error ?? 'Failed to load MCP configuration');
      }

      setServerStatus(serverStatusRes);
      setTunnelStatus(tunnelStatusRes?.data ?? null);
    } catch (error) {
      console.error('Failed to fetch MCP config:', error);
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to load MCP configuration',
        variant: 'destructive',
      });
    } finally {
      setLoading(false);
    }
  }, [pid, toast]);

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  const handleDirectDatabaseAccessChange = async (enabled: boolean) => {
    if (!pid || !config) return;

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        addDirectDatabaseAccess: enabled,
      });

      if (response.success && response.data) {
        setConfig(response.data);
        toast({
          title: 'Success',
          description: `Direct Database Access ${enabled ? 'enabled' : 'disabled'}`,
        });
      } else {
        throw new Error(response.error ?? 'Failed to update configuration');
      }
    } catch (error) {
      console.error('Failed to update MCP config:', error);
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to update configuration',
        variant: 'destructive',
      });
    } finally {
      setUpdating(false);
    }
  };

  const handleCopyUrl = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      setCopiedUrl(url);
      window.setTimeout(() => {
        setCopiedUrl((currentUrl) => (currentUrl === url ? null : currentUrl));
      }, 2000);
    } catch (err) {
      console.error('Failed to copy URL:', err);
    }
  };

  const getSetupInstructionsUrl = (url: string) =>
    `${appConfig?.authServerUrl}/mcp-setup?mcp_url=${encodeURIComponent(url)}`;

  const renderAddressBlock = (
    title: string,
    url: string,
    instructionsLabel: string
  ) => (
    <div className="space-y-2">
      <div className="text-sm font-medium text-text-primary">{title}</div>
      <div className="flex items-center gap-2">
        <div className="flex-1 rounded-lg border border-border-light bg-surface-secondary px-4 py-3 font-mono text-sm text-text-primary">
          {url}
        </div>
        <Button
          variant="outline"
          size="icon"
          onClick={() => void handleCopyUrl(url)}
          className="shrink-0"
          aria-label={`Copy ${title}`}
          title={copiedUrl === url ? 'Copied!' : 'Copy to clipboard'}
        >
          {copiedUrl === url ? (
            <Check className="h-4 w-4 text-green-500" />
          ) : (
            <Copy className="h-4 w-4" />
          )}
        </Button>
      </div>

      <a
        href={getSetupInstructionsUrl(url)}
        target="_blank"
        rel="noopener noreferrer"
        className="inline-flex items-center gap-2 text-sm text-brand-purple hover:underline"
      >
        <ExternalLink className="h-4 w-4" />
        {instructionsLabel}
      </a>
    </div>
  );

  const roles = getUserRoles();
  const hasConfigAccess = roles.includes('admin') || roles.includes('data');
  const privateServerUrl = config?.serverUrl;
  const publicTunnelUrl = tunnelStatus?.public_url;

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-text-secondary" />
      </div>
    );
  }

  if (!hasConfigAccess) {
    return (
      <div className="mx-auto max-w-4xl">
        <AppPageHeader
          title="MCP Server"
          slug="mcp-server"
          icon={<MCPLogo size={32} className="text-brand-purple" />}
          description="Configure the tools AI can use to access your data via Model Context Protocol (MCP), the industry standard for integration."
        />

        <Card>
          <CardHeader>
            <CardTitle className="text-lg">MCP Server Addresses</CardTitle>
          </CardHeader>
          <CardContent className="space-y-5">
            <p className="text-sm text-text-secondary">
              Choose the address that matches how your MCP client will reach this project.
            </p>
            {publicTunnelUrl
              ? renderAddressBlock(
                  'Public Address (Tunnel)',
                  publicTunnelUrl,
                  'Public MCP Setup Instructions'
                )
              : null}
            {privateServerUrl
              ? renderAddressBlock(
                  'Private Address (Server)',
                  privateServerUrl,
                  'Private MCP Setup Instructions'
                )
              : null}
          </CardContent>
        </Card>
      </div>
    );
  }

  const isAccessible = serverStatus?.accessible_for_business_users ?? false;
  return (
    <div className="mx-auto max-w-4xl">
      <AppPageHeader
        title="MCP Server"
        slug="mcp-server"
        icon={<MCPLogo size={32} className="text-brand-purple" />}
        description="Configure the tools AI can use to access your data via Model Context Protocol (MCP), the industry standard for integration."
      />

      <div className="space-y-6">
        {/* Deployment Checklist */}
        <div className="rounded-lg border border-border-light bg-surface-primary p-6">
          <h2 className="text-lg font-semibold text-text-primary">Deployment</h2>
          <p className="mt-2 text-sm text-text-secondary">
            These optional steps help other users reach this project&apos;s MCP Server over HTTPS.
          </p>
          <div className="mt-4 rounded-lg border border-border-light bg-surface-secondary/60 p-4">
            <div className="flex items-center justify-between gap-4">
              <div>
                <p className="text-sm font-medium text-text-primary">
                  MCP Server securely deployed
                </p>
                <p className="mt-1 text-sm text-text-secondary">
                  {isAccessible
                    ? 'Server is reachable by other users over HTTPS.'
                    : 'Configure HTTPS on a reachable domain for other users.'}
                </p>
              </div>
              <Button asChild variant="outline">
                <Link to={`/projects/${pid}/server-setup`}>
                  {isAccessible ? 'Review' : 'Configure'}
                </Link>
              </Button>
            </div>
          </div>
        </div>

        {config && (
          <>
            {/* Simplified URL Section */}
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">MCP Server Addresses</CardTitle>
              </CardHeader>
              <CardContent className="space-y-5">
                <p className="text-sm text-text-secondary">
                  Choose the address that matches how your MCP client will reach this project.
                </p>
                {publicTunnelUrl
                  ? renderAddressBlock(
                      'Public Address (Tunnel)',
                      publicTunnelUrl,
                      'Public MCP Setup Instructions'
                    )
                  : null}
                {renderAddressBlock(
                  'Private Address (Server)',
                  config.serverUrl,
                  'Private MCP Setup Instructions'
                )}

                <p className="text-sm text-text-secondary font-medium">
                  Note: Changes to configuration will take effect after restarting the MCP Client.
                </p>
              </CardContent>
            </Card>

            {/* Tool Configuration */}
            <Card>
              <CardHeader>
                <CardTitle>Tool Configuration</CardTitle>
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

                {/* Direct Database Access toggle */}
                <div className="flex items-center justify-between gap-4">
                  <div className="flex-1">
                    <span className="text-sm font-medium text-text-primary">Direct Database Access</span>
                    <p className="text-sm text-text-secondary mt-1">
                      Include direct database access tools for query, validate, sample, execute,
                      explain_query, and echo. Enables the MCP Client to inspect data, validate SQL,
                      and run arbitrary SQL against the datasource.
                    </p>
                  </div>
                  <Switch
                    checked={addDirectDatabaseAccess}
                    onCheckedChange={handleDirectDatabaseAccessChange}
                    disabled={updating}
                  />
                </div>
              </CardContent>
            </Card>

            {/* Tool Inventory */}
            <ToolInventory
              developerTools={config.developerTools}
              userTools={config.userTools}
              appNames={config.appNames}
              projectId={pid ?? ''}
            />

          </>
        )}
      </div>
    </div>
  );
};

export default MCPServerPage;
