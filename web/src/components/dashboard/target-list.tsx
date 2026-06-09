import { ChangeEvent, FormEvent, useRef, useState } from "react";
import { Download, FileUp, LayoutGrid, LayoutList, Plus } from "lucide-react";
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
  onCreate: (url: string, interval: number) => Promise<void>;
  onUpdate: (target: Target, interval: number) => Promise<void>;
  onToggle: (target: Target, disabled: boolean) => Promise<void>;
  onImport: (text: string, interval: number) => Promise<{ created: Target[]; skipped: number }>;
  onDelete: (targetID: number) => Promise<void>;
}

type TargetLayout = "list" | "grid";

export function TargetList({ targets, measurementsByTarget, measurementsCount, onCreate, onUpdate, onToggle, onImport, onDelete }: TargetListProps) {
  const [url, setURL] = useState("");
  const [interval, setInterval] = useState(300);
  const [layout, setLayout] = useState<TargetLayout>("list");
  const [importStatus, setImportStatus] = useState("");
  const fileInputRef = useRef<HTMLInputElement>(null);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!url.trim()) return;
    await onCreate(url, interval);
    setURL("");
    setInterval(300);
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
    const result = await onImport(await file.text(), interval);
    setImportStatus(`已导入 ${result.created.length}，忽略 ${result.skipped}`);
    setTimeout(() => setImportStatus(""), 2400);
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
          <input ref={fileInputRef} type="file" accept=".txt,.csv,text/plain" className="sr-only" onChange={importTargets} />
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
        <Button size="icon" title="添加域名">
          <Plus />
        </Button>
      </form>

      <div className={`target-list target-list-${layout}`}>
        {targets.map((target) => (
          <LatencyChart
            key={target.id}
            target={target}
            measurements={measurementsByTarget.get(target.id) ?? []}
            layout={layout}
            onUpdate={onUpdate}
            onToggle={onToggle}
            onDelete={onDelete}
          />
        ))}
      </div>
    </section>
  );
}
