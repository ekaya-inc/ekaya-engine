import { AlertTriangle, ExternalLink, Loader2, Check, Copy } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';

import AppPageHeader from '../components/AppPageHeader';
import MCPLogo from '../components/icons/MCPLogo';
import ToolInventory from '../components/mcp/ToolInventory';
import SetupChecklist from '../components/SetupChecklist';
import type { ChecklistItem } from '../components/SetupChecklist';
import { Button } from '../components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '../components/ui/Card';
import { Switch } from '../components/ui/Switch';
import { useConfig } from '../contexts/ConfigContext';
import { useToast } from '../hooks/useToast';
import { getUserRoles } from '../lib/auth-token';
import engineApi from '../services/engineApi';
import type { Datasource, MCPConfigResponse, ServerStatusResponse } from '../types';

const MCPServerPage = () => {
  const { pid } = useParams<{ pid: string }>();
  const { config: appConfig } = useConfig();
  const { toast } = useToast();

  const [config, setConfig] = useState<MCPConfigResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [updating, setUpdating] = useState(false);
  const [copied, setCopied] = useState(false);

  // Setup checklist state
  const [datasource, setDatasource] = useState<Datasource | null>(null);
  const [serverStatus, setServerStatus] = useState<ServerStatusResponse | null>(null);

  // Read tool group config from backend
  const addDirectDatabaseAccess = config?.toolGroups['tools']?.addDirectDatabaseAccess ?? true;

  const fetchConfig = useCallback(async () => {
    if (!pid) return;

    try {
      setLoading(true);
      const [mcpRes, datasourcesRes, serverStatusRes] = await Promise.all([
        engineApi.getMCPConfig(pid),
        engineApi.listDataSources(pid),
        engineApi.getServerStatus(),
      ]);

      if (mcpRes.success && mcpRes.data) {
        setConfig(mcpRes.data);
      } else {
        throw new Error(mcpRes.error ?? 'Failed to load MCP configuration');
      }

      const ds = datasourcesRes.data?.datasources?.[0] ?? null;
      setDatasource(ds);
      setServerStatus(serverStatusRes);
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

  const getChecklistItems = (): ChecklistItem[] => {
    return [
      {
        id: 'datasource',
        title: 'Datasource configured',
        description: datasource
          ? `Connected to ${datasource.name} (${datasource.type})`
          : 'Connect a database to enable the MCP Server',
        status: loading ? 'loading' : datasource ? 'complete' : 'pending',
        link: `/projects/${pid}/datasource`,
        linkText: datasource ? 'Manage' : 'Configure',
      },
    ];
  };

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

  const handleCopyUrl = async () => {
    if (!config) return;
    try {
      await navigator.clipboard.writeText(config.serverUrl);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy URL:', err);
    }
  };

  const roles = getUserRoles();
  const hasConfigAccess = roles.includes('admin') || roles.includes('data');

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-text-secondary" />
      </div>
    );
  }

  if (!hasConfigAccess) {
    const mcpSetupUrl = config?.serverUrl
      ? `${appConfig?.authServerUrl}/mcp-setup?mcp_url=${encodeURIComponent(config.serverUrl)}`
      : `${appConfig?.authServerUrl}/mcp-setup`;

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
            <CardTitle className="text-lg">Connect to the MCP Server</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-text-secondary">
              Follow the setup instructions to connect your MCP client to this project&apos;s MCP Server.
            </p>
            <a
              href={mcpSetupUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 text-sm text-brand-purple hover:underline"
            >
              <ExternalLink className="h-4 w-4" />
              MCP Setup Instructions
            </a>
          </CardContent>
        </Card>
      </div>
    );
  }

  const checklistItems = getChecklistItems();

  const isAccessible = serverStatus?.accessible_for_business_users ?? false;
  const deploymentItems: ChecklistItem[] = [
    {
      id: 'server-accessible',
      title: 'MCP Server securely deployed',
      description: isAccessible
        ? 'Server is reachable by other users over HTTPS'
        : 'Configure HTTPS on a reachable domain for other users',
      status: loading ? 'loading' : isAccessible ? 'complete' : 'pending',
      link: `/projects/${pid}/server-setup`,
      linkText: isAccessible ? 'Review' : 'Configure',
    },
  ];

  return (
    <div className="mx-auto max-w-4xl">
      <AppPageHeader
        title="MCP Server"
        slug="mcp-server"
        icon={<MCPLogo size={32} className="text-brand-purple" />}
        description="Configure the tools AI can use to access your data via Model Context Protocol (MCP), the industry standard for integration."
      />

      <div className="space-y-6">
        {/* Setup Checklist */}
        <SetupChecklist
          items={checklistItems}
          title="Setup Checklist"
          description="Complete these steps to enable the MCP Server"
          completeDescription="MCP Server is ready"
        />

        {/* Deployment Checklist */}
        <SetupChecklist
          items={deploymentItems}
          title="Deployment"
          description="Optional steps for sharing with other users"
          completeDescription="MCP Server is accessible to other users"
        />

        {config && (
          <>
            {/* Simplified URL Section */}
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">Your MCP Server URL</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex items-center gap-2">
                  <div className="flex-1 rounded-lg border border-border-light bg-surface-secondary px-4 py-3 font-mono text-sm text-text-primary">
                    {config.serverUrl}
                  </div>
                  <Button
                    variant="outline"
                    size="icon"
                    onClick={handleCopyUrl}
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

                <a
                  href={`${appConfig?.authServerUrl}/mcp-setup?mcp_url=${encodeURIComponent(config.serverUrl)}`}
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
                      Include tools for direct database access: echo, execute, and query.
                      Enables the MCP Client to run arbitrary SQL against the datasource.
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
