import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatInterval(seconds: number) {
  if (seconds < 60) return `${seconds}秒`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}分钟`;
  return `${Math.round(seconds / 3600)}小时`;
}

export function relativeTime(value?: string | null) {
  if (!value) return "等待 agent 注册";
  const diff = Math.max(0, Date.now() - new Date(value).getTime());
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return "刚刚在线";
  if (minutes < 60) return `${minutes}分钟前`;
  if (minutes < 24 * 60) return `${Math.floor(minutes / 60)}小时前`;
  return `${Math.floor(minutes / 1440)}天前`;
}

export function statusLabel(status: string) {
  return ({ online: "在线", stale: "延迟", offline: "离线", pending: "待安装" } as Record<string, string>)[status] ?? status;
}

export const intervalOptions = [
  { value: 30, label: "30秒" },
  { value: 60, label: "1分钟" },
  { value: 300, label: "5分钟" },
  { value: 900, label: "15分钟" },
  { value: 1800, label: "30分钟" },
  { value: 3600, label: "1小时" },
];
