// Fragment FR du domaine « portal » — clés préfixées "portal.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frPortal: Record<string, string> = {

  // — N°35-d — portail captif (vue Portail) —
  "portal.title": "Portail captif",
  "portal.subtitle":
    "Déploiement automatique du portail sur vos routeurs agents — zéro intervention humaine.",
  "portal.empty": "Aucun routeur agent en ligne",
  "portal.emptyHint":
    "Les routeurs en mode agent déploient le portail automatiquement. Ajoutez un routeur en mode agent dans la section Infrastructure pour activer le portail captif.",
  "portal.sigDeployed": "Portail à jour",
  "portal.sigPending": "Re-déploiement en attente",
  "portal.sigNever": "Jamais déployé",
  "portal.preview": "Aperçu",
  "portal.previewTitle": "Aperçu du portail — {name}",
  "portal.previewHint":
    "Aperçu statique du portail (le fetch live n'est pas actif en aperçu — le branding se rafraîchit automatiquement côté routeur).",
  "portal.redeploy": "Re-déployer",
  "portal.redeployConfirmTitle": "Re-déploiement du portail",
  "portal.redeployConfirm":
    "Forcer le re-déploiement du portail sur «{name}» ? Le routeur rechargera tous les fichiers (login.html, status.html, assets) à son prochain check-in (≤ 45 s).",
  "portal.redeployQueued": "Re-déploiement programmé",
  "portal.cancel": "Annuler",
  // N°135 — le logo du portail est celui du client : où le poser.
  "portal.brandingNote":
    "Le logo affiché sur le portail est celui de votre établissement : posez-le dans l'onglet Expérience (carte « Vouchers ») — la bannière d'en-tête, la liste « Nos Services » et le bandeau animé sous le logo se règlent dans la même zone. Tout changement de branding est re-déployé automatiquement sur vos routeurs (≤ 45 s).",
  "portal.journal": "Journal des déploiements",
  "portal.journalEmpty": "Aucun déploiement enregistré",

  // — N°182 — sites physiques + personnalisation par site et par routeur —
  "portal.chainNote":
    "Chaque routeur sert le premier portail défini en remontant sa chaîne : sa personnalisation propre → celle de son site → celle du compte. Un champ vide hérite toujours du niveau supérieur ; tout changement est re-déployé automatiquement (≤ 45 s).",
  "portal.sites": "Sites",
  "portal.sitesHint":
    "Regroupez vos routeurs par établissement (bâtiment, boutique, campus) et donnez à chaque site sa propre identité de portail.",
  "portal.newSite": "Nouveau site",
  "portal.noSites":
    "Aucun site — tous vos routeurs servent le portail unifié du compte (réglages de l'onglet Expérience).",
  "portal.siteRouters": "{count} routeur(s)",
  "portal.sitePortalCustom": "Portail personnalisé",
  "portal.sitePortalAccount": "Portail du compte",
  "portal.siteEdit": "Modifier",
  "portal.siteDelete": "Supprimer",
  "portal.siteDeleteTitle": "Supprimer le site «{name}» ?",
  "portal.siteDeleteConfirm":
    "Ses {count} routeur(s) seront détachés et serviront à nouveau le portail du compte — mise à jour automatique au check-in suivant (≤ 45 s).",
  "portal.siteDeleted": "Site supprimé",
  "portal.siteCreated": "Site créé",
  "portal.siteUpdated": "Site modifié",
  "portal.siteName": "Nom du site",
  "portal.siteLocation": "Localisation (ville, quartier)",
  "portal.siteDescription": "Description",
  "portal.identitySection": "Identité du portail — vide = hérite du compte",
  "portal.routerSection": "Routeurs",
  "portal.routerSite": "Site",
  "portal.routerNoSite": "Hors site (portail du compte)",
  "portal.regimeAccount": "Portail : compte",
  "portal.regimeSite": "Portail : site",
  "portal.regimeCustom": "Portail : personnalisé",
  "portal.customize": "Personnaliser",
  "portal.customizeTitle": "Portail du routeur «{name}»",
  "portal.customizeHint":
    "Ces réglages remplacent, pour CE routeur, ceux de son site puis ceux du compte. Un champ vide hérite — le portail se re-déploie automatiquement (≤ 45 s).",
  "portal.resetOverride": "Réinitialiser (hériter)",
  "portal.overrideSaved": "Portail du routeur mis à jour — re-déploiement ≤ 45 s",
  "portal.siteSaved": "Site enregistré — portails re-déployés automatiquement (≤ 45 s)",
  "portal.assignSaved": "Routeur rattaché au site",
  "portal.displayName": "Nom affiché sur le portail",
  "portal.logoUrl": "Logo (data:image/… ≤ 300 Ko)",
  "portal.bannerUrl": "Bannière (data:image/… ou URL https://)",
  "portal.waveLink": "Lien marchand Wave",
  "portal.style": "Style du portail",
  "portal.styleInherit": "Hériter",
  "portal.styleCommercial": "Commercial",
  "portal.styleHospitality": "Hospitalité",
  "portal.welcome": "Message de bienvenue (mode hospitalité)",
  "portal.whatsappNumber": "WhatsApp support (chiffres, 8-15)",
  "portal.whatsappLabel": "Libellé WhatsApp",
  "portal.ticker": "Messages du bandeau animé (un par ligne, 5 max)",
  "portal.services": "Services « Nos Services » (un par ligne, 6 max)",
  "portal.slides": "Slides du carrousel (URL https par ligne, 3 max)",
  "portal.save": "Enregistrer",
};
