// Fragment FR du domaine « wifi » — clés préfixées "wifi.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frWifi: Record<string, string> = {

  // — N°27 WiFi jetable —
  "wifi.title": "WiFi Jetable",
  "wifi.subtitle": "WiFi offert aux clients de vos établissements — restaurants, cafés, salons de coiffure, maquis et buvettes. Le client scanne un QR, laisse son numéro et reçoit son code.",
  "wifi.create": "Nouveau site WiFi",
  "wifi.empty": "Aucun site WiFi jetable",
  "wifi.emptyHint": "Créez un site pour offrir le WiFi à vos clients : QR sur les tables, code à quota gratuit, téléphone capté pour le marketing.",
  "wifi.name": "Nom de l'établissement",
  "wifi.namePh": "Maquis Chez Fofi",
  "wifi.router": "Routeur",
  "wifi.profile": "Quota offert (profil)",
  "wifi.profileHint": "Le prix du profil est le coût de chaque WiFi offert — 0 F recommandé.",
  "wifi.newProfile": "Nouveau quota",
  "wifi.freeTime": "Temps offert (min)",
  "wifi.freeTimeHint": "0 = hériter du profil",
  "wifi.freeData": "Data offerte (Mo)",
  "wifi.freeDataHint": "0 = hériter du profil",

  // N°54 — libellés dédoublonnés : les trois plafonds journaliers (perPhone /
  // perMac / dailyCap) portaient des intitulés interchangeables, le gérant
  // réglait le mauvais champ. Chaque champ dit maintenant QUI il limite.
  "wifi.perPhone": "Par numéro de téléphone",
  "wifi.perPhoneHint": "Tickets qu'UN même numéro peut obtenir en un jour (1 = un seul WiFi offert par numéro)",
  "wifi.perMac": "Par appareil (même WiFi)",
  "wifi.perMacHint": "Bloque un même téléphone qui tourne sous plusieurs numéros (claim du portail)",
  "wifi.dailyCap": "TOTAL offerts / jour (tous clients)",
  "wifi.dailyCapHint": "Plafond global du site : au-delà, le portail répond « épuisé » jusqu'à minuit",
  "wifi.optIn": "Consentement marketing demandé",
  "wifi.optInHint": "Case à cocher au client — numéros exportables (opt-in).",
  "wifi.active": "WiFi offert activé",
  "wifi.publicUrl": "URL publique",
  "wifi.copyUrl": "Copier l'URL",
  "wifi.poster": "Affiche QR",
  "wifi.guests": "Registre clients",
  "wifi.stats.capLabel": "Plafond du jour",
  "wifi.stats.today": "offerts aujourd'hui",
  "wifi.stats.optin": "opt-ins cumulés",
  "wifi.edit": "Modifier",
  "wifi.delete": "Supprimer",
  "wifi.deleteConfirm": "Supprimer ce site et son registre clients ? Les tickets déjà émis suivent leurs quotas.",
  "wifi.csv": "Exporter CSV",
  "wifi.guest.date": "Date",
  "wifi.guest.phone": "Téléphone",
  "wifi.guest.optin": "Opt-in",
  "wifi.guest.code": "Code",
  "wifi.guest.site": "Site",
  "wifi.saved": "Site WiFi enregistré",
  "wifi.deleted": "Site WiFi supprimé",

  // — N°63 wizard 2 étapes —
  "wifi.wiz.step1": "Le site",
  "wifi.wiz.step2": "L'offre",
  "wifi.wiz.step1Desc": "Identité de l'établissement — nom, routeur et quota de base.",
  "wifi.wiz.step2Desc": "Offre, réseau WiFi et protections anti-abus.",
  "wifi.wiz.stepOf": "Étape {n}/2",
  "wifi.wiz.continue": "Continuer",
  "wifi.wiz.back": "Retour",
  "wifi.wiz.err.name": "2 caractères minimum.",
  "wifi.wiz.noRouter": "Aucun routeur — ajoutez-en un d'abord dans l'onglet Routeurs.",
  "wifi.wiz.noProfile": "Aucun profil — créez-en un d'abord dans l'onglet Quotas.",
  "wifi.wiz.recap": "Récapitulatif de l'étape 1",
  "wifi.quota": "Offre",
  "wifi.postTitle": "WiFi Offert",
  "wifi.postScan": "Scannez, recevez votre code, connectez-vous",
  "wifi.postFoot": "Offert par l'établissement — au-delà du quota, passez à l'offre payante.",

  // — N°49 QR de connexion —
  "wifi.ssid": "SSID du réseau WiFi",
  "wifi.ssidHint": "Tel qu'il apparaît sur le téléphone du client — sert au QR de connexion de l'affiche.",
  "wifi.wifiPass": "Mot de passe WiFi (optionnel)",
  "wifi.wifiPassPh": "Vide si le réseau est ouvert",
  "wifi.wifiPassHint": "Renseigné : le QR connecte au réseau protégé (WPA).",
  "wifi.postWifi": "Scannez : le WiFi se connecte tout seul",
  "wifi.postWifiHint": "La page « WiFi Offert » s'ouvre à l'arrivée — entrez juste votre numéro.",
};
