import type { Host, Measurement, Target } from "../types";

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(options.headers ?? {}) },
    ...options,
  });
  if (!response.ok) {
    let message = response.statusText;
    try {
      const body = await response.json();
      message = body.error ?? message;
    } catch {
      // Keep the HTTP status text.
    }
    throw new Error(message);
  }
  return response.json() as Promise<T>;
}

export const api = {
  setupStatus: () => request<{ setup_required: boolean }>("/api/setup/status"),
  setup: (username: string, password: string) =>
    request<{ ok: boolean }>("/api/setup", { method: "POST", body: JSON.stringify({ username, password }) }),
  login: (username: string, password: string) =>
    request<{ ok: boolean }>("/api/login", { method: "POST", body: JSON.stringify({ username, password }) }),
  logout: () => request<{ ok: boolean }>("/api/logout", { method: "POST", body: "{}" }),
  hosts: () => request<Host[]>("/api/hosts"),
  createHost: (name: string) => request<Host>("/api/hosts", { method: "POST", body: JSON.stringify({ name }) }),
  updateHost: (hostID: number, name: string) =>
    request<Host>(`/api/hosts/${hostID}`, { method: "PATCH", body: JSON.stringify({ name }) }),
  deleteHost: (hostID: number) => request<{ ok: boolean }>(`/api/hosts/${hostID}`, { method: "DELETE" }),
  targets: (hostID: number) => request<Target[]>(`/api/hosts/${hostID}/targets`),
  createTarget: (hostID: number, url: string, intervalSeconds: number, userAgent = "") =>
    request<Target>(`/api/hosts/${hostID}/targets`, {
      method: "POST",
      body: JSON.stringify({ url, interval_seconds: intervalSeconds, user_agent: userAgent }),
    }),
  importTargets: (hostID: number, text: string, intervalSeconds: number, userAgent = "") =>
    request<{ created: Target[]; skipped: number }>(`/api/hosts/${hostID}/targets/import`, {
      method: "POST",
      body: JSON.stringify({ text, interval_seconds: intervalSeconds, user_agent: userAgent }),
    }),
  updateTarget: (target: Target, patch: Partial<Pick<Target, "interval_seconds" | "disabled" | "url" | "user_agent">>) =>
    request<Target>(`/api/targets/${target.id}`, {
      method: "PATCH",
      body: JSON.stringify({
        url: patch.url ?? target.url,
        interval_seconds: patch.interval_seconds ?? target.interval_seconds,
        disabled: patch.disabled ?? target.disabled,
        user_agent: patch.user_agent ?? target.user_agent,
      }),
    }),
  deleteTarget: (targetID: number) => request<{ ok: boolean }>(`/api/targets/${targetID}`, { method: "DELETE" }),
  measurements: (hostID: number, since: string) =>
    request<Measurement[]>(`/api/hosts/${hostID}/measurements?since=${encodeURIComponent(since)}`),
};
