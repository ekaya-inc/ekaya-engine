import {
  Globe,
  Loader2,
  RefreshCw,
  Trash2,
} from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';

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
import type {
  InstalledApp,
  TunnelConnectionStatus,
  TunnelStatusResponse,
} from '../types';

const statusPollIntervalMs = 2000;

const defaultTunnelStatus: TunnelStatusResponse = {
  tunnel_status: 'disconnected',
};

const statusLabelMap: Record<TunnelConnectionStatus, string> = {
  disconnected: 'Disconnected',
  connecting: 'Connecting',
  connected: 'Connected',
  reconnecting: 'Reconnecting',
};

const statusClassMap: Record<TunnelConnectionStatus, string> = {
  disconnected: 'bg-surface-secondary text-text-secondary',
  connecting: 'bg-amber-500/10 text-amber-700 dark:text-amber-400',
  connected: 'bg-green-500/10 text-green-700 dark:text-green-400',
  reconnecting: 'bg-amber-500/10 text-amber-700 dark:text-amber-400',
};

function getTunnelStatusDescription(status: TunnelConnectionStatus): string {
  switch (status) {
    case 'connecting':
      return 'The engine is establishing the outbound tunnel connection.';
    case 'connected':
      return 'The tunnel is connected and your public MCP URL is ready to share.';
    case 'reconnecting':
      return 'The tunnel was interrupted and is retrying the relay connection.';
    case 'disconnected':
    default:
      return 'The tunnel is not connected yet. Once ekaya-tunnel is reachable, refresh status to confirm the public URL.';
  }
}

function formatConnectedSince(connectedSince?: string): string | null {
  if (!connectedSince) {
    return null;
  }

  const parsed = new Date(connectedSince);
  if (Number.isNaN(parsed.getTime())) {
    return connectedSince;
  }

  return parsed.toLocaleString();
}

const MCPTunnelPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const { toast } = useToast();

  const [loading, setLoading] = useState(true);
  const [refreshingStatus, setRefreshingStatus] = useState(false);
  const [activating, setActivating] = useState(false);
  const [installedApp, setInstalledApp] = useState<InstalledApp | null>(null);
  const [tunnelStatus, setTunnelStatus] =
    useState<TunnelStatusResponse>(defaultTunnelStatus);

  const [confirmText, setConfirmText] = useState('');
  const [isUninstalling, setIsUninstalling] = useState(false);
  const [showUninstallDialog, setShowUninstallDialog] = useState(false);

  const fetchPageData = useCallback(async (
    showLoading = true,
    options?: { silent?: boolean }
  ) => {
    if (!pid) {
      return;
    }

    const silent = options?.silent === true;

    if (showLoading) {
      setLoading(true);
    } else if (!silent) {
      setRefreshingStatus(true);
    }

    try {
      const [installedAppRes, tunnelStatusRes] = await Promise.all([
        engineApi.getInstalledApp(pid, 'mcp-tunnel').catch(() => null),
        engineApi.getTunnelStatus(pid),
      ]);

      setInstalledApp(installedAppRes?.data ?? null);
      setTunnelStatus(tunnelStatusRes.data ?? defaultTunnelStatus);
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to load tunnel status',
        variant: 'destructive',
      });
      setTunnelStatus(defaultTunnelStatus);
    } finally {
      setLoading(false);
      if (!silent) {
        setRefreshingStatus(false);
      }
    }
  }, [pid, toast]);

  useEffect(() => {
    void fetchPageData();
  }, [fetchPageData]);

  useEffect(() => {
    const callbackAction = searchParams.get('callback_action');
    const callbackState = searchParams.get('callback_state');
    const callbackApp = searchParams.get('callback_app');
    const callbackStatus = searchParams.get('callback_status') ?? 'success';

    if (!callbackAction || !callbackState || callbackApp !== 'mcp-tunnel' || !pid) {
      return;
    }

    setSearchParams({}, { replace: true });

    if (callbackStatus === 'cancelled') {
      return;
    }

    const processCallback = async () => {
      try {
        const response = await engineApi.completeAppCallback(
          pid,
          callbackApp,
          callbackAction,
          callbackStatus,
          callbackState
        );
        if (response.error) {
          toast({
            title: 'Error',
            description: response.error,
            variant: 'destructive',
          });
          return;
        }

        if (callbackAction === 'uninstall') {
          navigate(`/projects/${pid}`);
          return;
        }

        await fetchPageData(false);
      } catch (error) {
        toast({
          title: 'Error',
          description: error instanceof Error ? error.message : 'Failed to complete action',
          variant: 'destructive',
        });
      }
    };

    void processCallback();
  }, [searchParams, setSearchParams, pid, navigate, toast, fetchPageData]);

  const handleActivate = async () => {
    if (!pid || installedApp == null) {
      return;
    }

    setActivating(true);
    try {
      const response = await engineApi.activateApp(pid, 'mcp-tunnel');
      if (response.data?.redirectUrl) {
        window.location.href = response.data.redirectUrl;
        return;
      }

      await fetchPageData(false);
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

  const handleRefreshStatus = async () => {
    await fetchPageData(false);
  };

  const handleUninstall = async () => {
    if (confirmText !== 'uninstall application' || !pid) {
      return;
    }

    setIsUninstalling(true);
    try {
      const response = await engineApi.uninstallApp(pid, 'mcp-tunnel');
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

  const appInstalled = installedApp != null;
  const activated = installedApp?.activated_at != null;
  const tunnelConnected =
    tunnelStatus.tunnel_status === 'connected' && tunnelStatus.public_url != null;
  const connectedSince = formatConnectedSince(tunnelStatus.connected_since);

  useEffect(() => {
    if (
      !activated ||
      (tunnelStatus.tunnel_status !== 'connecting' &&
        tunnelStatus.tunnel_status !== 'reconnecting')
    ) {
      return;
    }

    const intervalId = window.setInterval(() => {
      void fetchPageData(false, { silent: true });
    }, statusPollIntervalMs);

    return () => {
      window.clearInterval(intervalId);
    };
  }, [activated, tunnelStatus.tunnel_status, fetchPageData]);

  const checklistItems: ChecklistItem[] = [
    {
      id: 'activate',
      title: 'Activate MCP Tunnel',
      description: !appInstalled
        ? 'Install MCP Tunnel from the Applications page before activating it.'
        : activated
          ? 'MCP Tunnel activated'
          : 'Activate the application so the engine starts the outbound tunnel client.',
      status: loading ? 'loading' : activated ? 'complete' : 'pending',
      disabled: !appInstalled,
      ...(activated || !appInstalled
        ? {}
        : {
            onAction: handleActivate,
            actionText: 'Activate',
            actionDisabled: activating,
          }),
    },
    {
      id: 'connection',
      title: 'Confirm tunnel connection',
      description: !activated
        ? 'Complete step 1 before checking the tunnel connection.'
        : tunnelConnected
          ? 'Tunnel connected and public URL assigned'
          : getTunnelStatusDescription(tunnelStatus.tunnel_status),
      status: loading ? 'loading' : tunnelConnected ? 'complete' : 'pending',
      disabled: !activated,
      ...(!activated || tunnelConnected
        ? {}
        : {
            onAction: handleRefreshStatus,
            actionText: 'Refresh',
            actionDisabled: refreshingStatus,
          }),
    },
  ];

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
        title="MCP Tunnel"
        slug="mcp-tunnel"
        icon={<Globe className="h-8 w-8 text-green-500" />}
        description="Give your MCP Server a public URL so external MCP clients can reach it without firewall changes or inbound port exposure."
        showInfoLink={false}
      />

      <SetupChecklist
        items={checklistItems}
        title="Setup Checklist"
        description="Activate the tunnel and confirm the relay connection."
        completeDescription="MCP Tunnel is ready for external MCP clients."
      />

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between gap-4">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-green-500/10">
                <Globe className="h-5 w-5 text-green-500" />
              </div>
              <div>
                <CardTitle>Tunnel Status</CardTitle>
                <CardDescription>Current relay connection state</CardDescription>
              </div>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={handleRefreshStatus}
              disabled={refreshingStatus}
            >
              {refreshingStatus ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Refreshing...
                </>
              ) : (
                <>
                  <RefreshCw className="mr-2 h-4 w-4" />
                  Refresh
                </>
              )}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-3">
            <span
              className={`inline-flex rounded-full px-3 py-1 text-xs font-medium ${statusClassMap[tunnelStatus.tunnel_status]}`}
            >
              {statusLabelMap[tunnelStatus.tunnel_status]}
            </span>
            {connectedSince ? (
              <span className="text-sm text-text-secondary">
                Connected since {connectedSince}
              </span>
            ) : null}
          </div>

          <p className="text-sm text-text-secondary">
            {activated
              ? getTunnelStatusDescription(tunnelStatus.tunnel_status)
              : 'Activate MCP Tunnel to start the outbound connection to the relay service.'}
          </p>

          {tunnelStatus.public_url ? (
            <div className="rounded-lg border border-border-light bg-surface-secondary px-4 py-3">
              <Link
                to={`/projects/${pid}/mcp-server`}
                className="text-sm font-medium text-brand-purple hover:underline"
              >
                Find your MCP URL and setup instructions on the MCP Server page.
              </Link>
            </div>
          ) : null}
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
              <CardDescription>Remove MCP Tunnel from this project</CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <p className="mb-4 text-sm text-text-secondary">
            Uninstalling will stop the tunnel client and remove the public URL.
            External MCP clients will no longer be able to reach this project
            through the relay.
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
            <DialogTitle>Uninstall MCP Tunnel?</DialogTitle>
            <DialogDescription>
              This will disconnect the tunnel and remove the public URL.
              External MCP clients will no longer be able to reach your MCP
              Server through the relay.
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

export default MCPTunnelPage;
