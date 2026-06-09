import { Plus, RefreshCw } from "lucide-react";
import { FormEvent, useState } from "react";
import type { Host } from "../../types";
import { relativeTime } from "../../lib/utils";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { StatusBadge } from "./status-badge";

interface SidebarProps {
  hosts: Host[];
  selectedHostID?: number;
  onSelect: (hostID: number) => void;
  onCreate: (name: string) => Promise<void>;
  onRefresh: () => Promise<void>;
}

export function Sidebar({ hosts, selectedHostID, onSelect, onCreate, onRefresh }: SidebarProps) {
  const [name, setName] = useState("");
  const [creating, setCreating] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!name.trim()) return;
    setCreating(true);
    try {
      await onCreate(name);
      setName("");
    } finally {
      setCreating(false);
    }
  }

  return (
    <aside className="sidebar">
      <div className="sidebar-head">
        <div>
          <p className="eyebrow">Fleet</p>
          <h2>主机</h2>
        </div>
        <Button variant="ghost" size="icon" onClick={onRefresh} title="刷新">
          <RefreshCw />
        </Button>
      </div>

      <form className="host-create-form" onSubmit={submit}>
        <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="新主机名称" />
        <Button size="icon" disabled={creating} title="添加主机">
          <Plus />
        </Button>
      </form>

      <div className="host-list">
        {hosts.map((host) => (
          <button
            key={host.id}
            className={`host-row ${selectedHostID === host.id ? "is-active" : ""}`}
            onClick={() => onSelect(host.id)}
          >
            <span className={`status-dot status-${host.status}`} />
            <span className="host-row-main">
              <strong>{host.name}</strong>
              <span>{relativeTime(host.last_seen_at)}</span>
            </span>
            <StatusBadge status={host.status} />
          </button>
        ))}
      </div>
    </aside>
  );
}
