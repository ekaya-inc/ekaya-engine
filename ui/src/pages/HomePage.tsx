import { useState, useEffect } from 'react';

import { useConfig } from '../contexts/ConfigContext';

interface ServiceInfo {
  version: string;
  status: string;
  service: string;
}

/**
 * HomePage - Landing page for ekaya-region service
 *
 * Simple landing page that:
 * - Shows service information and branding
 * - Provides "Sign In" button to initiate OAuth flow
 * - No automatic redirects or JWT checking (those cause infinite loops with HttpOnly cookies)
 * - ProjectGuard handles all authentication for /projects/* routes
 */
export default function HomePage() {
  const [serviceInfo, setServiceInfo] = useState<ServiceInfo | null>(null);
  const { config, loading: configLoading } = useConfig();

  useEffect(() => {
    // Fetch service info from /ping endpoint
    fetch('/ping')
      .then(res => res.json())
      .then(data => setServiceInfo(data))
      .catch(err => console.error('Failed to fetch service info:', err));
  }, []);

  const handleSignIn = () => {
    if (!config) {
      console.error('Config not loaded yet');
      return;
    }

    // Simple redirect to auth server projects page
    // User will sign in (if needed) and select a project
    // Auth server will then redirect to localhost:3443/projects/{pid}
    // At that point, ekaya-region will initiate OAuth if no valid JWT exists
    window.location.href = `${config.authServerUrl}/projects`;
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100">
      <div className="w-full max-w-md rounded-lg bg-white p-8 shadow-lg">
        {/* Header */}
        <div className="mb-8 text-center">
          <h1 className="mb-2 text-3xl font-bold text-gray-900">Ekaya Region</h1>
          <p className="text-sm text-gray-500">Regional Data Access Service</p>
        </div>

        {/* Service Info */}
        {serviceInfo && (
          <div className="mb-6 rounded-md bg-gray-50 p-4">
            <div className="space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-gray-600">Status:</span>
                <span className="font-medium text-green-600">{serviceInfo.status}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-600">Version:</span>
                <span className="font-medium text-gray-900">{serviceInfo.version}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-600">Service:</span>
                <span className="font-medium text-gray-900">{serviceInfo.service}</span>
              </div>
            </div>
          </div>
        )}

        {/* Sign In Button */}
        <button
          onClick={handleSignIn}
          disabled={configLoading || !config}
          className="w-full rounded-md bg-blue-600 px-4 py-3 font-semibold text-white transition-colors hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:bg-gray-400"
        >
          Sign In
        </button>

        {/* Info Text */}
        <div className="mt-6 text-center text-xs text-gray-500">
          <p>Sign in to access your Ekaya projects</p>
          <p className="mt-1">You&apos;ll be redirected to the authentication server</p>
        </div>

        {/* Footer */}
        <div className="mt-8 border-t border-gray-200 pt-4 text-center text-xs text-gray-400">
          <p>Ekaya Regional Controller</p>
          <p className="mt-1">Secure multi-tenant data access</p>
        </div>
      </div>
    </div>
  );
}
