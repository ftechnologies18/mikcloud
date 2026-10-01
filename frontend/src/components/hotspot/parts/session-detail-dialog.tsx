"use client";

// N°197 — Dialog « détails de connexion » (vue Sessions) : le bouton d'action
// d'une ligne ouvre la carte du voucher ou de l'utilisateur régulier connecté.
// Le serveur agrège tout en UN appel (GET /api/users/{id|username}/connection-
// detail — handlers_user_detail.go) : dernière connexion, total des données
// consommées par le ticket/l'utilisateur (miroir cloud du limit-bytes-total),
// adresse MAC de l'appareil et marque PROBABLE (préfixe OUI IEEE), journal
// récent (F3) et traçabilité de vente quand le ticket vient d'un revendeur.
//
// L'identité de la carte vient de la LIGNE cliquée (session.username — le
// routeur le sert déjà via /api/sessions) ; l'agrégat peut être ouvert pour un
// utilisateur HORS registre cloud (créé dans Winbox : user=null) — la carte
// montre alors connexion et journal, sans section ticket, et le dit.

import { useQuery } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowUp,
  CalendarClock,
  Clock,
  Fingerprint,
  Gauge,
  History,
  Radio,
  Router,
  Smartphone,
  Timer,
  UserRound,
  Wallet,
} from "lucide-react";

import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatBytes, formatCurrency, formatDateTime, formatDuration, formatMb, timeAgo, userInitials } from "@/lib/hotspot/format";
// Sémantique trafic verrouillée : bytesIn=upload / bytesOut=download (RouterOS).
import { downBytes, upBytes } from "@/lib/hotspot/traffic-semantics";
import type { HotspotSession, UserConnectionDetail, UserLogAction } from "@/lib/hotspot/types";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { StatusBadge } from "@/components/hotspot/status-badge";

/** Libellés + couleurs des actions du journal (miroir logs-view). */
const LOG_ACTIONS: Record<UserLogAction, { labelKey: string; className: string }> = {
  login: { labelKey: "logs.login", className: "border-primary/25 bg-primary/10 text-primary" },
  logout: { labelKey: "logs.logout", className: "border-border bg-muted text-muted-foreground" },
  expire: { labelKey: "logs.expire", className: "border-orange-500/25 bg-orange-500/10 text-orange-500" },
  kick: { labelKey: "logs.kick", className: "border-destructive/25 bg-destructive/10 text-destructive" },
};

function InfoRow({
  icon: Icon,
  label,
  value,
  mono,
  title,
}: {
  icon: typeof Radio;
  label: string;
  value: string;
  mono?: boolean;
  title?: string;
}) {
  return (
    <div className="flex items-center gap-3 rounded-md px-2 py-2">
      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <Icon className="size-4" aria-hidden />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className={`truncate text-sm font-medium ${mono ? "font-mono text-[13px]" : ""}`} title={title ?? value}>
          {value || "—"}
        </p>
      </div>
    </div>
  );
}

/** Intitulé de section de la carte. */
function SectionTitle({ children }: { children: string }) {
  return <p className="px-2 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">{children}</p>;
}

export function SessionDetailDialog({
  session,
  onOpenChange,
}: {
  /** La ligne de session qui a ouvert la carte (null = fermée). */
  session: HotspotSession | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, lang } = useI18n();
  // {id} = ID du user du registre cloud, sinon le username (Winbox-only) ;
  // routerId = hint anti-homonymes (un même nom peut vivre sur 2 routeurs).
  const key = session ? session.userId || session.username : "";
  const path = session ? `/api/users/${encodeURIComponent(key)}/connection-detail?routerId=${encodeURIComponent(session.routerId)}` : "";
  const { data, isFetching, error, refetch } = useQuery<UserConnectionDetail>({
    queryKey: [path],
    queryFn: () => api<UserConnectionDetail>(path),
    enabled: !!session,
  });

  const user = data?.user ?? null;
  const live = data?.liveSessions ?? [];
  const current = live[0] ?? null;
  const isTicket = user?.kind === "voucher";
  // Marque de l'appareil : la meilleure MAC (session > dernier log).
  const brand = data?.deviceBrand ?? "";
  const mac = data?.mac ?? "";
  const quotaBytes = user && user.dataQuotaMb > 0 ? user.dataQuotaMb * 1024 * 1024 : 0;
  const usedTotal = user ? downBytes(user) + upBytes(user) : 0;

  return (
    <Dialog open={!!session} onOpenChange={onOpenChange}>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-md">
        {/* En-tête : identité depuis la ligne cliquée (le routeur sert déjà ce
            username via /api/sessions) + statut résolu de l'agrégat. */}
        <div className="relative flex items-center gap-4 px-6 pb-5 pt-6">
          <div className="absolute inset-x-0 top-0 h-20 bg-gradient-to-b from-primary/15 to-transparent" aria-hidden />
          <Avatar className="relative size-14 border-2 border-background shadow-lg shadow-primary/20">
            <AvatarFallback className="bg-primary/15 text-base font-semibold text-primary">
              {userInitials(session?.username ?? "")}
            </AvatarFallback>
          </Avatar>
          <div className="relative min-w-0">
            <DialogTitle className="truncate font-mono text-base leading-tight">{session?.username}</DialogTitle>
            <DialogDescription className="mt-1 flex flex-wrap items-center gap-1.5">
              <span>{user ? (isTicket ? t("sessions.detailKindTicket") : t("sessions.detailKindRegular")) : "—"}</span>
              {user && <StatusBadge status={user.status} />}
              {current?.throttled && (
                <Badge variant="outline" className="gap-1 border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400">
                  <Gauge className="size-3" aria-hidden />
                  {t("sessions.throttled")}
                </Badge>
              )}
            </DialogDescription>
          </div>
        </div>

        <Separator />

        {error ? (
          <div className="flex flex-col items-center gap-3 px-6 py-10 text-center">
            <p className="text-sm text-muted-foreground">{t("sessions.detailLoadError")}</p>
            <Button variant="outline" size="sm" className="h-9" onClick={() => refetch()}>
              {t("common.retry")}
            </Button>
          </div>
        ) : !data ? (
          <div className="space-y-3 px-4 py-5">
            <Skeleton className="h-4 w-32" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-10 w-full" />
          </div>
        ) : (
          <div className="max-h-[70vh] overflow-y-auto">
            {/* — Connexion en cours — */}
            <div className="space-y-1 px-3 py-4">
              <SectionTitle>{t("sessions.detailCurrent")}</SectionTitle>
              {current ? (
                <>
                  <InfoRow icon={Router} label={t("common.router")} value={current.routerName} />
                  <InfoRow icon={Radio} label={t("common.ip")} value={current.ip} mono />
                  <InfoRow
                    icon={Fingerprint}
                    label={t("common.mac")}
                    value={mac || t("sessions.detailMacUnknown")}
                    mono={!!mac}
                  />
                  <InfoRow icon={Smartphone} label={t("sessions.detailDevice")} value={brand || t("sessions.detailDeviceUnknown")} />
                  <InfoRow icon={Clock} label={t("sessions.connectedSince")} value={formatDuration(current.uptimeSec)} />
                  <InfoRow icon={CalendarClock} label={t("sessions.detailLastLogin")} value={current.startedAt ? `${formatDateTime(current.startedAt, lang)}` : ""} />
                  <div className="flex items-center gap-3 rounded-md px-2 py-2">
                    <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                      <ArrowDown className="size-4" aria-hidden />
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="text-xs text-muted-foreground">{t("sessions.detailSessionTraffic")}</p>
                      <p className="flex items-center gap-3 text-sm font-medium tabular-nums">
                        <span className="inline-flex items-center gap-1">
                          <ArrowDown className="size-3 opacity-60" aria-hidden />
                          {formatBytes(downBytes(current), lang)}
                        </span>
                        <span className="inline-flex items-center gap-1">
                          <ArrowUp className="size-3 opacity-60" aria-hidden />
                          {formatBytes(upBytes(current), lang)}
                        </span>
                      </p>
                    </div>
                  </div>
                </>
              ) : (
                <p className="px-2 pb-1 pt-2 text-sm text-muted-foreground">{t("sessions.detailNoLive")}</p>
              )}
            </div>

            <Separator />

            {/* — Consommation du ticket / de l'utilisateur — */}
            <div className="space-y-1 px-3 py-4">
              <SectionTitle>{t("sessions.detailConsumption")}</SectionTitle>
              {user ? (
                <>
                  <InfoRow
                    icon={ArrowDown}
                    label={t("sessions.detailTotalData")}
                    value={`${formatBytes(downBytes(user), lang)} ↓ · ${formatBytes(upBytes(user), lang)} ↑ (${formatBytes(usedTotal, lang)})`}
                  />
                  <InfoRow
                    icon={Gauge}
                    label={t("sessions.detailQuota")}
                    value={user.dataQuotaMb > 0 ? formatMb(user.dataQuotaMb) : t("sessions.detailUnlimited")}
                  />
                  {quotaBytes > 0 && (
                    <InfoRow
                      icon={Gauge}
                      label={t("sessions.detailQuotaLeft")}
                      value={formatBytes(Math.max(0, quotaBytes - usedTotal), lang)}
                    />
                  )}
                  <InfoRow icon={Timer} label={t("sessions.detailTimeUsed")} value={formatDuration(user.uptimeUsedSec)} />
                  {user.timeLimitMin > 0 && (
                    <InfoRow
                      icon={Timer}
                      label={t("sessions.detailTimeLimit")}
                      value={formatDuration(user.timeLimitMin * 60)}
                    />
                  )}
                  {user.expiresAt && (
                    <InfoRow icon={CalendarClock} label={t("sessions.detailExpiresAt")} value={formatDateTime(user.expiresAt, lang)} />
                  )}
                </>
              ) : (
                <p className="px-2 pb-1 pt-2 text-sm text-muted-foreground">{t("sessions.detailOffRegistry")}</p>
              )}
            </div>

            {(user?.resellerName || user?.soldAt) && (
              <>
                <Separator />
                {/* — Vente : traçabilité du ticket (N°8/N°19) — */}
                <div className="space-y-1 px-3 py-4">
                  <SectionTitle>{t("sessions.detailSale")}</SectionTitle>
                  <InfoRow icon={UserRound} label={t("sessions.detailSeller")} value={user?.resellerName ?? ""} />
                  {user?.soldAt && (
                    <InfoRow icon={CalendarClock} label={t("sessions.detailSoldAt")} value={formatDateTime(user.soldAt, lang)} />
                  )}
                  <InfoRow
                    icon={Wallet}
                    label={t("sessions.detailSoldVia")}
                    value={
                      user?.soldVia === "sell_mode"
                        ? t("sessions.detailSoldViaSellMode")
                        : user?.soldVia === "auto_connect"
                          ? t("sessions.detailSoldViaAuto")
                          : t("sessions.detailDirectSale")
                    }
                  />
                  <InfoRow icon={Wallet} label={t("common.price")} value={user ? formatCurrency(user.sellingPrice || user.price, "FCFA", lang) : ""} />
                </div>
              </>
            )}

            <Separator />

            {/* — Historique (F3) : dernière connexion, compteur 30 j, derniers
                événements — la liste défile (max-h) sans faire grandir la carte. */}
            <div className="space-y-1 px-3 py-4">
              <SectionTitle>{t("sessions.detailHistory")}</SectionTitle>
              <InfoRow
                icon={CalendarClock}
                label={t("sessions.detailLastLogin")}
                value={data.lastLoginAt ? `${formatDateTime(data.lastLoginAt, lang)} · ${timeAgo(data.lastLoginAt, lang)}` : "—"}
              />
              <InfoRow icon={History} label={t("sessions.detailLogins30d")} value={String(data.loginCount30d)} />
              {user?.usedAt && (
                <InfoRow icon={CalendarClock} label={t("sessions.detailFirstUse")} value={formatDateTime(user.usedAt, lang)} />
              )}
              {data.recentLogs.length > 0 ? (
                <ul className="mt-1 max-h-64 space-y-1 overflow-y-auto px-2">
                  {data.recentLogs.map((log) => {
                    const conf = LOG_ACTIONS[log.action] ?? LOG_ACTIONS.login;
                    return (
                      <li key={log.id} className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm">
                        <Badge variant="outline" className={`shrink-0 px-1.5 py-0 text-[11px] ${conf.className}`}>
                          {t(conf.labelKey)}
                        </Badge>
                        <span className="min-w-0 flex-1 truncate text-muted-foreground" title={`${log.routerName}${log.ip ? ` · ${log.ip}` : ""}`}>
                          {log.routerName}
                          {log.ip ? <span className="font-mono"> · {log.ip}</span> : null}
                        </span>
                        <span className="shrink-0 text-xs text-muted-foreground" title={formatDateTime(log.at, lang)}>
                          {timeAgo(log.at, lang)}
                        </span>
                      </li>
                    );
                  })}
                </ul>
              ) : (
                <p className="px-2 pb-1 pt-2 text-sm text-muted-foreground">{t("sessions.detailNoHistory")}</p>
              )}
            </div>
          </div>
        )}

        <Separator />

        {/* Pied : marque + fraîcheur de l'agrégat */}
        <div className="flex items-center justify-between px-5 py-3">
          <p className="text-xs text-muted-foreground">MikCloud</p>
          {isFetching ? <Skeleton className="h-3.5 w-28" /> : <p className="text-[11px] text-muted-foreground/70">{t("profile.upToDate")}</p>}
        </div>
      </DialogContent>
    </Dialog>
  );
}
