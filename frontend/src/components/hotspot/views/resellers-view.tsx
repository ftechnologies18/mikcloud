"use client";

// Vue Revendeurs — réseau de distribution : crédits, ventes, transactions.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  HandCoins,
  MoreVertical,
  Pencil,
  Power,
  Share2,
  Store,
  Trash2,
  UserPlus,
  Wallet,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { Reseller, Transaction } from "@/lib/hotspot/types";
import { formatCurrency, formatDateTime } from "@/lib/hotspot/format";
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
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { cn } from "@/lib/utils";

interface ResellerForm {
  name: string;
  username: string;
  phone: string;
  credit: string;
  pin: string; // N°8 — PIN Mode Vente (4-6 chiffres ; vide en édition = inchangé)
  paymentMode: "prepaid" | "deposit"; // N°19 — dépôt-vente : il vend puis verse
  debtCeiling: string;
}

const EMPTY_FORM: ResellerForm = { name: "", username: "", phone: "", credit: "", pin: "", paymentMode: "prepaid", debtCeiling: "" };

function initialsOf(name: string): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .map((part) => part[0])
    .slice(0, 2)
    .join("")
    .toUpperCase();
}

function creditClass(credit: number): string {
  if (credit > 10_000) return "text-primary";
  if (credit > 0) return "text-amber-500";
  return "text-destructive";
}

export default function ResellersView() {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const queryClient = useQueryClient();

  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Reseller | null>(null);
  const [creditTarget, setCreditTarget] = useState<Reseller | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Reseller | null>(null);

  const [form, setForm] = useState<ResellerForm>(EMPTY_FORM);
  const [creditAmount, setCreditAmount] = useState("");
  const [creditNote, setCreditNote] = useState("");
  // N°19 — encaissement de versement (dépôt-vente).
  const [settleTarget, setSettleTarget] = useState<Reseller | null>(null);
  const [settleAmount, setSettleAmount] = useState("");
  const [settleNote, setSettleNote] = useState("");
  // N°19 v2 — source du versement : cash au guichet ou compensation avec le
  // crédit prépayé (avance dormante héritée de l'ère prépayée).
  const [settleMethod, setSettleMethod] = useState<"cash" | "credit">("cash");
  // N°19 V2 — reçu du dernier versement (partageable WhatsApp).
  const [receipt, setReceipt] = useState<{ name: string; amount: number; debtAfter: number; creditAfter?: number; method: "cash" | "credit"; at: string } | null>(null);

  const { data: resellers, isLoading } = useQuery({
    queryKey: ["/api/resellers"],
    queryFn: () => api<Reseller[]>("/api/resellers"),
  });

  const { data: transactions } = useQuery({
    queryKey: ["/api/transactions"],
    queryFn: () => api<Transaction[]>("/api/transactions", { params: { limit: 20 } }),
    refetchInterval: 20_000,
  });

  const invalidateResellers = () => {
    queryClient.invalidateQueries({ queryKey: ["/api/resellers"] });
    queryClient.invalidateQueries({ queryKey: ["/api/transactions"] });
  };

  const createMutation = useMutation({
    mutationFn: (payload: { name: string; username: string; phone: string; credit: number; pin?: string; paymentMode?: string; debtCeiling?: number }) =>
      api<Reseller>("/api/resellers", { method: "POST", body: payload }),
    onSuccess: (reseller) => {
      toast.success(tf("resellers.createdToast", { name: reseller.name }));
      setCreateOpen(false);
      setForm(EMPTY_FORM);
      invalidateResellers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const updateMutation = useMutation({
    mutationFn: (payload: { id: string; name: string; phone: string; pin?: string; paymentMode?: string; debtCeiling?: number }) =>
      api<Reseller>(`/api/resellers/${payload.id}`, {
        method: "PUT",
        body: {
          name: payload.name,
          phone: payload.phone,
          ...(payload.pin !== undefined ? { pin: payload.pin } : {}),
          ...(payload.paymentMode !== undefined ? { paymentMode: payload.paymentMode } : {}),
          ...(payload.debtCeiling !== undefined ? { debtCeiling: payload.debtCeiling } : {}),
        },
      }),
    onSuccess: (reseller) => {
      toast.success(t("resellers.updatedToast"));
      setEditTarget(null);
      setForm(EMPTY_FORM);
      invalidateResellers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const toggleMutation = useMutation({
    mutationFn: (reseller: Reseller) =>
      api<Reseller>(`/api/resellers/${reseller.id}`, {
        method: "PUT",
        body: { status: reseller.status === "active" ? "disabled" : "active" },
      }),
    onSuccess: (reseller) => {
      toast.success(
        reseller.status === "active"
          ? tf("resellers.activatedToast", { name: reseller.name })
          : tf("resellers.deactivatedToast", { name: reseller.name }),
      );
      invalidateResellers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) =>
      api<{ ok: boolean; transactionsPurged: number }>(`/api/resellers/${id}`, { method: "DELETE" }),
    onSuccess: (res) => {
      // V2 — cascade : le backend retourne le volume d'historique purgé.
      toast.success(
        res.transactionsPurged > 0
          ? tf("resellers.deletedToastHistory", { n: res.transactionsPurged })
          : t("resellers.deletedToast"),
      );
      setDeleteTarget(null);
      invalidateResellers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const creditMutation = useMutation({
    mutationFn: (payload: { id: string; amount: number; note?: string }) =>
      api<{ reseller: Reseller; transaction: Transaction }>(`/api/resellers/${payload.id}/credit`, {
        method: "POST",
        body: { amount: payload.amount, note: payload.note || undefined },
      }),
    onSuccess: () => {
      toast.success(t("resellers.rechargedToast"));
      setCreditTarget(null);
      setCreditAmount("");
      setCreditNote("");
      invalidateResellers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // N°19 — encaissement d'un versement (dépôt-vente) ; v2 : method=credit
  // compense la dette avec le crédit prépayé (aucun cash au guichet).
  const settleMutation = useMutation({
    mutationFn: (payload: { id: string; amount: number; note?: string; method?: "cash" | "credit" }) =>
      api<{ ok: boolean; debtAfter: number; creditAfter?: number }>(`/api/resellers/${payload.id}/settle`, {
        method: "POST",
        body: { amount: payload.amount, note: payload.note || undefined, method: payload.method || undefined },
      }),
    onSuccess: (res) => {
      const byCredit = settleMutation.variables?.method === "credit";
      toast.success(
        byCredit
          ? tf("resellers.offsetToast", { debt: formatCurrency(res.debtAfter, currency, lang) })
          : tf("resellers.settledToast", { debt: formatCurrency(res.debtAfter, currency, lang) }),
      );
      // N°19 V2 — le dialog passe en mode reçu (partage WhatsApp).
      setReceipt({
        name: settleTarget?.name ?? "",
        amount: settleMutation.variables?.amount ?? 0,
        debtAfter: res.debtAfter,
        creditAfter: res.creditAfter,
        method: byCredit ? "credit" : "cash",
        at: new Date().toISOString(),
      });
      setSettleAmount("");
      setSettleNote("");
      invalidateResellers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const openCreate = () => {
    setForm(EMPTY_FORM);
    setCreateOpen(true);
  };

  const openEdit = (reseller: Reseller) => {
    setForm({
      name: reseller.name,
      username: reseller.username,
      phone: reseller.phone,
      credit: String(reseller.credit),
      pin: "",
      paymentMode: reseller.paymentMode ?? "prepaid",
      debtCeiling: reseller.debtCeiling ? String(reseller.debtCeiling) : "",
    });
    setEditTarget(reseller);
  };

  const openCredit = (reseller: Reseller) => {
    setCreditAmount("");
    setCreditNote("");
    setCreditTarget(reseller);
  };

  // N°19 — pré-remplit le versement avec la dette totale (tout régler).
  const openSettle = (reseller: Reseller) => {
    setSettleAmount(reseller.debt ? String(reseller.debt) : "");
    setSettleNote("");
    setSettleMethod("cash");
    setReceipt(null);
    setSettleTarget(reseller);
  };

  // N°19 V2 — reçu de versement : partage WhatsApp ou presse-papiers.
  async function shareReceipt() {
    if (!receipt) return;
    const dateLabel = new Date(receipt.at).toLocaleString(lang === "en" ? "en-GB" : "fr-FR", {
      day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit",
    });
    const text =
      receipt.method === "credit"
        ? tf("resellers.receiptTextOffset", {
            name: receipt.name,
            amount: formatCurrency(receipt.amount, currency, lang),
            debt: formatCurrency(receipt.debtAfter, currency, lang),
            credit: formatCurrency(receipt.creditAfter ?? 0, currency, lang),
            date: dateLabel,
          })
        : tf("resellers.receiptText", {
            name: receipt.name,
            amount: formatCurrency(receipt.amount, currency, lang),
            debt: formatCurrency(receipt.debtAfter, currency, lang),
            date: dateLabel,
          });
    try {
      if (navigator.share) {
        await navigator.share({ title: "MikCloud", text });
      } else {
        await navigator.clipboard.writeText(text);
        toast.success(t("resellers.receiptShared"));
      }
    } catch {
      /* partage annulé par l'utilisateur */
    }
  }

  const submitReseller = () => {
    const name = form.name.trim();
    if (!name) {
      toast.error(t("common.nameRequired"));
      return;
    }
    if (editTarget) {
      const pin = form.pin.trim();
      const ceiling = Number(form.debtCeiling);
      if (form.paymentMode === "deposit" && (!Number.isFinite(ceiling) || ceiling <= 0)) {
        toast.error(t("resellers.depositNeedCeiling"));
        return;
      }
      updateMutation.mutate({
        id: editTarget.id,
        name,
        phone: form.phone.trim(),
        paymentMode: form.paymentMode,
        debtCeiling: form.paymentMode === "deposit" && Number.isFinite(ceiling) ? Math.round(ceiling) : 0,
        ...(pin ? { pin } : {}),
      });
      return;
    }
    const username = form.username.trim();
    if (!username) {
      toast.error(t("common.usernameRequired"));
      return;
    }
    const credit = Number(form.credit);
    const pin = form.pin.trim();
    if (pin && !/^[0-9]{4,6}$/.test(pin)) {
      toast.error(t("resellers.pinInvalid"));
      return;
    }
    const ceiling = Number(form.debtCeiling);
    if (form.paymentMode === "deposit" && (!Number.isFinite(ceiling) || ceiling <= 0)) {
      toast.error(t("resellers.depositNeedCeiling"));
      return;
    }
    createMutation.mutate({
      name,
      username,
      phone: form.phone.trim(),
      credit: Number.isFinite(credit) && credit > 0 ? credit : 0,
      paymentMode: form.paymentMode,
      debtCeiling: form.paymentMode === "deposit" && Number.isFinite(ceiling) ? Math.round(ceiling) : 0,
      ...(pin ? { pin } : {}),
    });
  };

  const parsedAmount = Number(creditAmount);
  const amountValid = Number.isFinite(parsedAmount) && parsedAmount > 0;

  const submitCredit = () => {
    if (!creditTarget || !amountValid) return;
    creditMutation.mutate({ id: creditTarget.id, amount: Math.round(parsedAmount), note: creditNote.trim() || undefined });
  };

  const parsedSettle = Number(settleAmount);
  // N°19 v2 — en compensation, le montant est borné par la dette ET le crédit prépayé.
  const settleMax = settleTarget
    ? settleMethod === "credit"
      ? Math.min(settleTarget.debt ?? 0, settleTarget.credit)
      : (settleTarget.debt ?? 0)
    : 0;
  const settleValid = Number.isFinite(parsedSettle) && parsedSettle > 0 && !!settleTarget && Math.round(parsedSettle) <= settleMax;

  const submitSettle = () => {
    if (!settleTarget || !settleValid) return;
    settleMutation.mutate({ id: settleTarget.id, amount: Math.round(parsedSettle), note: settleNote.trim() || undefined, method: settleMethod });
  };

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("resellers.title")}
        description={t("resellers.description")}
        actions={
          <Button className="h-10" onClick={openCreate}>
            <UserPlus className="size-4" />
            {t("resellers.new")}
          </Button>
        }
      />

      {isLoading ? (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-48 rounded-xl" />
          ))}
        </div>
      ) : !resellers || resellers.length === 0 ? (
        <Card className="gap-0 py-0">
          <EmptyState
            icon={Store}
            title={t("resellers.empty")}
            description={t("resellers.emptyDesc")}
            action={
              <Button onClick={openCreate}>
                <UserPlus className="size-4" />
                {t("resellers.new")}
              </Button>
            }
          />
        </Card>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {resellers.map((reseller) => (
            <Card key={reseller.id} className="gap-4 py-4 sm:py-5">
              <CardContent className="flex items-start gap-3 px-4 sm:px-5">
                <Avatar className="size-10">
                  <AvatarFallback className="bg-primary/15 font-medium text-primary">
                    {initialsOf(reseller.name) || "?"}
                  </AvatarFallback>
                </Avatar>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="truncate font-medium">{reseller.name}</p>
                    <StatusBadge status={reseller.status} dot />
                    {/* N°19 — mode de paiement : dépôt-vente visible d'un coup d'œil. */}
                    {reseller.paymentMode === "deposit" && <StatusBadge status="debt" label={t("resellers.modeDeposit")} />}
                  </div>
                  <p className="mt-0.5 truncate text-xs text-muted-foreground">{reseller.phone || "—"}</p>
                </div>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="icon" className="size-10 text-muted-foreground" aria-label={tf("common.actionsFor", { name: reseller.name })}>
                      <MoreVertical className="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-52">
                    {reseller.paymentMode === "deposit" ? (
                      /* N°19 — dépôt-vente : on encaisse, on ne recharge pas. */
                      <DropdownMenuItem onClick={() => openSettle(reseller)}>
                        <HandCoins className="size-4" />
                        {t("resellers.settle")}
                      </DropdownMenuItem>
                    ) : (
                      <DropdownMenuItem onClick={() => openCredit(reseller)}>
                        <Wallet className="size-4" />
                        {t("resellers.recharge")}
                      </DropdownMenuItem>
                    )}
                    <DropdownMenuItem onClick={() => openEdit(reseller)}>
                      <Pencil className="size-4" />
                      {t("common.edit")}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => toggleMutation.mutate(reseller)}>
                      <Power className="size-4" />
                      {reseller.status === "active" ? t("common.deactivate") : t("common.activate")}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem variant="destructive" onClick={() => setDeleteTarget(reseller)}>
                      <Trash2 className="size-4" />
                      {t("common.delete")}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </CardContent>
              <CardContent className="px-4 sm:px-5">
                {reseller.paymentMode === "deposit" ? (
                  /* N°19 — dépôt-vente : la créance remplace le crédit disponible. */
                  <div className="rounded-lg border bg-muted/30 p-3">
                    <div className="flex items-center justify-between gap-2">
                      <p className="text-xs uppercase tracking-wide text-muted-foreground">{t("resellers.debtLabel")}</p>
                      <Button variant="outline" size="sm" className="h-7 px-2 text-xs" onClick={() => openSettle(reseller)}>
                        <HandCoins className="size-3.5" />
                        {t("resellers.settleSubmit")}
                      </Button>
                    </div>
                    <p className={cn("mt-1 text-xl font-semibold tabular-nums", (reseller.debt ?? 0) > 0 ? "text-amber-600 dark:text-amber-400" : "text-chart-1")}>
                      {formatCurrency(reseller.debt ?? 0, currency, lang)}
                    </p>
                    {!!reseller.debtCeiling && (
                      <p className="mt-0.5 text-xs text-muted-foreground">
                        {tf("resellers.ceilingLine", { ceiling: formatCurrency(reseller.debtCeiling, currency, lang) })}
                      </p>
                    )}
                  </div>
                ) : (
                  <div className="rounded-lg border bg-muted/30 p-3">
                    <p className="text-xs uppercase tracking-wide text-muted-foreground">{t("resellers.creditAvailable")}</p>
                    <p className={cn("mt-1 text-xl font-semibold tabular-nums", creditClass(reseller.credit))}>
                      {formatCurrency(reseller.credit, currency, lang)}
                    </p>
                  </div>
                )}
                {/* N°8 — stock vs vendus (traçabilité anti-vol) : l'écart
                    stock + vendus vs attribués révèle les tickets sortis
                    sans déclaration du revendeur. */}
                <div className="mt-3 grid grid-cols-3 gap-2">
                  <div className="rounded-lg border bg-muted/30 p-2.5 text-center">
                    <p className="text-[11px] text-muted-foreground">{t("resellers.stockCount")}</p>
                    <p className="mt-0.5 text-lg font-semibold tabular-nums">{reseller.stockCount ?? "—"}</p>
                  </div>
                  <div className="rounded-lg border bg-muted/30 p-2.5 text-center">
                    <p className="text-[11px] text-muted-foreground">{t("resellers.soldCount")}</p>
                    <p className="mt-0.5 flex items-baseline justify-center gap-1 text-lg font-semibold tabular-nums">
                      {reseller.soldCount ?? "—"}
                      {!!reseller.soldToday && (
                        <span className="text-[11px] font-medium text-primary">
                          {tf("resellers.soldTodayBadge", { n: reseller.soldToday })}
                        </span>
                      )}
                    </p>
                  </div>
                  <div className="rounded-lg border bg-muted/30 p-2.5 text-center">
                    <p className="text-[11px] text-muted-foreground">{t("resellers.assignedCount")}</p>
                    <p className="mt-0.5 text-lg font-semibold tabular-nums">{reseller.assignedCount ?? "—"}</p>
                  </div>
                </div>
                <p className="mt-2 text-xs text-muted-foreground">
                  {tf("resellers.revenueLine", {
                    today: formatCurrency(reseller.revenueToday ?? 0, currency, lang),
                    total: formatCurrency(reseller.revenueTotal ?? reseller.revenue, currency, lang),
                  })}
                </p>
                {/* N°19 V2 — confiance progressive : suggérer d'élargir le crédit
                    quand les versements sont réguliers et la dette soldée. */}
                {reseller.paymentMode === "deposit" && (reseller.settlementsCount ?? 0) >= 3 && (reseller.debt ?? 0) === 0 && (
                  <p className="mt-1.5 flex items-center gap-1 text-xs text-chart-1">
                    <HandCoins className="size-3" aria-hidden />
                    {tf("resellers.trustHint", { n: reseller.settlementsCount ?? 0 })}
                  </p>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <Card className="gap-0 py-0">
        <div className="flex flex-wrap items-center justify-between gap-2 px-4 pt-4 sm:px-6 sm:pt-5">
          <div>
            <h2 className="text-base font-semibold">{t("resellers.transactions")}</h2>
            <p className="mt-0.5 text-xs text-muted-foreground">{t("resellers.transactionsDesc")}</p>
          </div>
          <span className="text-xs text-muted-foreground">{t("resellers.last20")}</span>
        </div>
        <div className="mt-2 max-h-96 overflow-y-auto">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("common.type")}</TableHead>
                <TableHead className="text-muted-foreground">{t("common.reseller")}</TableHead>
                <TableHead className="text-right text-muted-foreground">{t("resellers.amount")}</TableHead>
                <TableHead className="max-w-48 text-muted-foreground">{t("resellers.note")}</TableHead>
                <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("common.date")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {!transactions || transactions.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                    {t("resellers.noTransactions")}
                  </TableCell>
                </TableRow>
              ) : (
                transactions.map((transaction) => (
                  <TableRow key={transaction.id}>
                    <TableCell className="pl-4 sm:pl-6">
                      <StatusBadge status={transaction.type} />
                    </TableCell>
                    <TableCell className="max-w-40 truncate">{transaction.resellerName}</TableCell>
                    <TableCell
                      className={cn(
                        "text-right font-medium tabular-nums",
                        transaction.type === "credit" || transaction.type === "settlement" ? "text-primary" : "text-destructive",
                      )}
                    >
                      {transaction.type === "credit" || transaction.type === "settlement" ? "+" : "−"}
                      {formatCurrency(Math.abs(transaction.amount), currency, lang)}
                    </TableCell>
                    <TableCell className="max-w-48 truncate text-muted-foreground">{transaction.note || "—"}</TableCell>
                    <TableCell className="pr-4 text-right text-muted-foreground sm:pr-6">
                      {formatDateTime(transaction.at, lang)}
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </Card>

      {/* Dialogue création / édition */}
      <Dialog
        open={createOpen || !!editTarget}
        onOpenChange={(open) => {
          if (!open) {
            setCreateOpen(false);
            setEditTarget(null);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editTarget ? t("resellers.editTitle") : t("resellers.newTitle")}</DialogTitle>
            <DialogDescription>
              {editTarget ? t("resellers.editDesc") : t("resellers.newDesc")}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="reseller-name">{t("resellers.name")}</Label>
              <Input
                id="reseller-name"
                value={form.name}
                onChange={(event) => setForm((f) => ({ ...f, name: event.target.value }))}
                placeholder={t("resellers.namePlaceholder")}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="reseller-username">{t("login.username")}</Label>
              <Input
                id="reseller-username"
                value={form.username}
                disabled={!!editTarget}
                onChange={(event) => setForm((f) => ({ ...f, username: event.target.value }))}
                placeholder={t("resellers.usernamePlaceholder")}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="reseller-phone">{t("resellers.phone")}</Label>
              <Input
                id="reseller-phone"
                value={form.phone}
                onChange={(event) => setForm((f) => ({ ...f, phone: event.target.value }))}
                placeholder={t("resellers.phonePlaceholder")}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="reseller-pin">
                {editTarget ? t("resellers.pinEdit") : t("resellers.pinCreate")}
              </Label>
              <Input
                id="reseller-pin"
                type="password"
                inputMode="numeric"
                pattern="[0-9]*"
                maxLength={6}
                autoComplete="new-password"
                value={form.pin}
                onChange={(event) => setForm((f) => ({ ...f, pin: event.target.value.replace(/\D/g, "") }))}
                placeholder="••••"
              />
              <p className="text-xs text-muted-foreground">
                {editTarget ? t("resellers.pinEditHint") : t("resellers.pinCreateHint")}
              </p>
            </div>
            {/* N°19 — mode de paiement : prépayé ou dépôt-vente (bascule à tout moment). */}
            <div className="grid gap-2">
              <Label>{t("resellers.mode")}</Label>
              <div className="grid gap-2 sm:grid-cols-2">
                {(["prepaid", "deposit"] as const).map((mode) => (
                  <button
                    key={mode}
                    type="button"
                    onClick={() => setForm((f) => ({ ...f, paymentMode: mode }))}
                    className={cn(
                      "rounded-lg border p-3 text-left transition-colors",
                      form.paymentMode === mode ? "border-primary bg-primary/5" : "hover:bg-muted/40",
                    )}
                    aria-pressed={form.paymentMode === mode}
                  >
                    <p className="text-sm font-medium">{mode === "prepaid" ? t("resellers.modePrepaid") : t("resellers.modeDeposit")}</p>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {mode === "prepaid" ? t("resellers.modePrepaidDesc") : t("resellers.modeDepositDesc")}
                    </p>
                  </button>
                ))}
              </div>
            </div>
            {form.paymentMode === "deposit" && (
              <div className="grid gap-2">
                <Label htmlFor="reseller-ceiling">{t("resellers.debtCeiling")}</Label>
                <Input
                  id="reseller-ceiling"
                  type="number"
                  min={1}
                  value={form.debtCeiling}
                  onChange={(event) => setForm((f) => ({ ...f, debtCeiling: event.target.value }))}
                  placeholder="25000"
                />
                <p className="text-xs text-muted-foreground">{t("resellers.debtCeilingHint")}</p>
              </div>
            )}
            {!editTarget && form.paymentMode === "prepaid" && (
              <div className="grid gap-2">
                <Label htmlFor="reseller-credit">{tf("resellers.initialCredit", { currency })}</Label>
                <Input
                  id="reseller-credit"
                  type="number"
                  min={0}
                  value={form.credit}
                  onChange={(event) => setForm((f) => ({ ...f, credit: event.target.value }))}
                  placeholder="10000"
                />
              </div>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setCreateOpen(false);
                setEditTarget(null);
              }}
            >
              {t("common.cancel")}
            </Button>
            <Button
              onClick={submitReseller}
              disabled={createMutation.isPending || updateMutation.isPending || !form.name.trim() || (!editTarget && !form.username.trim())}
            >
              {editTarget ? t("common.save") : t("resellers.createSubmit")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Dialogue rechargement de crédit */}
      <Dialog open={!!creditTarget} onOpenChange={(open) => !open && setCreditTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("resellers.recharge")}</DialogTitle>
            <DialogDescription>
              {creditTarget
                ? tf("resellers.rechargeDesc", {
                    name: creditTarget.name,
                    credit: formatCurrency(creditTarget.credit, currency, lang),
                  })
                : null}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="credit-amount">{tf("resellers.rechargeAmount", { currency })}</Label>
              <Input
                id="credit-amount"
                type="number"
                min={1}
                value={creditAmount}
                onChange={(event) => setCreditAmount(event.target.value)}
                placeholder="10000"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="credit-note">{t("resellers.rechargeNote")}</Label>
              <Input
                id="credit-note"
                value={creditNote}
                onChange={(event) => setCreditNote(event.target.value)}
                placeholder={t("resellers.rechargeNotePlaceholder")}
              />
            </div>
            {creditTarget && amountValid && (
              <p className="rounded-lg border bg-muted/40 px-3 py-2 text-sm">
                {t("resellers.newCredit")}{" "}
                <span className="font-semibold text-primary">
                  {formatCurrency(creditTarget.credit + Math.round(parsedAmount), currency, lang)}
                </span>
              </p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreditTarget(null)}>
              {t("common.cancel")}
            </Button>
            <Button onClick={submitCredit} disabled={!amountValid || creditMutation.isPending}>
              {creditMutation.isPending ? t("resellers.recharging") : t("resellers.rechargeSubmit")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* N°19 — Encaissement de versement (dépôt-vente) */}
      <Dialog open={!!settleTarget} onOpenChange={(open) => !open && setSettleTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("resellers.settleTitle")}</DialogTitle>
            <DialogDescription>
              {settleTarget
                ? tf("resellers.settleDesc", {
                    name: settleTarget.name,
                    debt: formatCurrency(settleTarget.debt ?? 0, currency, lang),
                  })
                : null}
            </DialogDescription>
          </DialogHeader>
          {receipt ? (
            <div className="space-y-3 rounded-lg border bg-muted/40 p-4 text-center">
              {receipt.method === "credit" ? (
                <Wallet className="mx-auto size-8 text-chart-1" aria-hidden />
              ) : (
                <HandCoins className="mx-auto size-8 text-chart-1" aria-hidden />
              )}
              <p className="text-sm font-semibold">
                {formatCurrency(receipt.amount, currency, lang)} — {receipt.name}
              </p>
              <p className="text-xs text-muted-foreground">
                {t("resellers.settleAfter")} {formatCurrency(receipt.debtAfter, currency, lang)}
                {receipt.method === "credit" && (
                  <>
                    {" · "}
                    {t("resellers.settleCreditAfter")} {formatCurrency(receipt.creditAfter ?? 0, currency, lang)}
                  </>
                )}
              </p>
            </div>
          ) : (
          <div className="grid gap-4">
            {/* N°19 v2 — source du versement : guichet (cash) ou compensation
                avec l'avance prépayée (proposée seulement si crédit > 0). */}
            {!!settleTarget?.credit && (
              <div className="grid gap-2">
                <Label>{t("resellers.settleSource")}</Label>
                <div className="grid grid-cols-2 gap-2">
                  <button
                    type="button"
                    onClick={() => setSettleMethod("cash")}
                    className={cn(
                      "rounded-xl border p-3 text-left text-sm transition-colors",
                      settleMethod === "cash" ? "border-primary bg-primary/5" : "hover:bg-muted/40",
                    )}
                    aria-pressed={settleMethod === "cash"}
                  >
                    <span className="flex items-center gap-2 font-medium">
                      <HandCoins className="size-4" aria-hidden />
                      {t("resellers.settleMethodCash")}
                    </span>
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setSettleMethod("credit");
                      // Le montant pré-rempli (dette totale) peut dépasser
                      // le crédit disponible → on borne au cap compensation.
                      const cap = Math.min(settleTarget.debt ?? 0, settleTarget.credit);
                      if (Number(settleAmount) > cap) setSettleAmount(String(cap));
                    }}
                    className={cn(
                      "rounded-xl border p-3 text-left text-sm transition-colors",
                      settleMethod === "credit" ? "border-primary bg-primary/5" : "hover:bg-muted/40",
                    )}
                    aria-pressed={settleMethod === "credit"}
                  >
                    <span className="flex items-center gap-2 font-medium">
                      <Wallet className="size-4" aria-hidden />
                      {t("resellers.settleMethodCredit")}
                    </span>
                    <span className="mt-1 block text-xs text-muted-foreground">
                      {tf("resellers.settleMethodCreditDesc", { credit: formatCurrency(settleTarget.credit, currency, lang) })}
                    </span>
                  </button>
                </div>
              </div>
            )}
            <div className="grid gap-2">
              <Label htmlFor="settle-amount">{t("resellers.settleAmount")}</Label>
              <div className="flex gap-2">
                <Input
                  id="settle-amount"
                  type="number"
                  min={1}
                  value={settleAmount}
                  onChange={(event) => setSettleAmount(event.target.value)}
                  placeholder="5000"
                />
                {settleMax > 0 && (
                  <Button type="button" variant="outline" onClick={() => setSettleAmount(String(settleMax))}>
                    {t("resellers.settleAll")}
                  </Button>
                )}
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="settle-note">{t("resellers.settleNote")}</Label>
              <Input
                id="settle-note"
                value={settleNote}
                onChange={(event) => setSettleNote(event.target.value)}
                placeholder={t("resellers.settleNotePlaceholder")}
              />
            </div>
            {settleTarget && settleValid && (
              <p className="rounded-lg border bg-muted/40 px-3 py-2 text-sm">
                {t("resellers.settleAfter")}{" "}
                <span className="font-semibold text-primary">
                  {formatCurrency(Math.max((settleTarget.debt ?? 0) - Math.round(parsedSettle), 0), currency, lang)}
                </span>
                {settleMethod === "credit" && (
                  <>
                    {" · "}
                    {t("resellers.settleCreditAfter")}{" "}
                    <span className="font-semibold text-primary">
                      {formatCurrency(Math.max(settleTarget.credit - Math.round(parsedSettle), 0), currency, lang)}
                    </span>
                  </>
                )}
              </p>
            )}
          </div>
          )}
          <DialogFooter>
            {receipt ? (
              <>
                <Button variant="outline" onClick={() => setSettleTarget(null)}>
                  {t("common.close")}
                </Button>
                <Button onClick={() => void shareReceipt()}>
                  <Share2 className="size-4" />
                  {t("resellers.receiptShare")}
                </Button>
              </>
            ) : (
              <>
                <Button variant="outline" onClick={() => setSettleTarget(null)}>
                  {t("common.cancel")}
                </Button>
                <Button onClick={submitSettle} disabled={!settleValid || settleMutation.isPending}>
                  {settleMutation.isPending ? t("resellers.settling") : settleMethod === "credit" ? t("resellers.offsetSubmit") : t("resellers.settleSubmit")}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Confirmation suppression */}
      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{tf("resellers.deleteTitle", { name: deleteTarget?.name ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>{t("resellers.deleteDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={(event) => {
                event.preventDefault();
                if (deleteTarget) deleteMutation.mutate(deleteTarget.id);
              }}
            >
              {deleteMutation.isPending ? t("common.deleting") : t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
