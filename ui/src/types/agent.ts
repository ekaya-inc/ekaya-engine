export interface Agent {
  id: string;
  name: string;
  query_ids: string[];
  created_at: string;
  updated_at?: string;
}

export interface AgentCreateResponse extends Agent {
  api_key: string;
}

export interface AgentListResponse {
  agents: Agent[];
}

export interface AgentKeyResponse {
  key: string;
  masked: boolean;
}
