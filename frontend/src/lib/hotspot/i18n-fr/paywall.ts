// Fragment FR du domaine « paywall » — clés préfixées "paywall.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frPaywall: Record<string, string> = {
  "paywall.feesNote": "Frais de paiement inclus — remise mobile money de 3 % sur le prix de liste quand vous payez par Wave.",
  "paywall.cardPay": "Payer par carte (prélèvement auto)",

  // — Paywall (P5) — mur total si compte suspendu (PeriodEnd + 30j) —
  "paywall.title": "Compte suspendu",
  "paywall.subtitle": "Votre accès est suspendu. Choisissez une formule et réglez le paiement — MikCloud réactivera votre compte dès réception. Vos données sont conservées.",
  "paywall.warningTitle": "Accès suspendu",
  "paywall.warningDesc": "Vos routeurs continuent de fonctionner pour vos clients, mais vous ne pouvez plus accéder à la console. Vos données (routeurs, vouchers, sessions, rapports) sont conservées en sécurité.",
  "paywall.subscribe": "Souscrire & payer",
  "paywall.processing": "Traitement…",
  "paywall.requested": "Demande enregistrée — MikCloud activera votre accès dès réception du paiement.",
  "paywall.contact": "Une question ? Contactez l'équipe MikCloud.",
  "paywall.failed": "Échec de la souscription",
  "paywall.logout": "Se déconnecter",
  "paywall.unlimited": "Routeurs illimités",
  "paywall.perRouter": "Payez au fil de votre croissance",
};
