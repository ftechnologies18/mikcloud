"use client";

// Vue Comptes SaaS (admin plateforme uniquement) — gestion multi-tenant :
// liste des comptes clients, statistiques d'usage, activation / désactivation.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Building2,
  Check,
  Copy,
  KeyRound,
  Loader2,
  LogIn,
  Power,
  ShieldOff,
  UserPlus,
} from "lucide-react";
import { toast } from "sonner";

import {
  ApiError,
  createClientAccount,
  fetchAccounts,
  impersonateAccount,
  setAccountStatus,
} from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { AccountStatus, AccountSummary } from "@/lib/hotspot/types";
import { formatCurrency, formatDate } from "@/lib/hotspot/format";
import { EmptyState } from "@/components/hotspot/empty-state";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { AccountDetailDialog } from "./account-detail-dialog";
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

export const ACCOUNTS_QUERY_KEY = ["/api/admin/accounts"] as const;

function randomPassword(): string {
  const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789";
  let out = "";
  const rnd = new Uint32Array(14);
  crypto.getRandomValues(rnd);
  for (const n of rnd) out += alphabet[n % alphabet.length];
  return out;
}

/** Badge santé d'abonnement (console plateforme). */
function SubscriptionBadge({ state }: { state?: "active" | "expired" | "beta" }) {
  const { t } = useI18n();
  if (state === "active") {
    return (
      <Badge variant="outline" className="border-emerald-500/30 bg-emerald-500/10 px-1.5 py-0 text-[10px] font-medium text-emerald-600 dark:text-emerald-400">
        {t("platform.sub.active")}
      </Badge>
    );
  }
  if (state === "expired") {
    return (
      <Badge variant="outline" className="border-destructive/30 bg-destructive/10 px-1.5 py-0 text-[10px] font-medium text-destructive">
        {t("platform.sub.expired")}
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="border-border bg-muted px-1.5 py-0 text-[10px] font-medium text-muted-foreground">
      {t("platform.sub.beta")}
    </Badge>
  );
}

export default function AccountsView() {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const queryClient = useQueryClient();
  const impersonate = useHotspotStore((s) => s.impersonate);
  const [confirmTarget, setConfirmTarget] = useState<AccountSummary | null>(null);
  const [detailTarget, setDetailTarget] = useState<AccountSummary | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [accName, setAccName] = useState("");
  const [accUsername, setAccUsername] = useState("");
  const [accPassword, setAccPassword] = useState("");
  const [createdCreds, setCreatedCreds] = useState<{ name: string; username: string; password: string } | null>(null);
  const [copied, setCopied] = useState<"username" | "password" | null>(null);

  const copyCred = async (value: string, kind: "username" | "password") => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(kind);
      setTimeout(() => setCopied(null), 1500);
    } catch {
      toast.error(t("accounts.copyError"));
    }
  };

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

  const createMutation = useMutation({
    mutationFn: () =>
      createClientAccount({
        name: accName.trim(),
        username: accUsername.trim().toLowerCase(),
        password: accPassword,
      }),
    onSuccess: (res) => {
      toast.success(tf("accounts.createdToast", { name: res.account.name }));
      setCreateOpen(false);
      setCreatedCreds({ name: res.account.name, username: res.owner.username, password: accPassword });
      setAccName("");
      setAccUsername("");
      setAccPassword("");
      void queryClient.invalidateQueries({ queryKey: ACCOUNTS_QUERY_KEY });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const canCreate = accName.trim().length > 0 && accUsername.trim().length >= 3 && accPassword.length >= 8;

  const submitStatus = (account: AccountSummary) => {
    const status: AccountStatus = account.status === "active" ? "disabled" : "active";
    statusMutation.mutate({ account, status });
  };

  const openConsole = (account: AccountSummary) => {
    impersonateAccount(account.id)
      .then((res) => {
        impersonate(res.token, res.user);
        queryClient.clear();
        toast.success(tf("shell.impersonateToast", { name: account.name }));
      })
      .catch((err: unknown) => {
        toast.error(err instanceof Error ? err.message : t("shell.impersonateError"));
      });
  };

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("accounts.title")}
        description={t("accounts.description")}
        actions={
          <Button onClick={() => setCreateOpen(true)} className="min-h-10">
            <UserPlus className="size-4" />
            {t("accounts.create")}
          </Button>
        }
      />

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
                    <TableHead className="text-muted-foreground">{t("accounts.subscription")}</TableHead>
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
                    const disabling = account.status === "active";
                    return (
                      <TableRow key={account.id} className={account.status === "disabled" ? "opacity-60" : undefined}>
                        <TableCell className="pl-4 sm:pl-6">
                          <button
                            type="button"
                            className="rounded font-medium underline-offset-4 hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring"
                            onClick={() => setDetailTarget(account)}
                            aria-label={t("accounts.openDetail")}
                          >
                            {account.name}
                          </button>
                        </TableCell>
                        <TableCell className="text-muted-foreground">{account.owner || "—"}</TableCell>
                        <TableCell>
                          <StatusBadge status={account.status} dot />
                        </TableCell>
                        <TableCell>
                          <SubscriptionBadge state={account.subscription} />
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
                          <div className="flex items-center justify-end gap-1.5">
                            <Button
                              variant="outline"
                              size="sm"
                              className="h-9"
                              onClick={() => openConsole(account)}
                              disabled={account.status !== "active"}
                              aria-label={tf("shell.impersonatingAs", { name: account.name })}
                            >
                              <LogIn className="size-4" />
                              {t("accounts.openConsole")}
                            </Button>
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
                          </div>
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

      {/* Fiche client — informations, abonnement, usage, suppression */}
      {detailTarget && (
        <AccountDetailDialog
          account={detailTarget}
          open
          onOpenChange={(o) => !o && setDetailTarget(null)}
        />
      )}

      {/* Création d'un compte client */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("accounts.createTitle")}</DialogTitle>
            <DialogDescription>{t("accounts.createDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="acc-name">{t("accounts.fieldName")}</Label>
              <Input
                id="acc-name"
                value={accName}
                onChange={(e) => setAccName(e.target.value)}
                autoComplete="off"
                placeholder={t("accounts.fieldNamePh")}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="acc-username">{t("accounts.fieldUsername")}</Label>
              <Input
                id="acc-username"
                value={accUsername}
                onChange={(e) => setAccUsername(e.target.value)}
                autoComplete="off"
                placeholder="client1"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="acc-password">{t("accounts.fieldPassword")}</Label>
              <div className="flex gap-2">
                <Input
                  id="acc-password"
                  type="text"
                  value={accPassword}
                  onChange={(e) => setAccPassword(e.target.value)}
                  autoComplete="new-password"
                  className="font-mono"
                />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  className="size-10 shrink-0"
                  onClick={() => setAccPassword(randomPassword())}
                  aria-label={t("accounts.generate")}
                >
                  <KeyRound className="size-4" />
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">{t("accounts.passwordHint")}</p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button disabled={!canCreate || createMutation.isPending} onClick={() => createMutation.mutate()}>
              {createMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("accounts.create")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Identifiants du compte créé — à remis au client */}
      <Dialog open={!!createdCreds} onOpenChange={(open) => !open && setCreatedCreds(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{tf("accounts.credsTitle", { name: createdCreds?.name ?? "" })}</DialogTitle>
            <DialogDescription>{t("accounts.credsDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="flex items-center justify-between gap-2 rounded-lg border p-3">
              <div className="min-w-0">
                <p className="text-xs text-muted-foreground">{t("accounts.fieldUsername")}</p>
                <p className="truncate font-mono text-sm font-semibold">{createdCreds?.username}</p>
              </div>
              <Button variant="outline" size="icon" className="size-9 shrink-0" onClick={() => createdCreds && void copyCred(createdCreds.username, "username")} aria-label={t("common.copy")}>
                {copied === "username" ? <Check className="size-4 text-emerald-600" /> : <Copy className="size-4" />}
              </Button>
            </div>
            <div className="flex items-center justify-between gap-2 rounded-lg border p-3">
              <div className="min-w-0">
                <p className="text-xs text-muted-foreground">{t("accounts.fieldPassword")}</p>
                <p className="truncate font-mono text-sm font-semibold">{createdCreds?.password}</p>
              </div>
              <Button variant="outline" size="icon" className="size-9 shrink-0" onClick={() => createdCreds && void copyCred(createdCreds.password, "password")} aria-label={t("common.copy")}>
                {copied === "password" ? <Check className="size-4 text-emerald-600" /> : <Copy className="size-4" />}
              </Button>
            </div>
          </div>
          <DialogFooter>
            <Button onClick={() => setCreatedCreds(null)}>{t("common.close")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

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
