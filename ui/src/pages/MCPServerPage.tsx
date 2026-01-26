import { ArrowLeft, ExternalLink, Loader2, Check, Copy } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import MCPLogo from '../components/icons/MCPLogo';
import AgentToolsSection from '../components/mcp/AgentToolsSection';
import DeveloperToolsSection from '../components/mcp/DeveloperToolsSection';
import UserToolsSection from '../components/mcp/UserToolsSection';
import { Button } from '../components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '../components/ui/Card';
import { TOOL_GROUP_IDS } from '../constants/mcpToolMetadata';
import { useToast } from '../hooks/useToast';
import engineApi from '../services/engineApi';
import type { MCPConfigResponse } from '../types';

const MCPServerPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();

  const [config, setConfig] = useState<MCPConfigResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [updating, setUpdating] = useState(false);
  const [agentApiKey, setAgentApiKey] = useState<string>('');
  const [copied, setCopied] = useState(false);

  // Read tool group configs from backend
  const approvedQueriesState = config?.toolGroups[TOOL_GROUP_IDS.USER];
  const allowOntologyMaintenance = approvedQueriesState?.allowOntologyMaintenance ?? true;

  const developerState = config?.toolGroups[TOOL_GROUP_IDS.DEVELOPER];
  const addQueryTools = developerState?.addQueryTools ?? true;
  const addOntologyMaintenance = developerState?.addOntologyMaintenance ?? true;

  const fetchConfig = useCallback(async () => {
    if (!pid) return;

    try {
      setLoading(true);
      const response = await engineApi.getMCPConfig(pid);
      if (response.success && response.data) {
        setConfig(response.data);
      } else {
        throw new Error(response.error ?? 'Failed to load MCP configuration');
      }
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

  // Fetch agent API key on mount
  useEffect(() => {
    const fetchAgentKey = async () => {
      if (!pid) return;

      try {
        const response = await engineApi.getAgentAPIKey(pid, true);
        if (response.success && response.data) {
          setAgentApiKey(response.data.key);
        }
      } catch (error) {
        console.error('Failed to fetch agent API key:', error);
      }
    };

    fetchAgentKey();
  }, [pid]);

  const handleAllowOntologyMaintenanceChange = async (enabled: boolean) => {
    if (!pid || !config) return;

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        toolGroups: {
          [TOOL_GROUP_IDS.USER]: {
            enabled: true,
            allowOntologyMaintenance: enabled,
          },
        },
      });

      if (response.success && response.data) {
        setConfig(response.data);
        toast({
          title: 'Success',
          description: `Allow Usage to Improve Ontology ${enabled ? 'enabled' : 'disabled'}`,
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

  const handleAddQueryToolsChange = async (enabled: boolean) => {
    if (!pid || !config) return;

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        toolGroups: {
          [TOOL_GROUP_IDS.DEVELOPER]: {
            enabled: true,
            addQueryTools: enabled,
            addOntologyMaintenance,
          },
        },
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
        toolGroups: {
          [TOOL_GROUP_IDS.DEVELOPER]: {
            enabled: true,
            addQueryTools,
            addOntologyMaintenance: enabled,
          },
        },
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

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-text-secondary" />
      </div>
    );
  }

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

      <div className="space-y-6">
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
                  href={`https://us.ekaya.ai/mcp-setup?mcp_url=${encodeURIComponent(config.serverUrl)}`}
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

            {/* User Tools Section */}
            <UserToolsSection
              projectId={pid!}
              allowOntologyMaintenance={allowOntologyMaintenance}
              onAllowOntologyMaintenanceChange={handleAllowOntologyMaintenanceChange}
              enabledTools={config.userTools}
              disabled={updating}
            />

            {/* Developer Tools Section */}
            <DeveloperToolsSection
              addQueryTools={addQueryTools}
              onAddQueryToolsChange={handleAddQueryToolsChange}
              addOntologyMaintenance={addOntologyMaintenance}
              onAddOntologyMaintenanceChange={handleAddOntologyMaintenanceChange}
              enabledTools={config.developerTools}
              disabled={updating}
            />

            {/* Agent Tools Section */}
            <AgentToolsSection
              projectId={pid!}
              serverUrl={config.serverUrl}
              agentApiKey={agentApiKey}
              onAgentApiKeyChange={setAgentApiKey}
              enabledTools={config.agentTools}
            />
          </>
        )}
      </div>
    </div>
  );
};

export default MCPServerPage;
