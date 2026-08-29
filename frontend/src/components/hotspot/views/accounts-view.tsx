"use client";

// Vue Comptes SaaS (admin plateforme uniquement) — gestion multi-tenant :
// liste des comptes clients, statistiques d'usage, activation / désactivation.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2, Power, ShieldOff } from "lucide-react";
import { toast } from "sonner";

import { ApiError, fetchAccounts, setAccountStatus } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
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
  const { t, tf, lang } = useI18n();
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
          ? tf("accounts.disabledToast", { name: payload.account.name })
          : tf("accounts.enabledToast", { name: payload.account.name }),
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
      <PageHeader title={t("accounts.title")} description={t("accounts.description")} />

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
            title={t("accounts.forbiddenTitle")}
            description={t("accounts.forbiddenDesc")}
          />
        </Card>
      ) : error ? (
        <Card className="gap-0 py-0">
          <EmptyState
            icon={Building2}
            title={t("accounts.loadError")}
            description={error.message}
          />
        </Card>
      ) : !accounts || accounts.length === 0 ? (
        <Card className="gap-0 py-0">
          <EmptyState
            icon={Building2}
            title={t("accounts.empty")}
            description={t("accounts.emptyDesc")}
          />
        </Card>
      ) : (
        <Card className="gap-0 py-0">
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("accounts.account")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("accounts.owner")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("common.status")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("accounts.created")}</TableHead>
                    <TableHead className="text-right text-muted-foreground">{t("accounts.users")}</TableHead>
                    <TableHead className="text-right text-muted-foreground">{t("accounts.routers")}</TableHead>
                    <TableHead className="text-right text-muted-foreground">{t("accounts.sessions")}</TableHead>
                    <TableHead className="text-right text-muted-foreground">{t("accounts.sales30")}</TableHead>
                    <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("common.actions")}</TableHead>
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
                                {t("accounts.platform")}
                              </Badge>
                            )}
                          </div>
                        </TableCell>
                        <TableCell className="text-muted-foreground">{account.owner || "—"}</TableCell>
                        <TableCell>
                          <StatusBadge status={account.status} dot />
                        </TableCell>
                        <TableCell className="text-muted-foreground">{formatDate(account.createdAt, lang)}</TableCell>
                        <TableCell className="text-right tabular-nums">{account.stats.users}</TableCell>
                        <TableCell className="text-right tabular-nums">{account.stats.routers}</TableCell>
                        <TableCell className="text-right tabular-nums">{account.stats.sessions}</TableCell>
                        <TableCell className="text-right">
                          <span className="tabular-nums">{account.stats.sales30d}</span>
                          <span className="ml-2 text-xs tabular-nums text-muted-foreground">
                            {formatCurrency(account.stats.revenue30d, currency, lang)}
                          </span>
                        </TableCell>
                        <TableCell className="pr-4 text-right sm:pr-6">
                          {isMain ? (
                            <span className="text-xs text-muted-foreground" title={t("accounts.mainAccount")}>
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
                              {disabling ? t("accounts.disable") : t("accounts.enable")}
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
                ? tf("accounts.disableTitle", { name: confirmTarget?.name ?? "" })
                : tf("accounts.enableTitle", { name: confirmTarget?.name ?? "" })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmTarget?.status === "active"
                ? t("accounts.disableDesc")
                : t("accounts.enableDesc")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
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
                ? t("accounts.applying")
                : confirmTarget?.status === "active"
                  ? t("accounts.disable")
                  : t("accounts.enable")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
