"use client";

// N°100 — vue « Votre maison » : le tableau de bord de la CONSOLE HOMENET.
//
// Un foyer n'est pas un cybercafé : pas de revenus, pas de vouchers, pas de
// créances revendeurs. Les questions d'un parent sont « ma box est-elle en
// ligne ? », « qui est connecté ? », « la famille est-elle protégée ? » et
// « le couvre-feu veille-t-il ce soir ? » — cette vue y répond en un coup
// d'œil, sans jamais montrer un mot du vocabulaire hotspot.
//
// Données : les SEULS endpoints ouverts aux deux usages (vérité terrain
// routes.go — Phase 1 N°98) : GET /api/routers (statut, protections
// n/4, couvre-feu, board, check-in) et GET /api/sessions (appareils
// connectés). Zéro endpoint neuf, zéro diff backend — la coquille vit
// du pont de données partagé. Le calcul du verdict réutilise les MÊMES
// helpers purs que la vue Protection et le bandeau (protection.ts) :
// zéro dérive possible entre les trois surfaces.

import { useMemo } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  Clock,
  MoonStar,
  MonitorSmartphone,
  Router as RouterIcon,
  ShieldCheck,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingCards, LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { SubscriptionBanner } from "@/components/hotspot/parts/sa-subscription-banner";
import { ProtectionScoreRing, ProtectionVerdictBadge } from "@/components/hotspot/parts/protection-cards";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatDuration, timeAgo } from "@/lib/hotspot/format";
import {
  familyGuardActiveNow,
  parseFamilyGuardSpec,
  protectionScore,
  protectionVerdict,
} from "@/lib/hotspot/protection";
import { viewToPath } from "@/lib/hotspot/view-path";
import type { HomeDevice, RouterDevice } from "@/lib/hotspot/types";

/** Carte d'une box : identité, santé, anneau de protection, actions.
 * Le foyer a rarement plusieurs routeurs — mais le modèle SaaS est
 * multi-sites : la grille reste prête sans jamais exiger plus. */
function RouterCard({
  router,
  lang,
  devicesOnline,
  onOpenProtection,
  onOpenDevices,
}: {
  router: RouterDevice;
  lang: "fr" | "en";
  devicesOnline: number;
  onOpenProtection: (routerId: string) => void;
  onOpenDevices: () => void;
}) {
  const { t } = useI18n();
  const online = router.status === "online";
  const score = protectionScore(router);
  const verdict = protectionVerdict(router);
  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-4 sm:p-5">
        <div className="flex items-center justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <RouterIcon className="size-4" />
            </span>
            <span className="truncate font-medium" title={router.name}>
              {router.name}
            </span>
          </div>
          <StatusBadge status={online ? "online" : "offline"} dot />
        </div>

        <div className="mt-4 flex items-center gap-4 sm:gap-5">
          <ProtectionScoreRing score={score} verdict={verdict} />
          <div className="min-w-0 flex-1 space-y-2">
            <ProtectionVerdictBadge verdict={verdict} />
            <dl className="grid grid-cols-2 gap-x-4 gap-y-1.5 text-xs">
              <div>
                <dt className="text-muted-foreground">{t("home.router.model")}</dt>
                <dd className="truncate font-medium" title={router.boardName || undefined}>
                  {router.boardName || "—"}
                </dd>
              </div>
              <div>
                <dt className="text-muted-foreground">{t("home.router.uptime")}</dt>
                <dd className="font-medium tabular-nums">{formatDuration(router.uptimeSec)}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">{t("home.router.devices")}</dt>
                <dd className="font-semibold tabular-nums">
                  {devicesOnline}
                  {router.status === "online" && <span className="live-dot ml-1.5 inline-block size-1.5 rounded-full bg-primary align-middle" aria-hidden />}
                </dd>
              </div>
              <div>
                <dt className="text-muted-foreground">{t("home.router.lastSeen")}</dt>
                <dd className="font-medium">{router.lastSeen ? timeAgo(router.lastSeen, lang) : "—"}</dd>
              </div>
            </dl>
          </div>
        </div>

        <div className="mt-4 flex flex-wrap items-center gap-2 border-t pt-3">
          <Button
            size="sm"
            variant="outline"
            className="h-9"
            onClick={() => onOpenProtection(router.id)}
          >
            <ShieldCheck className="size-3.5" />
            {t("home.router.protectionCta")}
          </Button>
          <Button size="sm" variant="ghost" className="h-9" onClick={() => onOpenDevices()}>
            <MonitorSmartphone className="size-3.5" />
            {t("home.router.devicesCta")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export default function HomeView() {
  const { t, tf, lang } = useI18n();
  const router = useRouter();
  // Pattern Phase D (routers-view) : push vers les chemins adressables —
  // la synchro URL↔store d'app-route fait le reste.
  const openProtection = (routerId: string) => router.push(viewToPath("protection", routerId), { scroll: false });
  const openDevices = () => router.push(viewToPath("devices"), { scroll: false });

  const { data: routers, isLoading, isError, refetch } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
    refetchInterval: 15_000,
  });

  // N°101 — le registre des appareils du foyer (bails DHCP rapportés par la
  // box) : la même source de vérité que la vue Appareils, et la fin du
  // pis-aller « sessions hotspot » de la Phase 2 (un foyer sans portail
  // captif ne produit PAS de sessions — ses appareils vivent dans ses baux).
  const { data: devices } = useQuery({
    queryKey: ["/api/devices"],
    queryFn: () => api<HomeDevice[]>("/api/devices"),
    refetchInterval: 10_000,
  });

  const list = useMemo(() => routers ?? [], [routers]);
  const onlineCount = list.filter((r) => r.status === "online").length;
  const devicesOnline = useMemo(() => (devices ?? []).filter((d) => d.status === "bound").length, [devices]);
  // N°102 — la vérité terrain de l'inventaire : le registre ne vit QUE des
  // bails DHCP rapportés par une box connectée en mode agent (cadenceur dédié
  // N°101). Sans elle, un « 0 » nu serait un faux zéro — le KPI dit pourquoi.
  const hasAgentRouter = useMemo(() => list.some((r) => r.mode === "agent"), [list]);
  const devicesEmpty = (devices ?? []).length === 0;
  const devicesSub =
    devicesEmpty && list.length > 0
      ? hasAgentRouter
        ? t("home.kpi.devicesWaiting") // box agent là, premier rapport en route
        : t("home.kpi.devicesAgent") // box en mode non-agent : l'enseigne dit tout
      : t("home.kpi.devicesSub");
  const devicesByRouter = useMemo(() => {
    const map = new Map<string, number>();
    for (const d of devices ?? []) {
      if (d.status === "bound") map.set(d.routerId, (map.get(d.routerId) ?? 0) + 1);
    }
    return map;
  }, [devices]);

  // Protection KPI : le maillon le plus faible du foyer (un réseau se juge
  // par sa porte la plus ouverte — même calcul que le bandeau du dashboard
  // hotspot, qui affiche LUI AUSSI le verdict global du parc).
  const weakest = list.reduce<RouterDevice | null>(
    (worst, r) => (!worst || protectionScore(r) < protectionScore(worst) ? r : worst),
    null,
  );
  const weakestScore = weakest ? protectionScore(weakest) : 0;
  const weakestVerdict = weakest ? protectionVerdict(weakest) : "unprotected";

  // Couvre-feu : la fenêtre FamilyGuard du foyer (spéc. canonique partagée
  // avec la fiche routeur — même parse, même UTC qu'Abidjan côté cloud).
  const curfew = list.reduce((best, r) => {
    const w = parseFamilyGuardSpec(r.familyGuardSpec);
    return w.enabled && !best.enabled ? w : best;
  }, parseFamilyGuardSpec(undefined));
  const curfewActive = familyGuardActiveNow(curfew, new Date());

  if (isLoading) {
    return (
      <div className="space-y-6">
        <PageHeader title={t("home.title")} description={t("home.description")} />
        <LoadingCards cards={4} />
        <Card>
          <LoadingRows rows={4} />
        </Card>
      </div>
    );
  }

  if (isError) {
    return (
      <Card>
        <EmptyState
          icon={Activity}
          title={t("home.loadError")}
          description={t("home.loadErrorDesc")}
          action={
            <Button variant="outline" onClick={() => void refetch()}>
              <Clock className="size-4" />
              {t("common.retry")}
            </Button>
          }
        />
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader title={t("home.title")} description={t("home.description")} />

      {/* Bannière abonnement (partagée) : la facturation du foyer reste
          visible — même SaaS, même honnêteté qu'en console hotspot. */}
      <SubscriptionBanner />

      {/* KPIs — les quatre questions du foyer */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          title={t("home.kpi.router")}
          value={`${onlineCount}/${list.length}`}
          sub={t("home.kpi.routerSub")}
          icon={RouterIcon}
        />
        {/* N°102 — le KPI Appareils devient la porte de la vue Appareils :
            c'est LE raccourci de la pause dîner (un parent pressé clique le
            compteur, choisit l'appareil, coupe). Bouton sémantique autour de
            la carte — la carte reste un div neutre, zéro bouton imbriqué. */}
        <button
          type="button"
          onClick={() => router.push(viewToPath("devices"), { scroll: false })}
          className="block cursor-pointer rounded-xl text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
        >
          <StatCard
            title={t("home.kpi.devices")}
            value={String(devicesOnline)}
            sub={devicesSub}
            icon={MonitorSmartphone}
            live={hasAgentRouter}
            className="w-full text-left"
          />
        </button>
        <StatCard
          title={t("home.kpi.protection")}
          value={weakest ? `${weakestScore}/4` : "—"}
          sub={weakest ? t(`protection.verdict.${weakestVerdict}`) : t("home.kpi.protectionNone")}
          icon={ShieldCheck}
        />
        <StatCard
          title={t("home.kpi.curfew")}
          value={curfew.enabled ? t(curfewActive ? "home.curfew.active" : "home.curfew.scheduled") : t("home.curfew.off")}
          sub={curfew.enabled ? `${curfew.start} → ${curfew.end}` : t("home.curfew.offSub")}
          icon={MoonStar}
        />
      </div>

      {/* La box du foyer — ou l'invitation à la connecter */}
      {list.length === 0 ? (
        <Card>
          <EmptyState
            icon={RouterIcon}
            title={t("home.empty.title")}
            description={t("home.empty.desc")}
            action={
              <Button onClick={() => router.push(viewToPath("routers"), { scroll: false })}>
                <RouterIcon className="size-4" />
                {t("home.empty.cta")}
              </Button>
            }
          />
        </Card>
      ) : (
        <section aria-label={t("home.routersSection")} className="space-y-3">
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <h2 className="text-sm font-semibold tracking-wide text-muted-foreground uppercase">
              {t("home.routersSection")}
            </h2>
            <p className="text-xs text-muted-foreground">
              {tf("home.routersCount", { n: list.length, p: list.length > 1 ? "s" : "" })}
            </p>
          </div>
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            {list.map((r) => (
              <RouterCard
                key={r.id}
                router={r}
                lang={lang}
                devicesOnline={devicesByRouter.get(r.id) ?? 0}
                onOpenProtection={openProtection}
                onOpenDevices={openDevices}
              />
            ))}
          </div>
          {/* N°102 — enseigne honnête : des routeurs existent MAIS aucun en
              mode agent → l'inventaire (et la pause dîner) ne peut pas vivre.
              Le foyer sait quoi faire, le faux zéro disparaît. */}
          {!hasAgentRouter && (
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3">
              <p className="flex min-w-0 items-start gap-2 text-sm text-amber-800 dark:text-amber-200">
                <MonitorSmartphone className="mt-0.5 size-4 shrink-0" aria-hidden />
                <span>{t("home.agentHint")}</span>
              </p>
              <Button
                size="sm"
                variant="outline"
                className="h-9 border-amber-500/40 text-amber-800 hover:bg-amber-500/10 dark:text-amber-200"
                onClick={() => router.push(viewToPath("routers"), { scroll: false })}
              >
                {t("home.agentHintCta")}
              </Button>
            </div>
          )}
        </section>
      )}
    </div>
  );
}
