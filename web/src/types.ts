export type HostStatus = "pending" | "online" | "stale" | "offline";

export interface Host {
  id: number;
  name: string;
  status: HostStatus;
  agent_version: string;
  last_seen_at?: string;
  created_at: string;
  install_command?: string;
  token?: string;
}

export interface Target {
  id: number;
  host_id: number;
  url: string;
  interval_seconds: number;
  user_agent: string;
  disabled: boolean;
  created_at: string;
}

export interface Measurement {
  id: number;
  host_id: number;
  target_id: number;
  url: string;
  checked_at: string;
  status_code: number;
  latency_ms: number;
  success: boolean;
  error?: string;
  colo?: string;
  failure_rate: number;
  top_ips?: TopIP[];
}

export interface TopIP {
  ip: string;
  latency_ms: number;
  status_code: number;
  success: boolean;
  colo?: string;
}
