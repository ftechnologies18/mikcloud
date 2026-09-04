"use client";

// Vue Sessions actives — temps réel (poll auto-refresh, durées qui avancent, kick).

import { useEffect, useMemo, useState } from "react";
import { usePathname } from "next/navigation";
import { AnimatePresence, motion } from "framer-motion";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowDownCircle,
  ArrowUp,
  ArrowUpCircle,
  LogOut,
  Radio,
  Search,
  WifiOff,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { detailFromPath } from "@/lib/hotspot/view-path";
import type { HotspotSession } from "@/lib/hotspot/types";
import { formatBytes, formatDuration } from "@/lib/hotspot/format";
// Sémantique trafic verrouillée : bytesIn=upload / bytesOut=download (RouterOS).
import { downBytes, upBytes } from "@/lib/hotspot/traffic-semantics";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
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

export default function SessionsView() {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const [refreshMs, setRefreshMs] = useState(5000);
  const [now, setNow] = useState(() => Date.now());
  const [kickTarget, setKickTarget] = useState<HotspotSession | null>(null);

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
  const [typedQuery, setTypedQuery] = useState<string | null>(null);
  const pathname = usePathname();
  const detailUsername = detailFromPath(pathname, "sessions");
  const query = typedQuery ?? detailUsername ?? "";

  // Horloge locale (1 s) : fait visuellement avancer les durées entre deux polls.
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  const sessions = useMemo(
    () => [...(data ?? [])].sort((a, b) => (a.startedAt < b.startedAt ? 1 : -1)),
    [data],
  );

  // Phase D — liste affichée = filtre local (les KPI restent globaux).
  const filteredSessions = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return sessions;
    return sessions.filter((s) => s.username.toLowerCase().includes(q));
  }, [sessions, query]);

  const elapsedSec = Math.max(0, Math.floor((now - dataUpdatedAt) / 1000));
  const totalIn = sessions.reduce((acc, s) => acc + s.bytesIn, 0);
  const totalOut = sessions.reduce((acc, s) => acc + s.bytesOut, 0);
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
                onChange={(event) => setTypedQuery(event.target.value)}
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

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <StatCard title={t("sessions.kpi.sessions")} value={String(sessions.length)} sub={t("sessions.kpi.sessionsSub")} icon={Radio} live />
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
            description={t("sessions.emptyDesc")}
          />
        ) : (
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
                {filteredSessions.map((session) => (
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
                      <Badge variant="outline">{session.profileName}</Badge>
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
