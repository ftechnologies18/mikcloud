"use client";

// N°27 — /join/[token] : page PUBLIQUE d'inscription par QR code (campus,
// écoles, administration, entreprise). Le token du lien fait l'authentification
// (whitelist middleware backend) — aucun état connecté requis.
// Scannée depuis un TÉLÉPHONE : mobile-first absolu — colonne centrée
// max-w-md, cibles tactiles ≥ 44 px, padding confortable, safe-area iOS.
// Étape 1 : GET /api/join/{token} (état du lien) — étape 2 : formulaire
// (components/hotspot/parts/join-form.tsx) — étape 3 : écrans de confirmation
// en état LOCAL (aucune route) avec bouton « Nouvelle inscription » (kiosque).

import { useState } from "react";
import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { motion, useReducedMotion } from "framer-motion";
import {
  Ban,
  CalendarClock,
  CheckCircle2,
  Copy,
  Hourglass,
  KeyRound,
  Link2Off,
  Loader2,
  PackageX,
  RotateCcw,
  WifiOff,
  Zap,
  type LucideIcon,
} from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import JoinForm from "@/components/hotspot/parts/join-form";
import type { JoinLinkInfo, SubmitOutcome } from "@/components/hotspot/parts/join-form";
import { copyToClipboard } from "@/components/hotspot/parts/uc-clipboard";
import { ApiError, api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { JoinLinkState } from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

type LockedState = Exclude<JoinLinkState, "active">;

/** Carte d'état verrouillé : icône + couleur par état (aucune action, ou Réessayer). */
const LOCKED_UI: Record<LockedState, { title: string; desc: string; icon: LucideIcon; iconClass: string }> = {
  revoked: { title: "joinPage.revoked.title", desc: "joinPage.revoked.desc", icon: Ban, iconClass: "text-destructive" },
  expired: { title: "joinPage.expired.title", desc: "joinPage.expired.desc", icon: CalendarClock, iconClass: "text-chart-3" },
  exhausted: { title: "joinPage.exhausted.title", desc: "joinPage.exhausted.desc", icon: PackageX, iconClass: "text-muted-foreground" },
};

export default function JoinPage() {
  const params = useParams<{ token: string | string[] }>();
  const token = Array.isArray(params.token) ? params.token[0] : params.token;
  const { lang, setLang, t } = useI18n();
  const reduce = useReducedMotion();

  // Écrans post-soumission (pending / approved / 429 / réseau) — état LOCAL,
  // pas de route : le bouton « Nouvelle inscription » remonte le formulaire.
  const [outcome, setOutcome] = useState<SubmitOutcome | null>(null);
  // Bump de clé = formulaire vierge remonté (usage kiosque).
  const [formKey, setFormKey] = useState(0);

  const linkQuery = useQuery({
    queryKey: ["join-link", token],
    queryFn: () => api<JoinLinkInfo>(`/api/join/${token}`),
    enabled: Boolean(token),
    // 4xx (lien inconnu, etc.) : inutile de réessayer ; erreur réseau :
    // une tentative de plus (réseau mobile instable).
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 1,
  });

  function resetForm() {
    setOutcome(null);
    setFormKey((k) => k + 1);
  }

  function renderBody() {
    // Jeton absent (route invariante) — défensif.
    if (!token) {
      return (
        <StateCard
          icon={Link2Off}
          iconClass="text-destructive"
          title={t("joinPage.unknown.title")}
          desc={t("joinPage.unknown.desc")}
        />
      );
    }
    // Étape 1 — chargement du lien.
    if (linkQuery.isPending) return <LoadingCard />;
    if (linkQuery.isError) {
      const err = linkQuery.error;
      if (err instanceof ApiError && err.status === 404) {
        // join_link_unknown — QR erroné ou lien supprimé ; Réessayer refait le GET.
        return (
          <StateCard
            icon={Link2Off}
            iconClass="text-destructive"
            title={t("joinPage.unknown.title")}
            desc={t("joinPage.unknown.desc")}
            onRetry={() => void linkQuery.refetch()}
          />
        );
      }
      return (
        <StateCard
          icon={WifiOff}
          iconClass="text-muted-foreground"
          title={t("joinPage.networkError.title")}
          desc={t("joinPage.networkError.desc")}
          onRetry={() => void linkQuery.refetch()}
        />
      );
    }
    const info = linkQuery.data;
    // Lien non actif : carte verrouillée (pas de bouton d'action).
    if (info.state !== "active") {
      const ui = LOCKED_UI[info.state];
      return <StateCard icon={ui.icon} iconClass={ui.iconClass} title={t(ui.title)} desc={t(ui.desc)} />;
    }
    // Écrans post-soumission.
    if (outcome && (outcome.kind === "pending" || outcome.kind === "approved")) {
      return <SuccessCard outcome={outcome} onReset={resetForm} />;
    }
    if (outcome?.kind === "rateLimited") {
      return (
        <StateCard
          icon={Hourglass}
          iconClass="text-chart-3"
          title={t("joinPage.rateLimited.title")}
          desc={t("joinPage.rateLimited.desc")}
          onRetry={() => setOutcome(null)}
        />
      );
    }
    if (outcome?.kind === "network") {
      return (
        <StateCard
          icon={WifiOff}
          iconClass="text-muted-foreground"
          title={t("joinPage.networkError.title")}
          desc={t("joinPage.networkError.desc")}
          onRetry={() => setOutcome(null)}
        />
      );
    }
    // Étape 2 — formulaire. Il reste MONTÉ (masqué) sous un écran d'issue
    // 429/réseau : « Réessayer » rend la main SANS perdre les valeurs saisies.
    return (
      <div className={cn(outcome && "hidden")}>
        <JoinForm
          key={formKey}
          token={token}
          link={info}
          onOutcome={setOutcome}
          onLinkClosed={() => void linkQuery.refetch()}
        />
      </div>
    );
  }

  return (
    <main className="flex min-h-screen flex-col bg-muted/30 px-4 pb-[max(2rem,env(safe-area-inset-bottom))] pt-[max(0.75rem,env(safe-area-inset-top))] sm:px-6">
      {/* Bascule de langue publique — autonymes FR/EN (noms de langues, non traduits). */}
      <div className="mx-auto flex w-full max-w-md justify-end">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-10 min-w-11 px-3 text-xs font-semibold text-muted-foreground"
          aria-label={t("joinPage.langAria")}
          onClick={() => setLang(lang === "fr" ? "en" : "fr")}
        >
          {lang === "fr" ? "EN" : "FR"}
        </Button>
      </div>
      {/* my-auto : centre quand l'écran est grand, dégrade en haut de page
          quand le formulaire déborde (aucun contenu coupé). */}
      <motion.div
        initial={reduce ? undefined : { opacity: 0, y: 14 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.4, ease: "easeOut" }}
        className="mx-auto my-auto w-full max-w-md pt-3"
      >
        {renderBody()}
      </motion.div>
    </main>
  );
}

/* ─── Écrans (état local) ─── */

/** Chargement du lien. */
function LoadingCard() {
  const { t } = useI18n();
  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
        <Loader2 className="size-6 animate-spin text-muted-foreground" aria-hidden />
        <p className="text-sm text-muted-foreground">{t("joinPage.loading")}</p>
      </CardContent>
    </Card>
  );
}

/** Carte d'état : lien verrouillé (révoqué/expiré/épuisé/inconnu), 429 ou réseau. */
function StateCard({
  icon: Icon,
  iconClass,
  title,
  desc,
  onRetry,
}: {
  icon: LucideIcon;
  iconClass: string;
  title: string;
  desc: string;
  onRetry?: () => void;
}) {
  const { t } = useI18n();
  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-4 py-8 text-center">
        <span className={cn("flex size-14 items-center justify-center rounded-2xl bg-muted", iconClass)}>
          <Icon className="size-7" aria-hidden />
        </span>
        <div className="space-y-1.5">
          <h2 className="text-lg font-semibold">{title}</h2>
          <p className="text-sm leading-relaxed text-muted-foreground">{desc}</p>
        </div>
        {onRetry && (
          <Button type="button" variant="outline" className="h-11 gap-2" onClick={onRetry}>
            <RotateCcw className="size-4" aria-hidden />
            {t("common.retry")}
          </Button>
        )}
      </CardContent>
    </Card>
  );
}

/** Écrans de confirmation — demande en attente (CheckCircle2 vert) ou compte
 * activé par lien kiosque (Zap teal). Rappel lisible des identifiants avec
 * bouton copier : l'utilisateur rejoindra le portail WiFi avec ces DEUX codes
 * distincts (mode régulier N°27, différent des vouchers nom=mot de passe). */
function SuccessCard({
  outcome,
  onReset,
}: {
  outcome: Extract<SubmitOutcome, { kind: "pending" | "approved" }>;
  onReset: () => void;
}) {
  const { t } = useI18n();
  const approved = outcome.kind === "approved";

  async function copyCredentials() {
    const ok = await copyToClipboard(`${outcome.username}\n${outcome.password}`);
    if (ok) toast.success(t("joinPage.copiedToast"));
    else toast.error(t("common.copyImpossible"));
  }

  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-5 py-8 text-center">
        <span
          className={cn(
            "flex size-14 items-center justify-center rounded-2xl",
            approved ? "bg-chart-2/15 text-chart-2" : "bg-primary/15 text-primary",
          )}
        >
          {approved ? <Zap className="size-7" aria-hidden /> : <CheckCircle2 className="size-7" aria-hidden />}
        </span>
        <div className="space-y-1.5">
          <h2 className="text-lg font-semibold">{t(approved ? "joinPage.approved.title" : "joinPage.pending.title")}</h2>
          <p className="text-sm leading-relaxed text-muted-foreground">
            {t(approved ? "joinPage.approved.desc" : "joinPage.pending.desc")}
          </p>
        </div>

        {approved && outcome.queued && (
          <p className="flex w-full items-start gap-2 rounded-lg border border-chart-3/30 bg-chart-3/10 px-3 py-2 text-left text-xs text-foreground/80">
            <Hourglass className="mt-0.5 size-3.5 shrink-0 text-chart-3" aria-hidden />
            {t("joinPage.approved.queued")}
          </p>
        )}

        <div className="w-full rounded-xl border bg-muted/40 p-4 text-left">
          <div className="mb-3 flex items-center justify-between gap-2">
            <span className="inline-flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
              <KeyRound className="size-3.5" aria-hidden />
              {t("joinPage.credentials.title")}
            </span>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-9 gap-1.5 px-2.5 text-xs"
              onClick={() => void copyCredentials()}
            >
              <Copy className="size-3.5" aria-hidden />
              {t("joinPage.copyCredentials")}
            </Button>
          </div>
          <dl className="space-y-3">
            <div>
              <dt className="text-xs text-muted-foreground">{t("joinPage.credentials.username")}</dt>
              <dd className="break-all font-mono text-sm font-semibold select-all">{outcome.username}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">{t("joinPage.credentials.password")}</dt>
              <dd className="break-all font-mono text-sm font-semibold select-all">{outcome.password}</dd>
            </div>
          </dl>
        </div>

        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-9 gap-1.5 text-muted-foreground"
          onClick={onReset}
        >
          <RotateCcw className="size-3.5" aria-hidden />
          {t("joinPage.newSignup")}
        </Button>
      </CardContent>
    </Card>
  );
}
