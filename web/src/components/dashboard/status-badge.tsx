import { Badge } from "../ui/badge";
import { cn, statusLabel } from "../../lib/utils";

export function StatusBadge({ status }: { status: string }) {
  return <Badge className={cn("status-badge", `status-${status}`)}>{statusLabel(status)}</Badge>;
}
