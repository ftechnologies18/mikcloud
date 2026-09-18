"use client";

// Console plateforme — ANNONCES AUX CLIENTS (N°152, super-admin uniquement).
// Le megaphone du SaaS : diffuser une annonce à tous les comptes MikCloud
// (maintenance, nouveauté, incident) — bandeau dans la console de chaque
// destinataire + entrée dans sa cloche + e-mail optionnel aux propriétaires.
// Contrats : GET/POST /api/admin/announcements, DELETE /api/admin/announcements/{id}
// (voir lib/hotspot/types.ts et api.ts).

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
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
import type { AdminAnnouncementRow, AnnouncementLevel, AnnouncementAudience } from "@/lib/hotspot/types";
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

/** Variantes visuelles du niveau — miroir des couleurs du bandeau client. */
const LEVEL_BADGE: Record<AnnouncementLevel, string> = {
  info: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
  warning: "bg-amber-500/10 text-amber-600 dark:text-amber-400",
  critical: "bg-destructive/10 text-destructive",
};

/** Durées proposées (jours) — 0 = jusqu'au retrait manuel. */
const EXPIRY_CHOICES = [0, 1, 7, 30, 90];

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
  const [expiresInDays, setExpiresInDays] = useState(7);
  const [email, setEmail] = useState(true);

  const { data: rows, isLoading, error } = useQuery({
    queryKey: ANNOUNCEMENTS_KEY,
    queryFn: fetchAnnouncements,
    retry: (_n, err) => !(err instanceof ApiError && err.status === 403),
  });
  const forbidden = error instanceof ApiError && error.status === 403;

  function invalidate() {
    void queryClient.invalidateQueries({ queryKey: ANNOUNCEMENTS_KEY });
  }

  const createMutation = useMutation({
    mutationFn: createAnnouncement,
    onSuccess: () => {
      toast.success(t("ann.created"));
      setCreateOpen(false);
      setTitle("");
      setBody("");
      setLevel("info");
      setAudience("all");
      setExpiresInDays(7);
      setEmail(true);
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

  const canSubmit =
    title.trim().length >= 3 &&
    title.length <= 120 &&
    body.length <= 2000 &&
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
                  {rows.map((row) => (
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
                          {row.active ? (
                            <Badge className="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400" variant="secondary">
                              {t("ann.status.active")}
                            </Badge>
                          ) : (
                            <Badge variant="outline" className="text-muted-foreground">
                              {t("ann.status.expired")}
                            </Badge>
                          )}
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
                  ))}
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
                    <SelectItem value="info">{t("ann.level.info")}</SelectItem>
                    <SelectItem value="warning">{t("ann.level.warning")}</SelectItem>
                    <SelectItem value="critical">{t("ann.level.critical")}</SelectItem>
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

            <div className="space-y-2">
              <Label>{t("ann.form.expiryLabel")}</Label>
              <Select
                value={String(expiresInDays)}
                onValueChange={(v) => setExpiresInDays(Number(v))}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {EXPIRY_CHOICES.map((d) => (
                    <SelectItem key={d} value={String(d)}>
                      {d === 0 ? t("ann.form.expiry.none") : tf("ann.form.expiry.days", { n: d })}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">{t("ann.form.expiryHint")}</p>
            </div>

            <div className="flex items-start gap-3 rounded-lg border border-border/60 p-3">
              <Switch id="ann-email" checked={email} onCheckedChange={setEmail} className="mt-0.5" />
              <div className="min-w-0">
                <Label htmlFor="ann-email" className="cursor-pointer text-sm">
                  {t("ann.form.email")}
                </Label>
                <p className="text-xs text-muted-foreground">{t("ann.form.emailHint")}</p>
              </div>
            </div>
          </div>

          <DialogFooter className="gap-2 sm:space-x-0">
            <Button variant="outline" onClick={() => setCreateOpen(false)} className="min-h-10 flex-1 sm:flex-none">
              {t("ann.form.cancel")}
            </Button>
            <Button
              onClick={() => createMutation.mutate({
                title: title.trim(),
                body: body.trim() || undefined,
                level,
                audience,
                expiresInDays: expiresInDays > 0 ? expiresInDays : undefined,
                email,
              })}
              disabled={!canSubmit}
              className="min-h-10 flex-1 sm:flex-none"
            >
              {createMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <Send className="size-4" />}
              {t("ann.form.submit")}
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
