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
  const [queryCount, setQueryCount] = useState(0);

  // Queries config state (V1: stored locally, will be persisted in backend later)
  const [forceMode, setForceMode] = useState(false);
  const [allowSuggestions, setAllowSuggestions] = useState(true);
  const [allowClientSuggestions, setAllowClientSuggestions] = useState(false);

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

  // Fetch query count when datasource is available
  useEffect(() => {
    const fetchQueryCount = async () => {
      if (!pid || !selectedDatasource?.datasourceId) return;

      try {
        const response = await engineApi.listQueries(pid, selectedDatasource.datasourceId);
        if (response.success && response.data) {
          setQueryCount(response.data.queries.length);
        }
      } catch (error) {
        console.error('Failed to fetch query count:', error);
      }
    };

    fetchQueryCount();
  }, [pid, selectedDatasource?.datasourceId]);

  const handleToggleForceMode = async (enabled: boolean) => {
    if (enabled && config?.toolGroups['developer']?.enabled) {
      // Auto-disable developer tools when enabling FORCE mode
      await handleToggleToolGroup('developer', false);
    }
    setForceMode(enabled);
    toast({
      title: 'Success',
      description: `FORCE Pre-Approved Queries ${enabled ? 'enabled' : 'disabled'}`,
    });
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
                    queryCount > 0
                      ? "Enable access to Pre-Approved Queries. This is the safest way to enable AI access to the datasource."
                      : <>No Pre-Approved Queries have been created. Click <Link to={`/projects/${pid}/queries`} className="text-brand-purple hover:underline">here</Link> to create some.</>
                  }
                  enabled={queryCount > 0 && allowSuggestions}
                  onToggle={(enabled) => {
                    if (queryCount === 0) {
                      toast({
                        title: 'No queries available',
                        description: 'Create Pre-Approved Queries first.',
                        variant: 'destructive',
                      });
                      return;
                    }
                    setAllowSuggestions(enabled);
                  }}
                  disabled={updating || queryCount === 0}
                  {...(queryCount > 0 && allowSuggestions ? {
                    subOptions: {
                      forceMode: {
                        enabled: forceMode,
                        name: 'FORCE all access through Pre-Approved Queries',
                        description: 'When enabled, MCP clients can only execute Pre-Approved Queries. This is the safest way to enable AI access to data.',
                        warning: 'Enabling this will disable Developer Tools.',
                      },
                      allowClientSuggestions: {
                        enabled: allowClientSuggestions,
                        name: 'Allow MCP Client to suggest queries',
                        description: 'Allow the MCP Client to suggest new queries that must be approved by an administrator. This will expose the Ontology and SQL of Pre-Approved Queries.',
                      },
                    },
                    onSubOptionToggle: (subOptionName: string, enabled: boolean) => {
                      if (subOptionName === 'forceMode') {
                        handleToggleForceMode(enabled);
                      } else if (subOptionName === 'allowClientSuggestions') {
                        setAllowClientSuggestions(enabled);
                      }
                    },
                  } : {})}
                />

                {/* Developer Tools - Last */}
                {Object.entries(config.toolGroups).map(([groupName, groupInfo]) => {
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
