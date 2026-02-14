import {
  ArrowLeft,
  Check,
  Copy,
  ExternalLink,
  Loader2,
  ScrollText,
  Trash2,
} from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';

import UserToolsSection from '../components/mcp/UserToolsSection';
import SetupChecklist from '../components/SetupChecklist';
import type { ChecklistItem } from '../components/SetupChecklist';
import { Button } from '../components/ui/Button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/Card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../components/ui/Dialog';
import { Input } from '../components/ui/Input';
import { TOOL_GROUP_IDS } from '../constants/mcpToolMetadata';
import { useConfig } from '../contexts/ConfigContext';
import { useToast } from '../hooks/useToast';
import engineApi from '../services/engineApi';
import type { DAGStatusResponse, Datasource, InstalledApp, MCPConfigResponse, ServerStatusResponse } from '../types';

// Developer tools added to the MCP Server by AI Data Liaison installation
const DATA_LIAISON_DEVELOPER_TOOLS = [
  { name: 'list_query_suggestions', description: 'View pending query suggestions' },
  { name: 'approve_query_suggestion', description: 'Approve a suggested query' },
  { name: 'reject_query_suggestion', description: 'Reject a suggested query with feedback' },
  { name: 'create_approved_query', description: 'Create query directly (bypass suggestion)' },
  { name: 'update_approved_query', description: 'Update an existing query' },
  { name: 'delete_approved_query', description: 'Delete a query' },
];

const AIDataLiaisonPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const { config: appConfig } = useConfig();
  const { toast } = useToast();

  // Uninstall dialog state
  const [confirmText, setConfirmText] = useState('');
  const [isUninstalling, setIsUninstalling] = useState(false);
  const [showUninstallDialog, setShowUninstallDialog] = useState(false);

  // Checklist state
  const [loading, setLoading] = useState(true);
  const [mcpServerReady, setMcpServerReady] = useState(false);
  const [serverStatus, setServerStatus] = useState<ServerStatusResponse | null>(null);
  const [mcpConfig, setMcpConfig] = useState<MCPConfigResponse | null>(null);
  const [installedApp, setInstalledApp] = useState<InstalledApp | null>(null);
  const [activating, setActivating] = useState(false);
  const [copied, setCopied] = useState(false);
  const [updatingConfig, setUpdatingConfig] = useState(false);

  // User Tools config from MCP config
  const allowOntologyMaintenance =
    mcpConfig?.toolGroups[TOOL_GROUP_IDS.USER]?.allowOntologyMaintenance ?? true;

  const fetchChecklistData = useCallback(async () => {
    if (!pid) return;

    setLoading(true);
    try {
      // Fetch MCP config, datasources, server status, and installed app in parallel
      const [mcpConfigRes, datasourcesRes, serverStatusRes, installedAppRes] = await Promise.all([
        engineApi.getMCPConfig(pid),
        engineApi.listDataSources(pid),
        engineApi.getServerStatus(),
        engineApi.getInstalledApp(pid, 'ai-data-liaison').catch(() => null),
      ]);

      setMcpConfig(mcpConfigRes.data ?? null);
      setServerStatus(serverStatusRes);
      setInstalledApp(installedAppRes?.data ?? null);

      // Check if MCP Server is ready: ontology DAG completed implies all prereqs are met
      const ds: Datasource | null = datasourcesRes.data?.datasources?.[0] ?? null;
      let dagCompleted = false;
      if (ds) {
        try {
          const dagRes = await engineApi.getOntologyDAGStatus(pid, ds.datasource_id);
          const dagData: DAGStatusResponse | null = dagRes.data ?? null;
          dagCompleted = dagData?.status === 'completed';
        } catch {
          // DAG might not exist yet
        }
      }
      setMcpServerReady(dagCompleted);
    } catch (error) {
      console.error('Failed to fetch checklist data:', error);
      toast({
        title: 'Error',
        description: 'Failed to load configuration status',
        variant: 'destructive',
      });
    } finally {
      setLoading(false);
    }
  }, [pid, toast]);

  useEffect(() => {
    fetchChecklistData();
  }, [fetchChecklistData]);

  // Process callback from central redirect (e.g., after billing confirmation)
  useEffect(() => {
    const callbackAction = searchParams.get('callback_action');
    const callbackState = searchParams.get('callback_state');
    const callbackApp = searchParams.get('callback_app');

    if (!callbackAction || !callbackState || !callbackApp || !pid) return;

    // Clear callback params from URL immediately to prevent re-processing
    setSearchParams({}, { replace: true });

    const processCallback = async () => {
      try {
        const response = await engineApi.completeAppCallback(
          pid, callbackApp, callbackAction, 'success', callbackState
        );
        if (response.error) {
          toast({ title: 'Error', description: response.error, variant: 'destructive' });
          return;
        }
        // Navigate based on completed action
        if (callbackAction === 'uninstall') {
          navigate(`/projects/${pid}`);
        } else {
          // For install/activate, refresh data to show updated state
          await fetchChecklistData();
        }
      } catch (error) {
        toast({
          title: 'Error',
          description: error instanceof Error ? error.message : 'Failed to complete action',
          variant: 'destructive',
        });
      }
    };

    processCallback();
  }, [searchParams, setSearchParams, pid, navigate, toast, fetchChecklistData]);

  const handleUninstall = async () => {
    if (confirmText !== 'uninstall application' || !pid) return;

    setIsUninstalling(true);
    try {
      const response = await engineApi.uninstallApp(pid, 'ai-data-liaison');
      if (response.error) {
        toast({
          title: 'Error',
          description: response.error,
          variant: 'destructive',
        });
        return;
      }
      if (response.data?.redirectUrl) {
        window.location.href = response.data.redirectUrl;
        return;
      }
      navigate(`/projects/${pid}`);
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to uninstall application',
        variant: 'destructive',
      });
    } finally {
      setIsUninstalling(false);
    }
  };

  const handleCopyUrl = async () => {
    if (!mcpConfig?.serverUrl) return;
    try {
      await navigator.clipboard.writeText(mcpConfig.serverUrl);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy URL:', err);
    }
  };

  const handleActivate = async () => {
    if (!pid) return;

    setActivating(true);
    try {
      const response = await engineApi.activateApp(pid, 'ai-data-liaison');
      if (response.data?.redirectUrl) {
        window.location.href = response.data.redirectUrl;
        return;
      }
      // Activation succeeded without redirect — refresh data
      await fetchChecklistData();
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to activate application',
        variant: 'destructive',
      });
    } finally {
      setActivating(false);
    }
  };

  const handleAllowOntologyMaintenanceChange = async (enabled: boolean) => {
    if (!pid) return;

    try {
      setUpdatingConfig(true);
      const response = await engineApi.updateMCPConfig(pid, {
        allowOntologyMaintenance: enabled,
      });

      if (response.success && response.data) {
        setMcpConfig(response.data);
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
      setUpdatingConfig(false);
    }
  };

  // Build checklist items based on current state
  const getChecklistItems = (): ChecklistItem[] => {
    const items: ChecklistItem[] = [];

    const isAccessible = serverStatus?.accessible_for_business_users ?? false;

    // 1. MCP Server set up
    items.push({
      id: 'mcp-server',
      title: 'MCP Server set up',
      description: mcpServerReady
        ? 'Datasource, schema, AI, and ontology configured'
        : 'Configure datasource, schema, AI, and extract ontology',
      status: loading ? 'loading' : mcpServerReady ? 'complete' : 'pending',
      link: `/projects/${pid}/mcp-server`,
      linkText: mcpServerReady ? 'Manage' : 'Configure',
    });

    // 2. MCP Server accessible to business users (optional — not required for activation)
    items.push({
      id: 'server-accessible',
      title: 'MCP Server accessible',
      description: isAccessible
        ? 'Server is reachable by business users over HTTPS'
        : 'Optional \u2014 configure when ready to share with business users',
      status: loading ? 'loading' : isAccessible ? 'complete' : 'pending',
      link: `/projects/${pid}/server-setup`,
      linkText: isAccessible ? 'Review' : 'Configure',
    });

    // 3. Activate (only requires step 1)
    const activated = installedApp?.activated_at != null;
    items.push({
      id: 'activate',
      title: 'Activate AI Data Liaison',
      description: activated
        ? 'AI Data Liaison activated'
        : mcpServerReady
          ? 'Activate to enable billing and start using the application'
          : 'Complete step 1 before activating',
      status: loading ? 'loading' : activated ? 'complete' : 'pending',
      disabled: !mcpServerReady && !activated,
      ...(activated ? {} : {
        onAction: handleActivate,
        actionText: 'Activate',
        actionDisabled: activating || !mcpServerReady,
      }),
    });

    return items;
  };

  const checklistItems = getChecklistItems();
  const allComplete = checklistItems.every((item) => item.status === 'complete');

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-text-secondary" />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header with back button */}
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="icon"
          aria-label="Back to project dashboard"
          onClick={() => navigate(`/projects/${pid}`)}
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-2xl font-bold">AI Data Liaison</h1>
          <p className="text-text-secondary">
            Ekaya acts as a data liaison between you and your business users. This application extends the Ekaya Engine with MCP tools that enable usage to enhance and extend the ontology and UI for you to manage queries suggested by users.
          </p>
        </div>
      </div>

      {/* Setup Checklist */}
      <SetupChecklist
        items={checklistItems}
        title="Setup Checklist"
        description="Complete these steps to enable AI Data Liaison"
        completeDescription="AI Data Liaison is ready for business users"
      />

      {/* MCP URL Quick Access (only when ready) */}
      {mcpConfig?.serverUrl && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Share with Business Users</CardTitle>
            <CardDescription>
              Business users connect their Claude Desktop to this MCP Server URL
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center gap-2">
              <div className="flex-1 rounded-lg border border-border-light bg-surface-secondary px-4 py-3 font-mono text-sm text-text-primary">
                {mcpConfig.serverUrl}
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
              href={`${appConfig?.authServerUrl}/mcp-setup?mcp_url=${encodeURIComponent(mcpConfig.serverUrl)}`}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 text-sm text-brand-purple hover:underline"
            >
              <ExternalLink className="h-4 w-4" />
              MCP Setup Instructions for Business Users
            </a>
          </CardContent>
        </Card>
      )}

      {/* User Tools Section */}
      {mcpConfig && (
        <UserToolsSection
          projectId={pid ?? ''}
          allowOntologyMaintenance={allowOntologyMaintenance}
          onAllowOntologyMaintenanceChange={handleAllowOntologyMaintenanceChange}
          enabledTools={mcpConfig.userTools}
          disabled={updatingConfig}
        />
      )}

      {/* Additional Developer Tools */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Additional Developer Tools</CardTitle>
          <CardDescription>
            AI Data Liaison adds these tools to the MCP Server for data engineers
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {DATA_LIAISON_DEVELOPER_TOOLS.map((tool) => (
              <div
                key={tool.name}
                className="flex items-center gap-3 rounded border border-border-light bg-surface-secondary px-3 py-2"
              >
                <code className="text-xs font-mono text-brand-purple">{tool.name}</code>
                <span className="text-sm text-text-secondary">{tool.description}</span>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Auditing (only when setup is complete) */}
      {allComplete && (
        <Card>
          <CardHeader>
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-brand-purple/10">
                <ScrollText className="h-5 w-5 text-brand-purple" />
              </div>
              <div>
                <CardTitle className="text-lg">Auditing</CardTitle>
                <CardDescription>
                  Review query executions, ontology changes, schema changes, and query approvals
                </CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <Link to={`/projects/${pid}/audit`}>
              <Button variant="outline">
                <ScrollText className="mr-2 h-4 w-4" />
                View Audit Log
              </Button>
            </Link>
          </CardContent>
        </Card>
      )}

      {/* Danger Zone */}
      <Card className="border-red-200 dark:border-red-900">
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-red-500/10">
              <Trash2 className="h-5 w-5 text-red-500" />
            </div>
            <div>
              <CardTitle className="text-red-600 dark:text-red-400">Danger Zone</CardTitle>
              <CardDescription>Remove AI Data Liaison from this project</CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-text-secondary mb-4">
            Uninstalling AI Data Liaison will disable the query suggestion workflow. Business users
            will no longer be able to suggest queries, and data engineers will lose access to
            suggestion management tools.
          </p>
          <Button
            variant="outline"
            onClick={() => setShowUninstallDialog(true)}
            className="text-red-600 hover:text-red-700 hover:bg-red-50 border-red-300"
          >
            <Trash2 className="mr-2 h-4 w-4" />
            Uninstall Application
          </Button>
        </CardContent>
      </Card>

      {/* Uninstall Confirmation Dialog */}
      <Dialog
        open={showUninstallDialog}
        onOpenChange={(open) => {
          setShowUninstallDialog(open);
          if (!open) {
            setConfirmText('');
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Uninstall AI Data Liaison?</DialogTitle>
            <DialogDescription>
              This will disable the query suggestion workflow. Business users will no longer be able
              to suggest queries for approval.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <label className="text-sm font-medium text-text-primary">
              Type{' '}
              <span className="font-mono bg-gray-100 dark:bg-gray-800 px-1 rounded">
                uninstall application
              </span>{' '}
              to confirm
            </label>
            <Input
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
              placeholder="uninstall application"
              className="mt-2"
              disabled={isUninstalling}
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowUninstallDialog(false)}
              disabled={isUninstalling}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleUninstall}
              disabled={confirmText !== 'uninstall application' || isUninstalling}
            >
              {isUninstalling ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Uninstalling...
                </>
              ) : (
                'Uninstall Application'
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default AIDataLiaisonPage;
