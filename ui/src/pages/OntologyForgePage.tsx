import {
  Anvil,
  ExternalLink,
  Lightbulb,
  Loader2,
  Trash2,
} from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';

import AppPageHeader from '../components/AppPageHeader';
import MCPEnabledTools from '../components/mcp/MCPEnabledTools';
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
import { useConfig } from '../contexts/ConfigContext';
import { useToast } from '../hooks/useToast';
import { getUserRoles } from '../lib/auth-token';
import engineApi from '../services/engineApi';
import type { MCPConfigResponse } from '../types';

const OntologyForgePage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const { config: appConfig } = useConfig();
  const { toast } = useToast();

  const [config, setConfig] = useState<MCPConfigResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [updating, setUpdating] = useState(false);

  // Uninstall state
  const [confirmText, setConfirmText] = useState('');
  const [isUninstalling, setIsUninstalling] = useState(false);
  const [showUninstallDialog, setShowUninstallDialog] = useState(false);

  // Read tool group config from backend
  const addOntologyMaintenanceTools = config?.toolGroups['tools']?.addOntologyMaintenanceTools ?? true;
  const addOntologySuggestions = config?.toolGroups['tools']?.addOntologySuggestions ?? true;

  const fetchConfig = useCallback(async () => {
    if (!pid) return;

    try {
      setLoading(true);
      const mcpRes = await engineApi.getMCPConfig(pid);

      if (mcpRes.success && mcpRes.data) {
        setConfig(mcpRes.data);
      } else {
        throw new Error(mcpRes.error ?? 'Failed to load MCP configuration');
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
          await fetchConfig();
          return;
        }
        if (callbackAction === 'uninstall') {
          navigate(`/projects/${pid}`);
        } else {
          await fetchConfig();
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
  }, [searchParams, setSearchParams, pid, navigate, toast, fetchConfig]);

  const handleUninstall = async () => {
    if (confirmText !== 'uninstall application' || !pid) return;

    setIsUninstalling(true);
    try {
      const response = await engineApi.uninstallApp(pid, 'ontology-forge');
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

  const handleOntologyMaintenanceToolsChange = async (enabled: boolean) => {
    if (!pid || !config) return;

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        addOntologyMaintenanceTools: enabled,
      });

      if (response.success && response.data) {
        setConfig(response.data);
        toast({
          title: 'Success',
          description: `Ontology Maintenance Tools ${enabled ? 'enabled' : 'disabled'}`,
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

  const handleOntologySuggestionsChange = async (enabled: boolean) => {
    if (!pid || !config) return;

    try {
      setUpdating(true);
      const response = await engineApi.updateMCPConfig(pid, {
        addOntologySuggestions: enabled,
      });

      if (response.success && response.data) {
        setConfig(response.data);
        toast({
          title: 'Success',
          description: `Ontology Suggestions ${enabled ? 'enabled' : 'disabled'}`,
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

  const roles = getUserRoles();
  const hasConfigAccess = roles.includes('admin') || roles.includes('data');

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-text-secondary" />
      </div>
    );
  }

  if (!hasConfigAccess) {
    const mcpSetupUrl = config?.serverUrl
      ? `${appConfig?.authServerUrl}/mcp-setup?mcp_url=${encodeURIComponent(config.serverUrl)}`
      : `${appConfig?.authServerUrl}/mcp-setup`;

    return (
      <div className="mx-auto max-w-4xl">
        <AppPageHeader
          title="Ontology Forge"
          slug="ontology-forge"
          icon={<Anvil className="h-8 w-8 text-brand-purple" />}
          description="Build a business semantic layer (ontology) on top of your schema with AI-powered extraction, enrichment, and developer tools"
        />

        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Connect to the MCP Server</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-text-secondary">
              Follow the setup instructions to connect your MCP client to this project&apos;s MCP Server.
            </p>
            <a
              href={mcpSetupUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 text-sm text-brand-purple hover:underline"
            >
              <ExternalLink className="h-4 w-4" />
              MCP Setup Instructions
            </a>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-4xl">
      <AppPageHeader
        title="Ontology Forge"
        slug="ontology-forge"
        icon={<Anvil className="h-8 w-8 text-brand-purple" />}
        description="Build a business semantic layer (ontology) on top of your schema with AI-powered extraction, enrichment, and developer tools"
      />

      <div className="space-y-6">
        {config && (
          <Card>
            <CardHeader>
              <CardTitle>Tool Configuration</CardTitle>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Developer section */}
              <div className="space-y-4">
                <div className="flex items-center justify-between gap-4">
                  <div className="flex-1">
                    <span className="text-sm font-medium text-text-primary">Add Ontology Maintenance Tools</span>
                    <p className="text-sm text-text-secondary mt-1">
                      Include tools to manage the ontology and approved query catalog: update
                      columns, refresh schema, review pending changes, and maintain reusable
                      approved queries.
                    </p>
                  </div>
                  <Switch
                    checked={addOntologyMaintenanceTools}
                    onCheckedChange={handleOntologyMaintenanceToolsChange}
                    disabled={updating}
                  />
                </div>

                <MCPEnabledTools
                  tools={config.developerTools.filter(t => t.appId === 'ontology-forge')}
                />
              </div>

              <div className="border-t border-border-light" />

              {/* User section */}
              <div className="space-y-4">
                <div className="flex items-center justify-between gap-4">
                  <div className="flex-1">
                    <span className="text-sm font-medium text-text-primary">Add Ontology Suggestions</span>
                    <span className="ml-2 text-xs font-medium text-brand-purple">[RECOMMENDED]</span>
                    <p className="text-sm text-text-secondary mt-1">
                      Enable ontology context, schema search, column probing, project knowledge
                      discovery, and reusable approved queries so the MCP Client can understand your
                      schema and run vetted queries without ad-hoc SQL.
                    </p>
                  </div>
                  <Switch
                    checked={addOntologySuggestions}
                    onCheckedChange={handleOntologySuggestionsChange}
                    disabled={updating}
                  />
                </div>

                <MCPEnabledTools
                  tools={config.userTools.filter(t => t.appId === 'ontology-forge')}
                />
              </div>

              {/* Pro tip */}
              <div className="flex items-start gap-2 rounded-md bg-brand-purple/10 p-3 text-sm text-brand-purple dark:bg-brand-purple/20">
                <Lightbulb className="mt-0.5 h-4 w-4 shrink-0" />
                <div>
                  <span className="font-semibold">Pro Tip:</span> Have AI answer questions about your
                  Ontology{' '}
                  <details className="inline">
                    <summary className="inline cursor-pointer underline">(more info)</summary>
                    <p className="mt-2 font-normal">
                      After you have extracted your Ontology there might be questions that Ekaya cannot
                      answer from the database schema and values alone. Connect your IDE to the MCP Server
                      so that your LLM can answer questions by reviewing your codebase or other project
                      documents saving you time.
                    </p>
                  </details>
                </div>
              </div>
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
                <CardDescription>Remove Ontology Forge from this project</CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-text-secondary mb-4">
              Uninstalling Ontology Forge will remove the business semantic layer tools including
              schema management, ontology extraction, and developer tools from this project.
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
      </div>

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
            <DialogTitle>Uninstall Ontology Forge?</DialogTitle>
            <DialogDescription>
              This will remove the business semantic layer tools including schema management,
              ontology extraction, and developer tools from this project.
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

export default OntologyForgePage;
