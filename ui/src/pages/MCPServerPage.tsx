import { ArrowLeft, ExternalLink, Loader2, Check, Copy } from 'lucide-react';
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
import { useToast } from '../hooks/useToast';
import engineApi from '../services/engineApi';
import type { AIConfigResponse, DAGStatusResponse, Datasource, MCPConfigResponse } from '../types';

const MCPServerPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { config: appConfig } = useConfig();
  const { toast } = useToast();

  const [config, setConfig] = useState<MCPConfigResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [updating, setUpdating] = useState(false);
  const [copied, setCopied] = useState(false);

  // Setup checklist state
  const [datasource, setDatasource] = useState<Datasource | null>(null);
  const [hasSelectedTables, setHasSelectedTables] = useState(false);
  const [aiConfig, setAiConfig] = useState<AIConfigResponse | null>(null);
  const [dagStatus, setDagStatus] = useState<DAGStatusResponse | null>(null);

  // Read tool group configs from backend
  const developerState = config?.toolGroups[TOOL_GROUP_IDS.DEVELOPER];
  const addQueryTools = developerState?.addQueryTools ?? true;
  const addOntologyMaintenance = developerState?.addOntologyMaintenance ?? true;

  const fetchConfig = useCallback(async () => {
    if (!pid) return;

    try {
      setLoading(true);
      const [mcpRes, datasourcesRes, aiConfigRes] = await Promise.all([
        engineApi.getMCPConfig(pid),
        engineApi.listDataSources(pid),
        engineApi.getAIConfig(pid),
      ]);

      if (mcpRes.success && mcpRes.data) {
        setConfig(mcpRes.data);
      } else {
        throw new Error(mcpRes.error ?? 'Failed to load MCP configuration');
      }

      const ds = datasourcesRes.data?.datasources?.[0] ?? null;
      setDatasource(ds);
      setAiConfig(aiConfigRes.data ?? null);

      if (ds) {
        try {
          const schemaRes = await engineApi.getSchema(pid, ds.datasource_id);
          const hasSelections = schemaRes.data?.tables?.some((t) => t.is_selected === true) ?? false;
          setHasSelectedTables(hasSelections);
        } catch {
          setHasSelectedTables(false);
        }

        try {
          const dagRes = await engineApi.getOntologyDAGStatus(pid, ds.datasource_id);
          setDagStatus(dagRes.data ?? null);
        } catch {
          setDagStatus(null);
        }
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

  const getChecklistItems = (): ChecklistItem[] => {
    const items: ChecklistItem[] = [];

    // 1. Datasource configured
    items.push({
      id: 'datasource',
      title: 'Datasource configured',
      description: datasource
        ? `Connected to ${datasource.name} (${datasource.type})`
        : 'Connect a database to enable the MCP Server',
      status: loading ? 'loading' : datasource ? 'complete' : 'pending',
      link: `/projects/${pid}/datasource`,
      linkText: datasource ? 'Manage' : 'Configure',
    });

    // 2. Schema selected
    const schemaItem: ChecklistItem = {
      id: 'schema',
      title: 'Schema selected',
      description: hasSelectedTables
        ? 'Tables and columns selected for analysis'
        : datasource
          ? 'Select which tables and columns to include'
          : 'Configure datasource first',
      status: loading ? 'loading' : hasSelectedTables ? 'complete' : 'pending',
      linkText: hasSelectedTables ? 'Manage' : 'Configure',
    };
    if (datasource) {
      schemaItem.link = `/projects/${pid}/schema`;
    }
    items.push(schemaItem);

    // 3. AI configured
    const isAIConfigured = !!aiConfig?.config_type && aiConfig.config_type !== 'none';
    items.push({
      id: 'ai-config',
      title: 'AI configured',
      description: isAIConfigured
        ? 'AI model configured'
        : hasSelectedTables
          ? 'Configure an AI model for ontology extraction'
          : 'Configure datasource and select schema first',
      status: loading ? 'loading' : isAIConfigured ? 'complete' : 'pending',
      link: `/projects/${pid}/ai-config`,
      linkText: isAIConfigured ? 'Manage' : 'Configure',
    });

    // 4. Ontology extracted
    const ontologyComplete = dagStatus?.status === 'completed';
    const ontologyRunning = dagStatus?.status === 'running';
    const ontologyFailed = dagStatus?.status === 'failed';

    const ontologyItem: ChecklistItem = {
      id: 'ontology',
      title: 'Ontology extracted',
      description: ontologyComplete
        ? 'Schema semantics extracted and ready'
        : ontologyRunning
          ? `Extracting... (${dagStatus?.current_node ?? 'starting'})`
          : ontologyFailed
            ? 'Extraction failed - click to retry'
            : datasource
              ? 'Extract semantic understanding from your schema'
              : 'Configure datasource first',
      status: loading
        ? 'loading'
        : ontologyComplete
          ? 'complete'
          : ontologyFailed
            ? 'error'
            : 'pending',
      linkText: ontologyComplete ? 'Manage' : ontologyFailed ? 'Retry' : 'Configure',
    };
    if (datasource) {
      ontologyItem.link = `/projects/${pid}/ontology`;
    }
    items.push(ontologyItem);

    return items;
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

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-text-secondary" />
      </div>
    );
  }

  const checklistItems = getChecklistItems();

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
        {/* Setup Checklist */}
        <SetupChecklist
          items={checklistItems}
          title="Setup Checklist"
          description="Complete these steps to enable the MCP Server"
          completeDescription="MCP Server is ready"
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
