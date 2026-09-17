"use client";

// N°141 — frontière d'erreur GLOBALE (anti-page-blanche, dernier filet).
//
// Cas plus rare que error.tsx : le crash vient du LAYOUT RACINE lui-même
// (provider, police, script). Next.js remplace alors TOUT le document —
// ce fichier doit donc embarquer ses propres <html>/<body> et ne peut NI
// compter sur globals.css NI sur les composants UI (la pile est suspecte).
// Styles inline volontairement minimaux, palette nuit MikCloud, mêmes
// deux sorties que error.tsx (Réessayer / Accueil).

import { useEffect } from "react";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error("[mikcloud] erreur globale :", error);
  }, [error]);

  const btn: React.CSSProperties = {
    border: 0,
    borderRadius: 12,
    padding: "12px 24px",
    fontSize: 15,
    fontWeight: 600,
    cursor: "pointer",
  };

  return (
    <html lang="fr">
      <body
        style={{
          margin: 0,
          minHeight: "100dvh",
          display: "grid",
          placeItems: "center",
          background: "#101012",
          color: "#fafafa",
          fontFamily: "system-ui, -apple-system, sans-serif",
          textAlign: "center",
          padding: 24,
        }}
      >
        <main>
          <div
            style={{
              width: 56,
              height: 56,
              margin: "0 auto 16px",
              borderRadius: 16,
              display: "grid",
              placeItems: "center",
              background: "rgba(239,68,68,0.15)",
              color: "#f87171",
              fontSize: 26,
            }}
            aria-hidden="true"
          >
            ⚠
          </div>
          <h1 style={{ fontSize: 20, margin: "0 0 8px" }}>Une erreur est survenue</h1>
          <p style={{ color: "#a1a1aa", fontSize: 14, lineHeight: 1.5, margin: "0 0 24px" }}>
            L&apos;application n&apos;a pas pu démarrer. Vos données ne sont pas perdues — le
            serveur les conserve.
            <br />
            <span style={{ fontSize: 12 }}>
              The app failed to start. Your data is safe on the server.
            </span>
          </p>
          <div style={{ display: "flex", gap: 12, justifyContent: "center", flexWrap: "wrap" }}>
            <button onClick={() => reset()} style={{ ...btn, background: "#10b981", color: "#052e22" }}>
              Réessayer
            </button>
            <a
              href="/"
              style={{
                ...btn,
                background: "transparent",
                color: "#fafafa",
                border: "1px solid #52525b",
                textDecoration: "none",
                display: "inline-block",
              }}
            >
              Accueil
            </a>
          </div>
        </main>
      </body>
    </html>
  );
}
