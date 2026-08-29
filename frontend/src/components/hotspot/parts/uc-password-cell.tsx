"use client";

// Mot de passe masqué + bascule de visibilité (œil) — partagé vues Utilisateurs / Vouchers.

import { Eye, EyeOff } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export function PasswordCell({
  password,
  visible,
  onToggle,
  label,
  className,
}: {
  password: string;
  visible: boolean;
  onToggle: () => void;
  label: string;
  className?: string;
}) {
  return (
    <span className={cn("inline-flex items-center gap-0.5", className)}>
      <span className={cn("font-mono text-sm", !visible && "tracking-widest text-muted-foreground")}>
        {visible ? password : "••••••"}
      </span>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-9 text-muted-foreground hover:text-foreground"
        onClick={onToggle}
        aria-label={label}
        title={label}
      >
        {visible ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
      </Button>
    </span>
  );
}
