"use client";

// Vue Profils / Forfaits — vitesse, durée, quota, prix et expiration des offres
// hotspot. Cartes + dialog d'édition (parts/profile-dialog.tsx, champs P0 : mode
// d'expiration, grâce, verrouillage, prix de vente).

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "framer-motion";
import {
  ArrowDownUp,
  Bell,
  CalendarClock,
  Database,
  Gauge,
  Loader2,
  Lock,
  MonitorSmartphone,
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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/hotspot/empty-state";
import { PageHeader } from "@/components/hotspot/page-header";
import { ProfileEditDialog } from "@/components/hotspot/parts/profile-dialog";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatBytes, formatCurrency, formatDuration, formatRateLimit , fmtRouterDuration } from "@/lib/hotspot/format";
import type { Profile } from "@/lib/hotspot/types";
import { useChartPalette } from "@/lib/hotspot/chart-theme";

/** Badge du mode d'expiration (F1) : cloche = notification, poubelle = suppression. */
function ExpiryBadge({ profile }: { profile: Profile }) {
  const { t, tf } = useI18n();
  const remove = profile.expMode === "remove";
  const grace = profile.gracePeriodMin ?? 0;
  const graceTitle = grace > 0 ? tf("profiles.expiry.graceTitle", { n: grace }) : t("profiles.expiry.immediateTitle");
  return (
    <Badge
      variant="outline"
      className={
        remove
          ? "gap-1 border-destructive/25 bg-destructive/10 text-destructive"
          : "gap-1 border-primary/25 bg-primary/10 text-primary"
      }
      title={
        remove
          ? `${t("profiles.expiry.removeTitle")}${graceTitle}`
          : `${t("profiles.expiry.notifyTitle")}${graceTitle}`
      }
    >
      {remove ? <Trash2 className="size-3" aria-hidden /> : <Bell className="size-3" aria-hidden />}
      {remove ? t("profiles.expiry.remove") : t("profiles.expiry.notify")}
      {grace > 0 ? tf("profiles.expiry.graceShort", { n: grace }) : ""}
    </Badge>
  );
}

export default function ProfilesView() {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  // Pastille par index de carte — palette thématée nuit/jour, 5 teintes aurora.
  const PALETTE = useChartPalette().series;
  const queryClient = useQueryClient();

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<Profile | null>(null);
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
    setDialogOpen(true);
  }

  function openEdit(profile: Profile) {
    setEditing(profile);
    setDialogOpen(true);
  }

  function closeDialog(open: boolean) {
    setDialogOpen(open);
    if (!open) setEditing(null);
  }

  const deleteMutation = useMutation({
    mutationFn: (profile: Profile) => api<{ ok: boolean }>(`/api/profiles/${profile.id}`, { method: "DELETE" }),
    onSuccess: (_res, profile) => {
      toast.success(tf("profiles.deletedToast", { name: profile.name }));
      setDeleting(null);
      invalidateProfiles();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const list = profiles ?? [];

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("profiles.title")}
        description={t("profiles.description")}
        actions={
          <Button className="h-10" onClick={openCreate}>
            <Plus className="size-4" />
            {t("profiles.new")}
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
            title={t("profiles.empty")}
            description={t("profiles.emptyDesc")}
            action={
              <Button onClick={openCreate}>
                <Plus className="size-4" />
                {t("profiles.new")}
              </Button>
            }
          />
        </Card>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          <AnimatePresence initial={false}>
            {list.map((profile, index) => {
              const selling = profile.sellingPrice ?? 0;
              const hasSelling = selling > 0 && selling !== profile.price;
              return (
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
                          {profile.lockUser && (
                            <span
                              className="flex size-5 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground"
                              title={t("profiles.locked")}
                            >
                              <Lock className="size-3" aria-hidden />
                              <span className="sr-only">{t("profiles.locked")}</span>
                            </span>
                          )}
                          {profile.lockFirstDevice && (
                            <span
                              className="flex size-5 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary"
                              title={t("profiles.lockedDevice")}
                            >
                              <MonitorSmartphone className="size-3" aria-hidden />
                              <span className="sr-only">{t("profiles.lockedDevice")}</span>
                            </span>
                          )}
                        </div>
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-10 text-muted-foreground hover:text-foreground"
                              aria-label={tf("common.actionsFor", { name: profile.name })}
                            >
                              <MoreHorizontal className="size-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end" className="w-48">
                            <DropdownMenuItem className="min-h-10" onClick={() => openEdit(profile)}>
                              <Pencil className="size-4" />
                              {t("common.edit")}
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              variant="destructive"
                              className="min-h-10"
                              onClick={() => setDeleting(profile)}
                            >
                              <Trash2 className="size-4" />
                              {t("common.delete")}
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>

                      <div>
                        {/* Prix / Vente : coût du forfait + prix de vente affiché voucher (F13). */}
                        <p className="text-2xl font-semibold tracking-tight tabular-nums">
                          {formatCurrency(profile.price, currency, lang)}
                        </p>
                        <p className="text-xs text-muted-foreground">
                          {hasSelling
                            ? tf("profiles.saleAt", { price: formatCurrency(selling, currency, lang) })
                            : t("profiles.perPlan")}
                        </p>
                      </div>

                      <div className="mt-auto grid grid-cols-1 gap-x-4 gap-y-2 border-t pt-4 sm:grid-cols-2">
                        <div className="flex min-h-6 items-center gap-2 text-sm">
                          <ArrowDownUp className="size-4 shrink-0 text-muted-foreground" />
                          <span className="text-muted-foreground">{t("profiles.bandwidth")}</span>
                          <span className="ml-auto font-medium">{formatRateLimit(profile.rateLimit)}</span>
                        </div>
                        <div className="flex min-h-6 items-center gap-2 text-sm">
                          <Timer className="size-4 shrink-0 text-muted-foreground" />
                          <span className="text-muted-foreground">{t("profiles.session")}</span>
                          <span className="ml-auto font-medium">
                            {formatDuration(profile.sessionTimeoutMin * 60)}
                          </span>
                        </div>
                        <div className="flex min-h-6 items-center gap-2 text-sm">
                          <CalendarClock className="size-4 shrink-0 text-muted-foreground" />
                          <span className="text-muted-foreground">{t("profiles.validity")}</span>
                          <span className="ml-auto font-medium">
                            {fmtRouterDuration(
                              profile.validityMin > 0 ? profile.validityMin : profile.validityDays * 1440,
                            )}
                          </span>
                        </div>
                        <div className="flex min-h-6 items-center gap-2 text-sm">
                          <Smartphone className="size-4 shrink-0 text-muted-foreground" />
                          <span className="text-muted-foreground">
                            {profile.lockUser ? t("profiles.sessionsLabel") : t("profiles.devices")}
                          </span>
                          <span className="ml-auto font-medium">
                            {profile.lockUser
                              ? t("profiles.oneAtATime")
                              : tf("profiles.devicesCount", { n: profile.sharedUsers })}
                          </span>
                        </div>
                        <div className="flex min-h-6 items-center gap-2 text-sm sm:col-span-2 sm:max-w-max">
                          <Database className="size-4 shrink-0 text-muted-foreground" />
                          <span className="text-muted-foreground">{t("profiles.quota")}</span>
                          <span className="ml-auto font-medium">
                            {profile.dataQuotaMb === 0
                              ? t("profiles.unlimited")
                              : formatBytes(profile.dataQuotaMb * 1048576, lang)}
                          </span>
                        </div>
                        <div className="flex min-h-6 items-center gap-2 text-sm sm:col-span-2">
                          <span className="text-muted-foreground">{t("profiles.expiration")}</span>
                          <span className="ml-auto">
                            <ExpiryBadge profile={profile} />
                          </span>
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                </motion.div>
              );
            })}
          </AnimatePresence>
        </div>
      )}

      {/* Dialogue création / édition (parts/profile-dialog.tsx) — monté
          conditionnellement pour réinitialiser le formulaire à chaque ouverture */}
      {dialogOpen && <ProfileEditDialog open onOpenChange={closeDialog} profile={editing} />}

      {/* Confirmation suppression */}
      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{tf("profiles.deleteTitle", { name: deleting?.name ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>{t("profiles.deleteDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={deleteMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                if (deleting) deleteMutation.mutate(deleting);
              }}
            >
              {deleteMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
