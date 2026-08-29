"use client";

// Vue Revendeurs — réseau de distribution : crédits, ventes, transactions.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  MoreVertical,
  Pencil,
  Power,
  Store,
  Trash2,
  UserPlus,
  Wallet,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/hotspot/api";
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
}

const EMPTY_FORM: ResellerForm = { name: "", username: "", phone: "", credit: "" };

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
  const currency = useCurrency();
  const queryClient = useQueryClient();

  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Reseller | null>(null);
  const [creditTarget, setCreditTarget] = useState<Reseller | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Reseller | null>(null);

  const [form, setForm] = useState<ResellerForm>(EMPTY_FORM);
  const [creditAmount, setCreditAmount] = useState("");
  const [creditNote, setCreditNote] = useState("");

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
    mutationFn: (payload: { name: string; username: string; phone: string; credit: number }) =>
      api<Reseller>("/api/resellers", { method: "POST", body: payload }),
    onSuccess: (reseller) => {
      toast.success(`Revendeur ${reseller.name} créé`);
      setCreateOpen(false);
      setForm(EMPTY_FORM);
      invalidateResellers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const updateMutation = useMutation({
    mutationFn: (payload: { id: string; name: string; phone: string }) =>
      api<Reseller>(`/api/resellers/${payload.id}`, { method: "PUT", body: { name: payload.name, phone: payload.phone } }),
    onSuccess: (reseller) => {
      toast.success("Revendeur modifié");
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
      toast.success(reseller.status === "active" ? `Revendeur ${reseller.name} activé` : `Revendeur ${reseller.name} désactivé`);
      invalidateResellers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api<{ ok: boolean }>(`/api/resellers/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Revendeur supprimé");
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
      toast.success("Crédit mis à jour");
      setCreditTarget(null);
      setCreditAmount("");
      setCreditNote("");
      invalidateResellers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const openCreate = () => {
    setForm(EMPTY_FORM);
    setCreateOpen(true);
  };

  const openEdit = (reseller: Reseller) => {
    setForm({ name: reseller.name, username: reseller.username, phone: reseller.phone, credit: String(reseller.credit) });
    setEditTarget(reseller);
  };

  const openCredit = (reseller: Reseller) => {
    setCreditAmount("");
    setCreditNote("");
    setCreditTarget(reseller);
  };

  const submitReseller = () => {
    const name = form.name.trim();
    if (!name) {
      toast.error("Le nom est obligatoire.");
      return;
    }
    if (editTarget) {
      updateMutation.mutate({ id: editTarget.id, name, phone: form.phone.trim() });
      return;
    }
    const username = form.username.trim();
    if (!username) {
      toast.error("L'identifiant est obligatoire.");
      return;
    }
    const credit = Number(form.credit);
    createMutation.mutate({ name, username, phone: form.phone.trim(), credit: Number.isFinite(credit) && credit > 0 ? credit : 0 });
  };

  const parsedAmount = Number(creditAmount);
  const amountValid = Number.isFinite(parsedAmount) && parsedAmount > 0;

  const submitCredit = () => {
    if (!creditTarget || !amountValid) return;
    creditMutation.mutate({ id: creditTarget.id, amount: Math.round(parsedAmount), note: creditNote.trim() || undefined });
  };

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title="Revendeurs"
        description="Votre réseau de distribution : crédits, ventes et performance"
        actions={
          <Button className="h-10" onClick={openCreate}>
            <UserPlus className="size-4" />
            Nouveau revendeur
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
            title="Aucun revendeur"
            description="Ajoutez des revendeurs pour distribuer vos vouchers contre crédits."
            action={
              <Button onClick={openCreate}>
                <UserPlus className="size-4" />
                Nouveau revendeur
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
                  </div>
                  <p className="mt-0.5 truncate text-xs text-muted-foreground">{reseller.phone || "—"}</p>
                </div>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="icon" className="size-10 text-muted-foreground" aria-label={`Actions pour ${reseller.name}`}>
                      <MoreVertical className="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-52">
                    <DropdownMenuItem onClick={() => openCredit(reseller)}>
                      <Wallet className="size-4" />
                      Recharger le crédit
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => openEdit(reseller)}>
                      <Pencil className="size-4" />
                      Modifier
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => toggleMutation.mutate(reseller)}>
                      <Power className="size-4" />
                      {reseller.status === "active" ? "Désactiver" : "Activer"}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem variant="destructive" onClick={() => setDeleteTarget(reseller)}>
                      <Trash2 className="size-4" />
                      Supprimer
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </CardContent>
              <CardContent className="px-4 sm:px-5">
                <div className="rounded-lg border bg-muted/30 p-3">
                  <p className="text-xs uppercase tracking-wide text-muted-foreground">Crédit disponible</p>
                  <p className={cn("mt-1 text-xl font-semibold tabular-nums", creditClass(reseller.credit))}>
                    {formatCurrency(reseller.credit, currency)}
                  </p>
                  <p className="mt-2 text-xs text-muted-foreground">
                    {reseller.vouchersSold} vouchers vendus · {formatCurrency(reseller.revenue, currency)} générés en revenu
                  </p>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <Card className="gap-0 py-0">
        <div className="flex flex-wrap items-center justify-between gap-2 px-4 pt-4 sm:px-6 sm:pt-5">
          <div>
            <h2 className="text-base font-semibold">Transactions récentes</h2>
            <p className="mt-0.5 text-xs text-muted-foreground">Derniers mouvements de crédits et ventes des revendeurs</p>
          </div>
          <span className="text-xs text-muted-foreground">20 dernières</span>
        </div>
        <div className="mt-2 max-h-96 overflow-y-auto">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className="pl-4 text-muted-foreground sm:pl-6">Type</TableHead>
                <TableHead className="text-muted-foreground">Revendeur</TableHead>
                <TableHead className="text-right text-muted-foreground">Montant</TableHead>
                <TableHead className="max-w-48 text-muted-foreground">Note</TableHead>
                <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">Date</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {!transactions || transactions.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                    Aucune transaction pour le moment.
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
                        transaction.type === "credit" ? "text-primary" : "text-destructive",
                      )}
                    >
                      {transaction.type === "credit" ? "+" : "−"}
                      {formatCurrency(Math.abs(transaction.amount), currency)}
                    </TableCell>
                    <TableCell className="max-w-48 truncate text-muted-foreground">{transaction.note || "—"}</TableCell>
                    <TableCell className="pr-4 text-right text-muted-foreground sm:pr-6">
                      {formatDateTime(transaction.at)}
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
            <DialogTitle>{editTarget ? "Modifier le revendeur" : "Nouveau revendeur"}</DialogTitle>
            <DialogDescription>
              {editTarget
                ? "Mettez à jour les informations de contact du revendeur."
                : "Le revendeur pourra vendre vos vouchers contre son crédit."}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="reseller-name">Nom</Label>
              <Input
                id="reseller-name"
                value={form.name}
                onChange={(event) => setForm((f) => ({ ...f, name: event.target.value }))}
                placeholder="Ex. Awa Diallo"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="reseller-username">Identifiant</Label>
              <Input
                id="reseller-username"
                value={form.username}
                disabled={!!editTarget}
                onChange={(event) => setForm((f) => ({ ...f, username: event.target.value }))}
                placeholder="Ex. awa.d"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="reseller-phone">Téléphone</Label>
              <Input
                id="reseller-phone"
                value={form.phone}
                onChange={(event) => setForm((f) => ({ ...f, phone: event.target.value }))}
                placeholder="+221 77 000 00 00"
              />
            </div>
            {!editTarget && (
              <div className="grid gap-2">
                <Label htmlFor="reseller-credit">Crédit initial ({currency})</Label>
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
              Annuler
            </Button>
            <Button
              onClick={submitReseller}
              disabled={createMutation.isPending || updateMutation.isPending || !form.name.trim() || (!editTarget && !form.username.trim())}
            >
              {editTarget ? "Enregistrer" : "Créer le revendeur"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Dialogue rechargement de crédit */}
      <Dialog open={!!creditTarget} onOpenChange={(open) => !open && setCreditTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Recharger le crédit</DialogTitle>
            <DialogDescription>
              {creditTarget ? `${creditTarget.name} — crédit actuel : ${formatCurrency(creditTarget.credit, currency)}` : null}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="credit-amount">Montant ({currency})</Label>
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
              <Label htmlFor="credit-note">Note (optionnelle)</Label>
              <Input
                id="credit-note"
                value={creditNote}
                onChange={(event) => setCreditNote(event.target.value)}
                placeholder="Ex. recharge hebdomadaire"
              />
            </div>
            {creditTarget && amountValid && (
              <p className="rounded-lg border bg-muted/40 px-3 py-2 text-sm">
                Nouveau crédit :{" "}
                <span className="font-semibold text-primary">
                  {formatCurrency(creditTarget.credit + Math.round(parsedAmount), currency)}
                </span>
              </p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreditTarget(null)}>
              Annuler
            </Button>
            <Button onClick={submitCredit} disabled={!amountValid || creditMutation.isPending}>
              {creditMutation.isPending ? "Rechargement…" : "Recharger"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Confirmation suppression */}
      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Supprimer {deleteTarget?.name} ?</AlertDialogTitle>
            <AlertDialogDescription>
              Cette action est irréversible. Le crédit restant et l'accès à la revente seront perdus.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Annuler</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={(event) => {
                event.preventDefault();
                if (deleteTarget) deleteMutation.mutate(deleteTarget.id);
              }}
            >
              {deleteMutation.isPending ? "Suppression…" : "Supprimer"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
