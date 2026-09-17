"use client";

// N°141 — frontière d'erreur racine (anti-page-blanche).
//
// Audit PWA : AUCUNE frontière d'erreur n'existait dans src/app/ — Next.js
// n'en installe pas par défaut en production. Toute erreur de rendu client
// non attrapée (donnée inattendue, chunk en échec, crash d'hydratation)
// remontait jusqu'à la racine et DEMONTAIT l'arbre React : page vide, zéro
// récupération, l'utilisateur ne peut même pas repartir. Ce fichier est la
// frontière racine des segments de route : elle garde le layout (sidebar,
// thème, toasts) et offre deux sorties — Réessayer (reset() retente le
// rendu du segment) et Accueil. global-error.tsx couvre le cas plus rare
// d'un crash DU layout racine (il embarque ses propres styles).
//
// Écran d'urgence : volontairement autonome (aucune dépendance au store ou
// aux providers — juste Tailwind + lucide), bilingue compact FR/EN.

import { useEffect } from "react";
import { AlertTriangle, Home, RotateCw } from "lucide-react";
import { Button } from "@/components/ui/button";

export default function RouteError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // Diagnostic volontaire : sans ce log, un écran d'erreur silencieux
    // rend les crashes invisibles en production (aucun report embarqué).
    console.error("[mikcloud] erreur d'affichage :", error);
  }, [error]);

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-4 bg-background p-6 text-center text-foreground">
      <div className="grid size-14 place-items-center rounded-2xl bg-destructive/15 text-destructive">
        <AlertTriangle className="size-7" aria-hidden="true" />
      </div>
      <h1 className="text-lg font-semibold">Une erreur est survenue</h1>
      <p className="max-w-sm space-y-1 text-sm text-muted-foreground">
        <span>
          Cet écran n&apos;a pas pu s&apos;afficher. Vos données ne sont pas perdues — le serveur
          les conserve.
        </span>
        <span className="block text-xs">
          An unexpected error occurred while displaying this screen. Your data is safe.
        </span>
      </p>
      <div className="mt-1 flex flex-wrap items-center justify-center gap-2">
        <Button onClick={() => reset()} variant="default">
          <RotateCw aria-hidden="true" />
          Réessayer
        </Button>
        {/* Navigation DURE volontaire (eslint désactivé ci-dessous) : en
            sortie d'erreur, on veut un document entièrement neuf — un
            router.push() réutiliserait la pile cassée qu'on fuit. */}
        {/* eslint-disable-next-line @next/next/no-location-assign-relative-destination */}
        <Button onClick={() => (window.location.href = "/")} variant="outline">
          <Home aria-hidden="true" />
          Accueil
        </Button>
      </div>
    </main>
  );
}
