"use client";

// Console plateforme — ANNONCES AUX CLIENTS (N°152, super-admin uniquement).
// Le megaphone du SaaS : diffuser une annonce à tous les comptes MikCloud
// (maintenance, nouveauté, incident) — bandeau dans la console de chaque
// destinataire + entrée dans sa cloche + e-mail optionnel aux propriétaires.
// N°165 — DIFFUSION PROGRAMMÉE : « Programmer une date » retarde l'apparition
// du bandeau (et l'e-mail) à l'instant choisi — une maintenance de samedi
// 04h se rédige vendredi matin. Le statut de chaque ligne dit si l'annonce
// est visible, programmée (publication automatique) ou expirée.
// Contrats : GET/POST /api/admin/announcements, DELETE /api/admin/announcements/{id}
// (voir lib/hotspot/types.ts et api.ts).

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Clock,
  Loader2,
  Megaphone,
  Send,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";

import {
  ApiError,
  createAnnouncement,
  deleteAnnouncement,
  fetchAnnouncements,
} from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type {
  AdminAnnouncementRow,
  AdminAnnouncementState,
  AnnouncementAudience,
  AnnouncementCreatePayload,
  AnnouncementLevel,
} from "@/lib/hotspot/types";
import { ANNOUNCEMENT_LEVELS } from "@/lib/hotspot/types";
import { EmptyState } from "@/components/hotspot/empty-state";
import { PageHeader } from "@/components/hotspot/page-header";
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
import {
  Card,
  CardContent,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";

const ANNOUNCEMENTS_KEY = ["/api/admin/announcements"] as const;

/** Variantes visuelles du niveau (N°179 — 5 niveaux) — miroir des couleurs
 * du bandeau client : l'émeraude passe aux nouveautés, la sarcelle à la
 * maintenance, l'info redevient neutre. */
const LEVEL_BADGE: Record<AnnouncementLevel, string> = {
  info: "bg-foreground/10 text-foreground",
  success: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
  maintenance: "bg-teal-500/10 text-teal-600 dark:text-teal-400",
  warning: "bg-amber-500/10 text-amber-600 dark:text-amber-400",
  critical: "bg-destructive/10 text-destructive",
};

/** Durées proposées (N°179 — heures ET jours) : 0/0 = jusqu'au retrait
 * manuel ; "custom" = durée personnalisée jours + heures. */
const EXPIRY_PRESETS = [
  { key: "none", days: 0, hours: 0 },
  { key: "2h", days: 0, hours: 2 },
  { key: "6h", days: 0, hours: 6 },
  { key: "12h", days: 0, hours: 12 },
  { key: "1d", days: 1, hours: 0 },
  { key: "3d", days: 3, hours: 0 },
  { key: "7d", days: 7, hours: 0 },
  { key: "30d", days: 30, hours: 0 },
  { key: "custom", days: -1, hours: -1 },
] as const;
type ExpiryKey = (typeof EXPIRY_PRESETS)[number]["key"];

/** Libellé d'un preset (heures pures, jour singulier, jours). */
function expiryLabel(
  key: ExpiryKey,
  t: (k: string) => string,
  tf: (k: string, p: Record<string, number | string>) => string,
): string {
  const preset = EXPIRY_PRESETS.find((p) => p.key === key);
  if (!preset || preset.days < 0) return t("ann.form.expiry.custom");
  if (preset.days === 0 && preset.hours === 0) return t("ann.form.expiry.none");
  if (preset.days === 0) return tf("ann.form.expiry.hours", { n: preset.hours });
  if (preset.days === 1) return tf("ann.form.expiry.day", { n: 1 });
  return tf("ann.form.expiry.days", { n: preset.days });
}

/** formatHours — durée totale en heures vers « X j Y h » lisible (N°179).
 * Le symbole des jours suit la langue (j/d — heures et minutes sont
 * naturellement identiques). */
function formatHours(total: number, lang: string): string {
  const daySym = lang === "fr" ? "j" : "d";
  const days = Math.floor(total / 24);
  const hours = total % 24;
  if (days === 0) return `${hours} h`;
  if (hours === 0) return `${days} ${daySym}`;
  return `${days} ${daySym} ${hours} h`;
}

/** Badge de statut (N°165) : visible / programmée / expirée — la
 * programmation porte l'horloge. Repli sur « active » si le backend déployé
 * n'envoie pas encore state (fenêtre de transition). */
function StatusBadge({ state }: { state: AdminAnnouncementState }) {
  const { t } = useI18n();
  if (state === "scheduled") {
    return (
      <Badge variant="secondary" className="bg-teal-500/10 text-teal-600 dark:text-teal-400">
        <Clock className="mr-1 size-3" aria-hidden />
        {t("ann.status.scheduled")}
      </Badge>
    );
  }
  if (state === "active") {
    return (
      <Badge className="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400" variant="secondary">
        {t("ann.status.active")}
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="text-muted-foreground">
      {t("ann.status.expired")}
    </Badge>
  );
}

/** Maintenant au format valeur locale d'un input datetime-local (borne min
 * du champ : on ne programme pas dans le passé). */
function localInputNow(): string {
  const d = new Date();
  d.setSeconds(0, 0);
  return new Date(d.getTime() - d.getTimezoneOffset() * 60_000).toISOString().slice(0, 16);
}

export default function PlatformAnnouncementsView() {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteRow, setDeleteRow] = useState<AdminAnnouncementRow | null>(null);

  // — formulaire de création —
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [level, setLevel] = useState<AnnouncementLevel>("info");
  const [audience, setAudience] = useState<AnnouncementAudience>("all");
  // N°179 — durée de visibilité : preset (heures et/ou jours) ou durée
  // personnalisée (champs jours + heures combinés).
  const [expiryKey, setExpiryKey] = useState<ExpiryKey>("7d");
  const [customDays, setCustomDays] = useState(0);
  const [customHours, setCustomHours] = useState(6);
  const [email, setEmail] = useState(true);
  // N°165 — diffusion immédiate ou programmée (datetime-local, heure du
  // navigateur du gérant — Abidjan GMT en pratique).
  const [publishMode, setPublishMode] = useState<"now" | "scheduled">("now");
  const [publishAtLocal, setPublishAtLocal] = useState("");

  const { data: rows, isLoading, error } = useQuery({
    queryKey: ANNOUNCEMENTS_KEY,
    queryFn: fetchAnnouncements,
    retry: (_n, err) => !(err instanceof ApiError && err.status === 403),
  });
  const forbidden = error instanceof ApiError && error.status === 403;

  function invalidate() {
    void queryClient.invalidateQueries({ queryKey: ANNOUNCEMENTS_KEY });
  }

  function resetForm() {
    setTitle("");
    setBody("");
    setLevel("info");
    setAudience("all");
    setExpiryKey("7d");
    setCustomDays(0);
    setCustomHours(6);
    setEmail(true);
    setPublishMode("now");
    setPublishAtLocal("");
  }

  const createMutation = useMutation({
    mutationFn: createAnnouncement,
    onSuccess: (ann, vars) => {
      // Garde de transition N°165 : un backend PAS ENCORE redéployé ignore
      // publishAt (champ JSON inconnu pour lui) — l'annonce serait partie
      // IMMÉDIATEMENT. On le dit au lieu de laisser croire à une programmation.
      if (vars.publishAt && !ann.publishAt) {
        toast.warning(t("ann.fallbackImmediate"));
      } else if (vars.publishAt) {
        toast.success(t("ann.scheduledToast"));
      } else if ((vars.expiresInHours ?? 0) > 0 && !ann.expiresAt) {
        // Garde de transition N°179 : un backend pas encore redéployé ignore
        // expiresInHours — une durée purement horaire serait perdue (annonce
        // sans expiration). On le dit au lieu de laisser croire à la durée.
        toast.warning(t("ann.fallbackNoExpiry"));
      } else {
        toast.success(t("ann.created"));
      }
      setCreateOpen(false);
      resetForm();
      invalidate();
    },
    onError: (err) => toast.error(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteAnnouncement(id),
    onSuccess: () => {
      toast.success(t("ann.deleted"));
      setDeleteRow(null);
      invalidate();
    },
    onError: (err) => toast.error(err.message),
  });

  // Date de programmation demandée : valide et dans le futur ?
  const scheduledDate =
    publishMode === "scheduled" && publishAtLocal ? new Date(publishAtLocal) : null;
  const publishOK =
    publishMode === "now" ||
    (scheduledDate !== null &&
      !Number.isNaN(scheduledDate.getTime()) &&
      scheduledDate.getTime() > Date.now());
  const isScheduled = scheduledDate !== null && publishOK;

  // N°179 — durée résolue depuis le preset ou les champs personnalisés :
  // total en heures, borné 1 h → 365 j (365*24 h).
  const expiryPreset = EXPIRY_PRESETS.find((p) => p.key === expiryKey)!;
  const expiryDays = expiryKey === "custom" ? customDays : expiryPreset.days;
  const expiryHours = expiryKey === "custom" ? customHours : expiryPreset.hours;
  const totalHours = expiryDays * 24 + expiryHours;
  const expiryOK = totalHours === 0 || (totalHours >= 1 && totalHours <= 365 * 24);

  const canSubmit =
    title.trim().length >= 3 &&
    title.length <= 120 &&
    body.length <= 2000 &&
    publishOK &&
    expiryOK &&
    !createMutation.isPending;

  const dateFmt = new Intl.DateTimeFormat(lang === "fr" ? "fr-FR" : "en-US", {
    day: "numeric",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("ann.title")}
        description={t("ann.subtitle")}
        actions={
          <Button onClick={() => setCreateOpen(true)} className="min-h-10">
            <Megaphone className="size-4" />
            {t("ann.new")}
          </Button>
        }
      />

      {isLoading ? (
        <Card className="gap-0 py-0">
          <div className="space-y-3 p-4 sm:p-6">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-14 rounded-lg" />
            ))}
          </div>
        </Card>
      ) : forbidden ? (
        <Card className="gap-0 py-0">
          <EmptyState icon={ShieldCheck} title={t("accounts.forbiddenTitle")} description={t("accounts.forbiddenDesc")} />
        </Card>
      ) : error ? (
        <Card className="gap-0 py-0">
          <EmptyState icon={Megaphone} title={t("platformTeam.loadError")} description={error.message} />
        </Card>
      ) : !rows || rows.length === 0 ? (
        <Card className="gap-0 py-0">
          <EmptyState icon={Megaphone} title={t("ann.empty")} description={t("ann.emptyHint")} />
        </Card>
      ) : (
        <Card className="gap-0 py-0">
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("ann.col.title")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("ann.col.level")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("ann.col.audience")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("ann.col.created")}</TableHead>
                    <TableHead className="hidden text-muted-foreground md:table-cell">{t("ann.col.expires")}</TableHead>
                    <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("ann.col.status")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((row) => {
                    // Repli de transition : un backend pas encore redéployé
                    // n'envoie pas state (N°165) — on le déduit d'active.
                    const state: AdminAnnouncementState = row.state ?? (row.active ? "active" : "expired");
                    return (
                      <TableRow key={row.id}>
                        <TableCell className="max-w-[280px] pl-4 sm:pl-6">
                          <p className="truncate font-medium">{row.title}</p>
                          <p className="mt-0.5 line-clamp-1 text-xs text-muted-foreground">
                            {row.body || "—"}
                          </p>
                        </TableCell>
                        <TableCell>
                          <Badge variant="secondary" className={LEVEL_BADGE[row.level]}>
                            {t(`ann.level.${row.level}`)}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          <p>{t(`ann.audience.${row.audience}`)}</p>
                          <p className="text-xs text-muted-foreground/70">{tf("ann.reach", { count: row.accountsCount })}</p>
                        </TableCell>
                        <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                          {dateFmt.format(new Date(row.createdAt))}
                          {row.publishAt ? (
                            <p className="mt-0.5 text-xs text-teal-600 dark:text-teal-400">
                              <Clock className="mr-0.5 inline size-3 align-[-1px]" aria-hidden />
                              {tf("ann.autoPublish", { date: dateFmt.format(new Date(row.publishAt)) })}
                            </p>
                          ) : null}
                          {row.emailedAt ? (
                            <p className="mt-0.5 text-xs text-muted-foreground/70">
                              ✉ {row.emailedCount ?? 0}
                            </p>
                          ) : null}
                        </TableCell>
                        <TableCell className="hidden whitespace-nowrap text-sm text-muted-foreground md:table-cell">
                          {row.expiresAt ? dateFmt.format(new Date(row.expiresAt)) : t("ann.noExpiry")}
                        </TableCell>
                        <TableCell className="pr-4 text-right sm:pr-6">
                          <div className="flex items-center justify-end gap-2">
                            <StatusBadge state={state} />
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-8 text-muted-foreground hover:text-destructive"
                              aria-label={t("ann.delete")}
                              onClick={() => setDeleteRow(row)}
                            >
                              <Trash2 className="size-4" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>
      )}

      {/* — création — */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("ann.form.title")}</DialogTitle>
            <DialogDescription>{t("ann.form.description")}</DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="ann-title">{t("ann.form.titleLabel")}</Label>
              <Input
                id="ann-title"
                value={title}
                maxLength={120}
                placeholder={t("ann.form.titlePlaceholder")}
                onChange={(e) => setTitle(e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="ann-body">{t("ann.form.bodyLabel")}</Label>
              <Textarea
                id="ann-body"
                value={body}
                rows={4}
                maxLength={2000}
                placeholder={t("ann.form.bodyPlaceholder")}
                onChange={(e) => setBody(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">{t("ann.form.bodyHint")}</p>
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label>{t("ann.form.levelLabel")}</Label>
                <Select value={level} onValueChange={(v) => setLevel(v as AnnouncementLevel)}>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {ANNOUNCEMENT_LEVELS.map((lv) => (
                      <SelectItem key={lv} value={lv}>
                        {t(`ann.level.${lv}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">{t("ann.form.levelHint")}</p>
              </div>

              <div className="space-y-2">
                <Label>{t("ann.form.audienceLabel")}</Label>
                <Select value={audience} onValueChange={(v) => setAudience(v as AnnouncementAudience)}>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t("ann.audience.all")}</SelectItem>
                    <SelectItem value="hotspot">{t("ann.audience.hotspot")}</SelectItem>
                    <SelectItem value="homenet">{t("ann.audience.homenet")}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* N°165 — diffusion immédiate ou programmée */}
            <div className="space-y-2">
              <Label>{t("ann.form.publishLabel")}</Label>
              <Select
                value={publishMode}
                onValueChange={(v) => setPublishMode(v as "now" | "scheduled")}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="now">{t("ann.form.publish.now")}</SelectItem>
                  <SelectItem value="scheduled">{t("ann.form.publish.schedule")}</SelectItem>
                </SelectContent>
              </Select>
              {publishMode === "scheduled" ? (
                <div className="space-y-2 rounded-lg border border-border/60 p-3">
                  <Label htmlFor="ann-publishat">{t("ann.form.publishAtLabel")}</Label>
                  <Input
                    id="ann-publishat"
                    type="datetime-local"
                    value={publishAtLocal}
                    min={localInputNow()}
                    step={300}
                    onChange={(e) => setPublishAtLocal(e.target.value)}
                    aria-invalid={!publishOK}
                  />
                  <p className="text-xs text-muted-foreground">
                    {publishOK ? t("ann.form.publishHint") : t("ann.form.publishInvalid")}
                  </p>
                </div>
              ) : null}
            </div>

            {/* N°179 — durée de visibilité : presets heures/jours OU durée
                personnalisée (jours + heures combinés, 1 h → 365 j). */}
            <div className="space-y-2">
              <Label>{t("ann.form.expiryLabel")}</Label>
              <Select value={expiryKey} onValueChange={(v) => setExpiryKey(v as ExpiryKey)}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {EXPIRY_PRESETS.map((p) => (
                    <SelectItem key={p.key} value={p.key}>
                      {expiryLabel(p.key, t, tf)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {expiryKey === "custom" ? (
                <div className="space-y-2 rounded-lg border border-border/60 p-3">
                  <div className="grid grid-cols-2 gap-3">
                    <div className="space-y-1.5">
                      <Label htmlFor="ann-exp-days">{t("ann.form.expiry.daysField")}</Label>
                      <Input
                        id="ann-exp-days"
                        type="number"
                        inputMode="numeric"
                        min={0}
                        max={365}
                        value={customDays}
                        onChange={(e) =>
                          setCustomDays(Math.max(0, Math.min(365, Number(e.target.value) || 0)))
                        }
                        aria-invalid={!expiryOK}
                      />
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor="ann-exp-hours">{t("ann.form.expiry.hoursField")}</Label>
                      <Input
                        id="ann-exp-hours"
                        type="number"
                        inputMode="numeric"
                        min={0}
                        max={23}
                        value={customHours}
                        onChange={(e) =>
                          setCustomHours(Math.max(0, Math.min(23, Number(e.target.value) || 0)))
                        }
                        aria-invalid={!expiryOK}
                      />
                    </div>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {expiryOK
                      ? totalHours === 0
                        ? t("ann.form.expiry.none")
                        : tf("ann.form.expiry.total", { n: formatHours(totalHours, lang) })
                      : t("ann.form.expiryCustomInvalid")}
                  </p>
                </div>
              ) : (
                <p className="text-xs text-muted-foreground">{t("ann.form.expiryHint")}</p>
              )}
            </div>

            <div className="flex items-start gap-3 rounded-lg border border-border/60 p-3">
              <Switch id="ann-email" checked={email} onCheckedChange={setEmail} className="mt-0.5" />
              <div className="min-w-0">
                <Label htmlFor="ann-email" className="cursor-pointer text-sm">
                  {t("ann.form.email")}
                </Label>
                <p className="text-xs text-muted-foreground">
                  {isScheduled ? t("ann.form.emailScheduledHint") : t("ann.form.emailHint")}
                </p>
              </div>
            </div>
          </div>

          <DialogFooter className="gap-2 sm:space-x-0">
            <Button variant="outline" onClick={() => setCreateOpen(false)} className="min-h-10 flex-1 sm:flex-none">
              {t("ann.form.cancel")}
            </Button>
            <Button
              onClick={() =>
                createMutation.mutate({
                  title: title.trim(),
                  body: body.trim() || undefined,
                  level,
                  audience,
                  expiresInDays: expiryDays > 0 ? expiryDays : undefined,
                  expiresInHours: expiryHours > 0 ? expiryHours : undefined,
                  email,
                  publishAt: isScheduled ? scheduledDate!.toISOString() : undefined,
                })
              }
              disabled={!canSubmit}
              className="min-h-10 flex-1 sm:flex-none"
            >
              {createMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : isScheduled ? <Clock className="size-4" /> : <Send className="size-4" />}
              {isScheduled ? t("ann.form.submitScheduled") : t("ann.form.submit")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* — retrait (confirmation) — */}
      <AlertDialog open={!!deleteRow} onOpenChange={(open) => !open && setDeleteRow(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("ann.deleteTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {tf("ann.deleteBody", { title: deleteRow?.title ?? "" })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("ann.deleteCancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              disabled={deleteMutation.isPending}
              onClick={(e) => {
                e.preventDefault(); // rester ouvert jusqu'au succès (pattern team)
                if (deleteRow) deleteMutation.mutate(deleteRow.id);
              }}
            >
              {deleteMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
              {t("ann.deleteConfirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
