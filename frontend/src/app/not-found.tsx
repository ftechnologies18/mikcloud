// N°141 — 404 cohérente (anti-page-blanche UX).
//
// Sans ce fichier, une URL inconnue servait la 404 générique Next.js —
// écran système hors identité, et en PWA standalone un cul-de-sac. Ici :
// même fond nuit, même langage que offline.html (N°8), une seule sortie —
// l'accueil (qui re-déclenche les gardes de session de page.tsx).

export default function NotFound() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-3 bg-background p-6 text-center text-foreground">
      <p className="text-5xl font-black tracking-tight text-muted-foreground/60" aria-hidden="true">
        404
      </p>
      <h1 className="text-lg font-semibold">Page introuvable</h1>
      <p className="max-w-sm text-sm text-muted-foreground">
        Cette page n&apos;existe pas ou a été déplacée.
        <span className="mt-1 block text-xs">This page does not exist or has been moved.</span>
      </p>
      <a
        href="/"
        className="mt-2 rounded-xl bg-primary px-5 py-2.5 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
      >
        Retour à l&apos;accueil
      </a>
    </main>
  );
}
