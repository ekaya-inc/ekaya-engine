import {
  Handshake,
  Loader2,
  ScrollText,
  Trash2,
} from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';

import AppPageHeader from '../components/AppPageHeader';
import MCPEnabledTools from '../components/mcp/MCPEnabledTools';
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
import { Switch } from '../components/ui/Switch';
import { useToast } from '../hooks/useToast';
import engineApi from '../services/engineApi';
import type { InstalledApp, MCPConfigResponse } from '../types';

const AIDataLiaisonPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const { toast } = useToast();

  // Uninstall dialog state
  const [confirmText, setConfirmText] = useState('');
  const [isUninstalling, setIsUninstalling] = useState(false);
  const [showUninstallDialog, setShowUninstallDialog] = useState(false);

  // Checklist state
  const [loading, setLoading] = useState(true);
  const [ontologyForgeReady, setOntologyForgeReady] = useState(false);
  const [glossaryReady, setGlossaryReady] = useState(false);
  const [mcpConfig, setMcpConfig] = useState<MCPConfigResponse | null>(null);
  const [installedApp, setInstalledApp] = useState<InstalledApp | null>(null);
  const [activating, setActivating] = useState(false);
  const [updatingConfig, setUpdatingConfig] = useState(false);

  // Per-app tool config from MCP config
  const addApprovalTools = mcpConfig?.toolGroups['tools']?.addApprovalTools ?? true;
  const addRequestTools = mcpConfig?.toolGroups['tools']?.addRequestTools ?? true;

  const fetchChecklistData = useCallback(async () => {
    if (!pid) return;

    setLoading(true);
    try {
      // Fetch MCP config, installed app status, ontology prerequisites, and glossary readiness in parallel.
      const [mcpConfigRes, installedAppRes, ontologyForgeRes, glossaryRes] = await Promise.all([
        engineApi.getMCPConfig(pid),
        engineApi.getInstalledApp(pid, 'ai-data-liaison').catch(() => null),
        engineApi.getInstalledApp(pid, 'ontology-forge').catch(() => null),
        engineApi.listGlossaryTerms(pid).catch(() => null),
      ]);

      setMcpConfig(mcpConfigRes.data ?? null);
      setInstalledApp(installedAppRes?.data ?? null);
      setOntologyForgeReady(ontologyForgeRes?.data?.activated_at != null);
      setGlossaryReady((glossaryRes?.data?.terms?.length ?? 0) > 0);
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
    const callbackStatus = searchParams.get('callback_status') ?? 'success';

    if (!callbackAction || !callbackState || !callbackApp || !pid) return;

    // Clear callback params from URL immediately to prevent re-processing
    setSearchParams({}, { replace: true });

    const processCallback = async () => {
      try {
        const response = await engineApi.completeAppCallback(
          pid, callbackApp, callbackAction, callbackStatus, callbackState
        );
        if (response.error) {
          toast({ title: 'Error', description: response.error, variant: 'destructive' });
          return;
        }
        if (callbackStatus === 'cancelled') {
          await fetchChecklistData();
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

    void processCallback();
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

  const handleApprovalToolsChange = async (enabled: boolean) => {
    if (!pid) return;

    try {
      setUpdatingConfig(true);
      const response = await engineApi.updateMCPConfig(pid, {
        addApprovalTools: enabled,
      });

      if (response.success && response.data) {
        setMcpConfig(response.data);
        toast({
          title: 'Success',
          description: `Approval Tools ${enabled ? 'enabled' : 'disabled'}`,
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

  const handleRequestToolsChange = async (enabled: boolean) => {
    if (!pid) return;

    try {
      setUpdatingConfig(true);
      const response = await engineApi.updateMCPConfig(pid, {
        addRequestTools: enabled,
      });

      if (response.success && response.data) {
        setMcpConfig(response.data);
        toast({
          title: 'Success',
          description: `Request Tools ${enabled ? 'enabled' : 'disabled'}`,
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
    const glossaryConfigured = glossaryReady;

    // 1. Ontology Forge set up
    items.push({
      id: 'ontology-forge',
      title: 'Ontology Forge set up',
      description: ontologyForgeReady
        ? 'Ontology Forge is configured and ready'
        : 'Set up Ontology Forge to extract your business semantic layer',
      status: loading ? 'loading' : ontologyForgeReady ? 'complete' : 'pending',
      link: `/projects/${pid}/ontology-forge`,
      linkText: ontologyForgeReady ? 'Manage' : 'Set up',
    });

    // 2. Glossary set up
    items.push({
      id: 'glossary',
      title: 'Glossary set up',
      description: glossaryConfigured
        ? 'Glossary is configured and ready'
        : ontologyForgeReady
          ? 'Set up the business glossary for consistent business terminology'
          : 'Complete step 1 first',
      status: loading ? 'loading' : glossaryConfigured ? 'complete' : 'pending',
      disabled: !ontologyForgeReady && !glossaryConfigured,
      link: `/projects/${pid}/glossary`,
      linkText: glossaryConfigured ? 'Manage' : 'Set up',
    });

    // 3. Activate AI Data Liaison after Ontology Forge and Glossary are ready
    const activated = installedApp?.activated_at != null;
    const activationReady = ontologyForgeReady && glossaryConfigured;
    items.push({
      id: 'activate',
      title: 'Activate AI Data Liaison',
      description: activated
        ? 'AI Data Liaison activated'
        : activationReady
          ? 'Activate to start using the application'
          : ontologyForgeReady
            ? 'Complete step 2 before activating'
            : 'Complete steps 1 and 2 before activating',
      status: loading ? 'loading' : activated ? 'complete' : 'pending',
      disabled: !activationReady && !activated,
      ...(activated ? {} : {
        onAction: handleActivate,
        actionText: 'Activate',
        actionDisabled: activating || !activationReady,
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
      <AppPageHeader
        title="AI Data Liaison"
        slug="ai-data-liaison"
        icon={<Handshake className="h-8 w-8 text-green-500" />}
        description="Ekaya acts as a data liaison between you and your business users. This application extends the Ekaya Engine with MCP tools that let teams manage query workflows, share glossary terminology, and collaborate through governed business definitions."
      />

      {/* Setup Checklist */}
      <SetupChecklist
        items={checklistItems}
        title="Setup Checklist"
        description="Complete these steps to enable AI Data Liaison"
        completeDescription="AI Data Liaison is ready for business users"
      />

      {/* Tool Configuration */}
      {mcpConfig && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Tool Configuration</CardTitle>
          </CardHeader>
          <CardContent className="space-y-6">
            {/* Developer section */}
            <div className="space-y-4">
              <div className="flex items-center justify-between gap-4">
                <div className="flex-1">
                  <span className="text-sm font-medium text-text-primary">Add Approval Tools</span>
                  <p className="text-sm text-text-secondary mt-1">
                    Include tools to review and manage query suggestions and glossary terms:
                    approve, reject, manage approved queries, and maintain shared business
                    terminology.
                  </p>
                </div>
                <Switch
                  checked={addApprovalTools}
                  onCheckedChange={handleApprovalToolsChange}
                  disabled={updatingConfig}
                />
              </div>

              <MCPEnabledTools
                tools={mcpConfig.developerTools.filter(t => t.appId === 'ai-data-liaison')}
              />
            </div>

            <div className="border-t border-border-light" />

            {/* User section */}
            <div className="space-y-4">
              <div className="flex items-center justify-between gap-4">
                <div className="flex-1">
                  <span className="text-sm font-medium text-text-primary">Add Request Tools</span>
                  <span className="ml-2 text-xs font-medium text-brand-purple">[RECOMMENDED]</span>
                  <p className="text-sm text-text-secondary mt-1">
                    Enable business users to suggest queries, request data access, and access
                    glossary terms through the MCP Client.
                  </p>
                </div>
                <Switch
                  checked={addRequestTools}
                  onCheckedChange={handleRequestToolsChange}
                  disabled={updatingConfig}
                />
              </div>

              <MCPEnabledTools
                tools={mcpConfig.userTools.filter(t => t.appId === 'ai-data-liaison')}
              />
            </div>
          </CardContent>
        </Card>
      )}

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
            Uninstalling AI Data Liaison will disable the query suggestion workflow and remove
            AI Data Liaison access to glossary functionality. Business users will no longer be
            able to suggest queries or access glossary terms through AI Data Liaison, and data
            engineers will lose access to suggestion and glossary management tools.
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
              This will disable the query suggestion workflow and remove AI Data Liaison access to
              glossary functionality. Business users will no longer be able to suggest queries or
              access glossary terms through AI Data Liaison.
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
