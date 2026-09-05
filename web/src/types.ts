export interface Profile {
  id: string;
  name: string;
  source: string;
  updated_at?: string;
  active: boolean;
}
export interface Status {
  core: {
    running: boolean;
    service_active: boolean;
    controller_healthy: boolean;
    state_query_ok?: boolean;
    service_state?: string;
    pid: number;
  };
  tun: {
    configured: boolean;
    enabled: boolean;
    runtime_enabled: boolean;
    interface_present: boolean;
    observation_ok?: boolean;
  };
  proxy_port: number;
  active_profile?: Profile;
  current_node?: string;
  current_group?: string;
  observed_at?: string;
}
export interface Node {
  name: string;
  type: string;
  delay: number;
}
export interface Group {
  name: string;
  type: string;
  now: string;
  nodes: Node[] | null;
}
export interface Rule {
  content: string;
  type: string;
  policy: string;
}
export interface Logging {
  enabled: boolean;
  total_bytes: number;
  current_file_bytes: number;
  has_error: boolean;
}
export interface Log {
  level: string;
  message: string;
  received_at: string;
}
