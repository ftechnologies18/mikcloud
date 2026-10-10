"use client";

// N°291 — vue « VPN WireGuard » (WireGuard VENDABLE, chantier ⑦ de N°289,
// arbitrage D1-D7 de l'exploitant) : le produit « VPN client final » sur le
// wg0 de la VM, à côté du tunnel de gestion des routeurs (N°285).
//
//   - D3-a : la génération passe par le mini-service hôte wg-mini (dot de
//     santé « Serveur VPN ») — la conf est créée CÔTÉ VM, la clé privée du
//     client n'existe nulle part ailleurs ;
//   - D4-a : wg0 partitionné par nommage (vpn-* vs routeurs), jauge de
//     slots GLOBALE (routeurs de gestion inclus) + check de réconciliation ;
//   - D5-a : conf chiffrée au repos côté cloud, re-livrable à la demande
//     (reveal + copie + QR + e-mail + Telegram) — l'argument produit.
//
// Le module est OPT-IN (settings.tenant.wgVpnEnabled, défaut OFF — même
// doctrine D6 que le Cybercafé) : la vue propose l'activation en un clic
// tant qu'il est éteint, puis se comporte comme les autres vues produit
// (poll ETag/304, mutations sans optimisme, toast du message serveur).
// La DÉSACTIVATION ne révoque RIEN (les accès payants restent en service) :
// la révocation reste un geste peer par peer.

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { QRCodeSVG } from "qrcode.react";
import {
  BadgeCheck,
  Copy,
  Gauge,
  Loader2,
  Mail,
  MoreHorizontal,
  Plus,
  Search,
  Send,
  Shield,
  ShieldCheck,
  ShieldOff,
  TriangleAlert,
  Wallet,
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
import { Card } from "@/components/ui/card";
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
import { Progress } from "@/components/ui/progress";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  api,
  createVpnPeer,
  emailVpnConf,
  fetchVpnConf,
  fetchVpnPeers,
  fetchVpnReconcile,
  fetchVpnStatus,
  revokeVpnPeer,
  setVpnEnabled,
  telegramVpnConf,
} from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatCurrency } from "@/lib/hotspot/format";
import { SETTINGS_QUERY_KEY, useCurrency, useSettings } from "@/components/hotspot/parts/sd-currency";
import { copyToClipboard } from "@/components/hotspot/parts/uc-clipboard";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import type { VpnPeer } from "@/lib/hotspot/types";

const REFRESH_OPTIONS = [
  { value: "10000", label: "10 s" },
  { value: "30000", label: "30 s" },
  { value: "60000", label: "60 s" },
];

/** Réponse GET /api/accounting?period=day — le DERNIER point de la série
 * porte la journée en cours au FUSEAU DU COMPTE (buckets N°198). */
interface AccountingSeriesPoint {
  label: string;
  revenue: number;
  sales: number;
}

interface AccountingDayResponse {
  period: string;
  series: AccountingSeriesPoint[];
}

/** Réponse GET /api/vpn/peers/{id}/conf — le reveal (D5-a). */
interface VpnConfResponse {
  id: string;
  name: string;
  label: string;
  ipv4: string;
  conf: string;
}

/** Réponse GET /api/vpn/reconcile — le check D4. */
interface VpnReconcileResponse {
  reachable: boolean;
  vmPeerCount: number;
  mine: number;
  missing: string[];
}

/** Nom affiché : libellé gérant → nom VM. */
function peerLabel(p: VpnPeer): string {
  return p.label || p.name;
}

export default function VpnView() {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const currency = useCurrency();
  const { data: settings, isLoading: settingsLoading } = useSettings();
  const vpnEnabled = settings?.tenant?.wgVpnEnabled === true;

  const [refreshMs, setRefreshMs] = useState(30000);
  const [query, setQuery] = useState("");
  const [busyId, setBusyId] = useState<string | null>(null);

  // — Activation / désactivation du module (rang 3 serveur) —
  const toggleModule = useMutation({
    mutationFn: (enabled: boolean) => setVpnEnabled(enabled),
    onSuccess: (_res, enabled) => {
      toast.success(t(enabled ? "vpn.activate.toast" : "vpn.deactivate.toast"));
      void queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
      void queryClient.invalidateQueries({ queryKey: ["/api/vpn/peers"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/vpn/status"] });
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("common.error")),
  });

  // — Données (poll ETag/304, jamais d'optimisme) —
  const { data: peersData, isLoading: peersLoading } = useQuery({
    queryKey: ["/api/vpn/peers"],
    queryFn: () => fetchVpnPeers(),
    refetchInterval: refreshMs,
    enabled: vpnEnabled,
  });
  const { data: status } = useQuery({
    queryKey: ["/api/vpn/status"],
    queryFn: () => fetchVpnStatus(),
    refetchInterval: 60_000,
    enabled: vpnEnabled,
  });
  const { data: accounting } = useQuery({
    queryKey: ["/api/accounting", "day"],
    queryFn: () => api<AccountingDayResponse>("/api/accounting?period=day"),
    refetchInterval: 60_000,
    enabled: vpnEnabled,
  });

  const peers = useMemo(() => peersData ?? [], [peersData]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return peers;
    return peers.filter(
      (p) =>
        peerLabel(p).toLowerCase().includes(q) ||
        p.name.toLowerCase().includes(q) ||
        p.ipv4.toLowerCase().includes(q),
    );
  }, [peers, query]);

  const activeCount = peers.filter((p) => p.state === "active").length;
  const today = accounting?.series?.length ? accounting.series[accounting.series.length - 1] : undefined;

  const invalidatePeers = () => {
    void queryClient.invalidateQueries({ queryKey: ["/api/vpn/peers"] });
    void queryClient.invalidateQueries({ queryKey: ["/api/vpn/status"] });
  };

  // — Création (génération côté VM, quelques secondes) —
  const [addOpen, setAddOpen] = useState(false);
  const [addLabel, setAddLabel] = useState("");
  const [addPrice, setAddPrice] = useState("");
  const createMutation = useMutation({
    mutationFn: () =>
      createVpnPeer({
        label: addLabel.trim(),
        kind: "fulltunnel",
        salePrice: Math.max(0, Math.round(Number(addPrice) || 0)),
      }),
    onSuccess: (peer) => {
      toast.success(t("vpn.addToast"));
      setAddOpen(false);
      setAddLabel("");
      setAddPrice("");
      invalidatePeers();
      // Le geste suivant naturel : la livraison de la conf au client.
      void openConf(peer.id, peer);
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("vpn.errorToast")),
  });

  // — Révocation —
  const [revokeTarget, setRevokeTarget] = useState<VpnPeer | null>(null);
  const revokeMutation = useMutation({
    mutationFn: (id: string) => revokeVpnPeer(id),
    onSuccess: () => {
      toast.success(t("vpn.revokeToast"));
      setRevokeTarget(null);
      invalidatePeers();
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("vpn.errorToast")),
    onSettled: () => setBusyId(null),
  });

  // — Reveal / livraison de la conf (D5-a) —
  const [confPeer, setConfPeer] = useState<VpnPeer | null>(null);
  const [confText, setConfText] = useState<string | null>(null);
  const [confLoading, setConfLoading] = useState(false);
  const openConf = async (id: string, known?: VpnPeer) => {
    const peer = known ?? peers.find((p) => p.id === id) ?? null;
    setConfPeer(peer);
    setConfText(null);
    setConfLoading(true);
    setConfOpen(true);
    try {
      const res = await fetchVpnConf(id);
      setConfText(res.conf);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("vpn.errorToast"));
      setConfOpen(false);
    } finally {
      setConfLoading(false);
    }
  };
  const [confOpen, setConfOpen] = useState(false);

  // — Livraison e-mail —
  const [emailOpen, setEmailOpen] = useState(false);
  const [emailTo, setEmailTo] = useState("");
  const emailMutation = useMutation({
    mutationFn: () => emailVpnConf(confPeer!.id, emailTo.trim()),
    onSuccess: (res) => {
      toast.success(res.message || t("vpn.emailToast"));
      setEmailOpen(false);
      setEmailTo("");
      setConfOpen(false);
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("vpn.errorToast")),
  });

  // — Livraison Telegram —
  const telegramMutation = useMutation({
    mutationFn: () => telegramVpnConf(confPeer!.id),
    onSuccess: (res) => {
      toast.success(res.message || t("vpn.telegramToast"));
      setConfOpen(false);
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("vpn.errorToast")),
  });

  // — Réconciliation (check D4, geste explicite rang 2) —
  const reconcileMutation = useMutation({
    mutationFn: () => fetchVpnReconcile(),
    onSuccess: (res) => {
      if (!res.reachable) {
        toast.error(t("vpn.reconcileDown"));
        return;
      }
      if (res.missing.length === 0) {
        toast.success(tf("vpn.reconcileOk", { mine: String(res.mine) }));
      } else {
        toast.warning(tf("vpn.reconcileMissing", { n: String(res.missing.length), names: res.missing.join(", ") }));
      }
      invalidatePeers();
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("vpn.errorToast")),
  });

  // État d'attente des réglages.
  if (settingsLoading) {
    return (
      <div className="space-y-4 sm:space-y-6">
        <PageHeader title={t("vpn.title")} description={t("vpn.description")} />
        <Card className="gap-0 py-0">
          <LoadingRows rows={6} />
        </Card>
      </div>
    );
  }
  // Module éteint : activation en un clic (même doctrine que le Cybercafé).
  if (!vpnEnabled) {
    return (
      <div className="space-y-4 sm:space-y-6">
        <PageHeader title={t("vpn.title")} description={t("vpn.description")} />
        <Card className="mx-auto max-w-2xl p-6 text-center">
          <Shield className="mx-auto size-10 text-muted-foreground" aria-hidden />
          <h2 className="mt-3 text-lg font-semibold">{t("vpn.activate.title")}</h2>
          <p className="mx-auto mt-2 max-w-md text-sm text-muted-foreground">{t("vpn.activate.desc")}</p>
          <Button
            className="mt-5"
            disabled={toggleModule.isPending}
            onClick={() => toggleModule.mutate(true)}
          >
            <BadgeCheck className="size-4" aria-hidden />
            {t("vpn.activate.cta")}
          </Button>
        </Card>
      </div>
    );
  }

  const miniUp = status?.miniReachable === true;
  const slotsUsed = status?.slotsUsed ?? 0;
  const slotsPool = status?.slotsPool ?? 253;

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("vpn.title")}
        description={t("vpn.description")}
        actions={
          <>
            <Button size="sm" className="h-10 gap-1.5" onClick={() => setAddOpen(true)}>
              <Plus className="size-4" aria-hidden />
              {t("vpn.add")}
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="h-10 gap-1.5"
              disabled={reconcileMutation.isPending}
              onClick={() => reconcileMutation.mutate()}
            >
              {reconcileMutation.isPending ? (
                <Loader2 className="size-4 animate-spin" aria-hidden />
              ) : (
                <ShieldCheck className="size-4" aria-hidden />
              )}
              {t("vpn.reconcile")}
            </Button>
            <div className="relative">
              <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
              <Input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t("vpn.search")}
                className="h-10 w-40 pl-9 sm:w-56"
                aria-label={t("vpn.search")}
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

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard title={t("vpn.kpi.peers")} value={String(activeCount)} sub={t("vpn.kpi.peersSub")} icon={Shield} live />
        <StatCard
          title={t("vpn.kpi.vm")}
          value={miniUp ? t("vpn.kpi.vmUp") : t("vpn.kpi.vmDown")}
          sub={miniUp ? t("vpn.kpi.vmUpSub") : (status?.miniError ?? t("vpn.kpi.vmDownSub"))}
          icon={miniUp ? ShieldCheck : TriangleAlert}
        />
        <div className="rounded-xl border bg-card p-4 shadow-sm">
          <div className="flex items-center gap-2 text-sm font-medium">
            <Gauge className="size-4 text-muted-foreground" aria-hidden />
            {t("vpn.kpi.slots")}
          </div>
          <div className="mt-1 text-2xl font-semibold tabular-nums">
            {slotsUsed}
            <span className="text-sm font-normal text-muted-foreground"> / {slotsPool}</span>
          </div>
          <Progress
            className="mt-2 h-1.5"
            value={Math.min(100, (slotsUsed / Math.max(1, slotsPool)) * 100)}
            aria-label={tf("vpn.kpi.slotsSub", { used: String(slotsUsed), pool: String(slotsPool) })}
          />
          <p className="mt-1.5 text-xs text-muted-foreground">
            {tf("vpn.kpi.slotsSub", { used: String(slotsUsed), pool: String(slotsPool) })}
          </p>
        </div>
        <StatCard
          title={t("vpn.kpi.caisse")}
          value={today ? formatCurrency(today.revenue, currency, lang) : "—"}
          sub={t("vpn.kpi.caisseSub")}
          icon={Wallet}
        />
      </div>

      <Card className="gap-0 py-0">
        {peersLoading ? (
          <LoadingRows rows={6} />
        ) : filtered.length === 0 ? (
          peers.length === 0 ? (
            <EmptyState icon={Shield} title={t("vpn.empty.title")} description={t("vpn.empty.desc")} />
          ) : (
            <EmptyState icon={Search} title={t("vpn.noMatch")} description={t("vpn.noMatchDesc")} />
          )
        ) : (
          <>
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("vpn.access")}</TableHead>
                  <TableHead className="text-muted-foreground">{t("vpn.state")}</TableHead>
                  <TableHead className="hidden font-mono text-muted-foreground md:table-cell">{t("vpn.address")}</TableHead>
                  <TableHead className="hidden text-muted-foreground lg:table-cell">{t("vpn.created")}</TableHead>
                  <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("vpn.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((peer) => {
                  const busy = busyId === peer.id || revokeMutation.isPending;
                  return (
                    <TableRow key={peer.id} className="transition-colors hover:bg-muted/50">
                      <TableCell className="max-w-56 pl-4 sm:pl-6">
                        <span className="flex items-center gap-2">
                          <Shield className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                          <span className="min-w-0">
                            <span className="block truncate text-sm font-medium" title={peerLabel(peer)}>
                              {peerLabel(peer)}
                            </span>
                            <span className="block truncate font-mono text-xs text-muted-foreground" title={peer.name}>
                              {peer.name}
                            </span>
                          </span>
                        </span>
                      </TableCell>
                      <TableCell>
                        {peer.state === "pending" ? (
                          <Badge className="gap-1.5 border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300" variant="outline">
                            <Loader2 className="size-3 animate-spin" aria-hidden />
                            {t("vpn.state.pending")}
                          </Badge>
                        ) : peer.state === "error" ? (
                          <Badge
                            className="gap-1.5 border-destructive/30 bg-destructive/10 text-destructive"
                            variant="outline"
                            title={peer.errorMsg ?? undefined}
                          >
                            <TriangleAlert className="size-3" aria-hidden />
                            {t("vpn.state.error")}
                          </Badge>
                        ) : (
                          <Badge className="gap-1.5 border-primary/25 bg-primary/10 text-primary" variant="outline">
                            <span className="live-dot size-1.5 rounded-full bg-primary" aria-hidden />
                            {t("vpn.state.active")}
                          </Badge>
                        )}
                      </TableCell>
                      <TableCell className="hidden font-mono text-muted-foreground md:table-cell">
                        {peer.ipv4 || t("vpn.noAddress")}
                      </TableCell>
                      <TableCell className="hidden text-muted-foreground lg:table-cell">
                        {peer.createdAt.slice(0, 10)}
                      </TableCell>
                      <TableCell className="pr-4 sm:pr-6">
                        <div className="flex items-center justify-end gap-1">
                          {peer.state === "active" && (
                            <Button
                              size="sm"
                              variant="outline"
                              className="h-8 gap-1.5"
                              disabled={busy}
                              onClick={() => void openConf(peer.id)}
                            >
                              <Send className="size-3.5" aria-hidden />
                              {t("vpn.deliver")}
                            </Button>
                          )}
                          {peer.state !== "pending" && (
                            <DropdownMenu>
                              <DropdownMenuTrigger asChild>
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  className="size-8"
                                  aria-label={t("vpn.more")}
                                  title={t("vpn.more")}
                                  disabled={busy}
                                >
                                  <MoreHorizontal className="size-4" aria-hidden />
                                </Button>
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="end">
                                {peer.state === "active" && (
                                  <DropdownMenuItem onClick={() => void openConf(peer.id)}>
                                    <Copy className="size-3.5" aria-hidden />
                                    {t("vpn.conf")}
                                  </DropdownMenuItem>
                                )}
                                <DropdownMenuSeparator />
                                <DropdownMenuItem className="text-destructive" onClick={() => setRevokeTarget(peer)}>
                                  <ShieldOff className="size-3.5" aria-hidden />
                                  {t("vpn.revoke")}
                                </DropdownMenuItem>
                                {peer.state === "error" && peer.errorMsg && (
                                  <DropdownMenuItem disabled className="max-w-64 whitespace-normal text-xs text-muted-foreground">
                                    {peer.errorMsg}
                                  </DropdownMenuItem>
                                )}
                              </DropdownMenuContent>
                            </DropdownMenu>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
            <p className="border-t px-4 py-2.5 text-xs text-muted-foreground sm:px-6">{t("vpn.addWarning")}</p>
          </>
        )}
      </Card>

      <p className="px-1 text-xs text-muted-foreground">{t("vpn.settingsHint")}</p>
      <Button
        size="sm"
        variant="ghost"
        className="h-8 text-muted-foreground"
        disabled={toggleModule.isPending}
        onClick={() => toggleModule.mutate(false)}
      >
        {t("vpn.deactivate.cta")}
      </Button>

      {/* — Dialog « Vendre un accès » — */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("vpn.addTitle")}</DialogTitle>
            <DialogDescription>{t("vpn.addDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3 py-1">
            <div className="space-y-1.5">
              <Label htmlFor="vpn-add-label">{t("vpn.addLabel")}</Label>
              <Input
                id="vpn-add-label"
                value={addLabel}
                onChange={(event) => setAddLabel(event.target.value)}
                placeholder={t("vpn.addLabelPlaceholder")}
                maxLength={48}
                autoFocus
              />
            </div>
            <div className="space-y-1.5">
              <Label>{t("vpn.addKind")}</Label>
              <Select value="fulltunnel" disabled>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="fulltunnel">{t("vpn.addKind.fulltunnel")}</SelectItem>
                  <SelectItem value="remote" disabled>{t("vpn.addKind.remote")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="vpn-add-price">{t("vpn.addPrice")}</Label>
              <Input
                id="vpn-add-price"
                value={addPrice}
                onChange={(event) => setAddPrice(event.target.value.replace(/[^\d]/g, ""))}
                placeholder={t("vpn.addPricePlaceholder")}
                inputMode="numeric"
              />
              <p className="text-xs text-muted-foreground">{t("vpn.addPriceHint")}</p>
            </div>
            <p className="rounded-md border bg-muted/40 p-2.5 text-xs text-muted-foreground">{t("vpn.addWarning")}</p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAddOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={() => createMutation.mutate()}
              disabled={createMutation.isPending || addLabel.trim() === ""}
            >
              {createMutation.isPending ? (
                <>
                  <Loader2 className="size-4 animate-spin" aria-hidden />
                  {t("vpn.addPending")}
                </>
              ) : (
                t("vpn.addCta")
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* — Dialog « Configuration » (reveal + livraison, D5-a) — */}
      <Dialog open={confOpen} onOpenChange={setConfOpen}>
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("vpn.confTitle")}</DialogTitle>
            <DialogDescription>
              {confPeer ? `${peerLabel(confPeer)} · ${confPeer.name}` : ""} — {t("vpn.confDesc")}
            </DialogDescription>
          </DialogHeader>
          {confLoading || confText === null ? (
            <div className="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" aria-hidden />
              {t("vpn.addPending")}
            </div>
          ) : (
            <div className="space-y-3 py-1">
              <div className="flex items-start gap-3">
                <div
                  role="img"
                  aria-label={t("vpn.confQr")}
                  className="shrink-0 rounded-lg border bg-white p-1.5"
                >
                  <QRCodeSVG value={confText} size={132} level="L" className="size-[132px]" />
                </div>
                <div className="min-w-0 flex-1 space-y-2">
                  <pre className="max-h-48 overflow-y-auto rounded-lg border bg-muted/40 p-2.5 font-mono text-[11px] leading-relaxed">
                    {confText}
                  </pre>
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-8 gap-1.5"
                    onClick={() => {
                      copyToClipboard(confText);
                      toast.success(t("vpn.confCopied"));
                    }}
                  >
                    <Copy className="size-3.5" aria-hidden />
                    {t("vpn.confCopy")}
                  </Button>
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button size="sm" variant="outline" className="h-8 gap-1.5" onClick={() => setEmailOpen(true)}>
                  <Mail className="size-3.5" aria-hidden />
                  {t("vpn.confEmail")}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  className="h-8 gap-1.5"
                  disabled={telegramMutation.isPending}
                  onClick={() => telegramMutation.mutate()}
                >
                  {telegramMutation.isPending ? (
                    <Loader2 className="size-3.5 animate-spin" aria-hidden />
                  ) : (
                    <Send className="size-3.5" aria-hidden />
                  )}
                  {t("vpn.confTelegram")}
                </Button>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* — Dialog « Envoyer par e-mail » — */}
      <Dialog open={emailOpen} onOpenChange={setEmailOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("vpn.emailTitle")}</DialogTitle>
            <DialogDescription>{t("vpn.emailDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-1.5 py-1">
            <Label htmlFor="vpn-email-to">{t("vpn.emailTo")}</Label>
            <Input
              id="vpn-email-to"
              type="email"
              value={emailTo}
              onChange={(event) => setEmailTo(event.target.value)}
              placeholder="client@exemple.com"
              autoFocus
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEmailOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={() => emailMutation.mutate()}
              disabled={emailMutation.isPending || !emailTo.includes("@")}
            >
              {emailMutation.isPending ? (
                <Loader2 className="size-4 animate-spin" aria-hidden />
              ) : (
                <Mail className="size-4" aria-hidden />
              )}
              {t("vpn.emailCta")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* — Confirmation de révocation — */}
      <AlertDialog
        open={revokeTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRevokeTarget(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("vpn.revokeConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {revokeTarget ? `${peerLabel(revokeTarget)} (${revokeTarget.name}) — ` : ""}
              {t("vpn.revokeConfirmDesc")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={revokeMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                if (!revokeTarget) return;
                setBusyId(revokeTarget.id);
                revokeMutation.mutate(revokeTarget.id);
              }}
            >
              {revokeMutation.isPending ? (
                <Loader2 className="size-4 animate-spin" aria-hidden />
              ) : (
                <ShieldOff className="size-4" aria-hidden />
              )}
              {t("vpn.revoke")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
