import { ChangeEvent, FormEvent, useMemo, useRef, useState } from "react";
import { CheckSquare, Download, FileUp, LayoutGrid, LayoutList, Plus, Save, Square, X } from "lucide-react";
import type { Measurement, Target } from "../../types";
import { intervalOptions } from "../../lib/utils";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Select } from "../ui/select";
import { LatencyChart } from "./latency-chart";

interface TargetListProps {
  targets: Target[];
  measurementsByTarget: Map<number, Measurement[]>;
  measurementsCount: number;
  onCreate: (url: string, interval: number, userAgent: string) => Promise<void>;
  onUpdate: (target: Target, patch: Partial<Pick<Target, "interval_seconds" | "user_agent">>) => Promise<void>;
  onToggle: (target: Target, disabled: boolean) => Promise<void>;
  onImport: (text: string, interval: number, userAgent: string) => Promise<{ created: Target[]; skipped: number }>;
  onDelete: (targetID: number) => Promise<void>;
}

type TargetLayout = "list" | "grid";

export function TargetList({ targets, measurementsByTarget, measurementsCount, onCreate, onUpdate, onToggle, onImport, onDelete }: TargetListProps) {
  const [url, setURL] = useState("");
  const [interval, setInterval] = useState(300);
  const [userAgent, setUserAgent] = useState("");
  const [layout, setLayout] = useState<TargetLayout>("list");
  const [importStatus, setImportStatus] = useState("");
  const [selectedIDs, setSelectedIDs] = useState<Set<number>>(new Set());
  const [bulkInterval, setBulkInterval] = useState(300);
  const [bulkUserAgent, setBulkUserAgent] = useState("");
  const [savingBulk, setSavingBulk] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const selectedTargets = useMemo(() => targets.filter((target) => selectedIDs.has(target.id)), [targets, selectedIDs]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!url.trim()) return;
    await onCreate(url, interval, userAgent);
    setURL("");
    setInterval(300);
    setUserAgent("");
  }

  function exportTargets() {
    const text = targets.map((target) => target.url).join("\n");
    const blob = new Blob([text, text ? "\n" : ""], { type: "text/plain;charset=utf-8" });
    const link = document.createElement("a");
    link.href = URL.createObjectURL(blob);
    link.download = `targets-${new Date().toISOString().slice(0, 10)}.txt`;
    link.click();
    URL.revokeObjectURL(link.href);
  }

  async function importTargets(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    const result = await onImport(await file.text(), interval, userAgent);
    setImportStatus(`已导入 ${result.created.length}，忽略 ${result.skipped}`);
    setTimeout(() => setImportStatus(""), 2400);
  }

  function toggleSelected(targetID: number, selected: boolean) {
    setSelectedIDs((current) => {
      const next = new Set(current);
      if (selected) next.add(targetID);
      else next.delete(targetID);
      return next;
    });
  }

  function toggleAll() {
    setSelectedIDs((current) => current.size === targets.length ? new Set() : new Set(targets.map((target) => target.id)));
  }

  async function saveBulk() {
    if (selectedTargets.length === 0) return;
    setSavingBulk(true);
    try {
      for (const target of selectedTargets) {
        await onUpdate(target, { interval_seconds: bulkInterval, user_agent: bulkUserAgent });
      }
      setSelectedIDs(new Set());
      setImportStatus(`已批量更新 ${selectedTargets.length} 个域名`);
      setTimeout(() => setImportStatus(""), 2400);
    } finally {
      setSavingBulk(false);
    }
  }

  return (
    <section className="section-stack">
      <div className="section-title">
        <div>
          <p className="eyebrow">Targets</p>
          <h2>测速域名</h2>
        </div>
        <div className="target-toolbar">
          <span className="muted">{importStatus || `${targets.length} targets · ${measurementsCount} points`}</span>
          <input ref={fileInputRef} type="file" accept=".txt,.csv,text/plain" className="hidden-file-input" onChange={importTargets} />
          <Button type="button" variant="outline" size="icon" onClick={() => fileInputRef.current?.click()} title="导入域名列表">
            <FileUp />
          </Button>
          <Button type="button" variant="outline" size="icon" onClick={exportTargets} disabled={targets.length === 0} title="导出域名列表">
            <Download />
          </Button>
          <div className="layout-toggle" role="group" aria-label="切换域名布局">
            <Button
              type="button"
              variant={layout === "list" ? "secondary" : "ghost"}
              size="icon"
              onClick={() => setLayout("list")}
              title="列表布局"
            >
              <LayoutList />
            </Button>
            <Button
              type="button"
              variant={layout === "grid" ? "secondary" : "ghost"}
              size="icon"
              onClick={() => setLayout("grid")}
              title="网格布局"
            >
              <LayoutGrid />
            </Button>
          </div>
        </div>
      </div>
      <form className="target-create-form" onSubmit={submit}>
        <Input value={url} onChange={(event) => setURL(event.target.value)} placeholder="example.com 或 https://example.com/path" />
        <Select value={interval} onChange={(event) => setInterval(Number(event.target.value))}>
          {intervalOptions.map((option) => (
            <option key={option.value} value={option.value}>{option.label}</option>
          ))}
        </Select>
        <Input value={userAgent} onChange={(event) => setUserAgent(event.target.value)} placeholder="User-Agent，可留空" />
        <Button size="icon" title="添加域名">
          <Plus />
        </Button>
      </form>

      {targets.length > 0 && (
        <div className="bulk-edit-bar">
          <Button type="button" variant="outline" size="icon" onClick={toggleAll} title={selectedIDs.size === targets.length ? "取消全选" : "全选域名"}>
            {selectedIDs.size === targets.length ? <CheckSquare /> : <Square />}
          </Button>
          <span className="muted">已选 {selectedTargets.length}</span>
          <Select value={bulkInterval} onChange={(event) => setBulkInterval(Number(event.target.value))} aria-label="批量测速频率">
            {intervalOptions.map((option) => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </Select>
          <Input value={bulkUserAgent} onChange={(event) => setBulkUserAgent(event.target.value)} placeholder="批量 User-Agent，留空表示使用默认" />
          <Button type="button" onClick={saveBulk} disabled={selectedTargets.length === 0 || savingBulk}>
            <Save />
            保存
          </Button>
          {selectedTargets.length > 0 && (
            <Button type="button" variant="ghost" size="icon" onClick={() => setSelectedIDs(new Set())} title="清空选择">
              <X />
            </Button>
          )}
        </div>
      )}

      <div className={`target-list target-list-${layout}`}>
        {targets.map((target) => (
          <LatencyChart
            key={target.id}
            target={target}
            measurements={measurementsByTarget.get(target.id) ?? []}
            layout={layout}
            selected={selectedIDs.has(target.id)}
            onSelectedChange={(selected) => toggleSelected(target.id, selected)}
            onUpdate={onUpdate}
            onToggle={onToggle}
            onDelete={onDelete}
          />
        ))}
      </div>
    </section>
  );
}
