"use client";

// Vue Notifications — alertes routeur hors ligne / stock bas / rapport quotidien,
// canaux Telegram + WhatsApp Cloud API + Email SMTP et historique des envois.
// Contrat API : GET/PUT /api/notifications, POST /api/notifications/test,
// GET /api/notifications/log (voir lib/hotspot/types.ts).

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BellOff,
  BellRing,
  History,
  Loader2,
  Mail,
  MessageCircle,
  RefreshCw,
  Send,
  TriangleAlert,
} from "lucide-react";
import { toast } from "sonner";
import type { LucideIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingCards, LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { api, ApiError } from "@/lib/hotspot/api";
import { formatDateTime } from "@/lib/hotspot/format";
import type { NotifChannel, NotifLogEntry, NotifSettings } from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

// Heures proposées pour le rapport quotidien (06 → 22, heure d'Abidjan = UTC+0).
const REPORT_HOURS = Array.from({ length: 17 }, (_, i) => i + 6);

// Défauts affichés si le serveur renvoie une valeur vide (première ouverture).
const DEFAULT_OFFLINE_SEC = 135;
const DEFAULT_LOW_STOCK = 25;

/** Formulaire local = réglages serveur + champs secrets saisis (jamais renvoyés par l'API). */
interface NotifForm extends NotifSettings {
  telegramBotToken: string;
  whatsappToken: string;
  smtpPass: string;
}

function toForm(settings: NotifSettings): NotifForm {
  return {
    ...settings,
    offlineAfterSec: settings.offlineAfterSec > 0 ? settings.offlineAfterSec : DEFAULT_OFFLINE_SEC,
    lowStockThreshold: settings.lowStockThreshold > 0 ? settings.lowStockThreshold : DEFAULT_LOW_STOCK,
    telegramBotToken: "",
    whatsappToken: "",
    smtpPass: "",
  };
}

/** Corps du PUT — les secrets restent absents (undefined) quand l'utilisateur les laisse vides. */
function toPayload(form: NotifForm) {
  return {
    enabled: form.enabled,
    telegramEnabled: form.telegramEnabled,
    telegramChatId: form.telegramChatId.trim(),
    whatsappEnabled: form.whatsappEnabled,
    whatsappPhoneId: form.whatsappPhoneId.trim(),
    whatsappTo: form.whatsappTo.trim(),
    emailEnabled: form.emailEnabled,
    smtpHost: form.smtpHost.trim(),
    smtpPort: Math.min(65535, Math.max(1, Math.round(form.smtpPort || 587))),
    smtpUser: form.smtpUser.trim(),
    emailTo: form.emailTo.trim(),
    offlineAfterSec: Math.max(60, Math.round(form.offlineAfterSec || DEFAULT_OFFLINE_SEC)),
    lowStockThreshold: Math.max(1, Math.round(form.lowStockThreshold || DEFAULT_LOW_STOCK)),
    dailyReport: form.dailyReport,
    reportHour: form.reportHour,
    telegramBotToken: form.telegramBotToken.trim() || undefined,
    whatsappToken: form.whatsappToken.trim() || undefined,
    smtpPass: form.smtpPass || undefined,
  };
}

export default function NotificationsView() {
  const { data, isLoading, isError, error, refetch, isRefetching } = useQuery({
    queryKey: ["/api/notifications"],
    queryFn: () => api<NotifSettings>("/api/notifications"),
    retry: 1,
  });

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title="Notifications"
        description="Recevez les alertes routeur hors ligne, stock bas et le rapport quotidien — un gérant ne doit pas découvrir une panne par ses clients."
      />

      {isLoading ? (
        <LoadingCards cards={3} />
      ) : isError ? (
        <NotifErrorCard error={error} onRetry={() => void refetch()} retrying={isRefetching} />
      ) : data ? (
        <>
          <NotificationsForm initial={data} />
          <NotifLogCard />
        </>
      ) : null}
    </div>
  );
}

/* ─────────────────────────── État d'erreur (backend pas encore déployé) ─────────────────────────── */

function NotifErrorCard({
  error,
  onRetry,
  retrying,
}: {
  error: Error | null;
  onRetry: () => void;
  retrying: boolean;
}) {
  const status = error instanceof ApiError ? error.status : undefined;
  const unavailable = status === 404 || status === 501;
  const message = unavailable
    ? "Le module Notifications n'est pas encore disponible sur le serveur (déploiement du backend en cours)."
    : (error?.message ?? "Impossible de charger les réglages des notifications.");

  return (
    <Card className="border-destructive/30">
      <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
        <span className="flex size-12 items-center justify-center rounded-xl bg-destructive/10 text-destructive">
          <TriangleAlert className="size-6" />
        </span>
        <div>
          <p className="text-sm font-medium">Notifications indisponibles</p>
          <p className="mt-1 max-w-md text-xs text-muted-foreground">{message}</p>
        </div>
        <Button variant="outline" className="min-h-10" onClick={onRetry} disabled={retrying}>
          <RefreshCw className={cn("size-4", retrying && "animate-spin")} />
          Réessayer
        </Button>
      </CardContent>
    </Card>
  );
}

/* ─────────────────────────── Réglages + canaux ─────────────────────────── */

function NotificationsForm({ initial }: { initial: NotifSettings }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<NotifForm>(() => toForm(initial));

  const saveMutation = useMutation({
    mutationFn: () => api<NotifSettings>("/api/notifications", { method: "PUT", body: toPayload(form) }),
    onSuccess: (saved) => {
      toast.success("Réglages des notifications enregistrés");
      setForm(toForm(saved));
      void queryClient.invalidateQueries({ queryKey: ["/api/notifications"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const testMutation = useMutation({
    mutationFn: (channel: NotifChannel) =>
      api<{ ok: boolean }>("/api/notifications/test", { method: "POST", body: { channel } }),
    onSuccess: () => toast.success("Message de test envoyé"),
    onError: (err: Error) => toast.error(err.message || "Test impossible"),
  });

  // Un canal est « configurable pour test » si activé, renseigné et prêt.
  const telegramConfigured =
    (form.telegramBotTokenSet || form.telegramBotToken.trim() !== "") && form.telegramChatId.trim() !== "";
  const whatsappConfigured =
    (form.whatsappTokenSet || form.whatsappToken.trim() !== "") &&
    form.whatsappPhoneId.trim() !== "" &&
    form.whatsappTo.trim() !== "";
  const emailConfigured =
    form.smtpHost.trim() !== "" && form.smtpPort > 0 && form.emailTo.trim() !== "";

  return (
    <div className="space-y-4 sm:space-y-6">
      {/* ─── Card Alertes : interrupteur général + seuils + rapport quotidien ─── */}
      <Card className="gap-4 py-4 sm:py-6">
        <CardHeader className="flex flex-row items-start justify-between gap-3 px-4 sm:px-6">
          <div className="min-w-0">
            <CardTitle className="flex items-center gap-2 text-base">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
                <BellRing className="size-4" />
              </span>
              Alertes
            </CardTitle>
            <CardDescription>
              Surveillance de vos routeurs et de votre stock de vouchers, rapport quotidien de l&apos;activité.
            </CardDescription>
          </div>
          <Button
            className="min-h-10 shrink-0"
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending}
          >
            {saveMutation.isPending && <Loader2 className="size-4 animate-spin" />}
            Enregistrer
          </Button>
        </CardHeader>

        <CardContent className="grid gap-4 px-4 sm:grid-cols-2 sm:px-6">
          {/* Interrupteur général */}
          <div className="flex items-center justify-between gap-4 rounded-lg border p-3 sm:col-span-2">
            <div className="min-w-0">
              <Label htmlFor="notif-enabled" className="font-medium">
                Activer les notifications
              </Label>
              <p className="mt-0.5 text-xs text-muted-foreground">
                Interrupteur général : aucune alerte n&apos;est envoyée s&apos;il est désactivé.
              </p>
            </div>
            <Switch
              id="notif-enabled"
              checked={form.enabled}
              onCheckedChange={(v) => setForm((f) => ({ ...f, enabled: v }))}
            />
          </div>

          {/* Seuil routeur hors ligne */}
          <div className="grid gap-2">
            <Label htmlFor="notif-offline">Routeur hors ligne après (secondes sans check-in)</Label>
            <Input
              id="notif-offline"
              type="number"
              min={60}
              inputMode="numeric"
              value={form.offlineAfterSec || ""}
              onChange={(e) =>
                setForm((f) => ({ ...f, offlineAfterSec: e.target.value === "" ? 0 : Number(e.target.value) }))
              }
            />
            <p className="text-xs text-muted-foreground">3 × 45 s = 135 s recommandé.</p>
          </div>

          {/* Seuil stock vouchers */}
          <div className="grid gap-2">
            <Label htmlFor="notif-stock">Seuil de stock de vouchers</Label>
            <Input
              id="notif-stock"
              type="number"
              min={1}
              inputMode="numeric"
              value={form.lowStockThreshold || ""}
              onChange={(e) =>
                setForm((f) => ({ ...f, lowStockThreshold: e.target.value === "" ? 0 : Number(e.target.value) }))
              }
            />
            <p className="text-xs text-muted-foreground">
              Alerte quand il reste moins de ce nombre de vouchers actifs (défaut 25).
            </p>
          </div>

          {/* Rapport quotidien */}
          <div className="flex flex-wrap items-center justify-between gap-4 rounded-lg border p-3 sm:col-span-2">
            <div className="min-w-0">
              <Label htmlFor="notif-daily" className="font-medium">
                Rapport quotidien
              </Label>
              <p className="mt-0.5 text-xs text-muted-foreground">
                Envoyé à l&apos;heure d&apos;Abidjan (UTC+0) — ventes du jour, nouveaux utilisateurs, routeurs en
                ligne, stock restant.
              </p>
            </div>
            <div className="flex shrink-0 items-center gap-3">
              <Select
                value={String(form.reportHour)}
                onValueChange={(v) => setForm((f) => ({ ...f, reportHour: Number(v) }))}
                disabled={!form.dailyReport}
              >
                <SelectTrigger className="h-10 w-28" aria-label="Heure d'envoi du rapport quotidien">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {REPORT_HOURS.map((hour) => (
                    <SelectItem key={hour} value={String(hour)}>
                      {String(hour).padStart(2, "0")} h 00
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Switch
                id="notif-daily"
                checked={form.dailyReport}
                onCheckedChange={(v) => setForm((f) => ({ ...f, dailyReport: v }))}
              />
            </div>
          </div>
        </CardContent>
      </Card>

      {/* ─── Canaux de notification ─── */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        {/* Telegram */}
        <ChannelCard
          icon={Send}
          title="Telegram"
          description="Alertes instantanées sur votre téléphone via un bot Telegram."
          enabled={form.telegramEnabled}
          onEnabledChange={(v) => setForm((f) => ({ ...f, telegramEnabled: v }))}
          canTest={telegramConfigured}
          testing={testMutation.isPending && testMutation.variables === "telegram"}
          onTest={() => testMutation.mutate("telegram")}
        >
          <div className="grid gap-2">
            <Label htmlFor="tg-token">Bot token</Label>
            <Input
              id="tg-token"
              type="password"
              autoComplete="off"
              placeholder={
                form.telegramBotTokenSet ? "••••• configuré — laissez vide pour conserver" : "123456789:AA…"
              }
              value={form.telegramBotToken}
              onChange={(e) => setForm((f) => ({ ...f, telegramBotToken: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="tg-chat">Chat ID</Label>
            <Input
              id="tg-chat"
              inputMode="numeric"
              placeholder="ex. 123456789"
              value={form.telegramChatId}
              onChange={(e) => setForm((f) => ({ ...f, telegramChatId: e.target.value }))}
            />
          </div>
          <div className="rounded-lg bg-muted/50 p-3 text-[11px] leading-relaxed text-muted-foreground">
            <p>1. Créez un bot avec @BotFather → copiez le token.</p>
            <p>2. Envoyez un message à votre bot.</p>
            <p>3. Récupérez votre chat ID via @userinfobot.</p>
          </div>
        </ChannelCard>

        {/* WhatsApp Cloud API */}
        <ChannelCard
          icon={MessageCircle}
          title="WhatsApp Cloud API"
          description="Envoi via l'API Cloud WhatsApp de Meta (compte Business)."
          enabled={form.whatsappEnabled}
          onEnabledChange={(v) => setForm((f) => ({ ...f, whatsappEnabled: v }))}
          canTest={whatsappConfigured}
          testing={testMutation.isPending && testMutation.variables === "whatsapp"}
          onTest={() => testMutation.mutate("whatsapp")}
        >
          <div className="grid gap-2">
            <Label htmlFor="wa-token">Access token Meta</Label>
            <Input
              id="wa-token"
              type="password"
              autoComplete="off"
              placeholder={
                form.whatsappTokenSet ? "••••• configuré — laissez vide pour conserver" : "EAAG… (token permanent)"
              }
              value={form.whatsappToken}
              onChange={(e) => setForm((f) => ({ ...f, whatsappToken: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="wa-phone-id">Phone Number ID</Label>
            <Input
              id="wa-phone-id"
              placeholder="ex. 123456789012345"
              value={form.whatsappPhoneId}
              onChange={(e) => setForm((f) => ({ ...f, whatsappPhoneId: e.target.value }))}
            />
            <p className="text-xs text-muted-foreground">Meta → WhatsApp → API Setup → Phone number ID.</p>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="wa-to">Numéro destinataire</Label>
            <Input
              id="wa-to"
              inputMode="numeric"
              placeholder="2250701020304"
              value={form.whatsappTo}
              onChange={(e) => setForm((f) => ({ ...f, whatsappTo: e.target.value }))}
            />
            <p className="text-xs text-muted-foreground">
              Format international sans + ni espaces, ex. 2250701020304.
            </p>
          </div>
        </ChannelCard>

        {/* Email SMTP */}
        <ChannelCard
          icon={Mail}
          title="Email SMTP"
          description="Alertes et rapports par e-mail via votre serveur SMTP."
          enabled={form.emailEnabled}
          onEnabledChange={(v) => setForm((f) => ({ ...f, emailEnabled: v }))}
          canTest={emailConfigured}
          testing={testMutation.isPending && testMutation.variables === "email"}
          onTest={() => testMutation.mutate("email")}
        >
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-[1fr_100px]">
            <div className="grid gap-2">
              <Label htmlFor="smtp-host">Serveur SMTP</Label>
              <Input
                id="smtp-host"
                placeholder="smtp.gmail.com"
                value={form.smtpHost}
                onChange={(e) => setForm((f) => ({ ...f, smtpHost: e.target.value }))}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="smtp-port">Port</Label>
              <Input
                id="smtp-port"
                type="number"
                min={1}
                max={65535}
                inputMode="numeric"
                placeholder="587"
                value={form.smtpPort || ""}
                onChange={(e) =>
                  setForm((f) => ({ ...f, smtpPort: e.target.value === "" ? 0 : Number(e.target.value) }))
                }
              />
            </div>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="smtp-user">Utilisateur</Label>
            <Input
              id="smtp-user"
              autoComplete="off"
              placeholder="alertes@mondomaine.ci"
              value={form.smtpUser}
              onChange={(e) => setForm((f) => ({ ...f, smtpUser: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="smtp-pass">Mot de passe</Label>
            <Input
              id="smtp-pass"
              type="password"
              autoComplete="new-password"
              placeholder={
                form.smtpPassSet ? "••••• configuré — laissez vide pour conserver" : "••••••••"
              }
              value={form.smtpPass}
              onChange={(e) => setForm((f) => ({ ...f, smtpPass: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="smtp-to">Destinataire</Label>
            <Input
              id="smtp-to"
              type="email"
              placeholder="gerant@mondomaine.ci"
              value={form.emailTo}
              onChange={(e) => setForm((f) => ({ ...f, emailTo: e.target.value }))}
            />
          </div>
        </ChannelCard>
      </div>
    </div>
  );
}

/* ─────────────────────────── Carte d'un canal ─────────────────────────── */

function ChannelCard({
  icon: Icon,
  title,
  description,
  enabled,
  onEnabledChange,
  children,
  canTest,
  onTest,
  testing,
}: {
  icon: LucideIcon;
  title: string;
  description: string;
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
  children: React.ReactNode;
  canTest: boolean;
  onTest: () => void;
  testing: boolean;
}) {
  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center justify-between gap-2 text-base">
          <span className="flex min-w-0 items-center gap-2">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
              <Icon className="size-4" />
            </span>
            <span className="truncate">{title}</span>
          </span>
          <Switch checked={enabled} onCheckedChange={onEnabledChange} aria-label={`Activer ${title}`} />
        </CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">{children}</CardContent>
      <CardFooter className="px-4 sm:px-6">
        <Button
          variant="outline"
          className="min-h-10 w-full"
          disabled={!enabled || !canTest || testing}
          onClick={onTest}
        >
          {testing ? <Loader2 className="size-4 animate-spin" /> : <Send className="size-4" />}
          Envoyer un test
        </Button>
      </CardFooter>
    </Card>
  );
}

/* ─────────────────────────── Historique des notifications ─────────────────────────── */

const CHANNEL_LABELS: Record<NotifLogEntry["channel"], string> = {
  telegram: "Telegram",
  whatsapp: "WhatsApp",
  email: "E-mail",
  system: "Système",
};

const KIND_LABELS: Record<NotifLogEntry["kind"], string> = {
  router_offline: "Routeur hors ligne",
  router_back: "Routeur de retour",
  low_stock: "Stock bas",
  daily_report: "Rapport quotidien",
  test: "Test",
  settings: "Réglages",
};

function NotifLogCard() {
  const { data, isLoading, isError, error, refetch, isRefetching } = useQuery({
    queryKey: ["/api/notifications/log"],
    queryFn: () => api<NotifLogEntry[]>("/api/notifications/log"),
    refetchInterval: 15_000,
  });
  const entries = data ?? [];

  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b px-4 py-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <History className="size-4" />
          </span>
          Historique des notifications
        </CardTitle>
        <CardDescription>Les 50 derniers envois — alertes, rapports et tests.</CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        {isLoading ? (
          <LoadingRows rows={6} />
        ) : isError ? (
          <div className="flex flex-col items-center gap-3 px-6 py-10 text-center">
            <p className="text-sm text-muted-foreground">
              Historique indisponible : {error instanceof Error ? error.message : "erreur inconnue"}
            </p>
            <Button variant="outline" className="min-h-10" onClick={() => void refetch()} disabled={isRefetching}>
              <RefreshCw className={cn("size-4", isRefetching && "animate-spin")} />
              Réessayer
            </Button>
          </div>
        ) : entries.length === 0 ? (
          <EmptyState
            icon={BellOff}
            title="Aucune notification envoyée pour le moment"
            description="Les alertes, rapports et tests envoyés apparaîtront ici."
          />
        ) : (
          <div className="max-h-96 overflow-auto">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="pl-4 text-muted-foreground sm:pl-6">Heure</TableHead>
                  <TableHead className="text-muted-foreground">Canal</TableHead>
                  <TableHead className="text-muted-foreground">Type</TableHead>
                  <TableHead className="text-muted-foreground">Titre</TableHead>
                  <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">Statut</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {entries.map((entry) => (
                  <TableRow key={entry.id}>
                    <TableCell className="whitespace-nowrap pl-4 tabular-nums text-muted-foreground sm:pl-6">
                      {formatDateTime(entry.at)}
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline" className="max-w-28 truncate">
                        {CHANNEL_LABELS[entry.channel] ?? entry.channel}
                      </Badge>
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-muted-foreground">
                      {KIND_LABELS[entry.kind] ?? entry.kind}
                    </TableCell>
                    <TableCell className="max-w-64">
                      <span className="line-clamp-1" title={entry.title}>
                        {entry.title}
                      </span>
                    </TableCell>
                    <TableCell className="pr-4 text-right sm:pr-6">
                      {entry.status === "sent" ? (
                        <Badge
                          variant="outline"
                          className="border-emerald-500/40 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                        >
                          Envoyé
                        </Badge>
                      ) : (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Badge variant="destructive" className="cursor-help">
                              Échec
                            </Badge>
                          </TooltipTrigger>
                          <TooltipContent className="max-w-64">
                            <p className="whitespace-pre-wrap break-words">
                              {entry.error || "Erreur inconnue"}
                            </p>
                          </TooltipContent>
                        </Tooltip>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
