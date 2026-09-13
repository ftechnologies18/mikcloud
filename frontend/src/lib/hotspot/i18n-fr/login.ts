// Fragment FR du domaine « login » — clés préfixées "login.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frLogin: Record<string, string> = {

  // — login —
  "login.username": "Identifiant",
  "login.password": "Mot de passe",
  "login.tabLogin": "Se connecter",
  "login.tabRegister": "Créer un compte",
  "login.register.name": "Nom complet / entreprise",
  "login.register.key": "Clé d'invitation (si requise)",
  "login.register.keyPlaceholder": "Optionnelle",
  "login.register.submit": "Créer mon compte",
  "login.welcome": "Bienvenue, {name}",
  "login.failed": "Connexion impossible",
  "login.serverWaking": "Réveil du serveur cloud en cours — nouvelle tentative (jusqu'à une minute)…",
  "login.networkError": "Serveur injoignable — vérifiez votre connexion Internet, puis réessayez.",
  "login.registerFailed": "Inscription impossible",
  "login.tagline": "Gestion Hotspot Cloud MikroTik",
  "login.logoAlt": "Logo MikCloud — nuage, signal WiFi et routeur MikroTik",
  "login.demoPrefix": "Démo : ",
  "login.useDemo": "Utiliser le compte démo",
  "login.noAccount": "Pas encore de compte ?",
  "login.createAccount": "Créer mon compte",
  "login.form.title": "Bon retour !",
  "login.form.subtitle": "Connectez-vous à votre console de gestion.",
  "login.showPassword": "Afficher le mot de passe",
  "login.hidePassword": "Masquer le mot de passe",
  "login.hero.badge": "Plateforme Hotspot Cloud",
  "login.hero.title": "Pilotez votre hotspot MikroTik, où que vous soyez.",
  "login.hero.subtitle": "Vouchers, profils, sessions et équipe — réunis dans une console unique.",
  "login.hero.f1.title": "Routeurs unifiés",
  "login.hero.f1.desc": "Tous vos MikroTik supervisés en temps réel depuis le cloud.",
  "login.hero.f2.title": "Vouchers & profils",
  "login.hero.f2.desc": "Génération par lots, quotas data et prix de vente automatisés.",
  "login.hero.f3.title": "Équipe & audit",
  "login.hero.f3.desc": "Rôles propriétaire/gérant et journal complet des actions.",
  "login.hero.f4.title": "Mode Vente terrain",
  "login.hero.f4.desc": "Vos revendeurs vendent en tournée depuis leur téléphone.",
  "login.totpPrompt": "Code 2FA requis — saisissez le code de votre application d'authentification",
  "login.totpCode": "Code 2FA",
  "login.totpHint": "Code à 6 chiffres affiché par votre application (renouvelé toutes les 30 secondes).",
  "login.totpSubmit": "Vérifier et se connecter",

  // — N°68 : mot de passe oublié (modale + page /reset-password) —
  "login.forgot": "Mot de passe oublié ?",

  // — N°8 : Mode Vente PWA —
  "login.tabSell": "Mode Vente",
  "login.tabRegisterShort": "Inscription",
  "login.tabSellShort": "Vente",
  "login.sellHint": "Revendeur en tournée : connectez-vous avec votre identifiant et le PIN donné par le gérant.",
  "login.sellUsername": "Identifiant revendeur",
  "login.sellPin": "PIN (4-6 chiffres)",
  "login.sellSubmit": "Ouvrir le Mode Vente",
};
