import {
  AlertTriangle,
  ArrowLeft,
  CheckCircle2,
  Copy,
  ExternalLink,
  Loader2,
  RefreshCw,
  Server,
  Shield,
  ShieldCheck,
} from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import { Button } from '../components/ui/Button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/Card';
import { useToast } from '../hooks/useToast';
import engineApi from '../services/engineApi';
import type { ServerStatusResponse } from '../types';

const ServerSetupPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();

  const [loading, setLoading] = useState(true);
  const [serverStatus, setServerStatus] = useState<ServerStatusResponse | null>(null);
  const [syncing, setSyncing] = useState(false);
  const [copied, setCopied] = useState(false);

  const fetchStatus = useCallback(async () => {
    setLoading(true);
    try {
      const status = await engineApi.getServerStatus();
      setServerStatus(status);
    } catch (error) {
      console.error('Failed to fetch server status:', error);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchStatus();
  }, [fetchStatus]);

  const handleSync = async () => {
    if (!pid) return;
    setSyncing(true);
    try {
      await engineApi.syncServerUrl(pid);
      toast({
        title: 'Success',
        description: 'Server URL synced to the Ekaya Service',
      });
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to sync server URL',
        variant: 'destructive',
      });
    } finally {
      setSyncing(false);
    }
  };

  const handleCopy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  const isAccessible = serverStatus?.accessible_for_business_users ?? false;

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-text-secondary" />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="icon"
          aria-label="Back to AI Data Liaison"
          onClick={() => navigate(`/projects/${pid}/ai-data-liaison`)}
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-2xl font-bold">Server Setup</h1>
          <p className="text-text-secondary">
            Configure your server for business user access over HTTPS
          </p>
        </div>
      </div>

      {/* Current Status */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            {isAccessible ? (
              <ShieldCheck className="h-5 w-5 text-green-500" />
            ) : (
              <AlertTriangle className="h-5 w-5 text-amber-500" />
            )}
            Current Status
          </CardTitle>
          <CardDescription>
            {isAccessible
              ? 'Your server is properly configured for business users'
              : 'Your server needs configuration before business users can connect'}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex items-center gap-3 rounded-lg border border-border-light bg-surface-secondary px-4 py-3">
            <Server className="h-5 w-5 text-text-secondary shrink-0" />
            <div className="flex-1 min-w-0">
              <div className="text-sm font-medium text-text-primary">Base URL</div>
              <code className="text-sm text-text-secondary break-all">{serverStatus?.base_url}</code>
            </div>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => handleCopy(serverStatus?.base_url ?? '')}
              className="shrink-0"
            >
              {copied ? (
                <CheckCircle2 className="h-4 w-4 text-green-500" />
              ) : (
                <Copy className="h-4 w-4" />
              )}
            </Button>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="flex items-center gap-2 rounded-lg border border-border-light px-3 py-2">
              {serverStatus?.is_https ? (
                <CheckCircle2 className="h-4 w-4 text-green-500" />
              ) : (
                <AlertTriangle className="h-4 w-4 text-amber-500" />
              )}
              <span className="text-sm">
                {serverStatus?.is_https ? 'HTTPS enabled' : 'HTTP only (HTTPS required)'}
              </span>
            </div>
            <div className="flex items-center gap-2 rounded-lg border border-border-light px-3 py-2">
              {!serverStatus?.is_localhost ? (
                <CheckCircle2 className="h-4 w-4 text-green-500" />
              ) : (
                <AlertTriangle className="h-4 w-4 text-amber-500" />
              )}
              <span className="text-sm">
                {serverStatus?.is_localhost ? 'Localhost (not reachable externally)' : 'External domain'}
              </span>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Configuration Guide */}
      {!isAccessible && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Shield className="h-5 w-5 text-brand-purple" />
              Configuration Guide
            </CardTitle>
            <CardDescription>
              Business users need HTTPS on a reachable domain. OAuth PKCE requires a secure context.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            {/* Step 1: Choose approach */}
            <div>
              <h3 className="text-sm font-semibold text-text-primary mb-3">
                Step 1: Choose your approach
              </h3>
              <div className="space-y-3">
                <div className="rounded-lg border border-border-light p-4">
                  <div className="flex items-center gap-2 mb-1">
                    <span className="text-sm font-medium text-text-primary">
                      Option A: Real certificates
                    </span>
                    <span className="rounded-full bg-green-100 px-2 py-0.5 text-xs text-green-700 dark:bg-green-900/30 dark:text-green-400">
                      Recommended
                    </span>
                  </div>
                  <p className="text-sm text-text-secondary">
                    Use Let&apos;s Encrypt, your organization&apos;s PKI, or a cloud provider certificate.
                    Set <code className="bg-surface-secondary px-1 rounded">base_url</code>,{' '}
                    <code className="bg-surface-secondary px-1 rounded">tls_cert_path</code>, and{' '}
                    <code className="bg-surface-secondary px-1 rounded">tls_key_path</code> in{' '}
                    <code className="bg-surface-secondary px-1 rounded">config.yaml</code>.
                  </p>
                </div>

                <div className="rounded-lg border border-border-light p-4">
                  <div className="text-sm font-medium text-text-primary mb-1">
                    Option B: Self-signed certificates
                  </div>
                  <p className="text-sm text-text-secondary mb-2">
                    Generate with <code className="bg-surface-secondary px-1 rounded">openssl</code> for
                    internal or testing use. Users&apos; browsers must trust the root CA.
                  </p>
                  <div className="rounded bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 px-3 py-2">
                    <p className="text-xs text-amber-800 dark:text-amber-300">
                      MCP clients using Node.js (Claude Code, Cursor) need the{' '}
                      <code className="font-semibold">NODE_EXTRA_CA_CERTS</code> environment variable
                      set to trust your CA certificate.
                    </p>
                  </div>
                </div>

                <div className="rounded-lg border border-border-light p-4">
                  <div className="text-sm font-medium text-text-primary mb-1">
                    Option C: Reverse proxy
                  </div>
                  <p className="text-sm text-text-secondary">
                    Use Caddy, nginx, or another proxy to terminate TLS. Keep ekaya-engine on HTTP
                    internally and set{' '}
                    <code className="bg-surface-secondary px-1 rounded">base_url</code> to the proxy&apos;s
                    public HTTPS URL.
                  </p>
                </div>
              </div>
            </div>

            {/* Step 2: Update config */}
            <div>
              <h3 className="text-sm font-semibold text-text-primary mb-3">
                Step 2: Update config.yaml
              </h3>
              <div className="rounded-lg bg-surface-secondary border border-border-light p-4 font-mono text-sm">
                <div className="text-text-secondary"># Tell Ekaya Engine your public URL</div>
                <div>base_url: <span className="text-brand-purple">&quot;https://yourservice.yourdomain:yourport&quot;</span></div>
                <div className="mt-2 text-text-secondary"># Provide TLS certificates (Options A/B only)</div>
                <div>tls_cert_path: <span className="text-brand-purple">&quot;/path/to/cert.pem&quot;</span></div>
                <div>tls_key_path: <span className="text-brand-purple">&quot;/path/to/key.pem&quot;</span></div>
              </div>
              <p className="text-xs text-text-secondary mt-2">
                Environment variables{' '}
                <code className="bg-surface-secondary px-1 rounded">BASE_URL</code>,{' '}
                <code className="bg-surface-secondary px-1 rounded">TLS_CERT_PATH</code>, and{' '}
                <code className="bg-surface-secondary px-1 rounded">TLS_KEY_PATH</code>{' '}
                override config.yaml values.
              </p>
            </div>

            {/* Step 3: Restart */}
            <div>
              <h3 className="text-sm font-semibold text-text-primary mb-2">
                Step 3: Restart the server
              </h3>
              <p className="text-sm text-text-secondary">
                After updating the configuration, restart ekaya-engine. The server will start with
                HTTPS if TLS certificates are configured. Navigate to this page on the new URL to
                verify and sync.
              </p>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Verify & Sync */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <RefreshCw className="h-5 w-5 text-brand-purple" />
            {isAccessible ? 'Sync to Ekaya Service' : 'Verify & Sync'}
          </CardTitle>
          <CardDescription>
            {isAccessible
              ? 'Push this server URL to the Ekaya Service so redirect URLs and MCP setup links are correct'
              : 'After configuring HTTPS, navigate to this page on the new URL and sync'}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {isAccessible && (
            <div className="flex items-center gap-2 rounded-lg border border-green-200 dark:border-green-800 bg-green-50 dark:bg-green-900/20 px-4 py-3">
              <CheckCircle2 className="h-5 w-5 text-green-500 shrink-0" />
              <span className="text-sm text-green-800 dark:text-green-300">
                This page loaded successfully over HTTPS on an external domain. Your server is
                properly configured.
              </span>
            </div>
          )}

          <div className="flex items-center gap-3">
            <Button onClick={handleSync} disabled={syncing}>
              {syncing ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Syncing...
                </>
              ) : (
                <>
                  <RefreshCw className="mr-2 h-4 w-4" />
                  Update Ekaya Service
                </>
              )}
            </Button>
            <Button variant="outline" onClick={fetchStatus}>
              Refresh Status
            </Button>
          </div>

          <p className="text-xs text-text-secondary">
            This updates the server URL stored in the Ekaya Service from the current{' '}
            <code className="bg-surface-secondary px-1 rounded">{serverStatus?.base_url}</code>.
            The Ekaya Service uses this URL for redirect links and MCP setup instructions.
          </p>
        </CardContent>
      </Card>

      {/* Node.js hint - always visible */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ExternalLink className="h-4 w-4 text-text-secondary" />
            MCP Client Configuration
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-text-secondary mb-3">
            If using self-signed certificates, MCP clients built on Node.js (Claude Code, Cursor)
            need to trust your CA:
          </p>
          <div className="rounded-lg bg-surface-secondary border border-border-light p-3 font-mono text-sm">
            <span className="text-text-secondary"># Add to your shell profile or MCP client config</span>
            <br />
            export NODE_EXTRA_CA_CERTS=<span className="text-brand-purple">&quot;/path/to/your/ca-cert.pem&quot;</span>
          </div>
        </CardContent>
      </Card>
    </div>
  );
};

export default ServerSetupPage;
