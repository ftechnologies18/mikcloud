"use client";

// N°60 — CTA d'installation PWA (« audit PWA », action 2).
//
// Constat : l'installation reposait sur le hasard — mini-infobar Chrome
// (supprimée définitivement après quelques rejets), aucun accompagnement iOS
// (Safari n'émet AUCUN événement d'installation). Ce composant porte le
// parcours complet :
// - Chrome/Edge/Android : l'événement `beforeinstallprompt` est capté AVANT
//   l'hydratation par le script inline du layout (slot window.__mikBip —
//   l'événement peut partir avant que React ne monte) puis relayé ici. Le
//   bouton déclenche le dialogue natif `prompt()` ; un prompt consommé ne
//   peut plus être rejoué (slot vidé) mais Chrome re-émet l'événement après
//   une nouvelle phase d'engagement — le CTA redevient proposable.
// - iOS : pas d'événement → le bouton ouvre une feuille d'instructions
//   « Partager → Sur l'écran d'accueil → Ajouter » (3 gestes, icônes à
//   l'appui). Détection iPadOS 13+ qui se présente comme un Mac desktop.
// - `appinstalled` → toast de confirmation (traduit hors rendu via le store,
//   pattern sell-shell) et purge du slot.
// - Rejet persistant (localStorage) : l'utilisateur garde la main — le CTA
//   ne réapparaît pas à chaque session. L'installation via le menu du
//   navigateur reste toujours possible.
// - Invisible quand l'app EST déjà lancée en standalone (hook N°8).
//
// Emplacements : login (funnel commun console + Mode Vente) et en-tête du
// Mode Vente (le revendeur connecté ne repasse jamais par le login — token
// persistant).

import { useEffect, useState } from "react";
import { Download, Loader2, Plus, Share2, Smartphone, X } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useIsStandalone } from "@/hooks/use-standalone";
import { tf as tfLang, useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";

/** Événement beforeinstallprompt — non standard, absent de lib.dom. */
interface BipEvent extends Event {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: "accepted" | "dismissed" }>;
}

/** Slot posé par le script inline du layout (capture pré-hydratation). */
type WindowWithBip = Window & { __mikBip?: BipEvent | null };

/** Rejet persistant du CTA — l'utilisateur ne doit pas le revoir à chaque session. */
const DISMISS_KEY = "mikcloud.pwa-install-dismissed";

function readBip(): BipEvent | null {
  return (window as WindowWithBip).__mikBip ?? null;
}

/** iOS / iPadOS. iPadOS 13+ se présente comme un Mac desktop : seul le
 * multi-touch le distingue d'un vrai macOS (qui n'a pas d'écran tactile). */
function isIosDevice(): boolean {
  if (typeof navigator === "undefined") return false;
  return (
    /iphone|ipad|ipod/i.test(navigator.userAgent) ||
    (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1)
  );
}

export function PwaInstallCta() {
  const { t } = useI18n();
  const standalone = useIsStandalone();

  const [bip, setBip] = useState<BipEvent | null>(null);
  const [ios, setIos] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  // Capture : reprise du slot pré-hydratation + écoute continue (l'événement
  // peut aussi arriver après le montage — Chrome exige une phase d'engagement
  // avant de le proposer).
  useEffect(() => {
    setIos(isIosDevice());
    setBip(readBip());
    setDismissed(localStorage.getItem(DISMISS_KEY) === "1");
    const onBip = (e: Event) => {
      e.preventDefault(); // la mini-infobar laisse la place à notre CTA
      (window as WindowWithBip).__mikBip = e as BipEvent;
      setBip(e as BipEvent);
    };
    const onInstalled = () => {
      // Toast traduit hors rendu (le handler vit au-delà du composant) —
      // même pattern que le replay hors-ligne du Mode Vente.
      (window as WindowWithBip).__mikBip = null;
      setBip(null);
      toast.success(tfLang(useHotspotStore.getState().lang, "pwa.installed", {}));
    };
    window.addEventListener("beforeinstallprompt", onBip);
    window.addEventListener("appinstalled", onInstalled);
    return () => {
      window.removeEventListener("beforeinstallprompt", onBip);
      window.removeEventListener("appinstalled", onInstalled);
    };
  }, []);

  if (standalone || dismissed || (!bip && !ios)) return null;

  async function install() {
    const ev = bip ?? readBip();
    // iOS : Safari n'émet jamais beforeinstallprompt — la feuille d'instructions.
    if (!ev) {
      setSheetOpen(true);
      return;
    }
    setBusy(true);
    try {
      await ev.prompt();
      // Le choix (accepted/dismissed) est purement informatif : appinstalled
      // confirme l'installation côté réussite, un refus laisse simplement le
      // CTA en place (Chrome re-proposera l'événement plus tard).
      await ev.userChoice;
    } catch {
      /* PWA optionnelle : échec silencieux, comme pwa-register. */
    } finally {
      setBusy(false);
      // Un prompt consommé ne peut plus être rejoué : purge du slot.
      (window as WindowWithBip).__mikBip = null;
      setBip(null);
    }
  }

  function dismiss() {
    localStorage.setItem(DISMISS_KEY, "1");
    setDismissed(true);
  }

  const iosSteps = [
    { icon: Share2, key: "pwa.ios.step1" },
    { icon: Plus, key: "pwa.ios.step2" },
    { icon: Smartphone, key: "pwa.ios.step3" },
  ] as const;

  return (
    <>
      <div className="mt-4 flex w-full items-center gap-3 rounded-xl border border-primary/20 bg-primary/5 p-3.5">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
          <Smartphone className="size-5" aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-sm leading-tight font-semibold">{t("pwa.install.title")}</p>
          <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">{t("pwa.install.desc")}</p>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Button size="sm" onClick={() => void install()} disabled={busy}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : <Download className="size-4" />}
            {t("pwa.install.button")}
          </Button>
          <Button
            size="icon"
            variant="ghost"
            onClick={dismiss}
            aria-label={t("pwa.install.later")}
            title={t("pwa.install.later")}
          >
            <X className="size-4" />
          </Button>
        </div>
      </div>

      {/* iOS — Safari n'a pas de bouton d'installation : la feuille guide
          les 3 gestes exacts (icônes à l'appui, dans l'ordre du menu). */}
      <Dialog open={sheetOpen} onOpenChange={setSheetOpen}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{t("pwa.ios.title")}</DialogTitle>
            <DialogDescription>{t("pwa.ios.desc")}</DialogDescription>
          </DialogHeader>
          <ol className="space-y-3">
            {iosSteps.map((step, i) => (
              <li key={step.key} className="flex items-center gap-3">
                <span className="flex size-9 shrink-0 items-center justify-center rounded-full border text-xs font-semibold text-muted-foreground">
                  {i + 1}
                </span>
                <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
                  <step.icon className="size-4" aria-hidden />
                </span>
                <span className="text-sm leading-relaxed">{t(step.key)}</span>
              </li>
            ))}
          </ol>
          <DialogFooter>
            <Button onClick={() => setSheetOpen(false)}>{t("pwa.ios.done")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
