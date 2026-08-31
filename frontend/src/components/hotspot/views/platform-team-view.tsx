"use client";

// Console plateforme — ÉQUIPE PLATEFORME (admin plateforme uniquement).
// Redondance opérationnelle : le propriétaire du SaaS crée d'autres
// super-admins (jamais de single point of failure). Garde-fous serveur :
// pas d'auto-retrait, pas de retrait du dernier admin plateforme.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Loader2, ShieldCheck, Trash2, UserPlus, UsersRound } from "lucide-react";
import { toast } from "sonner";

import { ApiError, createPlatformAdmin, deletePlatformAdmin, fetchPlatformTeam } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { PlatformTeamMember } from "@/lib/hotspot/types";
import { EmptyState } from "@/components/hotspot/empty-state";
import { PageHeader } from "@/components/hotspot/page-header";
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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

const TEAM_KEY = ["/api/admin/team"] as const;

function randomPassword(): string {
  const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789";
  let out = "";
  const rnd = new Uint32Array(14);
  crypto.getRandomValues(rnd);
  for (const n of rnd) out += alphabet[n % alphabet.length];
  return out;
}

export default function PlatformTeamView() {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [removeTarget, setRemoveTarget] = useState<PlatformTeamMember | null>(null);

  const { data: members, isLoading, error } = useQuery({
    queryKey: TEAM_KEY,
    queryFn: fetchPlatformTeam,
    retry: (failureCount, err) => !(err instanceof ApiError && err.status === 403) && failureCount < 1,
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: TEAM_KEY });
  };

  const createMutation = useMutation({
    mutationFn: () => createPlatformAdmin({ name: name.trim(), username: username.trim().toLowerCase(), password }),
    onSuccess: (_res, _vars) => {
      toast.success(tf("platformTeam.createdToast", { name: username.trim().toLowerCase() }));
      setCreateOpen(false);
      setName("");
      setUsername("");
      setPassword("");
      invalidate();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deletePlatformAdmin(id),
    onSuccess: (_res, id) => {
      const target = members?.find((m) => m.id === id);
      toast.success(tf("platformTeam.removedToast", { name: target?.name ?? "" }));
      setRemoveTarget(null);
      invalidate();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const forbidden = error instanceof ApiError && error.status === 403;
  const canSubmit = username.trim().length >= 3 && password.length >= 8;

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("platformTeam.title")}
        description={t("platformTeam.description")}
        actions={
          <Button onClick={() => setCreateOpen(true)} className="min-h-10">
            <UserPlus className="size-4" />
            {t("platformTeam.add")}
          </Button>
        }
      />

      {isLoading ? (
        <Card className="gap-0 py-0">
          <div className="space-y-3 p-4 sm:p-6">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-14 rounded-lg" />
            ))}
          </div>
        </Card>
      ) : forbidden ? (
        <Card className="gap-0 py-0">
          <EmptyState icon={ShieldCheck} title={t("accounts.forbiddenTitle")} description={t("accounts.forbiddenDesc")} />
        </Card>
      ) : error ? (
        <Card className="gap-0 py-0">
          <EmptyState icon={UsersRound} title={t("platformTeam.loadError")} description={error.message} />
        </Card>
      ) : !members || members.length === 0 ? (
        <Card className="gap-0 py-0">
          <EmptyState icon={UsersRound} title={t("platformTeam.empty")} description={t("platformTeam.emptyDesc")} />
        </Card>
      ) : (
        <Card className="gap-0 py-0">
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("team.fieldName")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("team.fieldUsername")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("team.fieldRole")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("accounts.created")}</TableHead>
                    <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("common.actions")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {members.map((member) => (
                    <TableRow key={member.id}>
                      <TableCell className="pl-4 sm:pl-6">
                        <div className="flex items-center gap-2">
                          <span className="font-medium">{member.name}</span>
                          {member.self && (
                            <Badge variant="outline" className="border-primary/25 bg-primary/10 px-1.5 py-0 text-[10px] text-primary">
                              {t("platformTeam.you")}
                            </Badge>
                          )}
                        </div>
                      </TableCell>
                      <TableCell className="text-muted-foreground">@{member.username}</TableCell>
                      <TableCell>
                        <Badge variant="outline" className="border-border bg-muted px-1.5 py-0 text-[10px] text-muted-foreground">
                          {t("platformTeam.role")}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">{new Date(member.createdAt).toLocaleDateString(lang === "fr" ? "fr-FR" : "en-US")}</TableCell>
                      <TableCell className="pr-4 text-right sm:pr-6">
                        {member.self ? (
                          <span className="text-xs text-muted-foreground" title={t("platformTeam.selfHint")}>
                            —
                          </span>
                        ) : (
                          <Button variant="outline" size="sm" className="h-9 text-destructive hover:bg-destructive/10 hover:text-destructive" onClick={() => setRemoveTarget(member)}>
                            <Trash2 className="size-4" />
                            {t("common.delete")}
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Création d'un admin plateforme */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("platformTeam.addTitle")}</DialogTitle>
            <DialogDescription>{t("platformTeam.addDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="pt-name">{t("team.fieldName")}</Label>
              <Input id="pt-name" value={name} onChange={(e) => setName(e.target.value)} autoComplete="off" />
            </div>
            <div className="space-y-2">
              <Label htmlFor="pt-username">{t("team.fieldUsername")}</Label>
              <Input id="pt-username" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="off" placeholder="admin2" />
            </div>
            <div className="space-y-2">
              <Label htmlFor="pt-password">{t("team.fieldPassword")}</Label>
              <div className="flex gap-2">
                <Input id="pt-password" type="text" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" className="font-mono" />
                <Button type="button" variant="outline" size="icon" className="size-10 shrink-0" onClick={() => setPassword(randomPassword())} aria-label={t("platformTeam.generate")}>
                  <KeyRound className="size-4" />
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">{t("platformTeam.passwordHint")}</p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button disabled={!canSubmit || createMutation.isPending} onClick={() => createMutation.mutate()}>
              {createMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("platformTeam.add")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Retrait confirmé */}
      <AlertDialog open={!!removeTarget} onOpenChange={(open) => !open && setRemoveTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{tf("platformTeam.removeTitle", { name: removeTarget?.name ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>{t("platformTeam.removeDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={(event) => {
                event.preventDefault();
                if (removeTarget) deleteMutation.mutate(removeTarget.id);
              }}
            >
              {deleteMutation.isPending && <Loader2 className="mr-1 inline size-4 animate-spin" />}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
