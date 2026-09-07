"use client";

// N°35-d — contenu « Portail » de la section Hotspot (N°57-d : ancienne vue
// Portail autonome, désormais ONGLET du hub components/hotspot/views/
// hotspot-view.tsx — /app/settings/hotspot/portail).
//
// Liste les routeurs en mode agent (les seuls qui déploient le portail captif
// via la commande hotspot_files N°35-a). Pour chaque routeur :
//   - statut de la signature (Portail à jour / Re-déploiement en attente / Jamais déployé) ;
//   - aperçu HTML du portail personnalisé (iframe srcDoc, fetch /api/routers/{id}/portal-preview) ;
//   - bouton « Re-déployer » (AlertDialog confirm) qui vide la sig côté backend
//     → ensureHotspotFilesLocked re-file automatiquement au prochain check-in (≤ 45 s) ;
//   - journal des déploiements (filtré sur /Portail captif déployé|Re-déploiement/).
//
// Contrainte architecturale (audit N°35-d) : le portail est déployé par ROUTEUR,
// pas par SITE WiFi. Ce contenu est donc au niveau routeur, pas dans la console
// WiFi qui liste des sites. Un routeur peut être lié à 0, 1 ou N sites WiFi —
// le branding affiché dans l'aperçu est résolu par buildPortalConfig (1er site
// actif lié au routeur, ou branding par défaut du tenant si aucun site).

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  CheckCircle2,
  Clock,
  Eye,
  Loader2,
  Monitor,
  RefreshCw,
  Router as RouterIcon,
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";

import { EmptyState } from "@/components/hotspot/empty-state";
import {
  AccountActivity,
  api,
  fetchAccountActivity,
  fetchRouterPortalPreview,
  redeployRouterPortal,
} from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { RouterDevice } from "@/lib/hotspot/types";

/** SigStatus — statut dérivé de la signature du portail. */
function sigStatus(router: RouterDevice): "deployed" | "pending" | "never" {
  // hotspotFilesSig est omitempty dans le JSON : absent = jamais déployé.
  const sig = (router as RouterDevice & { hotspotFilesSig?: string }).hotspotFilesSig;
  if (!sig) return "never";
  // Une sig non vide = un déploiement déjà confirmé. Une sig vide après un
  // re-déploiement forcé = en attente (le backend vient de la vider).
  // On ne peut pas distinguer "pending" de "deployed" sans état supplémentaire
  // côté backend — on considère que toute sig non vide = déployé.
  return "deployed";
}

/** Contenu de l'onglet « Portail » du hub Hotspot (N°57-d) — tout le
 * comportement de l'ancienne vue, sans son PageHeader (le hub porte le
 * titre de section). */
export function PortalContent() {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();

  // Liste des routeurs du compte.
  const routersQuery = useQuery<RouterDevice[]>({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
  });

  // Journal d'activité (pour filtrer les événements portail).
  const activityQuery = useQuery<AccountActivity[]>({
    queryKey: ["/api/activity"],
    queryFn: () => fetchAccountActivity(100),
    refetchInterval: 30_000, // refresh auto 30 s
  });

  // Filtre les routeurs agent (les seuls qui déploient le portail).
  const routers = (routersQuery.data ?? []).filter((r) => r.mode === "agent");

  // Filtre le journal sur les événements portail.
  const portalActivity = (activityQuery.data ?? []).filter(
    (a) =>
      a.type === "router" &&
      (/Portail captif déployé/i.test(a.message) ||
        /Re-déploiement du portail/i.test(a.message)),
  );

  // État local : routeur sélectionné pour l'aperçu, dialog re-déploiement.
  const [previewFor, setPreviewFor] = useState<RouterDevice | null>(null);
  const [redeployFor, setRedeployFor] = useState<RouterDevice | null>(null);

  // Mutation re-déploiement.
  const redeployMutation = useMutation({
    mutationFn: (routerId: string) => redeployRouterPortal(routerId),
    onSuccess: (data) => {
      toast.success(t("portal.redeployQueued"), { description: data.message });
      setRedeployFor(null);
      queryClient.invalidateQueries({ queryKey: ["/api/routers"] });
      queryClient.invalidateQueries({ queryKey: ["/api/activity"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  return (
    <div className="space-y-6">
      {routers.length === 0 ? (
        <EmptyState
          icon={Monitor}
          title={t("portal.empty")}
          description={t("portal.emptyHint")}
        />
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {routers.map((router) => {
            const status = sigStatus(router);
            return (
              <Card key={router.id}>
                <CardContent className="space-y-4 p-4">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <RouterIcon className="size-4 shrink-0 text-muted-foreground" />
                        <span className="truncate font-medium">{router.name}</span>
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {router.mode} · {router.status}
                      </p>
                    </div>
                    <SigBadge status={status} t={t} />
                  </div>

                  <div className="flex flex-wrap gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => setPreviewFor(router)}
                      className="gap-1.5"
                    >
                      <Eye className="size-3.5" />
                      {t("portal.preview")}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => setRedeployFor(router)}
                      className="gap-1.5"
                      disabled={redeployMutation.isPending}
                    >
                      <RefreshCw className="size-3.5" />
                      {t("portal.redeploy")}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      {/* Journal des déploiements */}
      <div className="space-y-3">
        <h2 className="text-lg font-semibold tracking-tight">
          {t("portal.journal")}
        </h2>
        {portalActivity.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("portal.journalEmpty")}</p>
        ) : (
          <ScrollArea className="max-h-96 rounded-lg border">
            <ul className="divide-y">
              {portalActivity.map((a) => (
                <li key={a.id} className="flex items-start gap-3 p-3 text-sm">
                  <div className="mt-0.5">
                    {/Re-déploiement/i.test(a.message) ? (
                      <RefreshCw className="size-4 text-amber-500" />
                    ) : (
                      <CheckCircle2 className="size-4 text-emerald-500" />
                    )}
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="text-sm">{a.message}</p>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {a.at}
                      {a.actorName ? ` · ${a.actorName}` : ""}
                    </p>
                  </div>
                </li>
              ))}
            </ul>
          </ScrollArea>
        )}
      </div>

      {/* Dialog aperçu */}
      <PreviewDialog
        router={previewFor}
        onClose={() => setPreviewFor(null)}
        t={t}
        tf={tf}
      />

      {/* AlertDialog re-déploiement */}
      <AlertDialog
        open={redeployFor !== null}
        onOpenChange={(open) => !open && setRedeployFor(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("portal.redeployConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {tf("portal.redeployConfirm", { name: redeployFor?.name ?? "" })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={redeployMutation.isPending}>
              {t("portal.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={redeployMutation.isPending}
              onClick={() => redeployFor && redeployMutation.mutate(redeployFor.id)}
              className="gap-1.5"
            >
              {redeployMutation.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <RefreshCw className="size-4" />
              )}
              {t("portal.redeploy")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

/** SigBadge — badge de statut de la signature du portail. */
function SigBadge({
  status,
  t,
}: {
  status: "deployed" | "pending" | "never";
  // Signature identique au t() du hook useI18n (traduction simple ;
  // l'interpolation {variable} passe par tf()).
  t: (key: string, fallback?: string) => string;
}) {
  switch (status) {
    case "deployed":
      return (
        <Badge className="gap-1 bg-emerald-100 text-emerald-700 hover:bg-emerald-100">
          <CheckCircle2 className="size-3" />
          {t("portal.sigDeployed")}
        </Badge>
      );
    case "pending":
      return (
        <Badge className="gap-1 bg-amber-100 text-amber-700 hover:bg-amber-100">
          <Clock className="size-3" />
          {t("portal.sigPending")}
        </Badge>
      );
    case "never":
      return (
        <Badge variant="outline" className="gap-1">
          <AlertTriangle className="size-3" />
          {t("portal.sigNever")}
        </Badge>
      );
  }
}

/** PreviewDialog — dialog avec iframe srcDoc affichant le HTML personnalisé
 * du portail pour le routeur sélectionné. Fetch via /api/routers/{id}/portal-preview. */
function PreviewDialog({
  router,
  onClose,
  t,
  tf,
}: {
  router: RouterDevice | null;
  onClose: () => void;
  t: (key: string, fallback?: string) => string;
  tf: (key: string, vars: Record<string, string | number>) => string;
}) {
  const [html, setHtml] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>("");

  // Fetch le HTML quand le dialog s'ouvre.
  useQuery({
    queryKey: ["portal-preview", router?.id],
    queryFn: async () => {
      if (!router) return "";
      setLoading(true);
      setError("");
      try {
        const htmlContent = await fetchRouterPortalPreview(router.id);
        setHtml(htmlContent);
        return htmlContent;
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        setError(msg);
        setHtml("");
        return "";
      } finally {
        setLoading(false);
      }
    },
    enabled: router !== null,
  });

  return (
    <Dialog open={router !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] max-w-4xl overflow-hidden">
        <DialogHeader>
          <DialogTitle>
            {tf("portal.previewTitle", { name: router?.name ?? "" })}
          </DialogTitle>
          <DialogDescription>{t("portal.previewHint")}</DialogDescription>
        </DialogHeader>
        <div className="h-[600px] w-full overflow-hidden rounded-lg border">
          {loading ? (
            <div className="flex h-full items-center justify-center text-muted-foreground">
              <Loader2 className="size-6 animate-spin" />
            </div>
          ) : error ? (
            <div className="flex h-full items-center justify-center p-4 text-center text-sm text-destructive">
              {error}
            </div>
          ) : (
            <iframe
              srcDoc={html}
              title="Portal preview"
              sandbox="allow-scripts allow-same-origin"
              className="h-full w-full border-0"
            />
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
