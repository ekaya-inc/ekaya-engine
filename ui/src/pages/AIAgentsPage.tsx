import { Bot, Loader2, Trash2 } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import AgentToolsSection from '../components/mcp/AgentToolsSection';
import AppPageHeader from '../components/AppPageHeader';
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
import { useToast } from '../hooks/useToast';
import engineApi from '../services/engineApi';
import type { MCPConfigResponse } from '../types';

const AIAgentsPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();

  const [config, setConfig] = useState<MCPConfigResponse | null>(null);
  const [agentApiKey, setAgentApiKey] = useState<string>('');
  const [loading, setLoading] = useState(true);

  // Checklist state
  const [ontologyForgeReady, setOntologyForgeReady] = useState(false);
  const [hasQueries, setHasQueries] = useState(false);

  // Uninstall dialog state
  const [confirmText, setConfirmText] = useState('');
  const [isUninstalling, setIsUninstalling] = useState(false);
  const [showUninstallDialog, setShowUninstallDialog] = useState(false);

  const fetchData = useCallback(async () => {
    if (!pid) return;

    setLoading(true);
    try {
      const [configRes, keyRes, ontologyForgeRes, datasourcesRes] = await Promise.all([
        engineApi.getMCPConfig(pid),
        engineApi.getAgentAPIKey(pid, true),
        engineApi.getInstalledApp(pid, 'ontology-forge').catch(() => null),
        engineApi.listDataSources(pid),
      ]);

      if (configRes.success && configRes.data) {
        setConfig(configRes.data);
      }
      if (keyRes.success && keyRes.data) {
        setAgentApiKey(keyRes.data.key);
      }
      setOntologyForgeReady(ontologyForgeRes?.data?.activated_at != null);

      // Check if any pre-approved queries exist
      const ds = datasourcesRes.data?.datasources?.[0] ?? null;
      if (ds) {
        try {
          const queriesRes = await engineApi.listQueries(pid, ds.datasource_id);
          const approvedCount = queriesRes.data?.queries?.filter((q) => q.status === 'approved').length ?? 0;
          setHasQueries(approvedCount > 0);
        } catch {
          setHasQueries(false);
        }
      }
    } catch (error) {
      console.error('Failed to fetch AI Agents config:', error);
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to load configuration',
        variant: 'destructive',
      });
    } finally {
      setLoading(false);
    }
  }, [pid, toast]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const getChecklistItems = (): ChecklistItem[] => {
    const items: ChecklistItem[] = [];

    // 1. Ontology Forge set up (same pattern as AI Data Liaison)
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

    // 2. Pre-Approved Queries created
    const queriesItem: ChecklistItem = {
      id: 'queries',
      title: 'Pre-Approved Queries created',
      description: hasQueries
        ? 'Queries available for agents to execute'
        : ontologyForgeReady
          ? 'Create queries that agents can run'
          : 'Complete step 1 first',
      status: loading ? 'loading' : hasQueries ? 'complete' : 'pending',
      linkText: hasQueries ? 'Manage' : 'Configure',
      disabled: !ontologyForgeReady && !hasQueries,
    };
    if (ontologyForgeReady) {
      queriesItem.link = `/projects/${pid}/queries`;
    }
    items.push(queriesItem);

    return items;
  };

  const handleUninstall = async () => {
    if (confirmText !== 'uninstall application' || !pid) return;

    setIsUninstalling(true);
    try {
      const response = await engineApi.uninstallApp(pid, 'ai-agents');
      if (response.error) {
        toast({
          title: 'Error',
          description: response.error,
          variant: 'destructive',
        });
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
        title="AI Agents"
        slug="ai-agents"
        icon={<Bot className="h-8 w-8 text-orange-500" />}
        description="Connect autonomous AI agents and processes to your data. Agents authenticate with an API key and can only use the enabled Pre-Approved Queries and Tools, giving you full control over access."
      />

      {/* Setup Checklist */}
      <SetupChecklist
        items={getChecklistItems()}
        title="Setup Checklist"
        description="Complete these steps to enable AI Agents"
        completeDescription="AI Agents is ready"
      />

      {/* Agent Tools Section (reused component) */}
      {config && (
        <AgentToolsSection
          projectId={pid ?? ''}
          serverUrl={config.serverUrl}
          agentApiKey={agentApiKey}
          onAgentApiKeyChange={setAgentApiKey}
          enabledTools={config.agentTools}
        />
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
              <CardDescription>Remove AI Agents from this project</CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-text-secondary mb-4">
            Uninstalling will revoke the Agent API Key and disable agent access to your data.
            Existing agents using this key will no longer be able to connect.
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
            <DialogTitle>Uninstall AI Agents?</DialogTitle>
            <DialogDescription>
              This will revoke the Agent API Key and disable all agent access.
              Existing agents using this key will no longer be able to connect.
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

export default AIAgentsPage;
