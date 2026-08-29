"use client";

// Vue Comptes SaaS (admin plateforme uniquement) — gestion multi-tenant :
// liste des comptes clients, statistiques d'usage, activation / désactivation.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2, Power, ShieldOff } from "lucide-react";
import { toast } from "sonner";

import { ApiError, fetchAccounts, setAccountStatus } from "@/lib/hotspot/api";
import type { AccountStatus, AccountSummary } from "@/lib/hotspot/types";
import { formatCurrency, formatDate } from "@/lib/hotspot/format";
import { EmptyState } from "@/components/hotspot/empty-state";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
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
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

export const ACCOUNTS_QUERY_KEY = ["/api/admin/accounts"] as const;

/** Compte principal de la plateforme — intouchable (ni désactivation, ni suppression). */
const MAIN_ACCOUNT_ID = "acc-main";

export default function AccountsView() {
  const currency = useCurrency();
  const queryClient = useQueryClient();
  const [confirmTarget, setConfirmTarget] = useState<AccountSummary | null>(null);

  const { data: accounts, isLoading, error } = useQuery({
    queryKey: ACCOUNTS_QUERY_KEY,
    queryFn: fetchAccounts,
    // Un 403 (rôle non admin) ne mérite pas de retry.
    retry: (failureCount, err) => !(err instanceof ApiError && err.status === 403) && failureCount < 1,
  });

  const statusMutation = useMutation({
    mutationFn: (payload: { account: AccountSummary; status: AccountStatus }) =>
      setAccountStatus(payload.account.id, payload.status),
    onSuccess: (_res, payload) => {
      toast.success(
        payload.status === "disabled"
          ? `Compte ${payload.account.name} désactivé`
          : `Compte ${payload.account.name} réactivé`,
      );
      setConfirmTarget(null);
      queryClient.invalidateQueries({ queryKey: ACCOUNTS_QUERY_KEY });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const forbidden = error instanceof ApiError && error.status === 403;

  const submitStatus = (account: AccountSummary) => {
    const status: AccountStatus = account.status === "active" ? "disabled" : "active";
    statusMutation.mutate({ account, status });
  };

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader title="Comptes SaaS" description="Gérez les comptes clients de la plateforme" />

      {isLoading ? (
        <Card className="gap-0 py-0">
          <div className="space-y-3 p-4 sm:p-6">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-14 rounded-lg" />
            ))}
          </div>
        </Card>
      ) : forbidden ? (
        <Card className="gap-0 py-0">
          <EmptyState
            icon={ShieldOff}
            title="Accès réservé à l'administration"
            description="Seul l'administrateur de la plateforme peut consulter et gérer les comptes SaaS."
          />
        </Card>
      ) : error ? (
        <Card className="gap-0 py-0">
          <EmptyState
            icon={Building2}
            title="Impossible de charger les comptes"
            description={error.message}
          />
        </Card>
      ) : !accounts || accounts.length === 0 ? (
        <Card className="gap-0 py-0">
          <EmptyState
            icon={Building2}
            title="Aucun compte client"
            description="Les comptes créés via l'écran d'inscription apparaîtront ici."
          />
        </Card>
      ) : (
        <Card className="gap-0 py-0">
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="pl-4 text-muted-foreground sm:pl-6">Compte</TableHead>
                    <TableHead className="text-muted-foreground">Propriétaire</TableHead>
                    <TableHead className="text-muted-foreground">Statut</TableHead>
                    <TableHead className="text-muted-foreground">Créé le</TableHead>
                    <TableHead className="text-right text-muted-foreground">Utilisateurs</TableHead>
                    <TableHead className="text-right text-muted-foreground">Routeurs</TableHead>
                    <TableHead className="text-right text-muted-foreground">Sessions</TableHead>
                    <TableHead className="text-right text-muted-foreground">Ventes 30 j</TableHead>
                    <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {accounts.map((account) => {
                    const isMain = account.id === MAIN_ACCOUNT_ID;
                    const disabling = account.status === "active";
                    return (
                      <TableRow key={account.id} className={account.status === "disabled" ? "opacity-60" : undefined}>
                        <TableCell className="pl-4 sm:pl-6">
                          <div className="flex items-center gap-2">
                            <span className="font-medium">{account.name}</span>
                            {isMain && (
                              <Badge
                                variant="outline"
                                className="border-border bg-muted px-1.5 py-0 text-[10px] font-medium text-muted-foreground"
                              >
                                Plateforme
                              </Badge>
                            )}
                          </div>
                        </TableCell>
                        <TableCell className="text-muted-foreground">{account.owner || "—"}</TableCell>
                        <TableCell>
                          <StatusBadge status={account.status} dot />
                        </TableCell>
                        <TableCell className="text-muted-foreground">{formatDate(account.createdAt)}</TableCell>
                        <TableCell className="text-right tabular-nums">{account.stats.users}</TableCell>
                        <TableCell className="text-right tabular-nums">{account.stats.routers}</TableCell>
                        <TableCell className="text-right tabular-nums">{account.stats.sessions}</TableCell>
                        <TableCell className="text-right">
                          <span className="tabular-nums">{account.stats.sales30d}</span>
                          <span className="ml-2 text-xs tabular-nums text-muted-foreground">
                            {formatCurrency(account.stats.revenue30d, currency)}
                          </span>
                        </TableCell>
                        <TableCell className="pr-4 text-right sm:pr-6">
                          {isMain ? (
                            <span className="text-xs text-muted-foreground" title="Compte principal de la plateforme">
                              —
                            </span>
                          ) : (
                            <Button
                              variant={disabling ? "outline" : "default"}
                              size="sm"
                              className="h-9"
                              onClick={() => setConfirmTarget(account)}
                              disabled={statusMutation.isPending}
                            >
                              <Power className="size-4" />
                              {disabling ? "Désactiver" : "Réactiver"}
                            </Button>
                          )}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Confirmation activation / désactivation */}
      <AlertDialog open={!!confirmTarget} onOpenChange={(open) => !open && setConfirmTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmTarget?.status === "active"
                ? `Désactiver le compte ${confirmTarget?.name} ?`
                : `Réactiver le compte ${confirmTarget?.name} ?`}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmTarget?.status === "active"
                ? "Le propriétaire et les utilisateurs de ce compte ne pourront plus se connecter à MikCloud tant qu'il restera désactivé."
                : "Le compte retrouvera immédiatement l'accès complet à MikCloud et à ses données."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Annuler</AlertDialogCancel>
            <AlertDialogAction
              className={
                confirmTarget?.status === "active" ? "bg-destructive text-white hover:bg-destructive/90" : undefined
              }
              onClick={(event) => {
                event.preventDefault();
                if (confirmTarget) submitStatus(confirmTarget);
              }}
            >
              {statusMutation.isPending
                ? "Application…"
                : confirmTarget?.status === "active"
                  ? "Désactiver"
                  : "Réactiver"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
