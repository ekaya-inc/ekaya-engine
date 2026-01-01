import { ArrowLeft, Loader2 } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';

import MCPLogo from '../components/icons/MCPLogo';
import MCPServerURL from '../components/mcp/MCPServerURL';
import MCPToolGroup from '../components/mcp/MCPToolGroup';
import { Button } from '../components/ui/Button';
import { useDatasourceConnection } from '../contexts/DatasourceConnectionContext';
import { useToast } from '../hooks/useToast';
import engineApi from '../services/engineApi';
import type { MCPConfigResponse } from '../types';

const MCPServerPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();
  const { selectedDatasource } = useDatasourceConnection();

  const [config, setConfig] = useState<MCPConfigResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [updating, setUpdating] = useState(false);
  const [enabledQueryCount, setEnabledQueryCount] = useState(0);

  // Read approved_queries config from backend
  const approvedQueriesConfig = config?.toolGroups['approved_queries'];
  const isApprovedQueriesEnabled = approvedQueriesConfig?.enabled ?? false;
  const forceMode = approvedQueriesConfig?.subOptions?.['forceMode']?.enabled ?? false;
  const allowClientSuggestions = approvedQueriesConfig?.subOptions?.['allowClientSuggestions']?.enabled ?? false;

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

  const handleToggleApprovedQueriesSubOption = async (subOptionName: string, enabled: boolean) => {
    if (!pid || !config) return;

    // Special handling for FORCE mode
    if (subOptionName === 'forceMode' && enabled && config.toolGroups['developer']?.enabled) {
      // Auto-disable developer tools when enabling FORCE mode
      await handleToggleToolGroup('developer', false);
    }

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        toolGroups: {
          approved_queries: {
            enabled: isApprovedQueriesEnabled,
            ...(subOptionName === 'forceMode' ? { forceMode: enabled } : { forceMode }),
            ...(subOptionName === 'allowClientSuggestions' ? { allowClientSuggestions: enabled } : { allowClientSuggestions }),
          },
        },
      });

      if (response.success && response.data) {
        setConfig(response.data);
        const subOptionInfo = approvedQueriesConfig?.subOptions?.[subOptionName];
        toast({
          title: 'Success',
          description: `${subOptionInfo?.name ?? subOptionName} ${enabled ? 'enabled' : 'disabled'}`,
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

    if (!enabled && enabledQueryCount === 0) {
      toast({
        title: 'No enabled queries',
        description: 'Create and enable Pre-Approved Queries first.',
        variant: 'destructive',
      });
      return;
    }

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        toolGroups: {
          approved_queries: {
            enabled,
            forceMode,
            allowClientSuggestions,
          },
        },
      });

      if (response.success && response.data) {
        setConfig(response.data);
        toast({
          title: 'Success',
          description: `Pre-Approved Queries ${enabled ? 'enabled' : 'disabled'}`,
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

  const handleToggleDevTools = async (enabled: boolean) => {
    if (enabled && forceMode) {
      toast({
        title: 'Not allowed',
        description: 'Only Pre-Approved Queries are allowed. Disable FORCE mode first.',
        variant: 'destructive',
      });
      return;
    }
    await handleToggleToolGroup('developer', enabled);
  };

  const handleToggleToolGroup = async (groupName: string, enabled: boolean) => {
    if (!pid || !config) return;

    const groupInfo = config.toolGroups[groupName];
    // Preserve existing sub-option values when toggling the main switch
    const enableExecute = groupInfo?.subOptions?.['enableExecute']?.enabled ?? false;

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        toolGroups: {
          [groupName]: { enabled, enableExecute },
        },
      });

      if (response.success && response.data) {
        setConfig(response.data);
        toast({
          title: 'Success',
          description: `${groupInfo?.name ?? groupName} ${enabled ? 'enabled' : 'disabled'}`,
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

  const handleToggleSubOption = async (groupName: string, subOptionName: string, enabled: boolean) => {
    if (!pid || !config) return;

    const groupInfo = config.toolGroups[groupName];
    const subOptionInfo = groupInfo?.subOptions?.[subOptionName];

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        toolGroups: {
          [groupName]: {
            enabled: groupInfo?.enabled ?? false,
            ...(subOptionName === 'enableExecute' ? { enableExecute: enabled } : {}),
          },
        },
      });

      if (response.success && response.data) {
        setConfig(response.data);
        toast({
          title: 'Success',
          description: `${subOptionInfo?.name ?? subOptionName} ${enabled ? 'enabled' : 'disabled'}`,
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
              docsUrl="https://docs.ekaya.ai/mcp-setup"
            />

            {/* Tool Configuration Section */}
            <div className="border-t border-border-light pt-6">
              <h2 className="mb-4 text-xl font-semibold text-text-primary">
                Tool Configuration
              </h2>
              <div className="space-y-4">
                {/* Pre-Approved Queries Tool Group - First */}
                <MCPToolGroup
                  name="Pre-Approved Queries"
                  description={
                    enabledQueryCount > 0
                      ? <>Enable access to Pre-Approved Queries. Click <Link to={`/projects/${pid}/queries`} className="text-brand-purple hover:underline">here</Link> to manage queries.</>
                      : <>No enabled Pre-Approved Queries. Click <Link to={`/projects/${pid}/queries`} className="text-brand-purple hover:underline">here</Link> to create and enable queries.</>
                  }
                  enabled={enabledQueryCount > 0 && isApprovedQueriesEnabled}
                  onToggle={handleToggleApprovedQueries}
                  disabled={updating || enabledQueryCount === 0}
                  {...(enabledQueryCount > 0 && isApprovedQueriesEnabled && approvedQueriesConfig?.subOptions != null ? {
                    subOptions: approvedQueriesConfig.subOptions,
                    onSubOptionToggle: handleToggleApprovedQueriesSubOption,
                  } : {})}
                />

                {/* Other Tool Groups (excluding approved_queries which is handled above) */}
                {Object.entries(config.toolGroups)
                  .filter(([groupName]) => groupName !== 'approved_queries')
                  .map(([groupName, groupInfo]) => {
                  // Use custom handler for developer tools to enforce FORCE mode
                  const onToggle = groupName === 'developer'
                    ? handleToggleDevTools
                    : (enabled: boolean) => handleToggleToolGroup(groupName, enabled);

                  const props = {
                    name: groupInfo.name,
                    description: groupInfo.description,
                    enabled: groupInfo.enabled,
                    onToggle,
                    disabled: updating,
                    ...(groupInfo.warning != null ? { warning: groupInfo.warning } : {}),
                    ...(groupInfo.subOptions != null ? { subOptions: groupInfo.subOptions } : {}),
                    onSubOptionToggle: (subOptionName: string, enabled: boolean) =>
                      handleToggleSubOption(groupName, subOptionName, enabled),
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
