import { ArrowLeft, ExternalLink, Info, Loader2, Check, Copy } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import MCPLogo from '../components/icons/MCPLogo';
import DeveloperToolsSection from '../components/mcp/DeveloperToolsSection';
import SetupChecklist from '../components/SetupChecklist';
import type { ChecklistItem } from '../components/SetupChecklist';
import { Button } from '../components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '../components/ui/Card';
import { TOOL_GROUP_IDS } from '../constants/mcpToolMetadata';
import { useConfig } from '../contexts/ConfigContext';
import { useProject } from '../contexts/ProjectContext';
import { useToast } from '../hooks/useToast';
import { getUserRoles } from '../lib/auth-token';
import engineApi from '../services/engineApi';
import type { Datasource, MCPConfigResponse, ServerStatusResponse } from '../types';

const MCPServerPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { config: appConfig } = useConfig();
  const { urls } = useProject();
  const { toast } = useToast();

  const [config, setConfig] = useState<MCPConfigResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [updating, setUpdating] = useState(false);
  const [copied, setCopied] = useState(false);

  // Setup checklist state
  const [datasource, setDatasource] = useState<Datasource | null>(null);
  const [serverStatus, setServerStatus] = useState<ServerStatusResponse | null>(null);

  // Read tool group configs from backend
  const developerState = config?.toolGroups[TOOL_GROUP_IDS.DEVELOPER];
  const addQueryTools = developerState?.addQueryTools ?? true;
  const addOntologyMaintenance = developerState?.addOntologyMaintenance ?? true;

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

  const handleAddQueryToolsChange = async (enabled: boolean) => {
    if (!pid || !config) return;

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        addQueryTools: enabled,
      });

      if (response.success && response.data) {
        setConfig(response.data);
        toast({
          title: 'Success',
          description: `Add Query Tools ${enabled ? 'enabled' : 'disabled'}`,
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

  const handleAddOntologyMaintenanceChange = async (enabled: boolean) => {
    if (!pid || !config) return;

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        addOntologyMaintenance: enabled,
      });

      if (response.success && response.data) {
        setConfig(response.data);
        toast({
          title: 'Success',
          description: `Add Ontology Maintenance ${enabled ? 'enabled' : 'disabled'}`,
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
        <div className="mb-6">
          <Button
            variant="ghost"
            onClick={() => navigate(`/projects/${pid}`)}
            className="mb-4"
          >
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
          <h1 className="text-3xl font-bold text-text-primary flex items-center gap-2">
            <MCPLogo size={32} className="text-brand-purple" />
            MCP Server
          </h1>
        </div>

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
      <div className="mb-6">
        <Button
          variant="ghost"
          onClick={() => navigate(`/projects/${pid}`)}
          className="mb-4"
        >
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Dashboard
        </Button>
        <div className="flex items-center justify-between">
          <h1 className="text-3xl font-bold text-text-primary flex items-center gap-2">
            <MCPLogo size={32} className="text-brand-purple" />
            MCP Server
          </h1>
          <a
            href={`${urls.projectsPageUrl ? new URL(urls.projectsPageUrl).origin : 'https://us.ekaya.ai'}/apps/mcp-server`}
            target="_blank"
            rel="noopener noreferrer"
            title="MCP Server documentation"
          >
            <Info className="h-7 w-7 text-text-secondary hover:text-brand-purple transition-colors" />
          </a>
        </div>
      </div>

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

            {/* Developer Tools Section */}
            <DeveloperToolsSection
              addQueryTools={addQueryTools}
              onAddQueryToolsChange={handleAddQueryToolsChange}
              addOntologyMaintenance={addOntologyMaintenance}
              onAddOntologyMaintenanceChange={handleAddOntologyMaintenanceChange}
              enabledTools={config.developerTools}
              disabled={updating}
            />

          </>
        )}
      </div>
    </div>
  );
};

export default MCPServerPage;
