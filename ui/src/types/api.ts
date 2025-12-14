/**
 * Generic API Response Types
 * These types match the response format from the Go backend
 */

export interface ApiResponse<T = unknown> {
  success: boolean;
  data?: T;
  error?: string;
  message?: string;
}

/**
 * OAuth 2.0 Authorization Server Metadata (RFC 8414)
 * Returned by /.well-known/oauth-authorization-server endpoint
 */
export interface OAuthDiscoveryMetadata {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  jwks_uri: string;
  scopes_supported: string[];
  response_types_supported: string[];
  grant_types_supported: string[];
  token_endpoint_auth_methods_supported: string[];
  code_challenge_methods_supported: string[];
}

export interface ApiError {
  error: string;
  message?: string;
  status?: number;
}

export interface HealthCheckResponse {
  status: string;
  version?: string;
  timestamp?: string;
}

export interface PingResponse {
  version: string;
  environment?: string;
}
