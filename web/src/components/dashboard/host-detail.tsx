import { Check, Copy, Pencil, Server, ShieldCheck, Trash2, X } from "lucide-react";
import { FormEvent, useMemo, useState } from "react";
import type { Host, Measurement, Target } from "../../types";
import { api } from "../../lib/api";
import { Button } from "../ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "../ui/card";
import { Input } from "../ui/input";
import { StatusBadge } from "./status-badge";
import { TargetList } from "./target-list";

interface HostDetailProps {
  host?: Host;
  targets: Target[];
  measurements: Measurement[];
  onChanged: () => Promise<void>;
  onRename: (hostID: number, name: string) => Promise<void>;
  onDelete: (hostID: number) => Promise<void>;
}

export function HostDetail({ host, targets, measurements, onChanged, onRename, onDelete }: HostDetailProps) {
  const [copied, setCopied] = useState(false);
  const [editingName, setEditingName] = useState(false);
  const [draftName, setDraftName] = useState("");
  const [savingName, setSavingName] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const grouped = useMemo(() => {
    const map = new Map<number, Measurement[]>();
    for (const measurement of measurements) {
      if (!map.has(measurement.target_id)) map.set(measurement.target_id, []);
      map.get(measurement.target_id)!.push(measurement);
    }
    return map;
  }, [measurements]);

  if (!host) {
    return (
      <Card className="empty-state">
        <Server />
        <CardTitle>选择或添加一台主机</CardTitle>
      </Card>
    );
  }

  async function copyInstall() {
    if (!host?.install_command) return;
    await navigator.clipboard.writeText(host.install_command);
    setCopied(true);
    setTimeout(() => setCopied(false), 1200);
  }

  function startRename() {
    if (!host) return;
    setDraftName(host.name);
    setEditingName(true);
  }

  async function submitRename(event: FormEvent) {
    event.preventDefault();
    if (!host || !draftName.trim()) return;
    setSavingName(true);
    try {
      await onRename(host.id, draftName);
      setEditingName(false);
    } finally {
      setSavingName(false);
    }
  }

  async function deleteHost() {
    if (!host) return;
    const confirmed = window.confirm(`删除主机「${host.name}」？该主机的测速域名、安装 token 和历史测量数据也会一并删除。`);
    if (!confirmed) return;
    setDeleting(true);
    try {
      await onDelete(host.id);
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div className="detail-stack">
      <Card className="hero-card">
        <CardHeader>
          <div className="hero-title">
            <div className="hero-icon"><Server /></div>
            <div>
              {editingName ? (
                <form className="host-rename-form" onSubmit={submitRename}>
                  <Input
                    value={draftName}
                    onChange={(event) => setDraftName(event.target.value)}
                    autoFocus
                    aria-label="主机名称"
                  />
                  <Button type="submit" size="icon" disabled={savingName || !draftName.trim()} title="保存名称">
                    <Check />
                  </Button>
                  <Button type="button" variant="outline" size="icon" onClick={() => setEditingName(false)} title="取消重命名">
                    <X />
                  </Button>
                </form>
              ) : (
                <div className="hero-name-row">
                  <CardTitle>{host.name}</CardTitle>
                  <Button type="button" variant="ghost" size="icon" onClick={startRename} title="重命名主机">
                    <Pencil />
                  </Button>
                </div>
              )}
              <div className="hero-meta">
                <StatusBadge status={host.status} />
                <span>{host.agent_version || "agent 未注册"}</span>
                <span>{host.last_seen_at ? new Date(host.last_seen_at).toLocaleString() : "无 last seen"}</span>
              </div>
            </div>
          </div>
          <div className="hero-actions">
            <Button onClick={copyInstall} disabled={!host.install_command}>
              <Copy />
              {copied ? "已复制" : "复制覆盖安装指令"}
            </Button>
            <Button type="button" variant="destructive" size="icon" onClick={deleteHost} disabled={deleting} title="删除主机">
              <Trash2 />
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <div className="command-bar">
            <ShieldCheck />
            <code>{host.install_command || "正在生成覆盖安装 token"}</code>
          </div>
        </CardContent>
      </Card>

      <TargetList
        targets={targets}
        measurementsByTarget={grouped}
        measurementsCount={measurements.length}
        onCreate={async (url, interval) => {
          await api.createTarget(host.id, url, interval);
          await onChanged();
        }}
        onUpdate={async (target, interval) => {
          await api.updateTarget(target, { interval_seconds: interval });
          await onChanged();
        }}
        onToggle={async (target, disabled) => {
          await api.updateTarget(target, { disabled });
          await onChanged();
        }}
        onImport={async (text, interval) => {
          const result = await api.importTargets(host.id, text, interval);
          await onChanged();
          return result;
        }}
        onDelete={async (targetID) => {
          await api.deleteTarget(targetID);
          await onChanged();
        }}
      />
    </div>
  );
}
