"use client";

// N°7 — Vue Équipe : gestion des membres du compte et de leurs rôles
// (owner uniquement — la route GET /api/team est requireRole(3) côté serveur).
// Matrice : Propriétaire (tout) > Gérant (tout sauf équipe/réglages).
// Le rôle « operator » a été retiré du produit. Chaque mutation est auditée
// avec l'acteur (visible dans le journal d'activité).
//
// N°57-d — réorganisation de la gestion des membres : statistiques réelles
// (membres / gérants / propriétaires) puis GRILLE DE CARTES membres (une
// carte = une personne : avatar, @identifiant, ancienneté, rôle, actions)
// — plus de liste à plat ; la matrice des rôles devient une carte repère
// en pied de page. Données 100 % réelles (GET /api/team).

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Pencil, ShieldCheck, Trash2, UserPlus, Users } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { StatCard } from "@/components/hotspot/stat-card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { api, ApiError } from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import { roleLabel, userInitials } from "@/lib/hotspot/format";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { TeamMember } from "@/lib/hotspot/types";

/** Badge coloré par rôle (hiérarchie N°7). */
function RoleBadge({ role }: { role: string }) {
  const { lang } = useI18n();
  const cls =
    role === "owner"
      ? "bg-primary/15 text-primary border-primary/30"
      : role === "manager"
        ? "bg-emerald-500/15 text-emerald-500 border-emerald-500/30"
        : "bg-muted text-muted-foreground border-border";
  return (
    <Badge variant="outline" className={`shrink-0 ${cls}`}>
      {roleLabel(role, lang)}
    </Badge>
  );
}

interface MemberDialogProps {
  open: boolean;
  onClose: () => void;
  member: TeamMember | null; // null = création
}

function MemberDialog({ open, onClose, member }: MemberDialogProps) {
  const { t } = useI18n();
  const qc = useQueryClient();
  const editing = member !== null;
  const [name, setName] = useState(member?.name ?? "");
  const [username, setUsername] = useState(member?.username ?? "");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<string>(member?.role === "owner" || member?.role === "manager" ? member.role : "manager");

  const reset = () => {
    setName(member?.name ?? "");
    setUsername(member?.username ?? "");
    setPassword("");
    setRole(member?.role ?? "manager");
  };

  const mutation = useMutation({
    mutationFn: async () => {
      if (editing) {
        const body: Record<string, string> = { name };
        if (password) body.password = password;
        if (member.role !== "admin" && member.role !== "platform_admin") body.role = role;
        return api<TeamMember>(`/api/team/${member.id}`, { method: "PUT", body });
      }
      return api<TeamMember>("/api/team", {
        method: "POST",
        body: { name, username, password, role },
      });
    },
    onSuccess: () => {
      toast.success(editing ? t("team.updated") : t("team.created"));
      qc.invalidateQueries({ queryKey: ["/api/team"] });
      qc.invalidateQueries({ queryKey: ["/api/activity"] });
      onClose();
    },
    onError: (e: Error) => toast.error(e instanceof ApiError ? e.message : t("team.error")),
  });

  const submit = () => {
    if (!name.trim()) return toast.error(t("team.errName"));
    if (!editing && username.trim().length < 3) return toast.error(t("team.errUsername"));
    if (!editing && password.length < 10) return toast.error(t("team.errPassword"));
    if (editing && password && password.length < 10) return toast.error(t("team.errPassword"));
    mutation.mutate();
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) {
          reset();
          onClose();
        }
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{editing ? t("team.editTitle") : t("team.addTitle")}</DialogTitle>
          <DialogDescription>{editing ? t("team.editDesc") : t("team.addDesc")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-1">
          <div className="space-y-2">
            <Label htmlFor="team-name">{t("team.fieldName")}</Label>
            <Input id="team-name" value={name} onChange={(e) => setName(e.target.value)} maxLength={80} placeholder={t("team.fieldNamePh")} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="team-username">{t("team.fieldUsername")}</Label>
            <Input
              id="team-username"
              value={username}
              onChange={(e) => setUsername(e.target.value.toLowerCase())}
              disabled={editing}
              placeholder="awa.vendeuse"
              autoComplete="off"
            />
            {editing && <p className="text-xs text-muted-foreground">{t("team.usernameLocked")}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="team-password">
              {editing ? t("team.fieldPasswordEdit") : t("team.fieldPassword")}
            </Label>
            <Input
              id="team-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              autoComplete="new-password"
            />
          </div>
          <div className="space-y-2">
            <Label>{t("team.fieldRole")}</Label>
            <Select value={role} onValueChange={setRole} disabled={editing && (member.role === "admin" || member.role === "platform_admin")}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="manager">
                  <div className="flex flex-col items-start py-0.5">
                    <span className="font-medium">{t("team.role.manager")}</span>
                    <span className="text-xs text-muted-foreground">{t("team.role.managerDesc")}</span>
                  </div>
                </SelectItem>
                <SelectItem value="owner">
                  <div className="flex flex-col items-start py-0.5">
                    <span className="font-medium">{t("team.role.owner")}</span>
                    <span className="text-xs text-muted-foreground">{t("team.role.ownerDesc")}</span>
                  </div>
                </SelectItem>
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">{t("team.roleHint")}</p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button onClick={submit} disabled={mutation.isPending}>
            {mutation.isPending && <Loader2 className="size-4 animate-spin" />}
            {editing ? t("common.save") : t("team.addBtn")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default function TeamView() {
  const { t, tf, lang } = useI18n();
  const qc = useQueryClient();
  const user = useHotspotStore((s) => s.user);
  const [dialog, setDialog] = useState<{ open: boolean; member: TeamMember | null }>({ open: false, member: null });
  const [confirmDelete, setConfirmDelete] = useState<TeamMember | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["/api/team"],
    queryFn: () => api<TeamMember[]>("/api/team"),
  });

  const remove = useMutation({
    mutationFn: (m: TeamMember) => api<{ ok: boolean }>(`/api/team/${m.id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success(t("team.deleted"));
      qc.invalidateQueries({ queryKey: ["/api/team"] });
      qc.invalidateQueries({ queryKey: ["/api/activity"] });
      setConfirmDelete(null);
    },
    onError: (e: Error) => toast.error(e instanceof ApiError ? e.message : t("team.error")),
  });

  const df = new Intl.DateTimeFormat(localeOf(lang), { day: "2-digit", month: "short", year: "numeric" });

  // Statistiques réelles — comptages directs de la liste serveur.
  const members = data ?? [];
  const owners = members.filter((m) => m.role === "owner" || m.role === "admin" || m.role === "platform_admin").length;
  const managers = members.filter((m) => m.role === "manager").length;

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("team.title")}
        description={t("team.description")}
        actions={
          <Button onClick={() => setDialog({ open: true, member: null })}>
            <UserPlus className="size-4" />
            {t("team.addBtn")}
          </Button>
        }
      />

      {/* N°57-d — statistiques de l'équipe (données réelles). */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <StatCard title={t("team.stat.members")} value={String(members.length)} icon={Users} />
        <StatCard title={t("team.stat.managers")} value={String(managers)} icon={ShieldCheck} />
        <StatCard title={t("team.stat.owners")} value={String(owners)} icon={Users} />
      </div>

      {isLoading ? (
        <Card>
          <LoadingRows rows={4} />
        </Card>
      ) : members.length === 0 ? (
        <Card>
          <EmptyState icon={Users} title={t("team.empty")} description={t("team.emptyDesc")} />
        </Card>
      ) : (
        /* N°57-d — grille de cartes membres : une carte = une personne. */
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {members.map((m) => {
            const self = m.id === user?.id;
            return (
              <Card key={m.id} className="py-0">
                <CardContent className="flex h-full flex-col gap-4 p-4 sm:p-6">
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex min-w-0 items-center gap-3">
                      <Avatar className="size-11 border">
                        <AvatarFallback className="text-xs font-semibold">
                          {userInitials(m.name)}
                        </AvatarFallback>
                      </Avatar>
                      <div className="min-w-0">
                        <p className="flex items-center gap-2 truncate font-medium">
                          <span className="truncate">{m.name}</span>
                          {self && (
                            <Badge variant="secondary" className="shrink-0 text-[10px]">
                              {t("team.you")}
                            </Badge>
                          )}
                        </p>
                        <p className="truncate text-xs text-muted-foreground">@{m.username}</p>
                      </div>
                    </div>
                    <RoleBadge role={m.role} />
                  </div>

                  <p className="text-xs text-muted-foreground">
                    {tf("team.memberSince", { date: df.format(new Date(m.createdAt)) })}
                  </p>

                  <div className="mt-auto flex items-center gap-1 border-t pt-3">
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-9"
                      onClick={() => setDialog({ open: true, member: m })}
                    >
                      <Pencil className="size-3.5" />
                      {t("common.edit")}
                    </Button>
                    {!self && (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="ml-auto h-9 text-destructive hover:text-destructive"
                        onClick={() => setConfirmDelete(m)}
                      >
                        <Trash2 className="size-3.5" />
                        {t("common.delete")}
                      </Button>
                    )}
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      {/* Matrice des rôles — carte repère en pied de page. */}
      <Card>
        <CardContent className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:p-5">
          <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <ShieldCheck className="size-5" />
          </span>
          <div className="min-w-0 text-sm">
            <p className="font-medium">{t("team.matrixTitle")}</p>
            <p className="mt-0.5 text-muted-foreground">{t("team.matrixDesc")}</p>
          </div>
        </CardContent>
      </Card>

      {dialog.open && <MemberDialog open member={dialog.member} onClose={() => setDialog({ open: false, member: null })} />}

      <AlertDialog open={confirmDelete !== null} onOpenChange={(v) => !v && setConfirmDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("team.deleteTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{tf("team.deleteDesc", { name: confirmDelete?.name ?? "" })}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={(e) => {
                e.preventDefault();
                if (confirmDelete) remove.mutate(confirmDelete);
              }}
            >
              {remove.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
