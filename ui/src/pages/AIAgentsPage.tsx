import { Bot, Eye, KeyRound, Loader2, Pencil, Plus, RotateCw, Trash2 } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

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
import type { Agent, MCPConfigResponse, Query } from '../types';

type AgentDraft = {
  id: string;
  name: string;
  queryIds: string[];
  key: string;
  keyVisible: boolean;
};

const DELETE_CONFIRM_TEXT = 'delete agent';

const AIAgentsPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();

  const [config, setConfig] = useState<MCPConfigResponse | null>(null);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [approvedQueries, setApprovedQueries] = useState<Query[]>([]);
  const [loading, setLoading] = useState(true);

  const [ontologyForgeReady, setOntologyForgeReady] = useState(false);
  const [hasQueries, setHasQueries] = useState(false);

  const [showAddDialog, setShowAddDialog] = useState(false);
  const [newAgentName, setNewAgentName] = useState('');
  const [newAgentQueryIDs, setNewAgentQueryIDs] = useState<string[]>([]);
  const [creatingAgent, setCreatingAgent] = useState(false);

  const [editingAgent, setEditingAgent] = useState<AgentDraft | null>(null);
  const [loadingAgent, setLoadingAgent] = useState(false);
  const [savingAgent, setSavingAgent] = useState(false);
  const [rotatingAgentKey, setRotatingAgentKey] = useState(false);

  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [agentToDelete, setAgentToDelete] = useState<Agent | null>(null);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');
  const [deletingAgent, setDeletingAgent] = useState(false);

  const [revealedKeys, setRevealedKeys] = useState<Record<string, string>>({});
  const [revealingAgentID, setRevealingAgentID] = useState<string | null>(null);

  const [confirmText, setConfirmText] = useState('');
  const [isUninstalling, setIsUninstalling] = useState(false);
  const [showUninstallDialog, setShowUninstallDialog] = useState(false);

  const approvedQueryOptions = useMemo(
    () =>
      approvedQueries.map((query) => ({
        id: query.query_id,
        label: query.natural_language_prompt,
      })),
    [approvedQueries]
  );

  const fetchData = useCallback(async () => {
    if (!pid) {
      return;
    }

    setLoading(true);
    try {
      const [configRes, ontologyForgeRes, datasourcesRes, agentsRes] = await Promise.all([
        engineApi.getMCPConfig(pid),
        engineApi.getInstalledApp(pid, 'ontology-forge').catch(() => null),
        engineApi.listDataSources(pid),
        engineApi.listAgents(pid),
      ]);

      if (configRes.success && configRes.data) {
        setConfig(configRes.data);
      }
      if (agentsRes.success && agentsRes.data) {
        setAgents(agentsRes.data.agents ?? []);
      } else {
        setAgents([]);
      }

      setOntologyForgeReady(ontologyForgeRes?.data?.activated_at != null);

      const datasource = datasourcesRes.data?.datasources?.[0] ?? null;
      if (datasource) {
        const queriesRes = await engineApi.listQueries(pid, datasource.datasource_id);
        const approved = queriesRes.data?.queries?.filter((query) => query.status === 'approved') ?? [];
        setApprovedQueries(approved);
        setHasQueries(approved.length > 0);
      } else {
        setApprovedQueries([]);
        setHasQueries(false);
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
    void fetchData();
  }, [fetchData]);

  const getChecklistItems = (): ChecklistItem[] => {
    const items: ChecklistItem[] = [];

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

  const resetAddDialog = () => {
    setNewAgentName('');
    setNewAgentQueryIDs([]);
    setShowAddDialog(false);
  };

  const toggleSelection = (current: string[], value: string) =>
    current.includes(value) ? current.filter((item) => item !== value) : [...current, value];

  const handleCreateAgent = async () => {
    if (!pid || !newAgentName.trim() || newAgentQueryIDs.length === 0) {
      return;
    }

    setCreatingAgent(true);
    try {
      const response = await engineApi.createAgent(pid, newAgentName.trim(), newAgentQueryIDs);
      if (response.success && response.data) {
        const createdAgent = response.data;
        setAgents((current) => [...current, createdAgent]);
        setRevealedKeys((current) => ({ ...current, [createdAgent.id]: createdAgent.api_key }));
        toast({
          title: 'Agent created',
          description: `Created ${createdAgent.name}`,
          variant: 'success',
        });
        resetAddDialog();
      }
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to create agent',
        variant: 'destructive',
      });
    } finally {
      setCreatingAgent(false);
    }
  };

  const openEditDialog = async (agentId: string) => {
    if (!pid) {
      return;
    }

    setLoadingAgent(true);
    try {
      const response = await engineApi.getAgent(pid, agentId);
      if (response.success && response.data) {
        setEditingAgent({
          id: response.data.id,
          name: response.data.name,
          queryIds: response.data.query_ids,
          key: revealedKeys[response.data.id] ?? '****',
          keyVisible: Boolean(revealedKeys[response.data.id]),
        });
      }
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to load agent',
        variant: 'destructive',
      });
    } finally {
      setLoadingAgent(false);
    }
  };

  const handleRevealAgentKey = async (agentId: string) => {
    if (!pid) {
      return;
    }

    setRevealingAgentID(agentId);
    try {
      const response = await engineApi.getAgentKey(pid, agentId, true);
      if (response.success && response.data) {
        const key = response.data.key;
        setRevealedKeys((current) => ({ ...current, [agentId]: key }));
        setEditingAgent((current) => {
          if (current?.id !== agentId) {
            return current;
          }

          return { ...current, key, keyVisible: true };
        });
      }
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to reveal agent key',
        variant: 'destructive',
      });
    } finally {
      setRevealingAgentID(null);
    }
  };

  const handleSaveAgent = async () => {
    if (!pid || !editingAgent || editingAgent.queryIds.length === 0) {
      return;
    }

    setSavingAgent(true);
    try {
      const response = await engineApi.updateAgentQueries(pid, editingAgent.id, editingAgent.queryIds);
      if (response.success && response.data) {
        const updatedAgent = response.data;
        setAgents((current) =>
          current.map((agent) => (agent.id === updatedAgent.id ? updatedAgent : agent))
        );
        setEditingAgent((current) => (current ? { ...current, queryIds: updatedAgent.query_ids } : current));
        toast({
          title: 'Agent updated',
          description: `${updatedAgent.name} query access updated`,
          variant: 'success',
        });
      }
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to update agent',
        variant: 'destructive',
      });
    } finally {
      setSavingAgent(false);
    }
  };

  const handleRotateKey = async () => {
    if (!pid || !editingAgent) {
      return;
    }

    setRotatingAgentKey(true);
    try {
      const response = await engineApi.rotateAgentKey(pid, editingAgent.id);
      if (response.success && response.data) {
        const apiKey = response.data.api_key;
        setRevealedKeys((current) => ({ ...current, [editingAgent.id]: apiKey }));
        setEditingAgent((current) =>
          current
            ? {
                ...current,
                key: apiKey,
                keyVisible: true,
              }
            : current
        );
        toast({
          title: 'Key rotated',
          description: `${editingAgent.name} now has a new API key`,
          variant: 'success',
        });
      }
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to rotate key',
        variant: 'destructive',
      });
    } finally {
      setRotatingAgentKey(false);
    }
  };

  const handleDeleteAgent = async () => {
    if (!pid || !agentToDelete || deleteConfirmText !== DELETE_CONFIRM_TEXT) {
      return;
    }

    setDeletingAgent(true);
    try {
      await engineApi.deleteAgent(pid, agentToDelete.id);
      setAgents((current) => current.filter((agent) => agent.id !== agentToDelete.id));
      setRevealedKeys((current) => {
        const next = { ...current };
        delete next[agentToDelete.id];
        return next;
      });
      setShowDeleteDialog(false);
      setDeleteConfirmText('');
      setAgentToDelete(null);
      toast({
        title: 'Agent deleted',
        description: 'The AI agent was removed',
        variant: 'success',
      });
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to delete agent',
        variant: 'destructive',
      });
    } finally {
      setDeletingAgent(false);
    }
  };

  const handleUninstall = async () => {
    if (confirmText !== 'uninstall application' || !pid) {
      return;
    }

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
    <div className="mx-auto max-w-5xl space-y-6">
      <AppPageHeader
        title="AI Agents"
        slug="ai-agents"
        icon={<Bot className="h-8 w-8 text-orange-500" />}
        description="Create multiple named AI agents with their own API keys and pre-approved query access."
      />

      <SetupChecklist
        items={getChecklistItems()}
        title="Setup Checklist"
        description="Complete these steps to enable AI Agents"
        completeDescription="AI Agents is ready"
      />

      <Card>
        <CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle>AI Agents</CardTitle>
            <CardDescription>
              Manage named agents, their API keys, and the queries each one can execute.
              {config?.serverUrl ? ` MCP server URL: ${config.serverUrl}` : ''}
            </CardDescription>
          </div>
          <Button onClick={() => setShowAddDialog(true)}>
            <Plus className="mr-2 h-4 w-4" />
            + Add Agent
          </Button>
        </CardHeader>
        <CardContent className="space-y-4">
          {agents.length === 0 ? (
            <div className="rounded-lg border border-dashed border-border-light p-6 text-sm text-text-secondary">
              No agents yet. Click + Add Agent to get started.
            </div>
          ) : (
            <div className="space-y-3">
              {agents.map((agent) => (
                <div
                  key={agent.id}
                  className="grid gap-3 rounded-lg border border-border-light p-4 md:grid-cols-[minmax(0,1fr)_160px_180px_auto]"
                >
                  <div className="min-w-0">
                    <div className="font-medium text-text-primary">{agent.name}</div>
                    <div className="text-sm text-text-secondary">
                      {agent.query_ids.length} query{agent.query_ids.length === 1 ? '' : 'ies'} assigned
                    </div>
                  </div>
                  <div className="text-sm text-text-secondary">
                    <div className="font-medium text-text-primary">Created</div>
                    <div>{new Date(agent.created_at).toLocaleDateString()}</div>
                  </div>
                  <div className="text-sm text-text-secondary">
                    <div className="font-medium text-text-primary">API Key</div>
                    <div className="flex items-center gap-2">
                      <code className="truncate rounded bg-surface-secondary px-2 py-1 text-xs">
                        {revealedKeys[agent.id] ?? '****'}
                      </code>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => void handleRevealAgentKey(agent.id)}
                        disabled={revealingAgentID === agent.id}
                      >
                        <Eye className="mr-1 h-3 w-3" />
                        Reveal
                      </Button>
                    </div>
                  </div>
                  <div className="flex flex-wrap items-start gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      aria-label={`Edit ${agent.name}`}
                      onClick={() => void openEditDialog(agent.id)}
                    >
                      <Pencil className="mr-1 h-3 w-3" />
                      Edit
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      aria-label={`Delete ${agent.name}`}
                      onClick={() => {
                        setAgentToDelete(agent);
                        setDeleteConfirmText('');
                        setShowDeleteDialog(true);
                      }}
                    >
                      <Trash2 className="mr-1 h-3 w-3" />
                      Delete
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

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
          <p className="mb-4 text-sm text-text-secondary">
            Uninstalling removes named agent access for this project. Existing agent API keys will
            stop working immediately.
          </p>
          <Button
            variant="outline"
            onClick={() => setShowUninstallDialog(true)}
            className="border-red-300 text-red-600 hover:bg-red-50 hover:text-red-700"
          >
            <Trash2 className="mr-2 h-4 w-4" />
            Uninstall Application
          </Button>
        </CardContent>
      </Card>

      <Dialog
        open={showAddDialog}
        onOpenChange={(open) => {
          if (!open) {
            resetAddDialog();
            return;
          }
          setShowAddDialog(true);
        }}
      >
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Add Agent</DialogTitle>
            <DialogDescription>
              Create a named agent and choose which pre-approved queries it can access.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <label htmlFor="new-agent-name" className="text-sm font-medium text-text-primary">
                Name
              </label>
              <Input
                id="new-agent-name"
                value={newAgentName}
                onChange={(event) => setNewAgentName(event.target.value)}
                placeholder="sales-bot"
              />
            </div>
            <QuerySelectionList
              selectedQueryIDs={newAgentQueryIDs}
              queryOptions={approvedQueryOptions}
              onToggle={(queryID) => setNewAgentQueryIDs((current) => toggleSelection(current, queryID))}
              onSelectAll={() => setNewAgentQueryIDs(approvedQueryOptions.map((query) => query.id))}
              onClearAll={() => setNewAgentQueryIDs([])}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={resetAddDialog} disabled={creatingAgent}>
              Cancel
            </Button>
            <Button
              onClick={() => void handleCreateAgent()}
              disabled={creatingAgent || !newAgentName.trim() || newAgentQueryIDs.length === 0}
            >
              {creatingAgent ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                'Save'
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={editingAgent != null}
        onOpenChange={(open) => {
          if (!open) {
            setEditingAgent(null);
          }
        }}
      >
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Edit Agent</DialogTitle>
            <DialogDescription>
              Update query access and rotate the API key for this agent.
            </DialogDescription>
          </DialogHeader>
          {loadingAgent || !editingAgent ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-text-secondary" />
            </div>
          ) : (
            <>
              <div className="space-y-4 py-2">
                <div className="space-y-2">
                  <label htmlFor="edit-agent-name" className="text-sm font-medium text-text-primary">
                    Name
                  </label>
                  <Input id="edit-agent-name" value={editingAgent.name} readOnly />
                </div>
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-medium text-text-primary">API Key</span>
                    <div className="flex gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => void handleRevealAgentKey(editingAgent.id)}
                        disabled={revealingAgentID === editingAgent.id}
                      >
                        <KeyRound className="mr-1 h-3 w-3" />
                        Reveal Key
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => void handleRotateKey()}
                        disabled={rotatingAgentKey}
                      >
                        <RotateCw className={`mr-1 h-3 w-3 ${rotatingAgentKey ? 'animate-spin' : ''}`} />
                        Rotate Key
                      </Button>
                    </div>
                  </div>
                  <Input value={editingAgent.keyVisible ? editingAgent.key : '****'} readOnly />
                </div>
                <QuerySelectionList
                  selectedQueryIDs={editingAgent.queryIds}
                  queryOptions={approvedQueryOptions}
                  onToggle={(queryID) =>
                    setEditingAgent((current) =>
                      current
                        ? { ...current, queryIds: toggleSelection(current.queryIds, queryID) }
                        : current
                    )
                  }
                  onSelectAll={() =>
                    setEditingAgent((current) =>
                      current
                        ? { ...current, queryIds: approvedQueryOptions.map((query) => query.id) }
                        : current
                    )
                  }
                  onClearAll={() =>
                    setEditingAgent((current) => (current ? { ...current, queryIds: [] } : current))
                  }
                />
              </div>
              <DialogFooter>
                <Button
                  variant="outline"
                  aria-label={`Delete ${editingAgent.name}`}
                  onClick={() => {
                    setAgentToDelete({
                      id: editingAgent.id,
                      name: editingAgent.name,
                      query_ids: editingAgent.queryIds,
                      created_at: '',
                    });
                    setDeleteConfirmText('');
                    setShowDeleteDialog(true);
                  }}
                  disabled={savingAgent}
                  className="mr-auto text-red-600 hover:text-red-700"
                >
                  <Trash2 className="mr-2 h-4 w-4" />
                  Delete
                </Button>
                <Button variant="outline" onClick={() => setEditingAgent(null)} disabled={savingAgent}>
                  Cancel
                </Button>
                <Button
                  onClick={() => void handleSaveAgent()}
                  disabled={savingAgent || editingAgent.queryIds.length === 0}
                >
                  {savingAgent ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Saving...
                    </>
                  ) : (
                    'Save Changes'
                  )}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      <Dialog
        open={showDeleteDialog}
        onOpenChange={(open) => {
          setShowDeleteDialog(open);
          if (!open) {
            setDeleteConfirmText('');
            setAgentToDelete(null);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Agent</DialogTitle>
            <DialogDescription>
              This permanently removes the agent and revokes its API key.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2 py-2">
            <label htmlFor="delete-agent-confirm" className="text-sm font-medium text-text-primary">
              Type delete agent to confirm
            </label>
            <Input
              id="delete-agent-confirm"
              value={deleteConfirmText}
              onChange={(event) => setDeleteConfirmText(event.target.value)}
              placeholder={DELETE_CONFIRM_TEXT}
              disabled={deletingAgent}
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setShowDeleteDialog(false);
                setDeleteConfirmText('');
                setAgentToDelete(null);
              }}
              disabled={deletingAgent}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => void handleDeleteAgent()}
              disabled={deleteConfirmText !== DELETE_CONFIRM_TEXT || deletingAgent}
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

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
              This will disable all named AI agents and revoke their access.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <label className="text-sm font-medium text-text-primary">
              Type{' '}
              <span className="rounded bg-gray-100 px-1 font-mono dark:bg-gray-800">
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

type QuerySelectionListProps = {
  selectedQueryIDs: string[];
  queryOptions: Array<{ id: string; label: string }>;
  onToggle: (queryID: string) => void;
  onSelectAll: () => void;
  onClearAll: () => void;
};

function QuerySelectionList({
  selectedQueryIDs,
  queryOptions,
  onToggle,
  onSelectAll,
  onClearAll,
}: QuerySelectionListProps) {
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div>
          <div className="text-sm font-medium text-text-primary">Pre-Approved Queries</div>
          <div className="text-xs text-text-secondary">
            Select at least one query for this agent.
          </div>
        </div>
        <div className="flex gap-2">
          <Button type="button" variant="outline" size="sm" onClick={onSelectAll}>
            Select All
          </Button>
          <Button type="button" variant="outline" size="sm" onClick={onClearAll}>
            Clear
          </Button>
        </div>
      </div>
      <div className="space-y-2 rounded-lg border border-border-light p-3">
        {queryOptions.length === 0 ? (
          <div className="text-sm text-text-secondary">No approved queries available yet.</div>
        ) : (
          queryOptions.map((query) => (
            <label key={query.id} className="flex items-center gap-3 text-sm text-text-primary">
              <input
                type="checkbox"
                checked={selectedQueryIDs.includes(query.id)}
                onChange={() => onToggle(query.id)}
              />
              <span>{query.label}</span>
            </label>
          ))
        )}
      </div>
    </div>
  );
}

export default AIAgentsPage;
