"use client";

// Fiche client — Console Plateforme (P2) : informations, abonnement
// (attribuer / renouveler / marquer payé), usage, routeurs, équipe, journal
// récent et suppression du compte. Le serveur reste la source de vérité :
// le statut d'abonnement affiché est l'état effectif calculé côté backend.

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  CalendarClock,
  Check,
  Copy,
  CreditCard,
  Loader2,
  LogIn,
  RefreshCcw,
  ShieldAlert,
  Trash2,
  Users,
} from "lucide-react";
import { toast } from "sonner";

import {
  ApiError,
  deleteClientAccount,
  fetchAccountDetail,
  impersonateAccount,
  updateAccountSubscription,
} from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { AccountSummary, SubscriptionInfo } from "@/lib/hotspot/types";
import { formatCurrency, formatDate, formatDateTime } from "@/lib/hotspot/format";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { ACCOUNTS_QUERY_KEY } from "./accounts-view";
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
import { Separator } from "@/components/ui/separator";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

/** Clé de cache de la fiche d'un compte (exportée pour l'invalidation). */
export const accountDetailKey = (id: string) => ["/api/admin/accounts", id] as const;

/** Badge d'état d'abonnement (statut effectif renvoyé par le serveur). */
function SubBadge({ sub }: { sub: SubscriptionInfo }) {
  const { t } = useI18n();
  if (sub.status === "active") {
    return (
      <Badge variant="outline" className="border-emerald-500/30 bg-emerald-500/10 px-1.5 py-0 text-[10px] font-medium text-emerald-600 dark:text-emerald-400">
        {t("platform.sub.active")}
      </Badge>
    );
  }
  if (sub.status === "expired") {
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

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className="truncate text-sm font-medium">{children}</div>
    </div>
  );
}

/** Preview locale (indicative) du montant et de l'échéance après attribution. */
function previewSubscription(
  current: SubscriptionInfo,
  planId: "essentiel" | "illimite" | "essai",
  months: number,
  slots: number,
): { amount: number; end: string | null; stacked: boolean } {
  const now = new Date();
  if (planId !== "essentiel" && planId !== "illimite") {
    return { amount: 0, end: null, stacked: false };
  }
  const stackable = current.planId === planId && current.status === "active" && current.periodEnd !== "";
  let base = now;
  if (stackable) {
    const end = new Date(current.periodEnd);
    if (!Number.isNaN(end.getTime()) && end > now) base = end;
  }
  const end = new Date(base);
  end.setMonth(end.getMonth() + months);
  const amount =
    planId === "essentiel"
      ? 1250 * Math.max(slots, 1) * months
      : 1000 * months;
  return { amount, end: end.toISOString(), stacked: stackable };
}

// ---------------------------------------------------------------------------
// Dialog d'attribution / renouvellement d'abonnement
// ---------------------------------------------------------------------------

function SubscriptionDialog({
  accountId,
  current,
  open,
  onOpenChange,
}: {
  accountId: string;
  current: SubscriptionInfo;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const queryClient = useQueryClient();

  const [planId, setPlanId] = useState<string>("essentiel");
  const [months, setMonths] = useState<string>(
    current.planId === "illimite" ? "12" : "1",
  );
  const [slots, setSlots] = useState<string>(
    String(current.routerSlots > 0 ? current.routerSlots : Math.max(current.routerCount, 1)),
  );
  const [markPaid, setMarkPaid] = useState(true);
  const [note, setNote] = useState("");

  const monthsNum = Math.max(1, Math.min(36, parseInt(months, 10) || 1));
  const slotsNum = Math.max(1, parseInt(slots, 10) || 1);
  const planKey = planId as "essentiel" | "illimite" | "essai";
  const preview = useMemo(
    () => previewSubscription(current, planKey, monthsNum, slotsNum),
    [current, planKey, monthsNum, slotsNum],
  );

  const mutation = useMutation({
    mutationFn: () =>
      updateAccountSubscription(accountId, {
        planId: planKey,
        months: planKey === "essentiel" || planKey === "illimite" ? monthsNum : undefined,
        routerSlots: planKey === "essentiel" ? slotsNum : undefined,
        markPaid,
        note: note.trim() || undefined,
      }),
    onSuccess: () => {
      toast.success(t("accounts.sub.appliedToast"));
      onOpenChange(false);
      void queryClient.invalidateQueries({ queryKey: ACCOUNTS_QUERY_KEY });
      void queryClient.invalidateQueries({ queryKey: accountDetailKey(accountId) });
      void queryClient.invalidateQueries({ queryKey: ["/api/admin/overview"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/admin/activity"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("accounts.sub.title")}</DialogTitle>
          <DialogDescription>{t("accounts.sub.desc")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="sub-plan">{t("accounts.sub.plan")}</Label>
            <Select value={planId} onValueChange={setPlanId}>
              <SelectTrigger id="sub-plan" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="essentiel">{t("accounts.sub.planEssentiel")}</SelectItem>
                <SelectItem value="illimite">{t("accounts.sub.planIllimite")}</SelectItem>
                <SelectItem value="essai">{t("accounts.sub.planBeta")}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {(planId === "essentiel" || planId === "illimite") && (
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label htmlFor="sub-months">{t("accounts.sub.months")}</Label>
                <Select value={months} onValueChange={setMonths}>
                  <SelectTrigger id="sub-months" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {["1", "3", "6", "12", "24"].map((m) => (
                      <SelectItem key={m} value={m}>
                        {m}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {planId === "essentiel" && (
                <div className="space-y-2">
                  <Label htmlFor="sub-slots">{t("accounts.sub.slots")}</Label>
                  <Input
                    id="sub-slots"
                    type="number"
                    min={1}
                    max={100}
                    value={slots}
                    onChange={(e) => setSlots(e.target.value)}
                  />
                </div>
              )}
            </div>
          )}

          {planId === "essentiel" && (
            <p className="text-xs text-muted-foreground">{t("accounts.sub.slotsHint")}</p>
          )}

          <div className="flex items-center justify-between gap-3 rounded-lg border p-3">
            <div className="min-w-0">
              <p className="text-sm font-medium">{t("accounts.sub.markPaid")}</p>
              <p className="text-xs text-muted-foreground">{t("accounts.sub.markPaidHint")}</p>
            </div>
            <Switch checked={markPaid} onCheckedChange={setMarkPaid} aria-label={t("accounts.sub.markPaid")} />
          </div>

          <div className="space-y-2">
            <Label htmlFor="sub-note">{t("accounts.sub.note")}</Label>
            <Input
              id="sub-note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder={t("accounts.sub.notePh")}
              maxLength={120}
            />
          </div>

          {/* Preview indicative — le serveur fait foi. */}
          <div className="rounded-lg bg-muted/60 p-3 text-sm">
            {(planId === "essentiel" || planId === "illimite") ? (
              <>
                <p className="font-medium">
                  {tf("accounts.sub.previewAmount", {
                    amount: formatCurrency(preview.amount, currency, lang),
                  })}
                </p>
                {preview.end && (
                  <p className="mt-0.5 flex items-center gap-1.5 text-muted-foreground">
                    <CalendarClock className="size-3.5" />
                    {tf("accounts.sub.previewEnd", { date: formatDate(preview.end, lang) })}
                    {preview.stacked && (
                      <Badge variant="outline" className="ml-1 px-1.5 py-0 text-[10px]">
                        {t("accounts.sub.previewStacked")}
                      </Badge>
                    )}
                  </p>
                )}
              </>
            ) : (
              <p className="font-medium">{t("accounts.sub.previewNone")}</p>
            )}
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button disabled={mutation.isPending} onClick={() => mutation.mutate()}>
            {mutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <CreditCard className="size-4" />}
            {t("accounts.sub.apply")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// Dialog de suppression du compte (cascade totale, confirmation par nom)
// ---------------------------------------------------------------------------

function DeleteAccountDialog({
  account,
  open,
  onOpenChange,
  onDeleted,
}: {
  account: AccountSummary;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDeleted: () => void;
}) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const [confirmName, setConfirmName] = useState("");

  const mutation = useMutation({
    mutationFn: () => deleteClientAccount(account.id),
    onSuccess: () => {
      toast.success(tf("accounts.delete.deletedToast", { name: account.name }));
      onOpenChange(false);
      onDeleted();
      void queryClient.invalidateQueries({ queryKey: ACCOUNTS_QUERY_KEY });
      void queryClient.invalidateQueries({ queryKey: ["/api/admin/overview"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/admin/activity"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const canDelete = confirmName.trim() === account.name;

  return (
    <AlertDialog
      open={open}
      onOpenChange={(o) => {
        if (!o) setConfirmName("");
        onOpenChange(o);
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle className="flex items-center gap-2">
            <ShieldAlert className="size-5 text-destructive" />
            {tf("accounts.delete.title", { name: account.name })}
          </AlertDialogTitle>
          <AlertDialogDescription>{t("accounts.delete.desc")}</AlertDialogDescription>
        </AlertDialogHeader>
        <div className="space-y-2">
          <Label htmlFor="delete-confirm">{tf("accounts.delete.confirmHint", { name: account.name })}</Label>
          <Input
            id="delete-confirm"
            value={confirmName}
            onChange={(e) => setConfirmName(e.target.value)}
            autoComplete="off"
          />
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            className="bg-destructive text-white hover:bg-destructive/90"
            disabled={!canDelete || mutation.isPending}
            onClick={(event) => {
              event.preventDefault();
              mutation.mutate();
            }}
          >
            {mutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
            {t("accounts.delete.confirm")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// ---------------------------------------------------------------------------
// Fiche principale
// ---------------------------------------------------------------------------

export function AccountDetailDialog({
  account,
  open,
  onOpenChange,
}: {
  account: AccountSummary;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const queryClient = useQueryClient();
  const impersonate = useHotspotStore((s) => s.impersonate);
  const [subOpen, setSubOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const [openingConsole, setOpeningConsole] = useState(false);

  const { data, isLoading, error } = useQuery({
    queryKey: accountDetailKey(account.id),
    queryFn: () => fetchAccountDetail(account.id),
    enabled: open,
    retry: false,
  });

  const copyUsername = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard indisponible */
    }
  };

  const openConsole = () => {
    setOpeningConsole(true);
    impersonateAccount(account.id)
      .then((res) => {
        impersonate(res.token, res.user);
        queryClient.clear();
        toast.success(tf("shell.impersonateToast", { name: account.name }));
        onOpenChange(false);
      })
      .catch((err: unknown) => {
        toast.error(err instanceof Error ? err.message : t("shell.impersonateError"));
      })
      .finally(() => setOpeningConsole(false));
  };

  const sub = data?.subscription;

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle className="flex flex-wrap items-center gap-2">
              {tf("accounts.detailTitle", { name: account.name })}
            </DialogTitle>
            <DialogDescription>{t("accounts.detailDesc")}</DialogDescription>
          </DialogHeader>

          {isLoading ? (
            <div className="space-y-3">
              {Array.from({ length: 4 }).map((_, i) => (
                <Skeleton key={i} className="h-20 rounded-lg" />
              ))}
            </div>
          ) : error || !data ? (
            <p className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
              {error instanceof ApiError ? error.message : t("accounts.loadError")}
            </p>
          ) : (
            <div className="space-y-4">
              {/* Informations + abonnement */}
              <div className="grid gap-4 md:grid-cols-2">
                <Card className="gap-2 py-4">
                  <CardContent className="space-y-3">
                    <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                      {t("accounts.detail.info")}
                    </p>
                    <div className="grid grid-cols-2 gap-3">
                      <Field label={t("accounts.detail.owner")}>
                        {data.owner ? (
                          <button
                            type="button"
                            onClick={() => data.owner && void copyUsername(data.owner.username)}
                            className="inline-flex items-center gap-1.5 font-mono hover:underline"
                            aria-label={t("common.copy")}
                          >
                            {data.owner.username}
                            {copied ? <Check className="size-3 text-emerald-600" /> : <Copy className="size-3 opacity-60" />}
                          </button>
                        ) : (
                          "—"
                        )}
                      </Field>
                      <Field label={t("common.status")}>
                        <StatusBadge status={data.status} dot />
                      </Field>
                      <Field label={t("accounts.detail.created")}>
                        {formatDate(data.createdAt, lang)}
                      </Field>
                      <Field label={tf("accounts.detail.team", { n: data.team.length })}>
                        <span className="inline-flex items-center gap-1">
                          <Users className="size-3.5 text-muted-foreground" />
                          {data.team.map((m) => m.username).join(", ") || "—"}
                        </span>
                      </Field>
                    </div>
                    <Button
                      size="sm"
                      variant="outline"
                      className="h-9 w-full"
                      onClick={openConsole}
                      disabled={data.status !== "active" || openingConsole}
                    >
                      {openingConsole ? <Loader2 className="size-4 animate-spin" /> : <LogIn className="size-4" />}
                      {t("accounts.openConsoleFull")}
                    </Button>
                  </CardContent>
                </Card>

                <Card className="gap-2 py-4">
                  <CardContent className="space-y-3">
                    <div className="flex items-center justify-between gap-2">
                      <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                        {t("accounts.detail.sub")}
                      </p>
                      <Button size="sm" variant="outline" className="h-8" onClick={() => setSubOpen(true)}>
                        <RefreshCcw className="size-3.5" />
                        {t("accounts.detail.manage")}
                      </Button>
                    </div>
                    {sub && (
                      <div className="grid grid-cols-2 gap-3">
                        <Field label={t("accounts.detail.subPlan")}>
                          <span className="inline-flex items-center gap-1.5">
                            {sub.planName}
                            <SubBadge sub={sub} />
                          </span>
                        </Field>
                        <Field label={t("accounts.detail.subPayment")}>
                          {sub.lastPaidAt ? (
                            <span className="text-emerald-600 dark:text-emerald-400">
                              {tf("accounts.detail.subPaidAt", { date: formatDate(sub.lastPaidAt, lang) })}
                            </span>
                          ) : (
                            <span className="text-amber-600 dark:text-amber-400">{t("accounts.detail.subUnpaid")}</span>
                          )}
                        </Field>
                        <Field label={t("accounts.detail.subPeriod")}>
                          {sub.periodEnd
                            ? `${formatDate(sub.periodStart, lang)} → ${formatDate(sub.periodEnd, lang)}`
                            : t("accounts.detail.subPeriodNone")}
                        </Field>
                        <Field label={t("accounts.detail.subSlots")}>
                          {sub.routerSlots > 0
                            ? tf("accounts.detail.subSlotsOf", { slots: sub.routerSlots, count: sub.routerCount })
                            : t("accounts.detail.subUnlimited")}
                        </Field>
                      </div>
                    )}
                  </CardContent>
                </Card>
              </div>

              {/* Usage */}
              <Card className="gap-2 py-4">
                <CardContent>
                  <p className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    {t("accounts.detail.usage")}
                  </p>
                  <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
                    {[
                      { label: t("accounts.detail.statRouters"), value: `${data.stats.routersOnline}/${data.stats.routers}`, hint: t("accounts.detail.statOnline") },
                      { label: t("accounts.detail.statUsers"), value: data.stats.users },
                      { label: t("accounts.detail.statVouchers"), value: data.stats.vouchersAvailable },
                      { label: t("accounts.detail.statSessions"), value: data.stats.sessions },
                      { label: t("accounts.detail.statSales"), value: data.stats.sales30d },
                      { label: t("accounts.detail.statRevenue"), value: formatCurrency(data.stats.revenue30d, currency, lang) },
                    ].map((kpi) => (
                      <div key={kpi.label} className="rounded-lg border p-3">
                        <p className="truncate text-xs text-muted-foreground">{kpi.label}</p>
                        <p className="mt-1 truncate text-lg font-semibold tabular-nums">{kpi.value}</p>
                        {kpi.hint && <p className="text-[10px] text-muted-foreground">{kpi.hint}</p>}
                      </div>
                    ))}
                  </div>
                </CardContent>
              </Card>

              {/* Routeurs */}
              <Card className="gap-0 py-4">
                <CardContent className="px-4">
                  <p className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    {t("accounts.detail.routers")}
                  </p>
                  {data.routers.length === 0 ? (
                    <p className="text-sm text-muted-foreground">{t("accounts.detail.routersEmpty")}</p>
                  ) : (
                    <div className="max-h-48 overflow-y-auto rounded-lg border">
                      <Table>
                        <TableHeader>
                          <TableRow className="hover:bg-transparent">
                            <TableHead className="pl-3 text-muted-foreground">{t("accounts.detail.routerName")}</TableHead>
                            <TableHead className="text-muted-foreground">{t("accounts.detail.routerMode")}</TableHead>
                            <TableHead className="text-muted-foreground">{t("common.status")}</TableHead>
                            <TableHead className="text-right text-muted-foreground">{t("accounts.detail.statUsers")}</TableHead>
                            <TableHead className="pr-3 text-right text-muted-foreground">{t("accounts.detail.routerLastSeen")}</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {data.routers.map((rt) => (
                            <TableRow key={rt.id}>
                              <TableCell className="pl-3 font-medium">{rt.name}</TableCell>
                              <TableCell className="text-muted-foreground">{rt.mode}</TableCell>
                              <TableCell>
                                <StatusBadge status={rt.status === "online" ? "online" : "offline"} dot />
                              </TableCell>
                              <TableCell className="text-right tabular-nums">{rt.users}</TableCell>
                              <TableCell className="pr-3 text-right text-xs text-muted-foreground">
                                {rt.lastSeen ? formatDateTime(rt.lastSeen, lang) : "—"}
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </div>
                  )}
                </CardContent>
              </Card>

              {/* Journal récent */}
              <Card className="gap-0 py-4">
                <CardContent className="px-4">
                  <p className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    {t("accounts.detail.activity")}
                  </p>
                  {data.activity.length === 0 ? (
                    <p className="text-sm text-muted-foreground">{t("accounts.detail.activityEmpty")}</p>
                  ) : (
                    <div className="max-h-64 space-y-1 overflow-y-auto pr-1">
                      {data.activity.map((row) => (
                        <div key={row.id} className="flex items-start justify-between gap-3 rounded-md px-2 py-1.5 hover:bg-muted/60">
                          <div className="min-w-0">
                            <p className="truncate text-sm">{row.message}</p>
                            {row.actorName && (
                              <p className="text-xs text-muted-foreground">{row.actorName}</p>
                            )}
                          </div>
                          <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                            {formatDateTime(row.at, lang)}
                          </span>
                        </div>
                      ))}
                    </div>
                  )}
                </CardContent>
              </Card>

              {/* Zone sensible */}
              <>
                <Separator />
                <div className="flex flex-col gap-2 rounded-lg border border-destructive/30 bg-destructive/5 p-4 sm:flex-row sm:items-center sm:justify-between">
                  <div className="min-w-0">
                    <p className="flex items-center gap-1.5 text-sm font-medium text-destructive">
                      <AlertTriangle className="size-4" />
                      {t("accounts.detail.danger")}
                    </p>
                    <p className="text-xs text-muted-foreground">{t("accounts.detail.dangerDesc")}</p>
                  </div>
                  <Button
                    variant="outline"
                    className="shrink-0 border-destructive/40 text-destructive hover:bg-destructive/10"
                    onClick={() => setDeleteOpen(true)}
                  >
                    <Trash2 className="size-4" />
                    {t("accounts.detail.delete")}
                  </Button>
                </div>
              </>
            </div>
          )}
        </DialogContent>
      </Dialog>

      {sub && (
        <SubscriptionDialog
          accountId={account.id}
          current={sub}
          open={subOpen}
          onOpenChange={setSubOpen}
        />
      )}
      <DeleteAccountDialog
        account={account}
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        onDeleted={() => onOpenChange(false)}
      />
    </>
  );
}
