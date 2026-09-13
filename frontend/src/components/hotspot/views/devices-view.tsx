"use client";

// N°100 — vue « Appareils » (console HomeNet) : qui est connecté à la maison.
//
// Miroir domestique de la vue Sessions actives : MÊME source de vérité
// (GET /api/sessions, la table tenue par read_state — endpoint ouvert aux
// deux usages, N°98), VOCABULAIRE de foyer : un « appareil » (le téléphone
// de la petite, la TV, l'ordinateur du grand) et non un « client », pas de
// colonne profil (un foyer ne vend pas de forfaits), pas d'action
// d'éjection — couper l'accès d'un membre de la famille se décide dans la
// vue Protection (couvre-feu FamilyGuard), pas au coup par coup.
// Lecture pure : aucun mutate, aucun optimistic UI — la table respire au
// rythme du poll 10 s et de l'horloge des durées.

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowDownCircle,
  ArrowUp,
  ArrowUpCircle,
  MonitorSmartphone,
  Search,
  WifiOff,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatBytes, formatDuration } from "@/lib/hotspot/format";
// Sémantique trafic verrouillée : bytesIn=upload / bytesOut=download (RouterOS).
import { downBytes, upBytes } from "@/lib/hotspot/traffic-semantics";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import type { HotspotSession } from "@/lib/hotspot/types";

const REFRESH_OPTIONS = [
  { value: "5000", label: "5 s" },
  { value: "10000", label: "10 s" },
  { value: "30000", label: "30 s" },
];

export default function DevicesView() {
  const { t, lang } = useI18n();
  const [refreshMs, setRefreshMs] = useState(10000);
  const [now, setNow] = useState(() => Date.now());
  const [query, setQuery] = useState("");

  const { data, dataUpdatedAt, isLoading } = useQuery({
    queryKey: ["/api/sessions"],
    queryFn: () => api<HotspotSession[]>("/api/sessions"),
    refetchInterval: refreshMs,
  });

  // Horloge locale (1 s) : fait visuellement avancer les durées entre deux
  // polls — même mécanique que la vue Sessions.
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  const devices = useMemo(
    () => [...(data ?? [])].sort((a, b) => (a.startedAt < b.startedAt ? 1 : -1)),
    [data],
  );

  // Filtre local (nom, IP ou MAC) — le tableau n'est pas paginé.
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return devices;
    return devices.filter(
      (d) =>
        d.username.toLowerCase().includes(q) ||
        d.ip.toLowerCase().includes(q) ||
        d.mac.toLowerCase().includes(q),
    );
  }, [devices, query]);

  const elapsedSec = Math.max(0, Math.floor((now - dataUpdatedAt) / 1000));
  // Sémantique RouterOS verrouillée : bytesOut = download (descendant),
  // bytesIn = upload (montant) — voir traffic-semantics.ts (doc MikroTik).
  const totalDown = devices.reduce((acc, d) => acc + downBytes(d), 0);
  const totalUp = devices.reduce((acc, d) => acc + upBytes(d), 0);

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("devices.title")}
        description={t("devices.description")}
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
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t("devices.searchPlaceholder")}
                className="h-10 w-40 pl-9 sm:w-56"
                aria-label={t("devices.searchPlaceholder")}
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
        <StatCard
          title={t("devices.kpi.connected")}
          value={String(devices.length)}
          sub={t("devices.kpi.connectedSub")}
          icon={MonitorSmartphone}
          live
        />
        <StatCard
          title={t("devices.kpi.download")}
          value={formatBytes(totalDown, lang)}
          sub={t("devices.kpi.downloadSub")}
          icon={ArrowDownCircle}
        />
        <StatCard
          title={t("devices.kpi.upload")}
          value={formatBytes(totalUp, lang)}
          sub={t("devices.kpi.uploadSub")}
          icon={ArrowUpCircle}
        />
      </div>

      <Card className="gap-0 py-0">
        {isLoading ? (
          <LoadingRows rows={6} />
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={WifiOff}
            title={devices.length === 0 ? t("devices.empty.title") : t("devices.noMatch")}
            description={devices.length === 0 ? t("devices.empty.desc") : t("devices.noMatchDesc")}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("devices.device")}</TableHead>
                <TableHead className="text-muted-foreground">{t("common.ip")}</TableHead>
                <TableHead className="hidden text-muted-foreground md:table-cell">{t("common.mac")}</TableHead>
                <TableHead className="hidden text-muted-foreground xl:table-cell">{t("common.router")}</TableHead>
                <TableHead className="text-muted-foreground">{t("sessions.connectedSince")}</TableHead>
                <TableHead className="pr-4 text-muted-foreground sm:pr-6">↓ / ↑</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((device) => (
                <TableRow key={device.id} className="transition-colors hover:bg-muted/50">
                  <TableCell className="max-w-56 pl-4 font-mono text-sm font-medium sm:pl-6">
                    <span className="flex items-center gap-2">
                      <MonitorSmartphone className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                      <span className="truncate" title={device.username}>
                        {device.username}
                      </span>
                    </span>
                  </TableCell>
                  <TableCell className="font-mono text-muted-foreground">{device.ip}</TableCell>
                  <TableCell className="hidden font-mono text-muted-foreground md:table-cell">{device.mac}</TableCell>
                  <TableCell className="hidden max-w-40 truncate text-muted-foreground xl:table-cell">
                    {device.routerName}
                  </TableCell>
                  <TableCell className="tabular-nums">
                    {formatDuration(device.uptimeSec + elapsedSec)}
                  </TableCell>
                  <TableCell className="pr-4 sm:pr-6">
                    <div className="flex items-center gap-3 text-muted-foreground">
                      {/* ↓ download = bytes-out · ↑ upload = bytes-in (RouterOS). */}
                      <span className="inline-flex items-center gap-1 tabular-nums">
                        <ArrowDown className="size-3 opacity-60" aria-hidden />
                        {formatBytes(downBytes(device), lang)}
                      </span>
                      <span className="inline-flex items-center gap-1 tabular-nums">
                        <ArrowUp className="size-3 opacity-60" aria-hidden />
                        {formatBytes(upBytes(device), lang)}
                      </span>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
    </div>
  );
}
