"use client";

// N°285 — carte « Tunnel WireGuard » de la fiche routeur (mode agent).
//
// RENFORT opt-in : un deuxième chemin routeur ↔ VM, chiffré, sans aucun port
// public — le mode agent reste le SOCLE et le tunnel n'est JAMAIS sur le
// chemin critique du check-in (zéro orphelin possible). Cycle guidé :
//   1. « Activer » → le routeur génère sa paire WG (clé privée jamais
//      transportée) et rapporte sa clé publique ;
//   2. l'exploitant embarque le peer côté serveur (dispatch ops-wg
//      peer-add-router) et lit le livret 600 root (.router.txt) ;
//   3. le gérant colle les 4 valeurs du livret → livraison au routeur au
//      prochain check-in (≤ 45 s) ;
//   4. « Tester » → dial direct 10.8.0.N:8728 depuis la VM (preuve de
//      joignabilité) ;
//   5. « Désactiver » → démontage routeur + rappel de la révocation serveur.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Check,
  Copy,
  Loader2,
  RefreshCw,
  ShieldCheck,
  ShieldOff,
  TriangleAlert,
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { roleRank } from "@/lib/hotspot/roles";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { RouterDevice, RouterWgActionResult, RouterWgStatus, RouterWgTestResult } from "@/lib/hotspot/types";

interface WgForm {
  peerName: string;
  address: string;
  serverPub: string;
  psk: string;
  endpoint: string;
}

const WG_FORM_EMPTY: WgForm = { peerName: "", address: "", serverPub: "", psk: "", endpoint: "" };

// CopyMono — valeur technique + copie en un clic (clipboard async, feedback toast).
function CopyMono({ value, label }: { value: string; label: string }) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error(t("routers.wg.copyFailed"));
    }
  }
  return (
    <div className="flex min-w-0 items-center gap-1.5">
      <code className="min-w-0 flex-1 truncate rounded bg-muted px-2 py-1 font-mono text-xs">{value}</code>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-8 shrink-0 text-muted-foreground hover:text-foreground"
        aria-label={`${t("common.copy")} ${label}`}
        onClick={copy}
      >
        {copied ? <Check className="size-3.5 text-emerald-600" /> : <Copy className="size-3.5" />}
      </Button>
    </div>
  );
}

export function RouterWireGuardCard({ router }: { router: RouterDevice }) {
  const { t } = useI18n();
  const role = useHotspotStore((s) => s.user?.role);
  const canManage = roleRank(role ?? "") >= 2;
  const queryClient = useQueryClient();
  const [form, setForm] = useState<WgForm>(WG_FORM_EMPTY);
  const [confirmDisable, setConfirmDisable] = useState(false);
  const [copiedPub, setCopiedPub] = useState(false);

  // État du renfort : poll 8 s UNIQUEMENT pendant les phases d'attente
  // (keygen/setup en vol) — l'état stable ne consomme rien (économie N°75).
  const { data: wg } = useQuery({
    queryKey: ["/api/routers", router.id, "wg"],
    queryFn: () => api<RouterWgStatus>(`/api/routers/${router.id}/wg`),
    refetchInterval: (q) => {
      const st = q.state.data?.state ?? "";
      return st === "pending_keygen" || st === "pending_setup" ? 8_000 : false;
    },
  });

  // peerNameVal — suggestion affichée en placeholder, utilisée si le champ
  // reste vide (aucun effet de synchronisation : dérivation au rendu,
  // discipline react-hooks/set-state-in-effect).
  const peerNameVal = form.peerName.trim() || wg?.wgPeerName || wg?.peerName || "";

  function invalidate() {
    void queryClient.invalidateQueries({ queryKey: ["/api/routers", router.id, "wg"] });
    void queryClient.invalidateQueries({ queryKey: ["/api/routers"] });
  }

  const enableMutation = useMutation({
    mutationFn: () => api<RouterWgActionResult>(`/api/routers/${router.id}/wg-enable`, { method: "POST" }),
    onSuccess: (res) => {
      toast.success(res.message ?? t("routers.wg.enableQueued"));
      invalidate();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const paramsMutation = useMutation({
    mutationFn: (body: WgForm) =>
      api<RouterWgActionResult>(`/api/routers/${router.id}/wg-params`, {
        method: "PUT",
        body: {
          peerName: (body.peerName.trim() || wg?.wgPeerName || wg?.peerName || "").trim(),
          address: body.address.trim(),
          serverPub: body.serverPub.trim(),
          psk: body.psk.trim(),
          endpoint: body.endpoint.trim(),
        },
      }),
    onSuccess: (res) => {
      toast.success(res.message ?? t("routers.wg.paramsQueued"));
      setForm(WG_FORM_EMPTY);
      invalidate();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const testMutation = useMutation({
    mutationFn: () => api<RouterWgTestResult>(`/api/routers/${router.id}/wg-test`, { method: "POST" }),
    onSuccess: (res) => {
      if (res.ok) toast.success(res.message);
      else toast.error(res.message);
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const disableMutation = useMutation({
    mutationFn: () => api<RouterWgActionResult>(`/api/routers/${router.id}/wg-disable`, { method: "POST" }),
    onSuccess: (res) => {
      toast.success(res.message ?? t("routers.wg.disableQueued"));
      invalidate();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  async function copyPub() {
    if (!wg?.wgPub) return;
    try {
      await navigator.clipboard.writeText(wg.wgPub);
      setCopiedPub(true);
      setTimeout(() => setCopiedPub(false), 1500);
    } catch {
      toast.error(t("routers.wg.copyFailed"));
    }
  }

  const state = wg?.state ?? "";
  const busy =
    enableMutation.isPending || paramsMutation.isPending || testMutation.isPending || disableMutation.isPending;

  const stateBadge = () => {
    switch (state) {
      case "pending_keygen":
      case "pending_setup":
        return (
          <Badge variant="outline" className="gap-1 border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400">
            <Loader2 className="size-3 animate-spin" />
            {t(state === "pending_keygen" ? "routers.wg.statePendingKeygen" : "routers.wg.statePendingSetup")}
          </Badge>
        );
      case "ready":
        return (
          <Badge variant="outline" className="gap-1 border-sky-500/40 bg-sky-500/10 text-sky-700 dark:text-sky-400">
            {t("routers.wg.stateReady")}
          </Badge>
        );
      case "active":
        return (
          <Badge variant="outline" className="gap-1 border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400">
            <ShieldCheck className="size-3" />
            {t("routers.wg.stateActive")}
          </Badge>
        );
      case "error":
        return (
          <Badge variant="outline" className="gap-1 border-red-500/40 bg-red-500/10 text-red-700 dark:text-red-400">
            <TriangleAlert className="size-3" />
            {t("routers.wg.stateError")}
          </Badge>
        );
      default:
        return (
          <Badge variant="outline" className="gap-1 text-muted-foreground">
            <ShieldOff className="size-3" />
            {t("routers.wg.stateOff")}
          </Badge>
        );
    }
  };

  const formValid =
    /^[a-z0-9][a-z0-9-]{0,31}$/.test(peerNameVal) &&
    /^10\.8\.0\.(\d{1,3})$/.test(form.address.trim()) &&
    /^[A-Za-z0-9+/]{43}=$/.test(form.serverPub.trim()) &&
    /^[A-Za-z0-9+/]{43}=$/.test(form.psk.trim()) &&
    form.endpoint.trim().length > 0;

  return (
    <Card className="py-0">
      <CardContent className="flex flex-col gap-4 p-4 sm:p-6">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <ShieldCheck className="size-4 text-muted-foreground" />
            <p className="font-semibold leading-tight">{t("routers.wg.title")}</p>
            {stateBadge()}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {state === "active" || state === "ready" || state === "error" ? (
              <>
                {state === "active" && canManage ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    className="h-9 gap-1.5"
                    disabled={busy}
                    onClick={() => testMutation.mutate()}
                  >
                    {testMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                    {t("routers.wg.test")}
                  </Button>
                ) : null}
                {canManage ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    className="h-9 gap-1.5 text-destructive hover:text-destructive"
                    disabled={busy}
                    onClick={() => setConfirmDisable(true)}
                  >
                    <ShieldOff className="size-4" />
                    {t("routers.wg.disable")}
                  </Button>
                ) : null}
              </>
            ) : canManage && (state === "" || state === "error") ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="h-9 gap-1.5"
                disabled={busy || !wg?.online}
                title={wg?.online ? undefined : t("routers.wg.needsOnline")}
                onClick={() => enableMutation.mutate()}
              >
                {enableMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <ShieldCheck className="size-4" />}
                {t("routers.wg.enable")}
              </Button>
            ) : null}
          </div>
        </div>

        <p className="text-sm text-muted-foreground">{t("routers.wg.description")}</p>

        {state === "error" && wg?.error ? (
          <p className="rounded-md border border-red-500/30 bg-red-500/5 px-3 py-2 text-sm text-red-700 dark:text-red-400">
            {wg.error}
          </p>
        ) : null}

        {/* Clé publique du routeur connue → étape serveur + formulaire. */}
        {wg?.wgPub ? (
          <div className="flex flex-col gap-3">
            <div>
              <Label className="text-xs text-muted-foreground">{t("routers.wg.routerPubLabel")}</Label>
              <div className="mt-1 flex items-center gap-1.5">
                <code className="min-w-0 flex-1 truncate rounded bg-muted px-2 py-1 font-mono text-xs">{wg.wgPub}</code>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-8 shrink-0 text-muted-foreground hover:text-foreground"
                  aria-label={t("common.copy")}
                  onClick={copyPub}
                >
                  {copiedPub ? <Check className="size-3.5 text-emerald-600" /> : <Copy className="size-3.5" />}
                </Button>
              </div>
            </div>

            <div className="rounded-md border bg-muted/40 p-3 text-xs text-muted-foreground">
              <p className="font-medium text-foreground">{t("routers.wg.stepServerTitle")}</p>
              <ol className="mt-1 list-decimal space-y-0.5 pl-4">
                <li>
                  {t("routers.wg.stepDispatch")}
                  {wg.peerName ? (
                    <code className="ml-1 rounded bg-muted px-1 font-mono">peer_name={wg.peerName}</code>
                  ) : null}
                </li>
                <li>{t("routers.wg.stepLivret")}</li>
                <li>{t("routers.wg.stepPaste")}</li>
              </ol>
            </div>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="flex flex-col gap-1">
                <Label htmlFor={`wg-peer-${router.id}`} className="text-xs">
                  {t("routers.wg.peerLabel")}
                </Label>
                <Input
                  id={`wg-peer-${router.id}`}
                  value={form.peerName}
                  onChange={(e) => setForm({ ...form, peerName: e.target.value })}
                  placeholder={wg.peerName || "routeur-1"}
                  className="h-9 font-mono text-xs"
                  autoComplete="off"
                  spellCheck={false}
                />
              </div>
              <div className="flex flex-col gap-1">
                <Label htmlFor={`wg-addr-${router.id}`} className="text-xs">
                  {t("routers.wg.addrLabel")}
                </Label>
                <Input
                  id={`wg-addr-${router.id}`}
                  value={form.address}
                  onChange={(e) => setForm({ ...form, address: e.target.value })}
                  placeholder="10.8.0.2"
                  className="h-9 font-mono text-xs"
                  autoComplete="off"
                  spellCheck={false}
                />
              </div>
              <div className="flex flex-col gap-1">
                <Label htmlFor={`wg-spub-${router.id}`} className="text-xs">
                  {t("routers.wg.serverPubLabel")}
                </Label>
                <Input
                  id={`wg-spub-${router.id}`}
                  value={form.serverPub}
                  onChange={(e) => setForm({ ...form, serverPub: e.target.value })}
                  placeholder="…="
                  className="h-9 font-mono text-xs"
                  autoComplete="off"
                  spellCheck={false}
                />
              </div>
              <div className="flex flex-col gap-1">
                <Label htmlFor={`wg-psk-${router.id}`} className="text-xs">
                  {t("routers.wg.pskLabel")}
                </Label>
                <Input
                  id={`wg-psk-${router.id}`}
                  type="password"
                  value={form.psk}
                  onChange={(e) => setForm({ ...form, psk: e.target.value })}
                  placeholder="…="
                  className="h-9 font-mono text-xs"
                  autoComplete="off"
                  spellCheck={false}
                />
              </div>
              <div className="flex flex-col gap-1 sm:col-span-2">
                <Label htmlFor={`wg-ep-${router.id}`} className="text-xs">
                  {t("routers.wg.endpointLabel")}
                </Label>
                <Input
                  id={`wg-ep-${router.id}`}
                  value={form.endpoint}
                  onChange={(e) => setForm({ ...form, endpoint: e.target.value })}
                  placeholder="203.0.113.10:51820"
                  className="h-9 font-mono text-xs"
                  autoComplete="off"
                  spellCheck={false}
                />
              </div>
            </div>

            {canManage ? (
              <div>
                <Button
                  type="button"
                  size="sm"
                  className="h-9 gap-1.5"
                  disabled={busy || !formValid}
                  onClick={() => paramsMutation.mutate(form)}
                >
                  {paramsMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <ShieldCheck className="size-4" />}
                  {t("routers.wg.deliver")}
                </Button>
              </div>
            ) : null}
          </div>
        ) : null}

        {/* Tunnel actif : coordonnées du chemin direct. */}
        {state === "active" && wg?.ipv4 ? (
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <CopyMono value={wg.ipv4} label={t("routers.wg.addrLabel")} />
            {wg.endpoint ? <CopyMono value={wg.endpoint} label={t("routers.wg.endpointLabel")} /> : null}
          </div>
        ) : null}

        <p className="text-xs text-muted-foreground">{t("routers.wg.footnote")}</p>
      </CardContent>

      <AlertDialog open={confirmDisable} onOpenChange={setConfirmDisable}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("routers.wg.disableConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("routers.wg.disableConfirmDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                disableMutation.mutate();
                setConfirmDisable(false);
              }}
            >
              {t("routers.wg.disable")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
