import { Bot, Check, Copy, Eye, EyeOff, Loader2, Pencil, Plus, RotateCw, Trash2 } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import AppPageHeader from '../components/AppPageHeader';
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
import type { Agent, Query } from '../types';

type AgentDraft = {
  id: string;
  name: string;
  queryIds: string[];
  key: string;
  keyVisible: boolean;
};

type AgentDetail = {
  id: string;
  name: string;
  queryIds: string[];
};

const DELETE_CONFIRM_TEXT = 'delete agent';

const AIAgentsPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();

  const [agents, setAgents] = useState<Agent[]>([]);
  const [approvedQueries, setApprovedQueries] = useState<Query[]>([]);
  const [loading, setLoading] = useState(true);

  const [showAddDialog, setShowAddDialog] = useState(false);
  const [newAgentName, setNewAgentName] = useState('');
  const [newAgentQueryIDs, setNewAgentQueryIDs] = useState<string[]>([]);
  const [creatingAgent, setCreatingAgent] = useState(false);

  // Detail dialog state
  const [detailAgent, setDetailAgent] = useState<AgentDetail | null>(null);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [serverUrl, setServerUrl] = useState<string>('');

  // Edit dialog state
  const [editingAgent, setEditingAgent] = useState<AgentDraft | null>(null);
  const [loadingAgent, setLoadingAgent] = useState(false);
  const [savingAgent, setSavingAgent] = useState(false);

  // Key management state
  const [revealedKeys, setRevealedKeys] = useState<Record<string, string>>({});
  const [revealingAgentID, setRevealingAgentID] = useState<string | null>(null);
  const [rotatingAgentKey, setRotatingAgentKey] = useState(false);
  const [copiedKey, setCopiedKey] = useState(false);

  // Delete dialog state
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [agentToDelete, setAgentToDelete] = useState<Agent | null>(null);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');
  const [deletingAgent, setDeletingAgent] = useState(false);

  // Uninstall state
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
      const [datasourcesRes, agentsRes, mcpConfigRes] = await Promise.all([
        engineApi.listDataSources(pid),
        engineApi.listAgents(pid),
        engineApi.getMCPConfig(pid).catch(() => null),
      ]);

      if (agentsRes.success && agentsRes.data) {
        setAgents(agentsRes.data.agents ?? []);
      } else {
        setAgents([]);
      }

      if (mcpConfigRes?.success && mcpConfigRes.data?.serverUrl) {
        setServerUrl(mcpConfigRes.data.serverUrl);
      }

      const datasource = datasourcesRes.data?.datasources?.[0] ?? null;
      if (datasource) {
        const queriesRes = await engineApi.listQueries(pid, datasource.datasource_id);
        const approved =
          queriesRes.data?.queries?.filter(
            (query) => query.status === 'approved' && query.is_enabled
          ) ?? [];
        setApprovedQueries(approved);
      } else {
        setApprovedQueries([]);
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

  const resetAddDialog = () => {
    setNewAgentName('');
    setNewAgentQueryIDs([]);
    setShowAddDialog(false);
  };

  const toggleSelection = (current: string[], value: string) =>
    current.includes(value) ? current.filter((item) => item !== value) : [...current, value];

  // --- Detail dialog ---

  const openDetailDialog = async (agentId: string) => {
    if (!pid) {
      return;
    }

    setLoadingDetail(true);
    setDetailAgent({ id: agentId, name: '', queryIds: [] });
    try {
      const response = await engineApi.getAgent(pid, agentId);
      if (response.success && response.data) {
        setDetailAgent({
          id: response.data.id,
          name: response.data.name,
          queryIds: response.data.query_ids,
        });
      }
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to load agent',
        variant: 'destructive',
      });
      setDetailAgent(null);
    } finally {
      setLoadingDetail(false);
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
        const revealedKey = response.data.key;
        setRevealedKeys((current) => ({ ...current, [agentId]: revealedKey }));
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

  const handleCopyKey = async (agentId: string) => {
    if (!pid) {
      return;
    }

    let key = revealedKeys[agentId];
    if (!key) {
      try {
        const response = await engineApi.getAgentKey(pid, agentId, true);
        if (response.success && response.data) {
          const fetchedKey = response.data.key;
          key = fetchedKey;
          setRevealedKeys((current) => ({ ...current, [agentId]: fetchedKey }));
        }
      } catch {
        toast({ title: 'Error', description: 'Failed to fetch key', variant: 'destructive' });
        return;
      }
    }

    if (!key) {
      return;
    }

    await navigator.clipboard.writeText(key);
    setCopiedKey(true);
    toast({ title: 'Copied', description: 'API key copied to clipboard', variant: 'success' });
    setTimeout(() => setCopiedKey(false), 2000);
  };

  const handleRotateKey = async (agentId: string) => {
    if (!pid) {
      return;
    }

    setRotatingAgentKey(true);
    try {
      const response = await engineApi.rotateAgentKey(pid, agentId);
      if (response.success && response.data) {
        const apiKey = response.data.api_key;
        setRevealedKeys((current) => ({ ...current, [agentId]: apiKey }));
        toast({
          title: 'Key rotated',
          description: 'The agent now has a new API key',
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

  // --- Create agent ---

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
        resetAddDialog();
        // Open detail dialog for the newly created agent
        setDetailAgent({
          id: createdAgent.id,
          name: createdAgent.name,
          queryIds: createdAgent.query_ids,
        });
        toast({
          title: 'Agent created',
          description: `Created ${createdAgent.name}`,
          variant: 'success',
        });
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

  // --- Edit dialog ---

  const openEditDialog = async (agentId: string) => {
    if (!pid) {
      return;
    }

    setDetailAgent(null);
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
        setEditingAgent(null);
        // Return to detail dialog
        setDetailAgent({
          id: updatedAgent.id,
          name: updatedAgent.name,
          queryIds: updatedAgent.query_ids,
        });
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

  // --- Delete ---

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
      setDetailAgent(null);
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

  // --- Uninstall ---

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

  // --- MCP config JSON ---

  const getMCPConfigJSON = (agentId: string) => {
    const key = revealedKeys[agentId] ?? '<your-api-key>';
    return JSON.stringify(
      {
        mcpServers: {
          ekaya: {
            type: 'http',
            url: serverUrl || '<mcp-server-url>',
            headers: {
              Authorization: `Bearer ${key}`,
            },
          },
        },
      },
      null,
      2
    );
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

      <Card>
        <CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle>AI Agent Management</CardTitle>
            <CardDescription>
              Manage named agents, their API keys, and the queries each one can execute.
            </CardDescription>
          </div>
          <Button onClick={() => setShowAddDialog(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Add Agent
          </Button>
        </CardHeader>
        <CardContent className="space-y-4">
          {agents.length === 0 ? (
            <div className="rounded-lg border border-dashed border-border-light p-6 text-sm text-text-secondary">
              No agents yet. Click + Add Agent to get started.
            </div>
          ) : (
            <div className="overflow-hidden rounded-lg border border-border-light">
              <div className="grid grid-cols-[minmax(120px,2fr)_repeat(3,1fr)_auto] items-center border-b border-border-light bg-surface-secondary/50 px-4 py-2 text-xs font-medium uppercase tracking-wider text-text-tertiary">
                <span>Agent</span>
                <span>Created</span>
                <span>Last Access</span>
                <span>Total MCP Calls</span>
                <span className="w-10" />
              </div>
              <div className="divide-y divide-border-light">
                {agents.map((agent) => (
                  <div
                    key={agent.id}
                    className="group grid cursor-pointer grid-cols-[minmax(120px,2fr)_repeat(3,1fr)_auto] items-center px-4 py-3 transition-colors hover:bg-surface-hover"
                    onClick={() => void openDetailDialog(agent.id)}
                  >
                    <div className="min-w-0">
                      <div className="truncate font-medium text-text-primary">{agent.name}</div>
                      <div className="text-xs text-text-tertiary">
                        {agent.query_ids.length} {agent.query_ids.length === 1 ? 'query' : 'queries'}
                      </div>
                    </div>
                    <div className="text-sm tabular-nums text-text-secondary">
                      {new Date(agent.created_at).toLocaleDateString()}
                    </div>
                    <div className="text-sm tabular-nums text-text-secondary">
                      {agent.last_access_at
                        ? new Date(agent.last_access_at).toLocaleString()
                        : '--'}
                    </div>
                    <div className="font-mono text-sm tabular-nums text-text-secondary">
                      {agent.mcp_call_count.toLocaleString()}
                    </div>
                    <div className="flex w-10 items-center justify-end">
                      <div className="relative z-10 flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            void openEditDialog(agent.id);
                          }}
                          className="rounded p-1.5 text-text-tertiary transition-colors hover:bg-surface-secondary hover:text-text-primary"
                          title={`Edit ${agent.name}`}
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </button>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            setAgentToDelete(agent);
                            setDeleteConfirmText('');
                            setShowDeleteDialog(true);
                          }}
                          className="rounded p-1.5 text-text-tertiary transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/30"
                          title={`Delete ${agent.name}`}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
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

      {/* Add Agent Dialog */}
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

      {/* Agent Detail Dialog */}
      <Dialog
        open={detailAgent != null}
        onOpenChange={(open) => {
          if (!open) {
            setDetailAgent(null);
          }
        }}
      >
        <DialogContent className="max-w-2xl overflow-hidden">
          <DialogHeader>
            <DialogTitle>Agent Details</DialogTitle>
            <DialogDescription>
              Connection details and API key for this agent.
            </DialogDescription>
          </DialogHeader>
          {loadingDetail || !detailAgent?.name ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-text-secondary" />
            </div>
          ) : (
            <>
              <div className="min-w-0 space-y-4 py-2">
                {/* Name */}
                <div className="space-y-2">
                  <label className="text-sm font-medium text-text-primary">Name</label>
                  <div className="flex h-10 w-full items-center rounded-md border border-border-light bg-surface-secondary/50 px-3 text-sm text-text-primary">
                    {detailAgent.name}
                  </div>
                </div>

                {/* API Key */}
                <div className="space-y-2">
                  <label className="text-sm font-medium text-text-primary">API Key</label>
                  <div className="flex min-w-0 items-center gap-2">
                    <Input
                      value={revealedKeys[detailAgent.id] ?? '••••••••••••••••••••••••••••••••'}
                      readOnly
                      className="min-w-0 shrink font-mono text-sm"
                    />
                    <Button
                      variant="outline"
                      size="sm"
                      className="shrink-0"
                      onClick={() => {
                        if (revealedKeys[detailAgent.id]) {
                          setRevealedKeys((current) => {
                            const next = { ...current };
                            delete next[detailAgent.id];
                            return next;
                          });
                        } else {
                          void handleRevealAgentKey(detailAgent.id);
                        }
                      }}
                      disabled={revealingAgentID === detailAgent.id}
                      title={revealedKeys[detailAgent.id] ? 'Hide key' : 'Reveal key'}
                    >
                      {revealedKeys[detailAgent.id] ? (
                        <EyeOff className="h-4 w-4" />
                      ) : (
                        <Eye className="h-4 w-4" />
                      )}
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      className="shrink-0"
                      onClick={() => void handleCopyKey(detailAgent.id)}
                      title="Copy key"
                    >
                      {copiedKey ? (
                        <Check className="h-4 w-4 text-green-500" />
                      ) : (
                        <Copy className="h-4 w-4" />
                      )}
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      className="shrink-0"
                      onClick={() => void handleRotateKey(detailAgent.id)}
                      disabled={rotatingAgentKey}
                      title="Rotate key"
                    >
                      <RotateCw className={`h-4 w-4 ${rotatingAgentKey ? 'animate-spin' : ''}`} />
                    </Button>
                  </div>
                  <p className="text-xs text-text-tertiary">
                    Distribute keys carefully and rotate them periodically.
                  </p>
                </div>

                {/* MCP Server Config */}
                <div className="space-y-2">
                  <label className="text-sm font-medium text-text-primary">MCP Server Configuration</label>
                  <div className="relative">
                    <pre className="overflow-x-auto rounded-lg border border-border-light bg-surface-secondary p-4 font-mono text-xs leading-relaxed text-text-primary">
                      {getMCPConfigJSON(detailAgent.id)}
                    </pre>
                    <CopyButton
                      text={getMCPConfigJSON(detailAgent.id)}
                      className="absolute right-2 top-2"
                    />
                  </div>
                  <p className="text-xs text-text-tertiary">
                    Add this to your MCP client configuration to connect this agent.
                  </p>
                </div>
              </div>

              <DialogFooter>
                <Button
                  variant="outline"
                  onClick={() => {
                    setAgentToDelete({
                      id: detailAgent.id,
                      name: detailAgent.name,
                      query_ids: detailAgent.queryIds,
                      created_at: '',
                      mcp_call_count: 0,
                    });
                    setDeleteConfirmText('');
                    setShowDeleteDialog(true);
                  }}
                  className="mr-auto text-red-600 hover:text-red-700"
                >
                  <Trash2 className="mr-2 h-4 w-4" />
                  Delete
                </Button>
                <Button
                  variant="outline"
                  onClick={() => void openEditDialog(detailAgent.id)}
                >
                  <Pencil className="mr-2 h-4 w-4" />
                  Edit Queries
                </Button>
                <Button onClick={() => setDetailAgent(null)}>
                  Close
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      {/* Edit Agent Dialog */}
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
              Update query access for this agent.
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
                  <div id="edit-agent-name" className="flex h-10 w-full items-center rounded-md border border-border-light bg-surface-secondary/50 px-3 text-sm text-text-primary">
                    {editingAgent.name}
                  </div>
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

      {/* Delete Agent Dialog */}
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

      {/* Uninstall Dialog */}
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

// --- Helper components ---

function CopyButton({ text, className }: { text: string; className?: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <button
      onClick={() => void handleCopy()}
      className={`rounded p-1.5 text-text-tertiary transition-colors hover:bg-surface-primary hover:text-text-primary ${className ?? ''}`}
      title="Copy to clipboard"
    >
      {copied ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
    </button>
  );
}

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
          <div className="text-sm text-text-secondary">
            No enabled approved queries available yet.
          </div>
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
