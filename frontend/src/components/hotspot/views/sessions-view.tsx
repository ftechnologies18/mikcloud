"use client";

// Vue Sessions actives — temps réel (poll auto-refresh, durées qui avancent, kick).

import { useEffect, useMemo, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowDownCircle,
  ArrowUp,
  ArrowUpCircle,
  LogOut,
  Radio,
  WifiOff,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { HotspotSession } from "@/lib/hotspot/types";
import { formatBytes, formatDuration } from "@/lib/hotspot/format";
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

const REFRESH_OPTIONS = [
  { value: "5000", label: "5 s" },
  { value: "10000", label: "10 s" },
  { value: "30000", label: "30 s" },
];

export default function SessionsView() {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const [refreshMs, setRefreshMs] = useState(5000);
  const [now, setNow] = useState(() => Date.now());
  const [kickTarget, setKickTarget] = useState<HotspotSession | null>(null);

  const { data, dataUpdatedAt, isLoading } = useQuery({
    queryKey: ["/api/sessions"],
    queryFn: () => api<HotspotSession[]>("/api/sessions"),
    refetchInterval: refreshMs,
  });

  // Horloge locale (1 s) : fait visuellement avancer les durées entre deux polls.
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  const sessions = useMemo(
    () => [...(data ?? [])].sort((a, b) => (a.startedAt < b.startedAt ? 1 : -1)),
    [data],
  );

  const elapsedSec = Math.max(0, Math.floor((now - dataUpdatedAt) / 1000));
  const totalIn = sessions.reduce((acc, s) => acc + s.bytesIn, 0);
  const totalOut = sessions.reduce((acc, s) => acc + s.bytesOut, 0);

  const kickMutation = useMutation({
    mutationFn: (id: string) => api<{ ok: boolean }>(`/api/sessions/${id}`, { method: "DELETE" }),
    onSuccess: (_, id) => {
      const target = sessions.find((s) => s.id === id);
      toast.success(
        tf("sessions.kicked", { name: target?.username ?? t("sessions.theUser") }),
      );
      setKickTarget(null);
      queryClient.invalidateQueries({ queryKey: ["/api/sessions"] });
    },
    onError: (error: Error) => toast.error(error.message),
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
          value={formatBytes(totalIn)}
          sub={t("sessions.kpi.downloadSub")}
          icon={ArrowDownCircle}
        />
        <StatCard title={t("sessions.kpi.upload")} value={formatBytes(totalOut)} sub={t("sessions.kpi.uploadSub")} icon={ArrowUpCircle} />
      </div>

      <Card className="gap-0 py-0">
        {isLoading ? (
          <LoadingRows rows={8} />
        ) : sessions.length === 0 ? (
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
                {sessions.map((session) => (
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
                        <span className="inline-flex items-center gap-1 tabular-nums">
                          <ArrowDown className="size-3 opacity-60" aria-hidden />
                          {formatBytes(session.bytesIn)}
                        </span>
                        <span className="inline-flex items-center gap-1 tabular-nums">
                          <ArrowUp className="size-3 opacity-60" aria-hidden />
                          {formatBytes(session.bytesOut)}
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
