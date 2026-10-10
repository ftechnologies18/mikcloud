// Fragment FR du domaine « cyber » — module Cybercafé (N°290).
// Clés préfixées "cyber." ; miroir exact de i18n-en/cyber.ts.

export const frCyber: Record<string, string> = {
  "cyber.title": "Cybercafé",
  "cyber.description":
    "Postes, codes-temps et caisse du cybercafé — un code par machine, vendu et encaissé en un geste.",

  // — Activation du module (opt-in, décision D6 de N°289) —
  "cyber.activate.title": "Module Cybercafé désactivé",
  "cyber.activate.desc":
    "Activez le module pour enregistrer vos postes, leur attribuer des codes-temps et couper une machine d'un clic. Les ventes partent dans Rapports et Comptabilité comme des ventes directes — rien à configurer ailleurs.",
  "cyber.activate.cta": "Activer le module",
  "cyber.activate.toast": "Module Cybercafé activé",
  "cyber.deactivate.toast": "Module Cybercafé désactivé (pauses de postes levées)",
  "cyber.settingsHint":
    "Module actif — le bouton ci-dessous le désactive (les pauses de postes sont levées automatiquement, aucune règle orpheline).",
  "cyber.deactivate.cta": "Désactiver le module",

  // — KPIs + caisse —
  "cyber.kpi.total": "Postes",
  "cyber.kpi.totalSub": "machines enregistrées",
  "cyber.kpi.busy": "Occupés",
  "cyber.kpi.busySub": "code-temps en cours",
  "cyber.kpi.paused": "En pause",
  "cyber.kpi.pausedSub": "internet coupé",
  "cyber.kpi.caisse": "Caisse du jour",
  "cyber.kpi.caisseSub": "ventes encaissées (tous canaux)",

  // — Table —
  "cyber.search": "Rechercher un poste…",
  "cyber.poste": "Poste",
  "cyber.router": "Routeur",
  "cyber.status": "Statut",
  "cyber.code": "Code-temps",
  "cyber.actions": "Actions",
  "cyber.status.free": "Libre",
  "cyber.status.busy": "Occupé",
  "cyber.status.paused": "En pause",
  "cyber.status.online": "En ligne",
  "cyber.status.active": "Jamais connecté",
  "cyber.status.used": "Utilisé",
  "cyber.status.expired": "Expiré",
  "cyber.status.disabled": "Désactivé",
  "cyber.code.remaining": "{used} / {limit} min",
  "cyber.assign": "Attribuer",
  "cyber.release": "Libérer",
  "cyber.pause": "Pause",
  "cyber.resume": "Reprendre",
  "cyber.rename": "Renommer",
  "cyber.delete": "Supprimer",
  "cyber.more": "Plus d'actions",
  "cyber.pause.30": "30 minutes",
  "cyber.pause.60": "1 heure",
  "cyber.pause.120": "2 heures",
  "cyber.pause.forever": "Jusqu'à réactivation",
  "cyber.pauseHint":
    "L'état affiché est l'état désiré : la box applique la coupure (ou la lève) à son prochain check-in — ≤ 45 s console ouverte.",

  // — États vides —
  "cyber.empty.title": "Aucun poste enregistré",
  "cyber.empty.desc":
    "Ajoutez vos machines une à une (la MAC est derrière le PC) ou importez-les depuis le DHCP de la box.",
  "cyber.empty.routerTitle": "Aucun routeur agent",
  "cyber.empty.routerDesc":
    "Installez l'agent MikCloud sur la box du cybercafé pour gérer les postes et la pause.",
  "cyber.noMatch": "Aucun poste ne correspond",
  "cyber.noMatchDesc": "Essayez un autre nom, une IP ou une MAC.",

  // — Ajout / import —
  "cyber.add": "Ajouter un poste",
  "cyber.import": "Importer (DHCP)",
  "cyber.addTitle": "Ajouter un poste",
  "cyber.addDesc":
    "La MAC est l'identité stable de la machine (l'IP tourne au gré des baux). Elle est unique par routeur.",
  "cyber.addMac": "Adresse MAC",
  "cyber.addMacPlaceholder": "AA:BB:CC:DD:EE:FF",
  "cyber.addName": "Nom du poste (optionnel)",
  "cyber.addNamePlaceholder": "PC-1, Machine fenêtre…",
  "cyber.addRouter": "Routeur (la box du cybercafé)",
  "cyber.addSave": "Ajouter",
  "cyber.addToast": "Poste ajouté",
  "cyber.discoverTitle": "Postes découverts (DHCP)",
  "cyber.discoverDesc":
    "Les bails DHCP rapportés par vos boxes agent — un clic pour enregistrer un poste.",
  "cyber.discoverEmpty":
    "Aucun appareil découvert pour l'instant : la box rapporte ses bails toutes les 2 minutes, revenez dans un instant.",
  "cyber.importCta": "Enregistrer",
  "cyber.imported": "Déjà enregistré",
  "cyber.importToast": "Poste enregistré depuis le DHCP",

  // — Attribution du code-temps —
  "cyber.assignTitle": "Attribuer un code-temps",
  "cyber.assignDesc":
    "Un code = un poste : le voucher (limit-uptime) est créé puis lié à la machine, et la vente part en caisse.",
  "cyber.assignProfile": "Forfait (profil)",
  "cyber.assignTime": "Quota temps en minutes (vide = forfait)",
  "cyber.assignTimePlaceholder": "Hériter du forfait",
  "cyber.assignCta": "Attribuer et encaisser",
  "cyber.assignedTitle": "Code attribué",
  "cyber.assignedDesc":
    "Le code est créé sur le routeur au prochain check-in (≤ 45 s). Saisissez-le sur l'écran de connexion du poste.",
  "cyber.assignedCode": "Code du poste",
  "cyber.assignedPrice": "Encaissé",
  "cyber.assignedHint": "Le code reste imprimable depuis Vouchers (lot du jour).",
  "cyber.assignedCopy": "Copier le code",
  "cyber.assignedCopied": "Code copié",
  "cyber.releaseToast": "Poste libéré (le code reste gérable dans Vouchers)",
  "cyber.releaseConfirmTitle": "Libérer ce poste ?",
  "cyber.releaseConfirmDesc":
    "Le poste redevient attribuable. Le code-temps lié reste actif jusqu'à épuisement — prolongez-le ou laissez-le courir depuis Vouchers.",
  "cyber.deleteConfirmTitle": "Supprimer ce poste ?",
  "cyber.deleteConfirmDesc":
    "Le poste quitte le registre. Si la machine était en pause, la coupure se lève au prochain check-in de la box.",
  "cyber.renameTitle": "Renommer le poste",
  "cyber.renamePlaceholder": "Nom du poste",
  "cyber.renameSave": "Enregistrer",
  "cyber.renameToast": "Poste renommé",
  "cyber.deleteToast": "Poste supprimé",
  "cyber.assignToast": "Code attribué au poste",
};
