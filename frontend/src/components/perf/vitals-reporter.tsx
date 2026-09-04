"use client";

// VitalsReporter — B2 « Speed App UX » : collecte des Core Web Vitals réels
// (LCP, INP, CLS, FCP, TTFB) de TOUS les usagers — visiteurs anonymes de la
// vitrine inclus — vers POST /api/vitals (backend Render, beacon text/plain
// sans preflight CORS, réponse 204 jamais lue — contrat CONTRACT-V2.md B2).
//
// Choix d'implémentation :
//   - Monté dans le layout racine → mesure / (vitrine), /login, /app, /sell ;
//   - import() dynamique de web-vitals (~1,5 Ko gzip) APRÈS hydratation : le
//     module est hors du chemin critique — il ne dégrade pas le LCP qu'il
//     mesure (chargement à l'idle du navigateur, sans blocage du rendu) ;
//   - navigator.sendBeacon (fallback fetch keepalive) : fire-and-forget qui
//     survit au déchargement de la page — les valeurs finales de LCP/INP/CLS
//     partent même si l'usager ferme l'onglet ;
//   - web-vitals re-rapporte une métrique quand sa valeur évolue : chaque
//     rapport est envoyé, le backend déduplique en « dernier rapport par
//     (session, page, métrique) » (recommandation Google pour les p75) ;
//   - sid : identifiant de mesure éphémère en sessionStorage (par onglet,
//     sans cookie ni donnée personnelle) — sert uniquement à la déduplication
//     côté serveur ;
//   - aucune donnée nominative : chemin de page, métrique, rating, type de
//     navigation. IP et User-Agent bruts ne quittent jamais le serveur.

import { useEffect } from "react";
import type { Metric } from "web-vitals";

const API_BASE = (process.env.NEXT_PUBLIC_API_BASE || "").replace(/\/$/, "");

const SID_KEY = "mikcloud-vitals-sid";

function sessionMeasureId(): string {
  try {
    let sid = sessionStorage.getItem(SID_KEY) || "";
    if (!sid) {
      sid = (crypto.randomUUID?.() ?? `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`).slice(0, 32);
      sessionStorage.setItem(SID_KEY, sid);
    }
    return sid;
  } catch {
    return "anon"; // sessionStorage indisponible (navigation privée stricte)
  }
}

export default function VitalsReporter() {
  useEffect(() => {
    const sid = sessionMeasureId();

    const send = (m: Metric) => {
      const body = JSON.stringify({
        path: location.pathname.slice(0, 128),
        sid,
        nav: m.navigationType ?? "",
        metrics: [
          {
            name: m.name,
            // 2 décimales : CLS est un score sans unité (~0,0x), les durées
            // sont en ms entières — au-delà, c'est du bruit de stockage.
            value: Math.round(m.value * 100) / 100,
            rating: m.rating ?? "",
          },
        ],
      });
      // Blob text/plain = requête « simple » : jamais de preflight CORS.
      const blob = new Blob([body], { type: "text/plain;charset=UTF-8" });
      if (typeof navigator.sendBeacon === "function" && navigator.sendBeacon(`${API_BASE}/api/vitals`, blob)) {
        return;
      }
      // Fallback (vieux WebKit) : keepalive laisse partir la requête au
      // déchargement ; l'erreur CORS éventuelle est volontairement ignorée.
      void fetch(`${API_BASE}/api/vitals`, {
        method: "POST",
        body,
        keepalive: true,
        headers: { "Content-Type": "text/plain;charset=UTF-8" },
      }).catch(() => {});
    };

    let cancelled = false;
    void import("web-vitals")
      .then(({ onCLS, onFCP, onINP, onLCP, onTTFB }) => {
        if (cancelled) return;
        onCLS(send);
        onFCP(send);
        onINP(send);
        onLCP(send);
        onTTFB(send);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  return null;
}
