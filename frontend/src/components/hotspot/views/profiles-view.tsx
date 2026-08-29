"use client";

// Vue Profils / Forfaits — vitesse, durée, quota et prix des offres hotspot.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "framer-motion";
import {
  ArrowDownUp,
  CalendarClock,
  Database,
  Gauge,
  Loader2,
  MoreHorizontal,
  Pencil,
  Plus,
  Smartphone,
  Timer,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";

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
import { EmptyState } from "@/components/hotspot/empty-state";
import { PageHeader } from "@/components/hotspot/page-header";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { api } from "@/lib/hotspot/api";
import { formatBytes, formatCurrency, formatDuration, formatRateLimit } from "@/lib/hotspot/format";
import type { Profile } from "@/lib/hotspot/types";

// Palette fixe (pastille par index de carte) — pas d'indigo/bleu.
const PALETTE = ["#10b981", "#14b8a6", "#f59e0b", "#f43f5e", "#84cc16", "#a78bfa"];

// Format RouterOS : "2M/2M", "512k/512k", "5M" (insensible à la casse pour le seed "512k/512k").
const RATE_LIMIT_RE = /^\d+[KM](\/\d+[KM])?$/i;

interface ProfileForm {
  name: string;
  rateLimit: string;
  sessionTimeoutMin: string;
  validityDays: string;
  sharedUsers: string;
  price: string;
  dataQuotaMb: string;
}

const DEFAULT_FORM: ProfileForm = {
  name: "",
  rateLimit: "",
  sessionTimeoutMin: "60",
  validityDays: "1",
  sharedUsers: "1",
  price: "0",
  dataQuotaMb: "0",
};

export default function ProfilesView() {
  const currency = useCurrency();
  const queryClient = useQueryClient();

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<Profile | null>(null);
  const [form, setForm] = useState<ProfileForm>(DEFAULT_FORM);
  const [deleting, setDeleting] = useState<Profile | null>(null);

  const { data: profiles, isLoading } = useQuery({
    queryKey: ["/api/profiles"],
    queryFn: () => api<Profile[]>("/api/profiles"),
  });

  function invalidateProfiles() {
    void queryClient.invalidateQueries({ queryKey: ["/api/profiles"] });
    void queryClient.invalidateQueries({ queryKey: ["/api/dashboard"] });
    void queryClient.invalidateQueries({ queryKey: ["/api/reports"] });
  }

  function openCreate() {
    setEditing(null);
    setForm(DEFAULT_FORM);
    setDialogOpen(true);
  }

  function openEdit(profile: Profile) {
    setEditing(profile);
    setForm({
      name: profile.name,
      rateLimit: profile.rateLimit,
      sessionTimeoutMin: String(profile.sessionTimeoutMin),
      validityDays: String(profile.validityDays),
      sharedUsers: String(profile.sharedUsers),
      price: String(profile.price),
      dataQuotaMb: String(profile.dataQuotaMb),
    });
    setDialogOpen(true);
  }

  function closeDialog(open: boolean) {
    setDialogOpen(open);
    if (!open) setEditing(null);
  }

  const saveMutation = useMutation({
    mutationFn: (payload: { id: string | null; body: Record<string, unknown> }) =>
      payload.id
        ? api<Profile>(`/api/profiles/${payload.id}`, { method: "PUT", body: payload.body })
        : api<Profile>("/api/profiles", { method: "POST", body: payload.body }),
    onSuccess: (_profile, variables) => {
      toast.success(variables.id ? "Profil modifié" : "Profil créé");
      closeDialog(false);
      invalidateProfiles();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (profile: Profile) => api<{ ok: boolean }>(`/api/profiles/${profile.id}`, { method: "DELETE" }),
    onSuccess: (_res, profile) => {
      toast.success(`Profil ${profile.name} supprimé`);
      setDeleting(null);
      invalidateProfiles();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const rateInvalid = form.rateLimit.trim() !== "" && !RATE_LIMIT_RE.test(form.rateLimit.trim());

  const sessionNum = parseInt(form.sessionTimeoutMin, 10);
  const validityNum = parseInt(form.validityDays, 10);
  const devicesNum = parseInt(form.sharedUsers, 10);
  const priceNum = Number(form.price);
  const quotaNum = parseInt(form.dataQuotaMb, 10);

  const formValid =
    form.name.trim() !== "" &&
    !rateInvalid &&
    form.rateLimit.trim() !== "" &&
    Number.isInteger(sessionNum) &&
    sessionNum >= 1 &&
    Number.isInteger(validityNum) &&
    validityNum >= 1 &&
    Number.isInteger(devicesNum) &&
    devicesNum >= 1 &&
    devicesNum <= 10 &&
    Number.isFinite(priceNum) &&
    priceNum >= 0 &&
    Number.isInteger(quotaNum) &&
    quotaNum >= 0;

  function submitProfile() {
    if (!formValid || saveMutation.isPending) {
      if (form.name.trim() === "") toast.error("Le nom est obligatoire.");
      else if (form.rateLimit.trim() === "" || rateInvalid)
        toast.error("Limite de débit invalide — format RouterOS, ex : 2M/2M.");
      else if (!Number.isInteger(sessionNum) || sessionNum < 1) toast.error("Durée de session invalide (minimum 1 minute).");
      else if (!Number.isInteger(validityNum) || validityNum < 1) toast.error("Validité invalide (minimum 1 jour).");
      else if (!Number.isInteger(devicesNum) || devicesNum < 1 || devicesNum > 10)
        toast.error("Appareils simultanés : entre 1 et 10.");
      else if (!Number.isFinite(priceNum) || priceNum < 0) toast.error("Prix invalide.");
      else if (!Number.isInteger(quotaNum) || quotaNum < 0) toast.error("Quota invalide.");
      return;
    }
    saveMutation.mutate({
      id: editing?.id ?? null,
      body: {
        name: form.name.trim(),
        rateLimit: form.rateLimit.trim(),
        sessionTimeoutMin: sessionNum,
        sharedUsers: devicesNum,
        validityDays: validityNum,
        price: Math.round(priceNum),
        dataQuotaMb: quotaNum,
      },
    });
  }

  const list = profiles ?? [];

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title="Profils"
        description="Forfaits hotspot : vitesse, durée, quota et prix"
        actions={
          <Button className="h-10" onClick={openCreate}>
            <Plus className="size-4" />
            Nouveau profil
          </Button>
        }
      />

      {isLoading ? (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-64 rounded-xl" />
          ))}
        </div>
      ) : list.length === 0 ? (
        <Card className="gap-0 py-0">
          <EmptyState
            icon={Gauge}
            title="Aucun profil"
            description="Créez votre premier forfait hotspot."
            action={
              <Button onClick={openCreate}>
                <Plus className="size-4" />
                Nouveau profil
              </Button>
            }
          />
        </Card>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          <AnimatePresence initial={false}>
            {list.map((profile, index) => (
              <motion.div
                key={profile.id}
                layout
                initial={{ opacity: 0, y: 14 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, scale: 0.96 }}
                transition={{ duration: 0.25, delay: Math.min(index * 0.04, 0.3), ease: "easeOut" }}
              >
                <Card className="h-full py-0">
                  <CardContent className="flex h-full flex-col gap-4 p-4 sm:p-6">
                    <div className="flex items-start justify-between gap-2">
                      <div className="flex min-w-0 items-center gap-2.5">
                        <span
                          className="size-3 shrink-0 rounded-full"
                          style={{ backgroundColor: PALETTE[index % PALETTE.length] }}
                          aria-hidden
                        />
                        <p className="truncate font-semibold leading-tight">{profile.name}</p>
                      </div>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-10 text-muted-foreground hover:text-foreground"
                            aria-label={`Actions pour ${profile.name}`}
                          >
                            <MoreHorizontal className="size-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-48">
                          <DropdownMenuItem className="min-h-10" onClick={() => openEdit(profile)}>
                            <Pencil className="size-4" />
                            Modifier
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            variant="destructive"
                            className="min-h-10"
                            onClick={() => setDeleting(profile)}
                          >
                            <Trash2 className="size-4" />
                            Supprimer
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>

                    <div>
                      <p className="text-2xl font-semibold tracking-tight tabular-nums">
                        {formatCurrency(profile.price, currency)}
                      </p>
                      <p className="text-xs text-muted-foreground">/ forfait</p>
                    </div>

                    <div className="mt-auto grid grid-cols-1 gap-x-4 gap-y-2 border-t pt-4 sm:grid-cols-2">
                      <div className="flex min-h-6 items-center gap-2 text-sm">
                        <ArrowDownUp className="size-4 shrink-0 text-muted-foreground" />
                        <span className="text-muted-foreground">Débit</span>
                        <span className="ml-auto font-medium">{formatRateLimit(profile.rateLimit)}</span>
                      </div>
                      <div className="flex min-h-6 items-center gap-2 text-sm">
                        <Timer className="size-4 shrink-0 text-muted-foreground" />
                        <span className="text-muted-foreground">Session</span>
                        <span className="ml-auto font-medium">
                          {formatDuration(profile.sessionTimeoutMin * 60)}
                        </span>
                      </div>
                      <div className="flex min-h-6 items-center gap-2 text-sm">
                        <CalendarClock className="size-4 shrink-0 text-muted-foreground" />
                        <span className="text-muted-foreground">Validité</span>
                        <span className="ml-auto font-medium">{profile.validityDays} j</span>
                      </div>
                      <div className="flex min-h-6 items-center gap-2 text-sm">
                        <Smartphone className="size-4 shrink-0 text-muted-foreground" />
                        <span className="text-muted-foreground">Appareils</span>
                        <span className="ml-auto font-medium">×{profile.sharedUsers}</span>
                      </div>
                      <div className="flex min-h-6 items-center gap-2 text-sm sm:col-span-2 sm:max-w-max">
                        <Database className="size-4 shrink-0 text-muted-foreground" />
                        <span className="text-muted-foreground">Quota</span>
                        <span className="ml-auto font-medium">
                          {profile.dataQuotaMb === 0
                            ? "Illimité"
                            : formatBytes(profile.dataQuotaMb * 1048576)}
                        </span>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              </motion.div>
            ))}
          </AnimatePresence>
        </div>
      )}

      {/* Dialogue création / édition */}
      <Dialog open={dialogOpen} onOpenChange={closeDialog}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{editing ? `Modifier ${editing.name}` : "Nouveau profil"}</DialogTitle>
            <DialogDescription>
              {editing
                ? "Ajustez les réglages du forfait. Les vouchers actifs conservent leurs réglages."
                : "Définissez vitesse, durée, quota et prix de votre forfait hotspot."}
            </DialogDescription>
          </DialogHeader>

          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              submitProfile();
            }}
          >
            <div className="grid gap-2">
              <Label htmlFor="profile-name">Nom</Label>
              <Input
                id="profile-name"
                placeholder="Ex. 24 Heures"
                value={form.name}
                onChange={(event) => setForm((f) => ({ ...f, name: event.target.value }))}
                disabled={saveMutation.isPending}
              />
            </div>

            <div className="grid gap-2">
              <Label htmlFor="profile-rate">Limite de débit (descendant/montant)</Label>
              <Input
                id="profile-rate"
                placeholder="2M/2M"
                value={form.rateLimit}
                onChange={(event) => setForm((f) => ({ ...f, rateLimit: event.target.value }))}
                disabled={saveMutation.isPending}
                aria-invalid={rateInvalid}
                className="font-mono"
              />
              <p className={rateInvalid ? "text-xs text-destructive" : "text-xs text-muted-foreground"}>
                {rateInvalid
                  ? "Format invalide — attendu : 2M/2M, 512k/512k ou 5M."
                  : "Format RouterOS, ex : 2M/2M"}
              </p>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <Label htmlFor="profile-session">Durée de session (min)</Label>
                <Input
                  id="profile-session"
                  type="number"
                  min={1}
                  value={form.sessionTimeoutMin}
                  onChange={(event) => setForm((f) => ({ ...f, sessionTimeoutMin: event.target.value }))}
                  disabled={saveMutation.isPending}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="profile-validity">Validité (jours)</Label>
                <Input
                  id="profile-validity"
                  type="number"
                  min={1}
                  value={form.validityDays}
                  onChange={(event) => setForm((f) => ({ ...f, validityDays: event.target.value }))}
                  disabled={saveMutation.isPending}
                />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <Label htmlFor="profile-devices">Appareils simultanés</Label>
                <Input
                  id="profile-devices"
                  type="number"
                  min={1}
                  max={10}
                  value={form.sharedUsers}
                  onChange={(event) => setForm((f) => ({ ...f, sharedUsers: event.target.value }))}
                  disabled={saveMutation.isPending}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="profile-price">Prix ({currency})</Label>
                <Input
                  id="profile-price"
                  type="number"
                  min={0}
                  value={form.price}
                  onChange={(event) => setForm((f) => ({ ...f, price: event.target.value }))}
                  disabled={saveMutation.isPending}
                />
              </div>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="profile-quota">Quota de données (Mo)</Label>
              <Input
                id="profile-quota"
                type="number"
                min={0}
                value={form.dataQuotaMb}
                onChange={(event) => setForm((f) => ({ ...f, dataQuotaMb: event.target.value }))}
                disabled={saveMutation.isPending}
              />
              <p className="text-xs text-muted-foreground">0 = illimité</p>
            </div>

            {saveMutation.isError && (
              <p className="text-sm text-destructive" role="alert">
                {saveMutation.error.message}
              </p>
            )}

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => closeDialog(false)} disabled={saveMutation.isPending}>
                Annuler
              </Button>
              <Button type="submit" disabled={!formValid || saveMutation.isPending}>
                {saveMutation.isPending && <Loader2 className="size-4 animate-spin" />}
                {editing ? "Enregistrer" : "Créer le profil"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Confirmation suppression */}
      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Supprimer le profil {deleting?.name} ?</AlertDialogTitle>
            <AlertDialogDescription>
              Les vouchers existants conserveront leurs réglages. Cette action est irréversible.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>Annuler</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={deleteMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                if (deleting) deleteMutation.mutate(deleting);
              }}
            >
              {deleteMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              Supprimer
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
