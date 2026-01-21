import { AlertTriangle, ArrowLeft, Loader2 } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';

import MCPLogo from '../components/icons/MCPLogo';
import AgentAPIKeyDisplay from '../components/mcp/AgentAPIKeyDisplay';
import MCPServerURL from '../components/mcp/MCPServerURL';
import MCPToolGroup from '../components/mcp/MCPToolGroup';
import { Button } from '../components/ui/Button';
import {
  TOOL_GROUP_IDS,
  TOOL_GROUP_METADATA,
  getToolOrder,
} from '../constants/mcpToolMetadata';
import { useToast } from '../hooks/useToast';
import engineApi from '../services/engineApi';
import type { MCPConfigResponse, SubOptionInfo, ToolGroupState } from '../types';

// Helper to get sub-option enabled state from flat ToolGroupState
const getSubOptionEnabled = (state: ToolGroupState | undefined, subOptionName: string): boolean => {
  if (!state) return false;
  switch (subOptionName) {
    // New sub-options
    case 'allowOntologyMaintenance':
      return state.allowOntologyMaintenance ?? false;
    case 'addQueryTools':
      return state.addQueryTools ?? false;
    case 'addOntologyMaintenance':
      return state.addOntologyMaintenance ?? false;
    // Legacy sub-options
    case 'enableExecute':
      return state.enableExecute ?? false;
    case 'forceMode':
      return state.forceMode ?? false;
    case 'allowClientSuggestions':
      return state.allowClientSuggestions ?? false;
    default:
      return false;
  }
};

const MCPServerPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();

  const [config, setConfig] = useState<MCPConfigResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [updating, setUpdating] = useState(false);
  const [agentApiKey, setAgentApiKey] = useState<string>('');

  // Read tool group configs from backend
  const approvedQueriesState = config?.toolGroups[TOOL_GROUP_IDS.APPROVED_QUERIES];
  const isApprovedQueriesEnabled = approvedQueriesState?.enabled ?? false;
  const allowOntologyMaintenance = approvedQueriesState?.allowOntologyMaintenance ?? false;

  const developerState = config?.toolGroups[TOOL_GROUP_IDS.DEVELOPER];
  const isDeveloperEnabled = developerState?.enabled ?? false;
  const addQueryTools = developerState?.addQueryTools ?? false;
  const addOntologyMaintenance = developerState?.addOntologyMaintenance ?? false;

  const isAgentToolsEnabled = config?.toolGroups[TOOL_GROUP_IDS.AGENT_TOOLS]?.enabled ?? false;

  // Get metadata for tool groups
  const approvedQueriesMetadata = TOOL_GROUP_METADATA[TOOL_GROUP_IDS.APPROVED_QUERIES];
  const developerMetadata = TOOL_GROUP_METADATA[TOOL_GROUP_IDS.DEVELOPER];

  // Sort enabled tools by canonical order
  const sortedEnabledTools = config?.enabledTools
    ? [...config.enabledTools].sort((a, b) => getToolOrder(a.name) - getToolOrder(b.name))
    : [];

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

  // Fetch agent API key when Agent Tools is enabled (for Agent Setup Example display)
  useEffect(() => {
    const fetchAgentKey = async () => {
      if (!pid || !isAgentToolsEnabled) return;

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
  }, [pid, isAgentToolsEnabled]);

  const handleToggleApprovedQueriesSubOption = async (subOptionName: string, enabled: boolean) => {
    if (!pid || !config) return;

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        toolGroups: {
          [TOOL_GROUP_IDS.APPROVED_QUERIES]: {
            enabled: isApprovedQueriesEnabled,
            ...(subOptionName === 'allowOntologyMaintenance' ? { allowOntologyMaintenance: enabled } : { allowOntologyMaintenance }),
          },
        },
      });

      if (response.success && response.data) {
        setConfig(response.data);
        const subOptionMeta = approvedQueriesMetadata?.subOptions?.[subOptionName];
        toast({
          title: 'Success',
          description: `${subOptionMeta?.name ?? subOptionName} ${enabled ? 'enabled' : 'disabled'}`,
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

  const handleToggleDeveloperSubOption = async (subOptionName: string, enabled: boolean) => {
    if (!pid || !config) return;

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        toolGroups: {
          [TOOL_GROUP_IDS.DEVELOPER]: {
            enabled: isDeveloperEnabled,
            ...(subOptionName === 'addQueryTools' ? { addQueryTools: enabled } : { addQueryTools }),
            ...(subOptionName === 'addOntologyMaintenance' ? { addOntologyMaintenance: enabled } : { addOntologyMaintenance }),
          },
        },
      });

      if (response.success && response.data) {
        setConfig(response.data);
        const subOptionMeta = developerMetadata?.subOptions?.[subOptionName];
        toast({
          title: 'Success',
          description: `${subOptionMeta?.name ?? subOptionName} ${enabled ? 'enabled' : 'disabled'}`,
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

  const handleToggleApprovedQueries = async (enabled: boolean) => {
    if (!pid || !config) return;

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        toolGroups: {
          [TOOL_GROUP_IDS.APPROVED_QUERIES]: {
            enabled,
            allowOntologyMaintenance,
          },
        },
      });

      if (response.success && response.data) {
        setConfig(response.data);
        if (enabled) {
          toast({
            title: 'Business User Tools Selected',
          });
        }
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

  const handleToggleDevTools = async (enabled: boolean) => {
    await handleToggleToolGroup(TOOL_GROUP_IDS.DEVELOPER, enabled);
  };

  const handleToggleAgentTools = async (enabled: boolean) => {
    if (!pid || !config) return;

    if (enabled) {
      // Fetch revealed API key for the setup example
      try {
        const response = await engineApi.getAgentAPIKey(pid, true);
        if (response.success && response.data) {
          setAgentApiKey(response.data.key);
        }
      } catch (error) {
        console.error('Failed to fetch agent API key:', error);
      }
    }

    await handleToggleToolGroup(TOOL_GROUP_IDS.AGENT_TOOLS, enabled);
  };

  const handleToggleToolGroup = async (groupName: string, enabled: boolean) => {
    if (!pid || !config) return;

    const metadata = TOOL_GROUP_METADATA[groupName];

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        toolGroups: {
          [groupName]: { enabled },
        },
      });

      if (response.success && response.data) {
        setConfig(response.data);
        if (enabled) {
          toast({
            title: `${metadata?.name ?? groupName} Selected`,
          });
        }
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

  // Build sub-options for approved_queries by merging state with frontend metadata
  const buildApprovedQueriesSubOptions = (): Record<string, SubOptionInfo> | undefined => {
    if (!approvedQueriesMetadata?.subOptions) return undefined;

    const subOptions: Record<string, SubOptionInfo> = {};

    for (const [subName, subMeta] of Object.entries(approvedQueriesMetadata.subOptions)) {
      subOptions[subName] = {
        enabled: getSubOptionEnabled(approvedQueriesState, subName),
        name: subMeta.name,
        description: subMeta.description,
        warning: subMeta.warning,
        tip: subMeta.tip,
      };
    }

    return subOptions;
  };

  // Build sub-options for developer tools by merging state with frontend metadata
  const buildDeveloperSubOptions = (): Record<string, SubOptionInfo> | undefined => {
    if (!developerMetadata?.subOptions) return undefined;

    const subOptions: Record<string, SubOptionInfo> = {};

    for (const [subName, subMeta] of Object.entries(developerMetadata.subOptions)) {
      subOptions[subName] = {
        enabled: getSubOptionEnabled(developerState, subName),
        name: subMeta.name,
        description: subMeta.description,
        warning: subMeta.warning,
        tip: subMeta.tip,
      };
    }

    return subOptions;
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
            <MCPServerURL
              serverUrl={config.serverUrl}
              docsUrl={`https://us.ekaya.ai/mcp-setup?mcp_url=${encodeURIComponent(config.serverUrl)}`}
              agentMode={isAgentToolsEnabled}
              agentApiKey={agentApiKey}
              enabledTools={sortedEnabledTools}
            />

            {/* Tool Configuration Section */}
            <div className="border-t border-border-light pt-6">
              <h2 className="mb-2 text-xl font-semibold text-text-primary">
                Tool Configuration
              </h2>
              <p className="mb-4 text-sm text-text-secondary">
                Configure the tools exposed to the MCP Client. If you need multiple configurations then create a separate project for each configuration. This ensures that only those project members and agents will have access to their intended tools. This is the safest way to isolate access to your datasource.
              </p>
              <div className="space-y-4">
                {/* Business User Tools - Pre-Approved Queries */}
                <MCPToolGroup
                  name="Business User Tools"
                  description={<>Enable pre-approved SQL queries and ad-hoc query capabilities. The MCP Client can use <Link to={`/projects/${pid}/queries`} className="text-brand-purple hover:underline">Pre-Approved Queries</Link> and the Ontology to craft SQL for ad-hoc requests.</>}
                  enabled={isApprovedQueriesEnabled}
                  onToggle={handleToggleApprovedQueries}
                  disabled={updating}
                  {...(isApprovedQueriesEnabled ? {
                    subOptions: buildApprovedQueriesSubOptions(),
                    onSubOptionToggle: handleToggleApprovedQueriesSubOption,
                  } : {})}
                />

                {/* Agent Tools Section */}
                {config.toolGroups[TOOL_GROUP_IDS.AGENT_TOOLS] && TOOL_GROUP_METADATA[TOOL_GROUP_IDS.AGENT_TOOLS] && (
                  <MCPToolGroup
                    name={TOOL_GROUP_METADATA[TOOL_GROUP_IDS.AGENT_TOOLS]!.name}
                    description={<>Enable AI Agents to access the database safely and securely with logging and auditing capabilities. AI Agents can only use the enabled <Link to={`/projects/${pid}/queries`} className="text-brand-purple hover:underline">Pre-Approved Queries</Link> so that you have full control over access.</>}
                    enabled={config.toolGroups[TOOL_GROUP_IDS.AGENT_TOOLS]!.enabled}
                    onToggle={handleToggleAgentTools}
                    disabled={updating}
                  >
                    {/* API Key section rendered inside the card */}
                    <div className="mt-4 border-t border-border-light pt-4 pl-4">
                      <AgentAPIKeyDisplay projectId={pid!} onKeyChange={setAgentApiKey} />
                      {/* Warning at bottom */}
                      <div className="mt-3 flex items-start gap-2 rounded-md bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-950 dark:text-amber-200">
                        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                        <span>Distribute keys carefully and rotate them periodically.</span>
                      </div>
                    </div>
                  </MCPToolGroup>
                )}

                {/* Developer Tools */}
                {config.toolGroups[TOOL_GROUP_IDS.DEVELOPER] && developerMetadata && (
                  <MCPToolGroup
                    name={developerMetadata.name}
                    description={developerMetadata.description}
                    enabled={isDeveloperEnabled}
                    onToggle={handleToggleDevTools}
                    disabled={updating}
                    {...(isDeveloperEnabled && developerMetadata.warning != null ? { warning: developerMetadata.warning } : {})}
                    {...(isDeveloperEnabled ? {
                      subOptions: buildDeveloperSubOptions(),
                      onSubOptionToggle: handleToggleDeveloperSubOption,
                    } : {})}
                  />
                )}
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  );
};

export default MCPServerPage;
