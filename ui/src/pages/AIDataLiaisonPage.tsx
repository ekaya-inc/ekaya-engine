import {
  ArrowLeft,
  Check,
  CheckCircle2,
  Circle,
  Copy,
  ExternalLink,
  Loader2,
  Trash2,
  XCircle,
} from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';

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
import type { DAGStatusResponse, Datasource, MCPConfigResponse } from '../types';

// Tools enabled by AI Data Liaison installation
const DATA_LIAISON_USER_TOOLS = [
  { name: 'suggest_approved_query', description: 'Propose new queries for approval' },
  { name: 'suggest_query_update', description: 'Propose updates to existing queries' },
];

const DATA_LIAISON_DEVELOPER_TOOLS = [
  { name: 'list_query_suggestions', description: 'View pending query suggestions' },
  { name: 'approve_query_suggestion', description: 'Approve a suggested query' },
  { name: 'reject_query_suggestion', description: 'Reject a suggested query with feedback' },
  { name: 'create_approved_query', description: 'Create query directly (bypass suggestion)' },
  { name: 'update_approved_query', description: 'Update an existing query' },
  { name: 'delete_approved_query', description: 'Delete a query' },
];

interface ChecklistItem {
  id: string;
  title: string;
  description: string;
  status: 'pending' | 'complete' | 'error' | 'loading';
  link?: string;
  linkText?: string;
}

const AIDataLiaisonPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();

  // Uninstall dialog state
  const [confirmText, setConfirmText] = useState('');
  const [isUninstalling, setIsUninstalling] = useState(false);
  const [showUninstallDialog, setShowUninstallDialog] = useState(false);

  // Checklist state
  const [loading, setLoading] = useState(true);
  const [datasource, setDatasource] = useState<Datasource | null>(null);
  const [dagStatus, setDagStatus] = useState<DAGStatusResponse | null>(null);
  const [mcpConfig, setMcpConfig] = useState<MCPConfigResponse | null>(null);
  const [copied, setCopied] = useState(false);

  const fetchChecklistData = useCallback(async () => {
    if (!pid) return;

    setLoading(true);
    try {
      // Fetch all data in parallel
      const [datasourcesRes, mcpConfigRes] = await Promise.all([
        engineApi.listDataSources(pid),
        engineApi.getMCPConfig(pid),
      ]);

      // Get first datasource (if any)
      const ds = datasourcesRes.data?.datasources?.[0] ?? null;
      setDatasource(ds);
      setMcpConfig(mcpConfigRes.data ?? null);

      // If datasource exists, fetch DAG status
      if (ds) {
        try {
          const dagRes = await engineApi.getOntologyDAGStatus(pid, ds.datasource_id);
          setDagStatus(dagRes.data ?? null);
        } catch {
          // DAG might not exist yet
          setDagStatus(null);
        }
      }
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

  // Build checklist items based on current state
  const getChecklistItems = (): ChecklistItem[] => {
    const items: ChecklistItem[] = [];

    // 1. Datasource configured
    items.push({
      id: 'datasource',
      title: 'Datasource configured',
      description: datasource
        ? `Connected to ${datasource.name} (${datasource.type})`
        : 'Connect a database to enable AI Data Liaison',
      status: loading ? 'loading' : datasource ? 'complete' : 'pending',
      link: `/projects/${pid}/datasources`,
      linkText: datasource ? 'Manage' : 'Configure',
    });

    // 2. Ontology extracted
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
      linkText: ontologyComplete ? 'View' : ontologyFailed ? 'Retry' : 'Extract',
    };
    if (datasource) {
      ontologyItem.link = `/projects/${pid}/ontology`;
    }
    items.push(ontologyItem);

    // 3. MCP Server URL available
    items.push({
      id: 'mcp-url',
      title: 'MCP Server URL ready',
      description: mcpConfig?.serverUrl
        ? 'Share this URL with business users'
        : 'MCP Server URL will be available after setup',
      status: loading ? 'loading' : mcpConfig?.serverUrl ? 'complete' : 'pending',
      link: `/projects/${pid}/mcp-server`,
      linkText: 'Configure',
    });

    // 4. AI Data Liaison installed (always complete on this page)
    items.push({
      id: 'installed',
      title: 'AI Data Liaison installed',
      description: 'Query suggestion workflow enabled',
      status: 'complete',
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
            Enable business users to query data through natural language
          </p>
        </div>
      </div>

      {/* Setup Checklist */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            {allComplete ? (
              <CheckCircle2 className="h-5 w-5 text-green-500" />
            ) : (
              <Circle className="h-5 w-5 text-text-secondary" />
            )}
            Setup Checklist
          </CardTitle>
          <CardDescription>
            {allComplete
              ? 'AI Data Liaison is ready for business users'
              : 'Complete these steps to enable AI Data Liaison'}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {checklistItems.map((item, index) => (
              <div
                key={item.id}
                className="flex items-start gap-3 rounded-lg border border-border-light p-3"
              >
                <div className="mt-0.5">
                  {item.status === 'loading' ? (
                    <Loader2 className="h-5 w-5 animate-spin text-text-secondary" />
                  ) : item.status === 'complete' ? (
                    <CheckCircle2 className="h-5 w-5 text-green-500" />
                  ) : item.status === 'error' ? (
                    <XCircle className="h-5 w-5 text-red-500" />
                  ) : (
                    <Circle className="h-5 w-5 text-text-secondary" />
                  )}
                </div>
                <div className="flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-text-primary">
                      {index + 1}. {item.title}
                    </span>
                  </div>
                  <p className="text-sm text-text-secondary">{item.description}</p>
                </div>
                {item.link && item.status !== 'complete' && (
                  <Link to={item.link}>
                    <Button variant="outline" size="sm">
                      {item.linkText}
                    </Button>
                  </Link>
                )}
                {item.link && item.status === 'complete' && (
                  <Link to={item.link}>
                    <Button variant="ghost" size="sm" className="text-text-secondary">
                      {item.linkText}
                    </Button>
                  </Link>
                )}
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

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
              href={`https://us.ekaya.ai/mcp-setup?mcp_url=${encodeURIComponent(mcpConfig.serverUrl)}`}
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

      {/* Enabled Tools */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Enabled Tools</CardTitle>
          <CardDescription>
            AI Data Liaison adds these MCP tools to your project
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {/* User Tools */}
          <div>
            <h4 className="text-sm font-medium text-text-primary mb-2">
              For Business Users (User Tools)
            </h4>
            <div className="space-y-2">
              {DATA_LIAISON_USER_TOOLS.map((tool) => (
                <div
                  key={tool.name}
                  className="flex items-center gap-3 rounded border border-border-light bg-surface-secondary px-3 py-2"
                >
                  <code className="text-xs font-mono text-brand-purple">{tool.name}</code>
                  <span className="text-sm text-text-secondary">{tool.description}</span>
                </div>
              ))}
            </div>
          </div>

          {/* Developer Tools */}
          <div>
            <h4 className="text-sm font-medium text-text-primary mb-2">
              For Data Engineers (Developer Tools)
            </h4>
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
          </div>
        </CardContent>
      </Card>

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
