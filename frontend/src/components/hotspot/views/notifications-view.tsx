"use client";

// N°57-d — Vue « Notifications » de la zone Paramètres
// (/app/settings/notifications), réorganisée en TROIS domaines nommés —
// SANS onglet interne, tout visible d'un coup d'œil :
//   1. Règles d'alerte — interrupteur, seuils (routeur hors ligne, stock),
//      rapport quotidien (carte « Alertes », enregistrement global) ;
//   2. Webhooks & canaux — les destinations : Telegram, WhatsApp Cloud API,
//      e-mail (SMTP direct ou API Resend N°67 — cartes canaux + test d'envoi) ;
//   3. Historique des envois — journal réel (GET /api/notifications/log).
// Contrat API : GET/PUT /api/notifications, POST /api/notifications/test,
// GET /api/notifications/log (voir lib/hotspot/types.ts).

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BellOff,
  BellRing,
  Building2,
  CheckCircle2,
  ChevronDown,
  ExternalLink,
  History,
  Loader2,
  Mail,
  MessageCircle,
  RefreshCw,
  Send,
  TriangleAlert,
  Webhook,
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
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
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
  resendApiKey: string;
}

function toForm(settings: NotifSettings): NotifForm {
  return {
    ...settings,
    // Le serveur normalise déjà, défense en profondeur côté formulaire.
    emailProvider: settings.emailProvider === "resend" ? "resend" : "smtp",
    offlineAfterSec: settings.offlineAfterSec > 0 ? settings.offlineAfterSec : DEFAULT_OFFLINE_SEC,
    lowStockThreshold: settings.lowStockThreshold > 0 ? settings.lowStockThreshold : DEFAULT_LOW_STOCK,
    telegramBotToken: "",
    whatsappToken: "",
    smtpPass: "",
    resendApiKey: "",
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
    emailProvider: form.emailProvider,
    smtpHost: form.smtpHost.trim(),
    smtpPort: Math.min(65535, Math.max(1, Math.round(form.smtpPort || 587))),
    smtpUser: form.smtpUser.trim(),
    emailTo: form.emailTo.trim(),
    resendFrom: form.resendFrom.trim(),
    offlineAfterSec: Math.max(60, Math.round(form.offlineAfterSec || DEFAULT_OFFLINE_SEC)),
    lowStockThreshold: Math.max(1, Math.round(form.lowStockThreshold || DEFAULT_LOW_STOCK)),
    dailyReport: form.dailyReport,
    reportHour: form.reportHour,
    telegramBotToken: form.telegramBotToken.trim() || undefined,
    whatsappToken: form.whatsappToken.trim() || undefined,
    smtpPass: form.smtpPass || undefined,
    resendApiKey: form.resendApiKey.trim() || undefined,
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
      <PageHeader
        title={t("notif.title")}
        description={data?.isPlatformAccount ? t("notif.platformDescription") : t("notif.description")}
      />

      {isLoading ? (
        <LoadingCards cards={3} />
      ) : isError ? (
        <NotifErrorCard error={error} onRetry={() => void refetch()} retrying={isRefetching} />
      ) : data ? (
        <>
          {/* N°153 — console du compte principal : bandeau différenciant avant
              tout le reste (le super-admin pilote les canaux PARTAGÉS). */}
          {data.isPlatformAccount && (
            <PlatformAccountBanner
              relayActive={data.emailPlatformRelay === true}
              botUsername={data.telegramBotUsername}
            />
          )}
          <NotificationsForm initial={data} />
          <NotifLogCard />
        </>
      ) : null}
    </div>
  );
}

/* ─────────────── N°153 — bandeau « compte principal de la plateforme » ─────────────── */

// La console du super-admin (sur SON compte, acc-main) remplace le discours
// client « relais disponible » par « VOS identifiants portent le relais » :
// les réglages e-mail d'ici envoient les alertes de tous les clients sans
// configuration propre, et le bot Telegram officiel est partagé. Le statut
// du relais (actif / à configurer) est calculé sur emailPlatformRelay —
// vrai dès que CES réglages portent des identifiants exploitables.
function PlatformAccountBanner({
  relayActive,
  botUsername,
}: {
  relayActive: boolean;
  botUsername?: string;
}) {
  const { t, tf } = useI18n();
  return (
    <Card className="gap-0 border-emerald-600/25 bg-emerald-500/5 py-0">
      <CardContent className="flex items-start gap-3 p-4 sm:p-6">
        <span
          className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"
          aria-hidden
        >
          <Building2 className="size-5" />
        </span>
        <div className="min-w-0 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-sm font-semibold text-emerald-700 dark:text-emerald-400">
              {t("notif.platformBannerTitle")}
            </h2>
            <Badge
              variant="outline"
              className="border-emerald-600/30 bg-emerald-500/10 text-[10px] font-semibold uppercase tracking-widest text-emerald-700 dark:text-emerald-400"
            >
              FTCI
            </Badge>
          </div>
          <p className="text-sm leading-relaxed text-foreground/80">
            {botUsername
              ? tf("notif.platformBannerBody", { bot: `@${botUsername}` })
              : t("notif.platformBannerBodyNoBot")}
          </p>
          {relayActive ? (
            <p className="flex items-center gap-2 text-xs font-medium text-emerald-700 dark:text-emerald-400">
              <CheckCircle2 className="size-4 shrink-0" aria-hidden />
              {t("notif.platformRelayActive")}
            </p>
          ) : (
            <p className="flex items-start gap-2 rounded-lg border border-amber-600/25 bg-amber-500/10 p-2.5 text-xs leading-relaxed text-amber-800 dark:text-amber-300">
              <TriangleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
              {t("notif.platformRelayInactive")}
            </p>
          )}
        </div>
      </CardContent>
    </Card>
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

/* ─────────────────────────── En-tête de domaine (N°57-d) ─────────────────────────── */

/** Titre de section — même gabarit que les cartes (pastille icône + libellé
 * + description) : les trois domaines de la page se lisent d'un coup d'œil. */
function SectionHeading({
  icon: Icon,
  title,
  description,
}: {
  icon: LucideIcon;
  title: string;
  description: string;
}) {
  return (
    <div className="flex items-start gap-3">
      <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
        <Icon className="size-4" aria-hidden />
      </span>
      <div className="min-w-0">
        <h2 className="text-base font-semibold tracking-tight">{title}</h2>
        <p className="mt-0.5 text-sm text-muted-foreground">{description}</p>
      </div>
    </div>
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

  // N°150 — pairage Telegram plateforme (lien magique) : demande de code,
  // ouverture de Telegram, polling du statut jusqu'à liaison (ou expiration).
  // Zéro useEffect (règle react-hooks/set-state-in-effect) : la liaison est
  // SYNCHRONISÉE PENDANT LE RENDU (patron officiel React « ajuster l'état
  // quand une valeur externe change », garde syncedChatId) et l'expiration
  // se rend EN LIGNE (bouton « nouveau code ») plutôt qu'en toast.
  const [pair, setPair] = useState<{ code: string; url: string } | null>(null);
  const pairMutation = useMutation({
    mutationFn: () =>
      api<{ code: string; url: string; botUsername: string; expiresAt: string }>(
        "/api/notifications/telegram/pair-code",
        { method: "POST", body: {} },
      ),
    onSuccess: (data) => {
      setPair({ code: data.code, url: data.url });
      window.open(data.url, "_blank", "noopener,noreferrer");
    },
    onError: (err: Error) => toast.error(err.message || t("notif.tgPairError")),
  });
  const pairQuery = useQuery({
    queryKey: ["/api/notifications/telegram/pair-status", pair?.code],
    queryFn: () =>
      api<{ status: "pending" | "linked" | "expired"; chatId?: string }>(
        `/api/notifications/telegram/pair-status?code=${encodeURIComponent(pair?.code ?? "")}`,
      ),
    enabled: !!pair && !form.telegramChatId,
    refetchInterval: (query) =>
      query.state.data?.status === "pending" ? 3000 : false,
    retry: false,
  });
  const linkedFromQuery = pairQuery.data?.status === "linked" ? pairQuery.data.chatId ?? "" : "";
  const pairExpired = pair !== null && pairQuery.data?.status === "expired";
  const [syncedChatId, setSyncedChatId] = useState<string | null>(null);
  if (linkedFromQuery && linkedFromQuery !== syncedChatId) {
    setSyncedChatId(linkedFromQuery);
    setForm((f) =>
      f.telegramChatId === linkedFromQuery
        ? f
        : { ...f, telegramEnabled: true, telegramChatId: linkedFromQuery },
    );
  }
  const telegramLinked = form.telegramChatId.trim() !== "";

  // N°153 — le porteur est le compte principal de la plateforme : la carte
  // e-mail devient « E-mail plateforme » (identifiants OUVERTS par défaut,
  // ils portent le relais) et la note relais client disparaît.
  const isPlatformAccount = initial.isPlatformAccount === true;

  // Un canal est « configurable pour test » si activé, renseigné et prêt.
  // N°150 — le bot FTCI et le relais e-mail du compte principal comptent
  // comme identifiants (le serveur a la même lecture, ConfiguredWithPlatform).
  const telegramConfigured =
    form.telegramChatId.trim() !== "" &&
    (form.telegramBotTokenSet ||
      form.telegramBotToken.trim() !== "" ||
      initial.telegramPlatformAvailable === true);
  const whatsappConfigured =
    (form.whatsappTokenSet || form.whatsappToken.trim() !== "") &&
    form.whatsappPhoneId.trim() !== "" &&
    form.whatsappTo.trim() !== "";
  // Email : les champs requis dépendent du fournisseur (N°67) — SMTP a besoin
  // d'un hôte + port, Resend d'une clé API (déjà stockée ou fraîchement saisie).
  // N°150 : sans identifiants propres, le relais plateforme suffit.
  const emailResendConfigured =
    (form.resendApiKeySet || form.resendApiKey.trim() !== "") && form.emailTo.trim() !== "";
  const emailOwnConfigured =
    form.emailProvider === "resend"
      ? emailResendConfigured
      : form.smtpHost.trim() !== "" && form.smtpPort > 0 && form.emailTo.trim() !== "";
  const emailConfigured = emailOwnConfigured || (initial.emailPlatformRelay === true && form.emailTo.trim() !== "");

  return (
    <div className="space-y-4 sm:space-y-6">
      {/* ─── Domaine 1 : règles d'alerte (interrupteur + seuils + rapport) ─── */}
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

      {/* ─── Domaine 2 : webhooks & canaux de diffusion ─── */}
      <SectionHeading icon={Webhook} title={t("notif.section.channels")} description={t("notif.section.channelsDesc")} />
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        {/* Telegram — N°150 : bot plateforme « zéro setup » (lien magique)
            quand le service l'expose, BYO repliable ; sinon carte BYO pleine. */}
        <ChannelCard
          icon={Send}
          title="Telegram"
          description={initial.telegramPlatformAvailable ? t("notif.tgPlatformDesc") : t("notif.tgDesc")}
          enabled={form.telegramEnabled}
          onEnabledChange={(v) => setForm((f) => ({ ...f, telegramEnabled: v }))}
          canTest={telegramConfigured}
          testing={testMutation.isPending && testMutation.variables === "telegram"}
          onTest={() => testMutation.mutate("telegram")}
        >
          {initial.telegramPlatformAvailable && (
            <div className="rounded-lg border border-primary/20 bg-primary/5 p-3">
              {telegramLinked ? (
                <div className="flex items-start gap-2.5">
                  <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-primary" />
                  <div className="min-w-0">
                    <p className="text-sm font-medium">{t("notif.tgLinked")}</p>
                    <p className="mt-0.5 text-xs text-muted-foreground">{t("notif.tgLinkedHint")}</p>
                    <Badge variant="outline" className="mt-2 border-primary/25 bg-primary/10 font-mono text-[11px] text-primary">
                      {form.telegramChatId}
                    </Badge>
                  </div>
                </div>
              ) : (
                <div className="grid gap-2">
                  <p className="text-xs leading-relaxed text-muted-foreground">
                    {t("notif.tgPlatformNote")}
                  </p>
                  {pair ? (
                    <div className="grid gap-2">
                      {pairExpired ? (
                        <p className="text-sm font-medium text-destructive">{t("notif.tgPairExpired")}</p>
                      ) : (
                        <p className="text-sm font-medium">{t("notif.tgPending")}</p>
                      )}
                      {!pairExpired && (
                        <div className="flex items-center gap-2 rounded-md bg-background px-3 py-2 font-mono text-xs tracking-widest">
                          <span aria-hidden>⏳</span>
                          {pair.code}
                        </div>
                      )}
                      <div className="flex flex-wrap gap-2">
                        {pairExpired ? (
                          <Button
                            type="button"
                            className="h-8 min-h-8 text-xs"
                            onClick={() => pairMutation.mutate()}
                            disabled={pairMutation.isPending}
                          >
                            {pairMutation.isPending ? <Loader2 className="size-3.5 animate-spin" /> : <RefreshCw className="size-3.5" />}
                            {t("notif.tgNewCode")}
                          </Button>
                        ) : (
                          <Button
                            type="button"
                            variant="outline"
                            className="h-8 min-h-8 text-xs"
                            onClick={() => window.open(pair.url, "_blank", "noopener,noreferrer")}
                          >
                            <ExternalLink className="size-3.5" />
                            {t("notif.tgOpenAgain")}
                          </Button>
                        )}
                        <Button
                          type="button"
                          variant="ghost"
                          className="h-8 min-h-8 text-xs"
                          onClick={() => setPair(null)}
                        >
                          {t("notif.tgCancel")}
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <Button
                      type="button"
                      className="min-h-10 w-full"
                      onClick={() => pairMutation.mutate()}
                      disabled={pairMutation.isPending}
                    >
                      {pairMutation.isPending ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <Send className="size-4" />
                      )}
                      {t("notif.tgConnect")}
                    </Button>
                  )}
                </div>
              )}
            </div>
          )}

          {/* BYO — replié par défaut quand la plateforme est dispo et qu'aucun
              bot propre n'est configuré ; ouvert sinon (comportement historique). */}
          <Collapsible
            defaultOpen={!initial.telegramPlatformAvailable || form.telegramBotTokenSet}
          >
            <CollapsibleTrigger className="group flex w-full items-center justify-between gap-2 rounded-md text-xs font-medium text-muted-foreground transition-colors hover:text-foreground">
              <span>{initial.telegramPlatformAvailable ? t("notif.tgAdvanced") : t("notif.tgManual")}</span>
              <ChevronDown className="size-3.5 shrink-0 transition-transform group-data-[state=open]:rotate-180" />
            </CollapsibleTrigger>
            <CollapsibleContent className="grid gap-4 pt-3">
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
              {!initial.telegramPlatformAvailable && (
                <div className="rounded-lg bg-muted/50 p-3 text-[11px] leading-relaxed text-muted-foreground">
                  <p>{t("notif.tgHelp1")}</p>
                  <p>{t("notif.tgHelp2")}</p>
                  <p>{t("notif.tgHelp3")}</p>
                </div>
              )}
            </CollapsibleContent>
          </Collapsible>
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

        {/* Email — SMTP direct ou API Resend (N°67) ; relais plateforme N°150 :
            adresse + interrupteur suffisent quand le service porte l'envoi.
            N°153 — compte principal : carte « E-mail plateforme », les
            identifiants d'ici portent le relais de TOUS les clients. */}
        <ChannelCard
          icon={Mail}
          title={isPlatformAccount ? t("notif.emailPlatformCardTitle") : t("notif.emailCardTitle")}
          description={isPlatformAccount ? t("notif.emailPlatformCardDesc") : t("notif.emailDesc")}
          enabled={form.emailEnabled}
          onEnabledChange={(v) => setForm((f) => ({ ...f, emailEnabled: v }))}
          canTest={emailConfigured}
          testing={testMutation.isPending && testMutation.variables === "email"}
          onTest={() => testMutation.mutate("email")}
        >
          {/* Destinataire — commun à tous les fournisseurs, toujours visible. */}
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

          {/* N°150 — relais plateforme : note quand le compte n'a pas
              d'identifiants propres (jamais pour le compte principal,
              qui EST la source du relais). */}
          {!isPlatformAccount && initial.emailPlatformRelay && !emailOwnConfigured && (
            <p className="rounded-lg border border-primary/20 bg-primary/5 p-3 text-xs leading-relaxed text-muted-foreground">
              {t("notif.emailRelayNote")}
            </p>
          )}

          {/* Fournisseur + identifiants — repliés quand le relais suffit ;
              OUVERTS pour le compte principal (c'est SA configuration).
              Le libellé du repli suit la console (client : « avancé »). */}
          <Collapsible defaultOpen={isPlatformAccount || !initial.emailPlatformRelay || emailOwnConfigured}>
            <CollapsibleTrigger className="group flex w-full items-center justify-between gap-2 rounded-md text-xs font-medium text-muted-foreground transition-colors hover:text-foreground">
              <span>{isPlatformAccount ? t("notif.emailPlatformConfig") : t("notif.emailAdvanced")}</span>
              <ChevronDown className="size-3.5 shrink-0 transition-transform group-data-[state=open]:rotate-180" />
            </CollapsibleTrigger>
            <CollapsibleContent className="grid gap-4 pt-3">
              {/* Fournisseur du canal e-mail : SMTP direct (défaut) ou API Resend. */}
              <div className="grid gap-2">
                <Label htmlFor="email-provider">{t("notif.emailProvider")}</Label>
                <Select
                  value={form.emailProvider}
                  onValueChange={(v) =>
                    setForm((f) => ({ ...f, emailProvider: v === "resend" ? "resend" : "smtp" }))
                  }
                >
                  <SelectTrigger id="email-provider" className="h-10">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="smtp">{t("notif.emailProviderSmtp")}</SelectItem>
                    <SelectItem value="resend">{t("notif.emailProviderResend")}</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {form.emailProvider === "resend" ? (
                <>
                  <div className="grid gap-2">
                    <Label htmlFor="resend-key">{t("notif.resendApiKey")}</Label>
                    <Input
                      id="resend-key"
                      type="password"
                      autoComplete="off"
                      placeholder={form.resendApiKeySet ? t("notif.secretConfigured") : "re_…"}
                      value={form.resendApiKey}
                      onChange={(e) => setForm((f) => ({ ...f, resendApiKey: e.target.value }))}
                    />
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="resend-from">{t("notif.resendFrom")}</Label>
                    <Input
                      id="resend-from"
                      placeholder="MikCloud <alertes@mondomaine.ci>"
                      value={form.resendFrom}
                      onChange={(e) => setForm((f) => ({ ...f, resendFrom: e.target.value }))}
                    />
                    <p className="text-xs text-muted-foreground">{t("notif.resendFromHint")}</p>
                  </div>
                </>
              ) : (
                <>
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
                </>
              )}
            </CollapsibleContent>
          </Collapsible>
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
