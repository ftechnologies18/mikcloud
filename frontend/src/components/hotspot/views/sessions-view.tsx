"use client";

// Vue Sessions actives — temps réel (poll auto-refresh, durées qui avancent, kick).
//
// N°193 — pagination côté client : la vue vivait SANS bornes (filtre local
// complet, Phase D) — sur un parc chargé (des centaines de connectés), le
// tableau devenait un défilement sans fin sur mobile. Le sélecteur « N / page »
// arrive avec : la page affichée est une FENÊTRE sur filteredSessions ; les
// KPI et les badges du rail restent calculés sur l'ENSEMBLE scopé (le compte
// « connectés » ne devient jamais le compte de la page courante).

import { useEffect, useMemo, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { AnimatePresence, motion } from "framer-motion";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowDownCircle,
  ArrowUp,
  ArrowUpCircle,
  ChevronLeft,
  ChevronRight,
  LogOut,
  Radio,
  Search,
  WifiOff,
  Gauge,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { STALE_TIME } from "@/lib/hotspot/query";
import { useHotspotStore } from "@/lib/hotspot/store";
import { detailFromPath, viewToPath } from "@/lib/hotspot/view-path";
import type { HotspotSession, RouterDevice } from "@/lib/hotspot/types";
import { formatBytes, formatDuration } from "@/lib/hotspot/format";
// Sémantique trafic verrouillée : bytesIn=upload / bytesOut=download (RouterOS).
import { downBytes, upBytes } from "@/lib/hotspot/traffic-semantics";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { RouterScopeRail } from "@/components/hotspot/parts/router-scope";
import { PageSizeSelect, usePageSize } from "@/components/hotspot/parts/page-size-select";
import { StatCard } from "@/components/hotspot/stat-card";
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
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

const REFRESH_OPTIONS = [
  { value: "5000", label: "5 s" },
  { value: "10000", label: "10 s" },
  { value: "30000", label: "30 s" },
];

// N°191 — miroir module de la saisie de recherche : le catch-all
// /app/[[...vue]] REMONTE à chaque changement de params (vérifié au
// navigateur — MÊME à nombre de segments constant, contrairement à la note
// N°190 qui ne documentait que les changements de NOMBRE) : basculer la
// loupe routeur réinitialiserait la saisie à chaque chip. Piège de
// séquencement (découvert en vérification navigateur) : l'initialisateur
// du NOUVEL arbre tourne AVANT le cleanup de l'ancien — une sauvegarde
// au démontage arrive TROP TARD. Le miroir est donc écrit EN CONTINU
// (effet sur la saisie) et vidé au démontage UNIQUEMENT si la vue a
// vraiment changé (remontage loupe → conserver ; vraie sortie → repartir
// propre, comportement inchangé pour les navigations normales).
let sessionsKeptSearch: string | null = null;

export default function SessionsView() {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const nav = useRouter();
  const [refreshMs, setRefreshMs] = useState(5000);
  const [now, setNow] = useState(() => Date.now());
  const [kickTarget, setKickTarget] = useState<HotspotSession | null>(null);

  // N°193 — pagination côté client (fenêtre sur filteredSessions) :
  // densité mémorisée par vue, comme toutes les tables de la console.
  const [pageSize, setPageSize] = usePageSize("sessions");
  const [page, setPage] = useState(1);

  // N°191 — parc routeurs pour la loupe (état du parc : bouge aux check-ins
  // agents ~45 s ; points de statut des chips).
  const routersQuery = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
    staleTime: STALE_TIME.operational,
  });
  const routers = routersQuery.data;

  const { data, dataUpdatedAt, isLoading } = useQuery({
    queryKey: ["/api/sessions"],
    queryFn: () => api<HotspotSession[]>("/api/sessions"),
    refetchInterval: refreshMs,
  });

  // Phase D — filtre local par utilisateur (le tableau n'est PAS paginé :
  // filtre client complet) + deep-link /app/sessions/<username> : le
  // segment EST le filtre tant que l'opérateur n'a pas tapé lui-même
  // (état DÉRIVÉ de l'URL — aucune synchronisation effet→état). Sortie du
  // détail sans saisie → le filtre retombe naturellement ; avec saisie →
  // la saisie est conservée (le segment ne marque que le point d'entrée).
  //
  // N°191 — le segment porte désormais DEUX lectures : le username
  // (deep-link Phase D) OU la loupe routeur « router:<id> » (préfixe
  // réservé — les usernames générés ne commencent jamais par « router: » ;
  // même convention que la clé canonique de l'éditeur Portail, N°190). La
  // loupe VIT dans l'URL (pattern Protection) : rafraîchissement, partage
  // et signet retombent sur la portée ; chaque changement de chip la
  // remplace (applyScope — replace, un réglage pas une navigation).
  const [typedQuery, setTypedQuery] = useState<string | null>(() => sessionsKeptSearch);
  const pathname = usePathname();
  const detailSegment = detailFromPath(pathname, "sessions");
  const scopeRouterId = detailSegment?.startsWith("router:")
    ? detailSegment.slice("router:".length)
    : null;
  const detailUsername = detailSegment && !detailSegment.startsWith("router:") ? detailSegment : null;
  const query = typedQuery ?? detailUsername ?? "";

  // N°191 — miroir écrit EN CONTINU (l'initialisateur du nouvel arbre court
  // avant le cleanup de l'ancien : toute sauvegarde différée arrive trop
  // tard, découvert en vérification navigateur).
  useEffect(() => {
    sessionsKeptSearch = typedQuery;
  }, [typedQuery]);
  useEffect(
    () => () => {
      // Démontage : vue TOUJOURS sur sessions → remontage du catch-all
      // (loupe) : le miroir continu fait son œuvre. Sinon → vraie sortie
      // de vue : vider (la prochaine entrée repart propre).
      if (useHotspotStore.getState().view !== "sessions") {
        sessionsKeptSearch = null;
      }
    },
    [],
  );

  function applyScope(routerId: string) {
    nav.replace(viewToPath("sessions", routerId ? `router:${routerId}` : undefined), { scroll: false });
    // N°193 — nouvelle portée → retour à la première page (miroir des
    // filtres des autres vues : un filtre qui rétrécit ne doit pas laisser
    // une page hors bornes).
    setPage(1);
  }

  // N°191 — segment orphelin (routeur supprimé, signet périmé) :
  // re-normalisation vers la racine de la vue — replace, zéro entrée
  // d'historique parasite (miroir Portail N°190). Attends le parc : un
  // routeur pas encore chargé n'est PAS orphelin.
  useEffect(() => {
    if (!scopeRouterId || routersQuery.isLoading) return;
    if (!routers?.some((r) => r.id === scopeRouterId)) {
      nav.replace(viewToPath("sessions"), { scroll: false });
    }
  }, [scopeRouterId, routersQuery.isLoading, routers, nav]);

  // Horloge locale (1 s) : fait visuellement avancer les durées entre deux polls.
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  const sessions = useMemo(
    () => [...(data ?? [])].sort((a, b) => (a.startedAt < b.startedAt ? 1 : -1)),
    [data],
  );

  // N°191 — la loupe scope TOUTE la page : KPI, table et empty state (le
  // filtre de recherche reste un filtre de table PAR-DESSUS, Phase D
  // inchangée). Les comptes des chips restent GLOBAUX (la loupe se choisit
  // précisément en voyant tout le parc d'un coup d'œil).
  const scopedSessions = useMemo(
    () => (scopeRouterId ? sessions.filter((s) => s.routerId === scopeRouterId) : sessions),
    [sessions, scopeRouterId],
  );

  // Phase D — liste affichée = filtre local (portée routeur × recherche).
  const filteredSessions = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return scopedSessions;
    return scopedSessions.filter((s) => s.username.toLowerCase().includes(q));
  }, [scopedSessions, query]);

  // N°193 — fenêtre paginée : bornes recalculées à chaque rendu (le poll
  // vivant fait bouger total ET contenu ; safePage recadre une page devenue
  // hors bornes — kick, expiration, sortie — sans état coincé).
  const maxPage = Math.max(1, Math.ceil(filteredSessions.length / pageSize));
  const safePage = Math.min(page, maxPage);
  const rangeStart = filteredSessions.length === 0 ? 0 : (safePage - 1) * pageSize + 1;
  const rangeEnd = Math.min(safePage * pageSize, filteredSessions.length);
  const pagedSessions = useMemo(
    () => filteredSessions.slice((safePage - 1) * pageSize, safePage * pageSize),
    [filteredSessions, safePage, pageSize],
  );

  // N°191 — badges du rail : sessions vivantes par routeur (glissées sur le
  // poll, zéro requête supplémentaire). Chaque routeur du parc porte un
  // badge, MÊME à 0 (l'opérateur distingue « vide » de « inconnu ») ; un
  // routeur supprimé du parc mais portant des sessions résiduelles reste
  // visible dans le total « tous » uniquement.
  const countsByRouter = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const r of routers ?? []) counts[r.id] = 0;
    for (const s of sessions) counts[s.routerId] = (counts[s.routerId] ?? 0) + 1;
    return counts;
  }, [routers, sessions]);

  // Routeur de la loupe (nom pour les libellés) — le parc peut être en
  // chargement : les KPI scopés affichent alors le sous-texte générique.
  const scopedRouter = routers?.find((r) => r.id === scopeRouterId) ?? null;

  const elapsedSec = Math.max(0, Math.floor((now - dataUpdatedAt) / 1000));
  // N°191 — KPI au niveau de la loupe (globaux quand elle est ouverte sur
  // « tous », par routeur sinon).
  const totalIn = scopedSessions.reduce((acc, s) => acc + s.bytesIn, 0);
  const totalOut = scopedSessions.reduce((acc, s) => acc + s.bytesOut, 0);
  // Sémantique RouterOS verrouillée : bytesOut = download (descendant),
  // bytesIn = upload (montant) — voir traffic-semantics.ts (doc MikroTik).
  const totalDown = totalOut;
  const totalUp = totalIn;

  const kickMutation = useMutation({
    // Phase D (UI optimiste) — la session quitte la liste DÈS le clic (le
    // poll 5 s + réseau rendaient l'aller-retour DELETE visible) ; rollback
    // si l'API refuse. Les KPI suivent automatiquement (sessions.length).
    mutationFn: (id: string) => api<{ ok: boolean }>(`/api/sessions/${id}`, { method: "DELETE" }),
    onMutate: async (id) => {
      await queryClient.cancelQueries({ queryKey: ["/api/sessions"] });
      const snapshot = queryClient.getQueryData<HotspotSession[]>(["/api/sessions"]);
      queryClient.setQueryData<HotspotSession[]>(["/api/sessions"], (old) =>
        (old ?? []).filter((s) => s.id !== id),
      );
      return { snapshot };
    },
    onSuccess: (_, id) => {
      const target = sessions.find((s) => s.id === id);
      toast.success(
        tf("sessions.kicked", { name: target?.username ?? t("sessions.theUser") }),
      );
      setKickTarget(null);
      queryClient.invalidateQueries({ queryKey: ["/api/sessions"] });
    },
    onError: (error: Error, _id, ctx) => {
      if (ctx?.snapshot) queryClient.setQueryData(["/api/sessions"], ctx.snapshot);
      toast.error(error.message);
    },
  });

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("sessions.title")}
        description={t("sessions.description")}
        actions={
          <>
            <Badge variant="outline" className="gap-2 border-primary/25 bg-primary/10 py-1 text-primary">
              <span className="live-dot size-2 rounded-full bg-primary" aria-hidden />
              {t("sessions.live")}
            </Badge>
            <div className="relative">
              <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
              <Input
                value={query}
                onChange={(event) => {
                  setTypedQuery(event.target.value);
                  // N°193 — la recherche rétrécit la liste : retour page 1
                  // (même règle que les filtres des vues paginées serveur).
                  setPage(1);
                }}
                placeholder={t("sessions.searchPlaceholder")}
                className="h-10 w-40 pl-9 sm:w-56"
                aria-label={t("sessions.searchPlaceholder")}
              />
            </div>
            <div className="flex items-center gap-2">
              <span className="hidden text-xs text-muted-foreground sm:inline">{t("sessions.refresh")}</span>
              <Select value={String(refreshMs)} onValueChange={(value) => setRefreshMs(Number(value))}>
                <SelectTrigger size="sm" className="h-10 w-24" aria-label={t("sessions.refreshLabel")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {REFRESH_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </>
        }
      />

      {/* N°191 — loupe routeur : porte TOUTE la vue ci-dessous (KPI + table).
          Masquée sous 2 routeurs ; badges = sessions vivantes par point d'accès. */}
      <RouterScopeRail
        routers={routers}
        value={scopeRouterId ?? ""}
        onChange={applyScope}
        counts={countsByRouter}
        total={sessions.length}
        countTitle={(name, count) =>
          name === null
            ? tf("sessions.scopeAllTitle", { count })
            : tf("sessions.scopeCountTitle", { name, count })
        }
      />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <StatCard
          title={t("sessions.kpi.sessions")}
          value={String(scopedSessions.length)}
          sub={
            scopedRouter
              ? tf("sessions.kpi.sessionsSubScoped", { name: scopedRouter.name })
              : t("sessions.kpi.sessionsSub")
          }
          icon={Radio}
          live
        />
        <StatCard
          title={t("sessions.kpi.download")}
          value={formatBytes(totalDown, lang)}
          sub={t("sessions.kpi.downloadSub")}
          icon={ArrowDownCircle}
        />
        <StatCard title={t("sessions.kpi.upload")} value={formatBytes(totalUp, lang)} sub={t("sessions.kpi.uploadSub")} icon={ArrowUpCircle} />
      </div>

      <Card className="gap-0 py-0">
        {isLoading ? (
          <LoadingRows rows={8} />
        ) : filteredSessions.length === 0 ? (
          <EmptyState
            icon={WifiOff}
            title={t("sessions.empty")}
            description={
              scopedRouter
                ? tf("sessions.emptyScopedDesc", { name: scopedRouter.name })
                : t("sessions.emptyDesc")
            }
          />
        ) : (
          <>
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("common.user")}</TableHead>
                  <TableHead className="text-muted-foreground">{t("common.profile")}</TableHead>
                  <TableHead className="text-muted-foreground">{t("common.ip")}</TableHead>
                  <TableHead className="hidden text-muted-foreground md:table-cell">{t("common.mac")}</TableHead>
                  <TableHead className="hidden text-muted-foreground xl:table-cell">{t("common.router")}</TableHead>
                  <TableHead className="text-muted-foreground">{t("sessions.connectedSince")}</TableHead>
                  <TableHead className="text-muted-foreground">↓ / ↑</TableHead>
                  <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("common.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <AnimatePresence initial={false}>
                  {pagedSessions.map((session) => (
                    <motion.tr
                      key={session.id}
                      layout
                      initial={{ opacity: 0, y: -8 }}
                      animate={{ opacity: 1, y: 0 }}
                      exit={{ opacity: 0, y: -8 }}
                      transition={{ duration: 0.25, ease: "easeOut" }}
                      className="border-b transition-colors hover:bg-muted/50"
                    >
                      <TableCell className="pl-4 font-mono text-sm font-medium sm:pl-6">
                        {session.username}
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-wrap items-center gap-1.5">
                          <Badge variant="outline">{session.profileName}</Badge>
                          {session.throttled && (
                            <Badge
                              variant="outline"
                              className="gap-1 border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400"
                              title={t("sessions.throttledTitle")}
                            >
                              <Gauge className="size-3" aria-hidden />
                              {t("sessions.throttled")}
                            </Badge>
                          )}
                        </div>
                      </TableCell>
                      <TableCell className="font-mono text-muted-foreground">{session.ip}</TableCell>
                      <TableCell className="hidden font-mono text-muted-foreground md:table-cell">
                        {session.mac}
                      </TableCell>
                      <TableCell className="hidden max-w-40 truncate text-muted-foreground xl:table-cell">
                        {session.routerName}
                      </TableCell>
                      <TableCell className="tabular-nums">
                        {formatDuration(session.uptimeSec + elapsedSec)}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-3 text-muted-foreground">
                          {/* ↓ download = bytes-out · ↑ upload = bytes-in (RouterOS). */}
                          <span className="inline-flex items-center gap-1 tabular-nums">
                            <ArrowDown className="size-3 opacity-60" aria-hidden />
                            {formatBytes(downBytes(session), lang)}
                          </span>
                          <span className="inline-flex items-center gap-1 tabular-nums">
                            <ArrowUp className="size-3 opacity-60" aria-hidden />
                            {formatBytes(upBytes(session), lang)}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell className="pr-4 text-right sm:pr-6">
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-10 text-destructive hover:bg-destructive/10 hover:text-destructive"
                          onClick={() => setKickTarget(session)}
                          aria-label={tf("sessions.kickAria", { name: session.username })}
                          title={t("sessions.kick")}
                        >
                          <LogOut className="size-4" />
                        </Button>
                      </TableCell>
                    </motion.tr>
                  ))}
                </AnimatePresence>
              </TableBody>
            </Table>

            {/* N°193 — pagination : la fenêtre courante de la liste vivante.
                Le total porte sur l'ensemble filtré (pas la page) : le compte
                reste cohérent avec le KPI « connectés » et les badges du rail. */}
            <div className="flex flex-wrap items-center justify-between gap-3 border-t px-4 py-3 sm:px-6">
              <p className="text-xs text-muted-foreground">
                {tf("common.range", { start: rangeStart, end: rangeEnd, total: filteredSessions.length })}
              </p>
              <div className="flex items-center gap-2">
                <PageSizeSelect
                  value={pageSize}
                  onChange={(size) => {
                    setPageSize(size);
                    setPage(1);
                  }}
                />
                <Button
                  variant="outline"
                  size="sm"
                  className="h-10"
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={safePage <= 1}
                >
                  <ChevronLeft className="size-4" />
                  {t("common.previous")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-10"
                  onClick={() => setPage((p) => Math.min(maxPage, p + 1))}
                  disabled={safePage >= maxPage}
                >
                  {t("common.next")}
                  <ChevronRight className="size-4" />
                </Button>
              </div>
            </div>
          </>
        )}
      </Card>

      <AlertDialog open={!!kickTarget} onOpenChange={(open) => !open && setKickTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{tf("sessions.kickTitle", { name: kickTarget?.username ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>{t("sessions.kickDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={(event) => {
                event.preventDefault();
                if (kickTarget) kickMutation.mutate(kickTarget.id);
              }}
            >
              {kickMutation.isPending ? t("sessions.kickPending") : t("sessions.kick")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
