"use client";

// Dialog Profil — informations de l'utilisateur connecté, rafraîchies via GET /api/auth/me
// à chaque ouverture (les changements de rôle/compte effectués par l'admin y apparaissent).

import { useQuery } from "@tanstack/react-query";
import { AtSign, Building2, Fingerprint, ShieldCheck } from "lucide-react";

import { api } from "@/lib/hotspot/api";
import { roleLabel, userInitials } from "@/lib/hotspot/format";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { AuthUser } from "@/lib/hotspot/types";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";

/** Réponse de GET /api/auth/me. */
interface MeResponse {
  user: AuthUser;
}

interface ProfileDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function InfoRow({
  icon: Icon,
  label,
  value,
  mono,
}: {
  icon: typeof AtSign;
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="flex items-center gap-3 rounded-md px-2 py-2">
      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <Icon className="size-4" aria-hidden />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className={`truncate text-sm font-medium ${mono ? "font-mono text-[13px]" : ""}`} title={value}>
          {value || "—"}
        </p>
      </div>
    </div>
  );
}

export function ProfileDialog({ open, onOpenChange }: ProfileDialogProps) {
  const storedUser = useHotspotStore((s) => s.user);

  const { data, isFetching } = useQuery({
    queryKey: ["/api/auth/me"],
    queryFn: () => api<MeResponse>("/api/auth/me"),
    enabled: open,
  });

  // Données fraîches si disponibles, sinon utilisateur en session (affichage immédiat).
  const user = data?.user ?? storedUser;
  const name = user?.name ?? "Utilisateur";
  const accountName = user?.accountName ?? "";
  const accountId = user?.accountId ?? "";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-sm">
        {/* En-tête : identité */}
        <div className="relative flex flex-col items-center gap-3 px-6 pb-5 pt-7">
          <div
            className="absolute inset-x-0 top-0 h-24 bg-gradient-to-b from-primary/15 to-transparent"
            aria-hidden
          />
          <Avatar className="relative size-16 border-2 border-background shadow-lg shadow-primary/20">
            <AvatarFallback className="bg-primary/15 text-lg font-semibold text-primary">
              {userInitials(name)}
            </AvatarFallback>
          </Avatar>
          <div className="relative flex flex-col items-center gap-1 text-center">
            <DialogTitle className="text-lg leading-tight">{name}</DialogTitle>
            <DialogDescription className="font-mono text-[13px]">@{user?.username ?? "—"}</DialogDescription>
            <Badge
              variant="outline"
              className="mt-0.5 border-primary/25 bg-primary/10 px-2 py-0 text-[11px] font-semibold text-primary"
            >
              {roleLabel(user?.role ?? "")}
            </Badge>
          </div>
        </div>

        <Separator />

        {/* Détails du compte */}
        <div className="space-y-1 px-3 py-4">
          <InfoRow icon={AtSign} label="Identifiant de connexion" value={user?.username ?? ""} mono />
          <InfoRow icon={ShieldCheck} label="Rôle" value={roleLabel(user?.role ?? "")} />
          <InfoRow icon={Building2} label="Compte SaaS" value={accountName} />
          <InfoRow icon={Fingerprint} label="ID du compte" value={accountId} mono />
        </div>

        <Separator />

        {/* Pied : marque + état de synchronisation */}
        <div className="flex items-center justify-between px-5 py-3">
          <p className="text-xs text-muted-foreground">MikCloud</p>
          {isFetching ? (
            <Skeleton className="h-3.5 w-28" />
          ) : (
            <p className="text-[11px] text-muted-foreground/70">Informations à jour</p>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
