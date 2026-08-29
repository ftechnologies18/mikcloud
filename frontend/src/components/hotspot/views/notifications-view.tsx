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
import { useI18n } from "@/lib/hotspot/i18n";
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
  const { t } = useI18n();
  const { data, isLoading, isError, error, refetch, isRefetching } = useQuery({
    queryKey: ["/api/notifications"],
    queryFn: () => api<NotifSettings>("/api/notifications"),
    retry: 1,
  });

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader title={t("notif.title")} description={t("notif.description")} />

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
  const { t } = useI18n();
  const status = error instanceof ApiError ? error.status : undefined;
  const unavailable = status === 404 || status === 501;
  const message = unavailable ? t("notif.errorModule") : (error?.message ?? t("notif.errorLoad"));

  return (
    <Card className="border-destructive/30">
      <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
        <span className="flex size-12 items-center justify-center rounded-xl bg-destructive/10 text-destructive">
          <TriangleAlert className="size-6" />
        </span>
        <div>
          <p className="text-sm font-medium">{t("notif.errorTitle")}</p>
          <p className="mt-1 max-w-md text-xs text-muted-foreground">{message}</p>
        </div>
        <Button variant="outline" className="min-h-10" onClick={onRetry} disabled={retrying}>
          <RefreshCw className={cn("size-4", retrying && "animate-spin")} />
          {t("common.retry")}
        </Button>
      </CardContent>
    </Card>
  );
}

/* ─────────────────────────── Réglages + canaux ─────────────────────────── */

function NotificationsForm({ initial }: { initial: NotifSettings }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<NotifForm>(() => toForm(initial));

  const saveMutation = useMutation({
    mutationFn: () => api<NotifSettings>("/api/notifications", { method: "PUT", body: toPayload(form) }),
    onSuccess: (saved) => {
      toast.success(t("notif.saved"));
      setForm(toForm(saved));
      void queryClient.invalidateQueries({ queryKey: ["/api/notifications"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const testMutation = useMutation({
    mutationFn: (channel: NotifChannel) =>
      api<{ ok: boolean }>("/api/notifications/test", { method: "POST", body: { channel } }),
    onSuccess: () => toast.success(t("notif.testSent")),
    onError: (err: Error) => toast.error(err.message || t("notif.testFailed")),
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
              {t("notif.alerts")}
            </CardTitle>
            <CardDescription>{t("notif.alertsDesc")}</CardDescription>
          </div>
          <Button
            className="min-h-10 shrink-0"
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending}
          >
            {saveMutation.isPending && <Loader2 className="size-4 animate-spin" />}
            {t("common.save")}
          </Button>
        </CardHeader>

        <CardContent className="grid gap-4 px-4 sm:grid-cols-2 sm:px-6">
          {/* Interrupteur général */}
          <div className="flex items-center justify-between gap-4 rounded-lg border p-3 sm:col-span-2">
            <div className="min-w-0">
              <Label htmlFor="notif-enabled" className="font-medium">
                {t("notif.enable")}
              </Label>
              <p className="mt-0.5 text-xs text-muted-foreground">{t("notif.enableHint")}</p>
            </div>
            <Switch
              id="notif-enabled"
              checked={form.enabled}
              onCheckedChange={(v) => setForm((f) => ({ ...f, enabled: v }))}
            />
          </div>

          {/* Seuil routeur hors ligne */}
          <div className="grid gap-2">
            <Label htmlFor="notif-offline">{t("notif.offlineAfter")}</Label>
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
            <p className="text-xs text-muted-foreground">{t("notif.offlineHint")}</p>
          </div>

          {/* Seuil stock vouchers */}
          <div className="grid gap-2">
            <Label htmlFor="notif-stock">{t("notif.lowStock")}</Label>
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
            <p className="text-xs text-muted-foreground">{t("notif.lowStockHint")}</p>
          </div>

          {/* Rapport quotidien */}
          <div className="flex flex-wrap items-center justify-between gap-4 rounded-lg border p-3 sm:col-span-2">
            <div className="min-w-0">
              <Label htmlFor="notif-daily" className="font-medium">
                {t("notif.daily")}
              </Label>
              <p className="mt-0.5 text-xs text-muted-foreground">{t("notif.dailyHint")}</p>
            </div>
            <div className="flex shrink-0 items-center gap-3">
              <Select
                value={String(form.reportHour)}
                onValueChange={(v) => setForm((f) => ({ ...f, reportHour: Number(v) }))}
                disabled={!form.dailyReport}
              >
                <SelectTrigger className="h-10 w-28" aria-label={t("notif.dailyHour")}>
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
          description={t("notif.tgDesc")}
          enabled={form.telegramEnabled}
          onEnabledChange={(v) => setForm((f) => ({ ...f, telegramEnabled: v }))}
          canTest={telegramConfigured}
          testing={testMutation.isPending && testMutation.variables === "telegram"}
          onTest={() => testMutation.mutate("telegram")}
        >
          <div className="grid gap-2">
            <Label htmlFor="tg-token">{t("notif.botToken")}</Label>
            <Input
              id="tg-token"
              type="password"
              autoComplete="off"
              placeholder={form.telegramBotTokenSet ? t("notif.secretConfigured") : "123456789:AA…"}
              value={form.telegramBotToken}
              onChange={(e) => setForm((f) => ({ ...f, telegramBotToken: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="tg-chat">{t("notif.chatId")}</Label>
            <Input
              id="tg-chat"
              inputMode="numeric"
              placeholder={t("notif.chatIdPlaceholder")}
              value={form.telegramChatId}
              onChange={(e) => setForm((f) => ({ ...f, telegramChatId: e.target.value }))}
            />
          </div>
          <div className="rounded-lg bg-muted/50 p-3 text-[11px] leading-relaxed text-muted-foreground">
            <p>{t("notif.tgHelp1")}</p>
            <p>{t("notif.tgHelp2")}</p>
            <p>{t("notif.tgHelp3")}</p>
          </div>
        </ChannelCard>

        {/* WhatsApp Cloud API */}
        <ChannelCard
          icon={MessageCircle}
          title="WhatsApp Cloud API"
          description={t("notif.waDesc")}
          enabled={form.whatsappEnabled}
          onEnabledChange={(v) => setForm((f) => ({ ...f, whatsappEnabled: v }))}
          canTest={whatsappConfigured}
          testing={testMutation.isPending && testMutation.variables === "whatsapp"}
          onTest={() => testMutation.mutate("whatsapp")}
        >
          <div className="grid gap-2">
            <Label htmlFor="wa-token">{t("notif.waToken")}</Label>
            <Input
              id="wa-token"
              type="password"
              autoComplete="off"
              placeholder={form.whatsappTokenSet ? t("notif.secretConfigured") : "EAAG…"}
              value={form.whatsappToken}
              onChange={(e) => setForm((f) => ({ ...f, whatsappToken: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="wa-phone-id">{t("notif.waPhoneId")}</Label>
            <Input
              id="wa-phone-id"
              placeholder={t("notif.waPhonePlaceholder")}
              value={form.whatsappPhoneId}
              onChange={(e) => setForm((f) => ({ ...f, whatsappPhoneId: e.target.value }))}
            />
            <p className="text-xs text-muted-foreground">{t("notif.waPhoneHint")}</p>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="wa-to">{t("notif.waTo")}</Label>
            <Input
              id="wa-to"
              inputMode="numeric"
              placeholder="2250701020304"
              value={form.whatsappTo}
              onChange={(e) => setForm((f) => ({ ...f, whatsappTo: e.target.value }))}
            />
            <p className="text-xs text-muted-foreground">{t("notif.waToHint")}</p>
          </div>
        </ChannelCard>

        {/* Email SMTP */}
        <ChannelCard
          icon={Mail}
          title="Email SMTP"
          description={t("notif.emailDesc")}
          enabled={form.emailEnabled}
          onEnabledChange={(v) => setForm((f) => ({ ...f, emailEnabled: v }))}
          canTest={emailConfigured}
          testing={testMutation.isPending && testMutation.variables === "email"}
          onTest={() => testMutation.mutate("email")}
        >
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-[1fr_100px]">
            <div className="grid gap-2">
              <Label htmlFor="smtp-host">{t("notif.smtpHost")}</Label>
              <Input
                id="smtp-host"
                placeholder="smtp.gmail.com"
                value={form.smtpHost}
                onChange={(e) => setForm((f) => ({ ...f, smtpHost: e.target.value }))}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="smtp-port">{t("notif.smtpPort")}</Label>
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
            <Label htmlFor="smtp-user">{t("notif.smtpUser")}</Label>
            <Input
              id="smtp-user"
              autoComplete="off"
              placeholder="alertes@mondomaine.ci"
              value={form.smtpUser}
              onChange={(e) => setForm((f) => ({ ...f, smtpUser: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="smtp-pass">{t("notif.smtpPass")}</Label>
            <Input
              id="smtp-pass"
              type="password"
              autoComplete="new-password"
              placeholder={form.smtpPassSet ? t("notif.secretConfigured") : "••••••••"}
              value={form.smtpPass}
              onChange={(e) => setForm((f) => ({ ...f, smtpPass: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="smtp-to">{t("notif.recipient")}</Label>
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
  const { t, tf } = useI18n();
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
          <Switch checked={enabled} onCheckedChange={onEnabledChange} aria-label={tf("notif.enableChannel", { title })} />
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
          {t("notif.sendTest")}
        </Button>
      </CardFooter>
    </Card>
  );
}

/* ─────────────────────────── Historique des notifications ─────────────────────────── */

// Libellés des canaux/types : clés i18n, la valeur brute reste en repli.
const CHANNEL_KEYS: Record<NotifLogEntry["channel"], string> = {
  telegram: "notif.channel.telegram",
  whatsapp: "notif.channel.whatsapp",
  email: "notif.channel.email",
  system: "notif.channel.system",
};

const KIND_KEYS: Record<NotifLogEntry["kind"], string> = {
  router_offline: "notif.kind.router_offline",
  router_back: "notif.kind.router_back",
  low_stock: "notif.kind.low_stock",
  daily_report: "notif.kind.daily_report",
  test: "notif.kind.test",
  settings: "notif.kind.settings",
};

function NotifLogCard() {
  const { t, tf } = useI18n();
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
          {t("notif.logTitle")}
        </CardTitle>
        <CardDescription>{t("notif.logDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        {isLoading ? (
          <LoadingRows rows={6} />
        ) : isError ? (
          <div className="flex flex-col items-center gap-3 px-6 py-10 text-center">
            <p className="text-sm text-muted-foreground">
              {tf("notif.logUnavailable", {
                error: error instanceof Error ? error.message : t("notif.unknownError"),
              })}
            </p>
            <Button variant="outline" className="min-h-10" onClick={() => void refetch()} disabled={isRefetching}>
              <RefreshCw className={cn("size-4", isRefetching && "animate-spin")} />
              {t("common.retry")}
            </Button>
          </div>
        ) : entries.length === 0 ? (
          <EmptyState
            icon={BellOff}
            title={t("notif.logEmptyTitle")}
            description={t("notif.logEmptyDesc")}
          />
        ) : (
          <div className="max-h-96 overflow-auto">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("notif.logTime")}</TableHead>
                  <TableHead className="text-muted-foreground">{t("notif.logChannel")}</TableHead>
                  <TableHead className="text-muted-foreground">{t("notif.logKind")}</TableHead>
                  <TableHead className="text-muted-foreground">{t("notif.logTitleCol")}</TableHead>
                  <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("notif.logStatus")}</TableHead>
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
                        {t(CHANNEL_KEYS[entry.channel] ?? "", entry.channel)}
                      </Badge>
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-muted-foreground">
                      {t(KIND_KEYS[entry.kind] ?? "", entry.kind)}
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
                          {t("notif.sent")}
                        </Badge>
                      ) : (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Badge variant="destructive" className="cursor-help">
                              {t("notif.failed")}
                            </Badge>
                          </TooltipTrigger>
                          <TooltipContent className="max-w-64">
                            <p className="whitespace-pre-wrap break-words">
                              {entry.error || t("notif.unknownError")}
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
