import { ArrowLeft, Loader2, Server } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import MCPServerURL from '../components/mcp/MCPServerURL';
import MCPToolGroup from '../components/mcp/MCPToolGroup';
import { Button } from '../components/ui/Button';
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
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-brand-purple/10">
            <Server className="h-5 w-5 text-brand-purple" />
          </div>
          <h1 className="text-3xl font-bold text-text-primary">MCP Server</h1>
        </div>
      </div>

      <div className="space-y-6">
        {config && (
          <>
            <MCPServerURL
              serverUrl={config.serverUrl}
              docsUrl="https://docs.ekaya.ai/mcp-setup"
            />

            <div className="border-t border-border-light pt-6">
              <h2 className="mb-4 text-xl font-semibold text-text-primary">
                Tool Configuration
              </h2>
              <div className="space-y-4">
                {Object.entries(config.toolGroups).map(([groupName, groupInfo]) => {
                  const props = {
                    name: groupInfo.name,
                    description: groupInfo.description,
                    enabled: groupInfo.enabled,
                    onToggle: (enabled: boolean) => handleToggleToolGroup(groupName, enabled),
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
