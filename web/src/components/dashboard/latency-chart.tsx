import { Activity, AlertTriangle, LineChart, RadioTower, Save, Trash2, X } from "lucide-react";
import { useEffect, useState } from "react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { Measurement, Target } from "../../types";
import { formatInterval, intervalOptions } from "../../lib/utils";
import { Button } from "../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { Input } from "../ui/input";
import { Select } from "../ui/select";
import { Switch } from "../ui/switch";

interface LatencyChartProps {
  target: Target;
  measurements: Measurement[];
  layout?: "list" | "grid";
  selected?: boolean;
  onSelectedChange?: (selected: boolean) => void;
  onUpdate?: (target: Target, patch: Partial<Pick<Target, "interval_seconds" | "user_agent">>) => Promise<void>;
  onToggle?: (target: Target, disabled: boolean) => Promise<void>;
  onDelete?: (targetID: number) => Promise<void>;
}

export function LatencyChart({ target, measurements, layout = "list", selected = false, onSelectedChange, onUpdate, onToggle, onDelete }: LatencyChartProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [interval, setInterval] = useState(target.interval_seconds);
  const [userAgent, setUserAgent] = useState(target.user_agent ?? "");
  const data = measurements.map((point) => ({
    time: new Date(point.checked_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    checkedAt: point.checked_at,
    latency: point.success ? point.latency_ms : null,
    status: point.success ? `${point.latency_ms}ms` : failureReason(point),
    success: point.success,
    topIPs: point.top_ips ?? [],
  }));
  const compactData = sampleChartData(data, layout === "grid" ? 42 : 72);
  const successes = measurements.filter((point) => point.success && point.latency_ms > 0);
  const failures = measurements.filter((point) => !point.success);
  const latestFailure = failures.at(-1);
  const latest = measurements.at(-1);
  const average = successes.length
    ? Math.round(successes.reduce((sum, point) => sum + point.latency_ms, 0) / successes.length)
    : 0;
  const latestLabel = latest?.success ? `${latest.latency_ms}ms` : latest ? "失败" : "等待";
  const detailTitle = `${target.url} 详情`;

  useEffect(() => {
    setInterval(target.interval_seconds);
    setUserAgent(target.user_agent ?? "");
  }, [target.interval_seconds, target.user_agent]);

  useEffect(() => {
    if (!isOpen) return;
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === "Escape") setIsOpen(false);
    }
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [isOpen]);

  return (
    <>
      <Card className={`chart-card chart-card-compact target-card target-card-${layout}`}>
        <CardHeader className="chart-card-head">
          {onSelectedChange && (
            <input
              type="checkbox"
              className="target-select"
              checked={selected}
              onChange={(event) => onSelectedChange(event.target.checked)}
              aria-label={`选择 ${target.url}`}
            />
          )}
          <div className="target-card-copy">
            <CardTitle>{target.url}</CardTitle>
            <CardDescription>{target.disabled ? "已禁用" : formatInterval(target.interval_seconds)} · {target.user_agent ? target.user_agent : "默认 UA"} · {measurements.length} samples</CardDescription>
          </div>
          <div className="chart-card-actions">
            <div className="chart-metric">
              <Activity />
              <strong>{latestLabel}</strong>
            </div>
            {onToggle && (
              <Switch
                checked={!target.disabled}
                onCheckedChange={(checked) => onToggle(target, !checked)}
                title={target.disabled ? "恢复定时测速" : "禁用定时测速"}
                aria-label={target.disabled ? "恢复定时测速" : "禁用定时测速"}
              />
            )}
          </div>
        </CardHeader>
        <CardContent className="target-card-body">
          <div className="target-card-controls">
            <Select value={interval} onChange={(event) => setInterval(Number(event.target.value))} aria-label="测速频率">
              {intervalOptions.map((option) => (
                <option key={option.value} value={option.value}>{option.label}</option>
              ))}
            </Select>
            <Input
              className="target-user-agent-input"
              value={userAgent}
              onChange={(event) => setUserAgent(event.target.value)}
              placeholder="User-Agent"
              aria-label="User-Agent"
            />
            {onUpdate && (
              <Button variant="outline" size="icon" onClick={() => onUpdate(target, { interval_seconds: interval, user_agent: userAgent })} title="保存频率和 User-Agent">
                <Save />
              </Button>
            )}
            {onDelete && (
              <Button variant="destructive" size="icon" onClick={() => onDelete(target.id)} title="删除">
                <Trash2 />
              </Button>
            )}
            <Button variant="outline" size="icon" onClick={() => setIsOpen(true)} title="查看详情">
              <LineChart />
            </Button>
          </div>
          <div className="target-card-summary">
            <div className="target-stats">
              <span>平均 {successes.length ? `${average}ms` : "--"}</span>
              <span>成功 {successes.length}/{measurements.length}</span>
              {failures.length > 0 && <span className="target-stat-danger">失败 {failures.length}</span>}
            </div>
            {measurements.length === 0 ? (
              <div className="empty-chart empty-chart-compact">
                <RadioTower />
                <span>等待 agent 上报第一条数据</span>
              </div>
            ) : (
              <div className="chart-canvas chart-canvas-compact" aria-label={`${target.url} 延迟概览`}>
                <LatencyAreaChart data={compactData} gradientID={`latency-${target.id}-compact`} height={128} compact />
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {isOpen && (
        <div className="dialog-backdrop" role="presentation" onClick={() => setIsOpen(false)}>
          <section
            className="detail-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby={`target-detail-${target.id}`}
            onClick={(event) => event.stopPropagation()}
          >
            <div className="detail-dialog-head">
              <div>
                <p className="eyebrow">Target detail</p>
                <h3 id={`target-detail-${target.id}`}>{detailTitle}</h3>
                <span>{formatInterval(target.interval_seconds)} · {measurements.length} samples · {successes.length ? `avg ${average}ms` : "暂无成功样本"}</span>
              </div>
              <Button variant="ghost" size="icon" onClick={() => setIsOpen(false)} title="关闭详情">
                <X />
              </Button>
            </div>

            <div className="detail-dialog-body">
              <div className="detail-metric-row">
                <div>
                  <span>最新</span>
                  <strong>{latestLabel}</strong>
                </div>
                <div>
                  <span>平均</span>
                  <strong>{successes.length ? `${average}ms` : "--"}</strong>
                </div>
                <div>
                  <span>成功样本</span>
                  <strong>{successes.length}/{measurements.length}</strong>
                </div>
              </div>

              {latestFailure && (
                <div className="failure-summary" role="status">
                  <AlertTriangle />
                  <div>
                    <span>最近失败原因</span>
                    <strong>{failureReason(latestFailure)}</strong>
                  </div>
                </div>
              )}

              {measurements.length === 0 ? (
                <div className="empty-chart">
                  <RadioTower />
                  <span>等待 agent 上报第一条数据</span>
                </div>
              ) : (
                <div className="chart-canvas">
                  <LatencyAreaChart data={data} gradientID={`latency-${target.id}-detail`} height={300} />
                </div>
              )}

              {measurements.some((point) => point.top_ips?.length) && (
                <div className="top-ip-timeline" aria-label="每个时间点延迟最低的前 5 个 IP">
                  {measurements.map((point) => (
                    <div className="top-ip-sample" key={point.id || point.checked_at}>
                      <div className="top-ip-time">
                        <strong>{new Date(point.checked_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</strong>
                        <span>{point.latency_ms}ms</span>
                      </div>
                      <ol>
                        {(point.top_ips ?? []).slice(0, 5).map((item) => (
                          <li key={item.ip}>
                            <code>{item.ip}</code>
                            <span>{item.latency_ms}ms</span>
                          </li>
                        ))}
                      </ol>
                    </div>
                  ))}
                </div>
              )}

              {failures.length > 0 && (
                <div className="failure-timeline" aria-label="最近失败样本">
                  {failures.slice(-8).reverse().map((point) => (
                    <div className="failure-sample" key={point.id || point.checked_at}>
                      <div className="failure-sample-head">
                        <strong>{new Date(point.checked_at).toLocaleString([], { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" })}</strong>
                        <span>{point.status_code ? `HTTP ${point.status_code}` : "无响应"}</span>
                      </div>
                      <p>{failureReason(point)}</p>
                      <div className="failure-sample-meta">
                        <span>失败率 {Math.round(point.failure_rate * 100)}%</span>
                        {point.colo && <span>Colo {point.colo}</span>}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </section>
        </div>
      )}
    </>
  );
}

function failureReason(point: Measurement) {
  if (point.error) return point.error;
  if (point.status_code) return `unexpected status code ${point.status_code}`;
  return "request failed before receiving an HTTP response";
}

function sampleChartData(data: ChartPoint[], maxPoints: number) {
  if (data.length <= maxPoints) return data;
  const sampled: ChartPoint[] = [];
  const lastIndex = data.length - 1;
  for (let i = 0; i < maxPoints; i++) {
    sampled.push(data[Math.round((i * lastIndex) / (maxPoints - 1))]);
  }
  return sampled;
}

function LatencyAreaChart({ data, gradientID, height, compact = false }: { data: ChartPoint[]; gradientID: string; height: number; compact?: boolean }) {
  return (
    <ResponsiveContainer width="100%" height={height}>
      <AreaChart data={data} margin={{ top: 16, right: 22, left: 6, bottom: 2 }}>
        <defs>
          <linearGradient id={gradientID} x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="hsl(var(--chart-1))" stopOpacity={0.32} />
            <stop offset="95%" stopColor="hsl(var(--chart-1))" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="4 6" vertical={false} />
        <XAxis dataKey="time" tickLine={false} axisLine={false} minTickGap={28} />
        <YAxis tickLine={false} axisLine={false} tickFormatter={(value) => `${value}ms`} width={58} />
        <Tooltip content={<ChartTooltip />} />
        <Area
          type="monotone"
          dataKey="latency"
          stroke="hsl(var(--chart-1))"
          strokeWidth={3}
          fill={`url(#${gradientID})`}
          connectNulls={false}
          dot={compact ? false : { r: 4, strokeWidth: 2, fill: "hsl(var(--background))" }}
          activeDot={{ r: compact ? 5 : 6 }}
        />
      </AreaChart>
    </ResponsiveContainer>
  );
}

interface ChartPoint {
  time: string;
  latency: number | null;
  status: string;
  topIPs: Array<{ ip: string; latency_ms: number; colo?: string }>;
}

function ChartTooltip({ active, payload, label }: { active?: boolean; payload?: Array<{ payload: { status: string; topIPs: Array<{ ip: string; latency_ms: number; colo?: string }> } }>; label?: string }) {
  if (!active || !payload?.length) return null;
  const point = payload[0].payload;
  return (
    <div className="chart-tooltip">
      <span>{label}</span>
      <strong>{point.status}</strong>
      {point.topIPs.length > 0 && (
        <ol>
          {point.topIPs.slice(0, 5).map((item) => (
            <li key={item.ip}>
              <code>{item.ip}</code>
              <span>{item.latency_ms}ms{item.colo ? ` · ${item.colo}` : ""}</span>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}
