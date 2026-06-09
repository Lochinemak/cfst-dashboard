import type { ButtonHTMLAttributes } from "react";
import { cn } from "../../lib/utils";

interface SwitchProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "onChange"> {
  checked: boolean;
  onCheckedChange?: (checked: boolean) => void;
}

export function Switch({ checked, onCheckedChange, className, disabled, ...props }: SwitchProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      className={cn("switch", checked && "switch-checked", className)}
      onClick={() => onCheckedChange?.(!checked)}
      {...props}
    >
      <span />
    </button>
  );
}
