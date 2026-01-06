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
} from '../constants/mcpToolMetadata';
import { useDatasourceConnection } from '../contexts/DatasourceConnectionContext';
import { useToast } from '../hooks/useToast';
import engineApi from '../services/engineApi';
import type { MCPConfigResponse, SubOptionInfo, ToolGroupState } from '../types';

// Helper to get sub-option enabled state from flat ToolGroupState
const getSubOptionEnabled = (state: ToolGroupState | undefined, subOptionName: string): boolean => {
  if (!state) return false;
  switch (subOptionName) {
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

const SHOW_AI_DATA_LIAISON = false;

const MCPServerPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();
  const { selectedDatasource } = useDatasourceConnection();

  const [config, setConfig] = useState<MCPConfigResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [updating, setUpdating] = useState(false);
  const [enabledQueryCount, setEnabledQueryCount] = useState(0);
  const [secureAdhocEnabled, setSecureAdhocEnabled] = useState(false);
  const [agentApiKey, setAgentApiKey] = useState<string>('');

  // Read approved_queries config from backend (now flat state structure)
  const approvedQueriesState = config?.toolGroups[TOOL_GROUP_IDS.APPROVED_QUERIES];
  const isApprovedQueriesEnabled = approvedQueriesState?.enabled ?? false;
  const forceMode = approvedQueriesState?.forceMode ?? false;
  const allowClientSuggestions = approvedQueriesState?.allowClientSuggestions ?? false;
  const isAgentToolsEnabled = config?.toolGroups[TOOL_GROUP_IDS.AGENT_TOOLS]?.enabled ?? false;

  // Get metadata for approved_queries
  const approvedQueriesMetadata = TOOL_GROUP_METADATA[TOOL_GROUP_IDS.APPROVED_QUERIES];

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

  // Fetch enabled query count when datasource is available
  useEffect(() => {
    const fetchEnabledQueryCount = async () => {
      if (!pid || !selectedDatasource?.datasourceId) return;

      try {
        const response = await engineApi.listQueries(pid, selectedDatasource.datasourceId);
        if (response.success && response.data) {
          const count = response.data.queries.filter(q => q.is_enabled).length;
          setEnabledQueryCount(count);
        }
      } catch (error) {
        console.error('Failed to fetch enabled query count:', error);
      }
    };

    fetchEnabledQueryCount();
  }, [pid, selectedDatasource?.datasourceId]);

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

    // Handle UI-only secureAdhocRequests option
    if (subOptionName === 'secureAdhocRequests') {
      setSecureAdhocEnabled(enabled);
      toast({
        title: 'Success',
        description: `Secure Ad-Hoc Requests ${enabled ? 'enabled' : 'disabled'}`,
      });
      return;
    }

    // Special handling for FORCE mode
    if (subOptionName === 'forceMode' && enabled && config.toolGroups[TOOL_GROUP_IDS.DEVELOPER]?.enabled) {
      // Auto-disable developer tools when enabling FORCE mode
      await handleToggleToolGroup(TOOL_GROUP_IDS.DEVELOPER, false);
    }

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        toolGroups: {
          [TOOL_GROUP_IDS.APPROVED_QUERIES]: {
            enabled: isApprovedQueriesEnabled,
            ...(subOptionName === 'forceMode' ? { forceMode: enabled } : { forceMode }),
            ...(subOptionName === 'allowClientSuggestions' ? { allowClientSuggestions: enabled } : { allowClientSuggestions }),
          },
        },
      });

      if (response.success && response.data) {
        setConfig(response.data);
        // Get sub-option name from frontend metadata
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

  const handleToggleApprovedQueries = async (enabled: boolean) => {
    if (!pid || !config) return;

    if (enabled && enabledQueryCount === 0) {
      toast({
        title: 'No enabled queries',
        description: 'Create and enable queries first.',
        variant: 'destructive',
      });
      return;
    }

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        toolGroups: {
          [TOOL_GROUP_IDS.APPROVED_QUERIES]: {
            enabled,
            forceMode,
            allowClientSuggestions,
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

  // Note: handleToggleSubOption is no longer used for developer tools (enableExecute removed).
  // It remains for approved_queries sub-options which are handled by handleToggleApprovedQueriesSubOption.

  // Build sub-options for approved_queries by merging state with frontend metadata
  const buildApprovedQueriesSubOptions = (): Record<string, SubOptionInfo> | undefined => {
    if (!approvedQueriesMetadata?.subOptions) return undefined;

    const subOptions: Record<string, SubOptionInfo> = {
      // UI-only option (not persisted to backend)
      secureAdhocRequests: {
        enabled: secureAdhocEnabled,
        name: 'Secure Ad-Hoc Requests [Recommended]',
        description: secureAdhocEnabled ? (
          <>
            Examine the SQL generated by the MCP Client to prevent injection attacks and detect potential data leakage. This requires Ekaya&apos;s Security models.
            <p className="mt-2 text-center">
              Contact <a href="mailto:sales@ekaya.ai?subject=Add Security Models to my installation" className="text-brand-purple hover:underline">sales@ekaya.ai</a> to discuss embedding secure, dedicated models so data never leaves your data boundary.
            </p>
          </>
        ) : (
          'Examine the SQL generated by the MCP Client to prevent injection attacks and detect potential data leakage. This requires Ekaya\'s Security models.'
        ),
      },
    };

    // Add backend-persisted sub-options from metadata
    for (const [subName, subMeta] of Object.entries(approvedQueriesMetadata.subOptions)) {
      subOptions[subName] = {
        enabled: getSubOptionEnabled(approvedQueriesState, subName),
        name: subMeta.name,
        description: subMeta.description,
        warning: subMeta.warning,
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
              enabledTools={config.enabledTools}
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
                  description={
                    enabledQueryCount > 0
                      ? <>Enable access to <Link to={`/projects/${pid}/queries`} className="text-brand-purple hover:underline">Pre-Approved Queries</Link>. The MCP Client will use the Pre-Approved Queries if they match the Business User&apos;s request and fall back on using the Ontology to craft new SQL queries to answer their questions. This offers the most flexibility in terms of answering ad-hoc requests.</>
                      : <>No enabled queries. <Link to={`/projects/${pid}/queries`} className="text-brand-purple hover:underline">Create Pre-Approved Queries</Link> to enable this tool.</>
                  }
                  enabled={enabledQueryCount > 0 && isApprovedQueriesEnabled}
                  onToggle={handleToggleApprovedQueries}
                  disabled={updating || enabledQueryCount === 0}
                />

                {/* AI Data Liaison */}
                {SHOW_AI_DATA_LIAISON && (
                  <MCPToolGroup
                    name="AI Data Liaison"
                    description={
                      enabledQueryCount > 0
                        ? <>Enable access to <Link to={`/projects/${pid}/queries`} className="text-brand-purple hover:underline">Pre-Approved Queries</Link>. The MCP Client will use the Pre-Approved Queries if they match the Business User&apos;s request and fall back on using the Ontology to craft new SQL queries to answer their questions. This offers the most flexibility in terms of answering ad-hoc requests.</>
                        : <>No enabled queries. <Link to={`/projects/${pid}/queries`} className="text-brand-purple hover:underline">Create Pre-Approved Queries</Link> to enable this tool.</>
                    }
                    enabled={enabledQueryCount > 0 && isApprovedQueriesEnabled}
                    onToggle={handleToggleApprovedQueries}
                    disabled={updating || enabledQueryCount === 0}
                    {...(enabledQueryCount > 0 && isApprovedQueriesEnabled ? {
                      subOptions: buildApprovedQueriesSubOptions(),
                      onSubOptionToggle: handleToggleApprovedQueriesSubOption,
                    } : {})}
                  />
                )}

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

                {/* Other Tool Groups (excluding approved_queries and agent_tools which are handled above) */}
                {Object.entries(config.toolGroups)
                  .filter(([groupName]) => groupName !== TOOL_GROUP_IDS.APPROVED_QUERIES && groupName !== TOOL_GROUP_IDS.AGENT_TOOLS)
                  .map(([groupName, groupState]) => {
                    // Get metadata for this tool group
                    const metadata = TOOL_GROUP_METADATA[groupName];
                    if (!metadata) {
                      console.warn(`Unknown tool group: ${groupName}`);
                      return null;
                    }

                    // Use custom handler for developer tools
                    const onToggle = groupName === TOOL_GROUP_IDS.DEVELOPER
                      ? handleToggleDevTools
                      : (enabled: boolean) => handleToggleToolGroup(groupName, enabled);

                    // Pro Tip for Developer Tools (only show when enabled)
                    const developerToolsTip = groupName === TOOL_GROUP_IDS.DEVELOPER && groupState.enabled ? (
                      <div>
                        <span className="font-semibold">Pro Tip:</span> Use Developer Tools to help you answer questions about your Ontology{' '}
                        <details className="inline">
                          <summary className="inline cursor-pointer underline">(more info)</summary>
                          <p className="mt-2 font-normal">
                            After you have extracted your Ontology there might be questions that Ekaya cannot answer from the database schema and values alone. Connect your IDE to the MCP Server so that your LLM can answer questions by reviewing your codebase or other project documents saving you time.
                          </p>
                        </details>
                      </div>
                    ) : undefined;

                    // Only show warning when the tool group is enabled
                    const showWarning = groupState.enabled && metadata.warning != null;

                    const props = {
                      name: metadata.name,
                      description: metadata.description,
                      enabled: groupState.enabled,
                      onToggle,
                      disabled: updating,
                      ...(showWarning ? { warning: metadata.warning } : {}),
                      ...(developerToolsTip != null ? { tip: developerToolsTip } : {}),
                    };
                    return <MCPToolGroup key={groupName} {...props} />;
                  })}
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  );
};

export default MCPServerPage;
