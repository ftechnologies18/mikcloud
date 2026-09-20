// Fragment EN du domaine « banner » — clés préfixées "banner.".
// Extrait du monolithe i18n-en.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const enBanner: Record<string, string> = {
  "banner.expired.title": "Subscription expired",
  "banner.expired.desc": "Your subscription expired on {date}. The console is read-only — contact MikCloud to renew.",
  "banner.expired.descNoDate": "Your subscription has expired. The console is read-only — contact MikCloud to renew.",
  // N°164 — resilient boot: platform banner for suspended persistence.
  "banner.persistenceDegraded": "Persistence suspended — degraded mode: the service runs memory-only, automatic recovery in progress (details: Platform settings → Persistence health).",
};
