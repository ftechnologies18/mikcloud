// Fragment FR du domaine « signup » — clés préfixées "signup.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frSignup: Record<string, string> = {

  // — Signup (modale 2 étapes) —
  "signup.title": "Créer mon compte",
  "signup.titleStep2": "Votre profil",
  "signup.subtitle": "Essai gratuit de 90 jours — 1 routeur inclus, sans carte bancaire.",
  "signup.subtitleStep2": "Pour vous aider et segmenter notre offre — nous ne spammons jamais.",
  "signup.step1": "Compte",
  "signup.step2": "Profil",
  // N°101 — usage du réseau : la première question de l'inscription
  // choisit la console d'atterrissage (lieu public vs foyer).
  "signup.usage.label": "Votre réseau",
  "signup.usage.hotspot.title": "Un lieu public",
  "signup.usage.hotspot.desc": "Maquis, cybercafé, boutique — vous vendez l'accès internet.",
  "signup.usage.homenet.title": "Ma maison",
  "signup.usage.homenet.desc": "Vous protégez le réseau familial et ses appareils.",
  "signup.name": "Nom du compte",
  "signup.username": "Identifiant",
  "signup.usernameHint": "3 à 32 caractères : a-z, 0-9, tirets et tirets bas",
  "signup.password": "Mot de passe",
  "signup.passwordHint": "Minimum 10 caractères",
  "signup.email": "Email",
  "signup.emailInvalid": "Format d'email invalide",
  "signup.phone": "Numéro WhatsApp",
  "signup.phoneInvalid": "Numéro invalide (8 à 15 chiffres)",
  "signup.phoneHint": "Pour le support WhatsApp — nous ne partageons jamais ce numéro",
  "signup.country": "Pays",
  "signup.countryPlaceholder": "Sélectionnez votre pays",
  "signup.city": "Ville d'activité",
  "signup.continue": "Continuer",
  "signup.back": "Retour",
  "signup.submit": "Créer mon compte",
  "signup.trialNote": "Votre essai de 90 jours démarre immédiatement. 1 routeur inclus, sans carte bancaire.",
  "signup.profileNote": "Ces informations restent confidentielles et servent au support et à la segmentation.",
  "signup.privacyPrefix": "J'ai lu et j'accepte la",
  "signup.privacyLink": "politique de confidentialité",
  "signup.success": "Compte créé — bienvenue sur MikCloud !",
  "signup.failed": "Inscription impossible",
};
