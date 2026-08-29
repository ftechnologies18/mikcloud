"use client";

// Vue Modèles de vouchers (F2) — gabarits d'impression personnalisables :
// - liste des modèles (GET /api/templates) : nom, format (A4 / 58 mm / 80 mm),
//   défaut, taille HTML, actions (éditer, définir par défaut, dupliquer, supprimer) ;
// - éditeur : nom, format, bodyHtml (font mono) + chips de variables insérées au
//   curseur + aperçu live (voucher d'exemple, QR réel) + presets MikCloud ;
// - aperçu d'impression sur vouchers d'exemple (UcPrintDialog en mode modèle).

import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Copy,
  FileCode2,
  Loader2,
  MoreHorizontal,
  Pencil,
  Plus,
  Printer,
  Star,
  Trash2,
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
import { Checkbox } from "@/components/ui/checkbox";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { EmptyState } from "@/components/hotspot/empty-state";
import { PageHeader } from "@/components/hotspot/page-header";
import { useCurrency, useSettings } from "@/components/hotspot/parts/sd-currency";
import {
  TEMPLATE_PRESETS,
  TEMPLATE_VARIABLES,
  renderTemplate,
  type TemplateRenderContext,
} from "@/components/hotspot/parts/template-render";
import { UcPrintDialog } from "@/components/hotspot/parts/uc-print-dialog";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatDate } from "@/lib/hotspot/format";
import type { HotspotUser, Profile, VoucherFormat, VoucherTemplate } from "@/lib/hotspot/types";

const FORMAT_LABELS: Record<VoucherFormat, string> = {
  a4: "A4",
  "58mm": "58 mm",
  "80mm": "80 mm",
};

const MAX_BODY_LENGTH = 20_000; // contrat F2

function htmlSize(bodyHtml: string, t: (key: string) => string): string {
  if (bodyHtml.length < 1024) return t("templates.htmlSizeChars").replace("{n}", String(bodyHtml.length));
  return t("templates.htmlSizeKb").replace("{n}", (bodyHtml.length / 1024).toFixed(1));
}

/** Voucher d'exemple déterministe pour les aperçus (QR réel, variables réelles). */
function buildSampleVoucher(num: number, profile: Profile | undefined): HotspotUser {
  const code = (0x100000 + num * 7331).toString(36).toUpperCase().slice(0, 6);
  const price = profile?.price ?? 500;
  const sellingPrice = profile && profile.sellingPrice > 0 ? profile.sellingPrice : price;
  const nowIso = new Date().toISOString();
  return {
    id: `sample-${num}`,
    kind: "voucher",
    username: `MC-${code}`,
    password: `${((num + 11) * 97).toString(36)}k${num}p`,
    profileId: profile?.id ?? "",
    profileName: profile?.name ?? "24 Heures",
    routerId: "",
    routerName: "Routeur principal",
    status: "active",
    batchId: "",
    resellerId: "",
    resellerName: "",
    comment: num === 1 ? "Exemple de commentaire" : "",
    bytesIn: 0,
    bytesOut: 0,
    uptimeUsedSec: 0,
    createdAt: nowIso,
    expiresAt: nowIso,
    usedAt: "",
    price,
    sellingPrice,
  };
}

// ─── Éditeur de modèle ───

interface TemplateEditorDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Modèle en cours d'édition — null = création. */
  template: VoucherTemplate | null;
  ctx: TemplateRenderContext;
  sampleVoucher: HotspotUser;
}

// Éditeur monté conditionnellement par la vue (réinitialisation du formulaire à
// chaque ouverture sans effet de synchronisation).
function TemplateEditorDialog({ open, onOpenChange, template, ctx, sampleVoucher }: TemplateEditorDialogProps) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const initial = template ?? TEMPLATE_PRESETS[0];
  const [name, setName] = useState(() => initial.name);
  const [format, setFormat] = useState<VoucherFormat>(() => initial.format);
  const [bodyHtml, setBodyHtml] = useState(() => initial.bodyHtml);
  const [isDefault, setIsDefault] = useState(() => template?.isDefault ?? false);
  const bodyRef = useRef<HTMLTextAreaElement>(null);

  // Aperçu live (async : génération du QR code). « preview » conserve le corps
  // rendu : tant qu'il diffère du corps courant, l'aperçu est considéré en cours.
  const [preview, setPreview] = useState<{ body: string; html: string } | null>(null);
  const previewHtml = preview && preview.body === bodyHtml ? preview.html : null;

  useEffect(() => {
    let cancelled = false;
    renderTemplate(bodyHtml, sampleVoucher, ctx, 1)
      .then((html) => {
        if (!cancelled) setPreview({ body: bodyHtml, html });
      })
      .catch(() => {
        if (!cancelled) setPreview({ body: bodyHtml, html: "" });
      });
    return () => {
      cancelled = true;
    };
  }, [bodyHtml, sampleVoucher, ctx]);

  function insertVariable(variable: string) {
    const el = bodyRef.current;
    if (!el) {
      setBodyHtml((b) => b + variable);
      return;
    }
    const start = el.selectionStart ?? el.value.length;
    const end = el.selectionEnd ?? start;
    setBodyHtml(el.value.slice(0, start) + variable + el.value.slice(end));
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(start + variable.length, start + variable.length);
    });
  }

  const saveMutation = useMutation({
    mutationFn: () =>
      template
        ? api<VoucherTemplate>(`/api/templates/${template.id}`, {
            method: "PUT",
            body: { name: name.trim(), format, bodyHtml, isDefault },
          })
        : api<VoucherTemplate>("/api/templates", {
            method: "POST",
            body: { name: name.trim(), format, bodyHtml, isDefault },
          }),
    onSuccess: (saved) => {
      toast.success(
        template
          ? tf("templates.updatedToast", { name: saved.name })
          : tf("templates.createdToast", { name: saved.name }),
      );
      onOpenChange(false);
      void queryClient.invalidateQueries({ queryKey: ["/api/templates"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const nameInvalid = name.trim() === "" || name.trim().length > 60;
  const bodyInvalid = bodyHtml.trim() === "" || bodyHtml.length > MAX_BODY_LENGTH;
  const formValid = !nameInvalid && !bodyInvalid;

  function submitTemplate() {
    if (!formValid) {
      if (name.trim() === "") toast.error(t("common.nameRequired"));
      else if (name.trim().length > 60) toast.error(t("templates.editor.nameTooLong"));
      else if (bodyHtml.trim() === "") toast.error(t("templates.editor.bodyRequired"));
      else toast.error(tf("templates.editor.bodyTooLong", { n: MAX_BODY_LENGTH }));
      return;
    }
    saveMutation.mutate();
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[92vh] overflow-y-auto sm:max-w-5xl">
        <DialogHeader>
          <DialogTitle>
            {template
              ? tf("templates.editor.editTitle", { name: template.name })
              : t("templates.editor.newTitle")}
          </DialogTitle>
          <DialogDescription>{t("templates.editor.desc")}</DialogDescription>
        </DialogHeader>

        <form
          className="grid gap-5 lg:grid-cols-2"
          onSubmit={(event) => {
            event.preventDefault();
            submitTemplate();
          }}
        >
          {/* ─── Colonne éditeur ─── */}
          <div className="grid content-start gap-4">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="tpl-name">{t("common.name")}</Label>
                <Input
                  id="tpl-name"
                  placeholder={t("templates.editor.namePlaceholder")}
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  maxLength={60}
                  disabled={saveMutation.isPending}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="tpl-format">{t("templates.editor.format")}</Label>
                <Select
                  value={format}
                  onValueChange={(value) => setFormat(value as VoucherFormat)}
                  disabled={saveMutation.isPending}
                >
                  <SelectTrigger id="tpl-format" className="h-10 w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="a4">{t("templates.editor.formatA4")}</SelectItem>
                    <SelectItem value="58mm">{t("templates.editor.format58")}</SelectItem>
                    <SelectItem value="80mm">{t("templates.editor.format80")}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            {!template && (
              <div className="grid gap-2">
                <p className="text-xs font-medium text-muted-foreground">{t("templates.editor.startFrom")}</p>
                <div className="flex flex-wrap gap-2">
                  {TEMPLATE_PRESETS.map((preset) => (
                    <Button
                      key={preset.id}
                      type="button"
                      variant="outline"
                      className="h-9"
                      disabled={saveMutation.isPending}
                      onClick={() => {
                        setName(preset.name);
                        setFormat(preset.format);
                        setBodyHtml(preset.bodyHtml);
                      }}
                    >
                      {preset.label}
                    </Button>
                  ))}
                </div>
              </div>
            )}

            <div className="grid gap-2">
              <div className="flex items-baseline justify-between gap-2">
                <Label htmlFor="tpl-body">{t("templates.editor.body")}</Label>
                <span className="text-[11px] tabular-nums text-muted-foreground">
                  {bodyHtml.length}/{MAX_BODY_LENGTH}
                </span>
              </div>
              <Textarea
                id="tpl-body"
                ref={bodyRef}
                className="min-h-64 resize-y font-mono text-[13px] leading-relaxed"
                placeholder="<div style=…> … </div>"
                value={bodyHtml}
                onChange={(event) => setBodyHtml(event.target.value)}
                disabled={saveMutation.isPending}
                aria-invalid={bodyInvalid}
                spellCheck={false}
              />
              <p className="text-xs text-muted-foreground">{t("templates.editor.bodyHint")}</p>
            </div>

            <div className="grid gap-2">
              <p className="text-xs font-medium text-muted-foreground">
                {t("templates.editor.variables")}
              </p>
              <div className="flex max-h-28 flex-wrap gap-1.5 overflow-y-auto">
                {TEMPLATE_VARIABLES.map((variable) => (
                  <button
                    key={variable}
                    type="button"
                    onClick={() => insertVariable(variable)}
                    className="h-8 rounded-md border bg-muted/40 px-2 font-mono text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                    aria-label={tf("templates.editor.insertVariable", { variable })}
                  >
                    {variable}
                  </button>
                ))}
              </div>
            </div>

            <label
              htmlFor="tpl-default"
              className="flex min-h-11 cursor-pointer items-center gap-3 rounded-md border bg-muted/30 px-3"
            >
              <Checkbox
                id="tpl-default"
                checked={isDefault}
                onCheckedChange={(checked) => setIsDefault(checked === true)}
                disabled={saveMutation.isPending}
              />
              <span className="text-sm">
                {t("templates.editor.defaultLabel")}
                <span className="block text-xs text-muted-foreground">
                  {t("templates.editor.defaultHint")}
                </span>
              </span>
            </label>

            {saveMutation.isError && (
              <p className="text-sm text-destructive" role="alert">
                {saveMutation.error.message}
              </p>
            )}
          </div>

          {/* ─── Colonne aperçu live ─── */}
          <div className="grid content-start gap-2">
            <div className="flex items-center justify-between gap-2">
              <p className="text-xs font-medium text-muted-foreground">{t("templates.editor.livePreview")}</p>
              <Badge variant="outline" className="text-[10px]">
                {FORMAT_LABELS[format]}
              </Badge>
            </div>
            <div className="max-h-[60vh] min-h-64 overflow-auto rounded-lg border bg-neutral-100 p-4">
              <div className="mx-auto w-fit max-w-full bg-white text-black shadow-sm">
                {bodyHtml.trim() === "" ? (
                  <p className="w-64 p-6 text-center text-xs text-neutral-400">
                    {t("templates.editor.previewEmpty")}
                  </p>
                ) : previewHtml === null ? (
                  <p className="flex w-64 items-center justify-center gap-2 p-6 text-xs text-neutral-400">
                    <Loader2 className="size-3.5 animate-spin" aria-hidden />
                    {t("templates.editor.generating")}
                  </p>
                ) : previewHtml === "" ? (
                  <p className="w-64 p-6 text-center text-xs text-neutral-400">
                    {t("templates.editor.previewFailed")}
                  </p>
                ) : (
                  <div
                    className="tpl-ticket p-2"
                    dangerouslySetInnerHTML={{ __html: previewHtml }}
                  />
                )}
              </div>
            </div>
            <p className="text-xs text-muted-foreground">
              {t("templates.editor.sample")} <span className="font-mono">{sampleVoucher.username}</span> ·
              {t("templates.editor.sampleHint")}
            </p>
          </div>
        </form>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saveMutation.isPending}>
            {t("common.cancel")}
          </Button>
          <Button type="button" disabled={!formValid || saveMutation.isPending} onClick={submitTemplate}>
            {saveMutation.isPending && <Loader2 className="size-4 animate-spin" />}
            {template ? t("common.save") : t("templates.editor.createSubmit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ─── Vue ───

export default function TemplatesView() {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const { data: settings } = useSettings();
  const queryClient = useQueryClient();

  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<VoucherTemplate | null>(null);
  const [deleting, setDeleting] = useState<VoucherTemplate | null>(null);
  const [printOpen, setPrintOpen] = useState(false);

  const { data: templates, isLoading } = useQuery({
    queryKey: ["/api/templates"],
    queryFn: () => api<VoucherTemplate[]>("/api/templates"),
  });

  const { data: profiles } = useQuery({
    queryKey: ["/api/profiles"],
    queryFn: () => api<Profile[]>("/api/profiles"),
  });

  const tenantName = settings?.tenant.name || "MikCloud";

  const ctx = useMemo<TemplateRenderContext>(
    () => ({
      tenantName,
      dnsName: settings?.tenant.dnsName,
      logoUrl: settings?.tenant.logoUrl,
      currency,
      profiles: profiles ?? [],
    }),
    [tenantName, settings, currency, profiles],
  );

  // Vouchers d'exemple ×10 pour l'aperçu d'impression.
  const sampleVouchers = useMemo(() => {
    const profile = profiles?.[0];
    return Array.from({ length: 10 }, (_, i) => buildSampleVoucher(i + 1, profile));
  }, [profiles]);

  const sampleVoucher = useMemo(() => sampleVouchers[0], [sampleVouchers]);

  function invalidateTemplates() {
    void queryClient.invalidateQueries({ queryKey: ["/api/templates"] });
  }

  function openCreate() {
    setEditing(null);
    setEditorOpen(true);
  }

  function openEdit(template: VoucherTemplate) {
    setEditing(template);
    setEditorOpen(true);
  }

  function closeEditor(open: boolean) {
    setEditorOpen(open);
    if (!open) setEditing(null);
  }

  const setDefaultMutation = useMutation({
    mutationFn: (template: VoucherTemplate) =>
      api<VoucherTemplate>(`/api/templates/${template.id}`, { method: "PUT", body: { isDefault: true } }),
    onSuccess: (saved) => {
      toast.success(tf("templates.defaultToast", { name: saved.name }));
      invalidateTemplates();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const duplicateMutation = useMutation({
    mutationFn: (template: VoucherTemplate) =>
      api<VoucherTemplate>("/api/templates", {
        method: "POST",
        body: {
          name: `${template.name} (copie)`.slice(0, 60),
          format: template.format,
          bodyHtml: template.bodyHtml,
          isDefault: false,
        },
      }),
    onSuccess: (created) => {
      toast.success(tf("templates.duplicateToast", { name: created.name }));
      invalidateTemplates();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (template: VoucherTemplate) =>
      api<{ ok: boolean }>(`/api/templates/${template.id}`, { method: "DELETE" }),
    onSuccess: (_res, template) => {
      toast.success(tf("templates.deletedToast", { name: template.name }));
      setDeleting(null);
      invalidateTemplates();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const list = templates ?? [];

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("templates.title")}
        description={t("templates.description")}
        actions={
          <>
            <Button variant="outline" className="h-10" onClick={() => setPrintOpen(true)}>
              <Printer className="size-4" />
              {t("templates.preview")}
            </Button>
            <Button className="h-10" onClick={openCreate}>
              <Plus className="size-4" />
              {t("templates.new")}
            </Button>
          </>
        }
      />

      {isLoading ? (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-40 rounded-xl" />
          ))}
        </div>
      ) : list.length === 0 ? (
        <Card className="gap-0 py-0">
          <EmptyState
            icon={FileCode2}
            title={t("templates.empty")}
            description={t("templates.emptyDesc")}
            action={
              <Button onClick={openCreate}>
                <Plus className="size-4" />
                {t("templates.new")}
              </Button>
            }
          />
        </Card>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {list.map((template) => (
            <Card key={template.id} className="py-0">
              <CardContent className="flex h-full flex-col gap-3 p-4 sm:p-6">
                <div className="flex items-start justify-between gap-2">
                  <div className="flex min-w-0 flex-col items-start gap-1.5">
                    <p className="truncate font-semibold leading-tight">{template.name}</p>
                    <div className="flex flex-wrap items-center gap-1.5">
                      <Badge variant="outline" className="text-[10px] font-semibold">
                        {FORMAT_LABELS[template.format]}
                      </Badge>
                      {template.isDefault && (
                        <Badge
                          variant="outline"
                          className="border-primary/25 bg-primary/10 text-[10px] font-semibold text-primary"
                        >
                          {t("templates.default")}
                        </Badge>
                      )}
                    </div>
                  </div>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-10 text-muted-foreground hover:text-foreground"
                        aria-label={tf("common.actionsFor", { name: template.name })}
                      >
                        <MoreHorizontal className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-52">
                      <DropdownMenuItem className="min-h-10" onClick={() => openEdit(template)}>
                        <Pencil className="size-4" />
                        {t("templates.edit")}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        className="min-h-10"
                        disabled={template.isDefault || setDefaultMutation.isPending}
                        onClick={() => setDefaultMutation.mutate(template)}
                      >
                        <Star className="size-4" />
                        {t("templates.setDefault")}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        className="min-h-10"
                        disabled={duplicateMutation.isPending}
                        onClick={() => duplicateMutation.mutate(template)}
                      >
                        <Copy className="size-4" />
                        {t("templates.duplicate")}
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        className="min-h-10"
                        onClick={() => setDeleting(template)}
                      >
                        <Trash2 className="size-4" />
                        {t("common.delete")}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>

                <p className="text-xs text-muted-foreground">
                  {htmlSize(template.bodyHtml, t)} ·{" "}
                  {tf("templates.createdOn", { date: formatDate(template.createdAt, lang) })}
                </p>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {/* Éditeur création / modification — monté conditionnellement pour
          réinitialiser le formulaire à chaque ouverture */}
      {editorOpen && (
        <TemplateEditorDialog
          open
          onOpenChange={closeEditor}
          template={editing}
          ctx={ctx}
          sampleVoucher={sampleVoucher}
        />
      )}

      {/* Aperçu d'impression sur vouchers d'exemple (mode modèle) */}
      <UcPrintDialog
        open={printOpen}
        onOpenChange={setPrintOpen}
        vouchers={sampleVouchers}
        title={t("templates.previewTitle")}
        description={t("templates.previewDesc")}
        tenantName={tenantName}
        profiles={profiles ?? []}
        templates={list}
      />

      {/* Confirmation suppression */}
      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{tf("templates.deleteTitle", { name: deleting?.name ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>{t("templates.deleteDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={deleteMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                if (deleting) deleteMutation.mutate(deleting);
              }}
            >
              {deleteMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
