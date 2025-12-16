import { useEffect, useState } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';

/**
 * OAuth callback page - handles authorization code and completes flow
 * This page is served at /oauth/callback after redirect from auth.dev.ekaya.ai
 */
export default function OAuthCallbackPage() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<string>('Processing authentication...');

  useEffect(() => {
    async function completeOAuth() {
      try {
        // Extract authorization code and state from URL
        const code = searchParams.get('code');
        const state = searchParams.get('state');

        if (!code || !state) {
          setError('Missing authorization code or state parameter');
          return;
        }

        // Validate state parameter to prevent CSRF attacks
        const storedState = sessionStorage.getItem('oauth_state');
        if (!storedState) {
          // Clear all OAuth session data to prevent loops
          sessionStorage.removeItem('oauth_code_verifier');
          sessionStorage.removeItem('oauth_auth_server_url');
          sessionStorage.removeItem('oauth_return_url');
          setError('Missing stored state - please try logging in again');
          return;
        }
        if (state !== storedState) {
          // Clear all OAuth session data to prevent loops
          sessionStorage.removeItem('oauth_state');
          sessionStorage.removeItem('oauth_code_verifier');
          sessionStorage.removeItem('oauth_auth_server_url');
          sessionStorage.removeItem('oauth_return_url');
          setError('State parameter mismatch - potential CSRF attack detected');
          return;
        }

        setStatus('Retrieving verification data...');

        // Retrieve code_verifier from sessionStorage (stored before redirect)
        const codeVerifier = sessionStorage.getItem('oauth_code_verifier');
        if (!codeVerifier) {
          // Clear all OAuth session data to prevent loops
          sessionStorage.removeItem('oauth_state');
          sessionStorage.removeItem('oauth_auth_server_url');
          sessionStorage.removeItem('oauth_return_url');
          setError('Missing PKCE verifier - please try logging in again');
          return;
        }

        // Retrieve auth server URL from sessionStorage (stored before redirect)
        // This is required when using dynamic auth_url from query parameter
        const authServerUrl = sessionStorage.getItem('oauth_auth_server_url');
        if (!authServerUrl) {
          // Clear all OAuth session data to prevent loops
          sessionStorage.removeItem('oauth_state');
          sessionStorage.removeItem('oauth_code_verifier');
          sessionStorage.removeItem('oauth_return_url');
          setError('Missing auth server URL - please try logging in again');
          return;
        }

        setStatus('Exchanging authorization code for token...');

        // Call backend API to complete OAuth flow
        const response = await fetch('/api/auth/complete-oauth', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          credentials: 'include', // Send cookies
          body: JSON.stringify({
            code: code,
            state: state,
            code_verifier: codeVerifier,
            auth_url: authServerUrl,
          }),
        });

        if (!response.ok) {
          const errorData = await response.text();
          throw new Error(`Authentication failed: ${errorData}`);
        }

        const data = await response.json();

        // Clean up sessionStorage
        sessionStorage.removeItem('oauth_code_verifier');
        sessionStorage.removeItem('oauth_state');
        sessionStorage.removeItem('oauth_auth_server_url');
        const originalUrl = sessionStorage.getItem('oauth_return_url') ?? data.redirect_url ?? '/';
        sessionStorage.removeItem('oauth_return_url');

        setStatus('Authentication successful! Redirecting...');

        // Redirect to original page
        setTimeout(() => {
          navigate(originalUrl, { replace: true });
        }, 500);

      } catch (err) {
        console.error('OAuth callback error:', err);
        // Clear all OAuth session data to prevent loops on error
        sessionStorage.removeItem('oauth_state');
        sessionStorage.removeItem('oauth_code_verifier');
        sessionStorage.removeItem('oauth_auth_server_url');
        sessionStorage.removeItem('oauth_return_url');
        setError(err instanceof Error ? err.message : 'Unknown error occurred');
      }
    }

    completeOAuth();
  }, [searchParams, navigate]);

  if (error) {
    return (
      <div style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        minHeight: '100vh',
        padding: '20px',
        textAlign: 'center'
      }}>
        <h1 style={{ color: '#dc2626', marginBottom: '20px' }}>Authentication Error</h1>
        <p style={{ color: '#6b7280', marginBottom: '30px' }}>{error}</p>
        <button
          onClick={() => navigate('/', { replace: true })}
          style={{
            padding: '10px 20px',
            backgroundColor: '#2563eb',
            color: 'white',
            border: 'none',
            borderRadius: '5px',
            cursor: 'pointer'
          }}
        >
          Return Home
        </button>
      </div>
    );
  }

  return (
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      minHeight: '100vh'
    }}>
      <div style={{ textAlign: 'center' }}>
        <div style={{
          border: '4px solid #f3f4f6',
          borderTop: '4px solid #2563eb',
          borderRadius: '50%',
          width: '50px',
          height: '50px',
          animation: 'spin 1s linear infinite',
          margin: '0 auto 20px'
        }}></div>
        <p style={{ color: '#6b7280' }}>{status}</p>
      </div>
      <style>{`
        @keyframes spin {
          0% { transform: rotate(0deg); }
          100% { transform: rotate(360deg); }
        }
      `}</style>
    </div>
  );
}
