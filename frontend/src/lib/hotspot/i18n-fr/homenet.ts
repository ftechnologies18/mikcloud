// Fragment FR du domaine « homenet » (N°100) — clés préfixées "home." et
// "devices." : la console du FOYER (tableau de bord maison + appareils).
// Le vocabulaire y est domestique (« box », « appareil », « couvre-feu ») :
// un parent n'y croise jamais « voucher », « revendeur » ni « client ».
// N°101 — Phase 3 : les appareils vivent des bails DHCP de la box (fin du
// pis-aller « sessions ») : noms affectés, statut En ligne/Hors ligne et
// LA pause dîner (couper l'internet d'un appareil, 30 min ou plus).
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frHomeNet: Record<string, string> = {

  // — home (tableau de bord maison) —
  "home.title": "Votre maison",
  "home.description": "L'état de votre réseau domestique, en un coup d'œil",
  "home.loadError": "Impossible de charger votre réseau",
  "home.loadErrorDesc": "Vérifiez votre connexion, puis réessayez.",

  // KPIs — les quatre questions du foyer.
  "home.kpi.router": "Routeur",
  "home.kpi.routerSub": "Votre box MikroTik",
  "home.kpi.devices": "Appareils en ligne",
  "home.kpi.devicesSub": "sur votre WiFi, maintenant",
  // N°102 — le zéro inexpliqué n'existe plus : la box découvre (agent là,
  // premier rapport en route) ou l'enseigne dit le geste à faire (non-agent).
  "home.kpi.devicesWaiting": "votre box découvre votre réseau…",
  "home.kpi.devicesAgent": "connectez votre box en mode agent",
  "home.kpi.protection": "Protection",
  "home.kpi.protectionNone": "Ajoutez d'abord un routeur",
  "home.kpi.curfew": "Couvre-feu internet",
  "home.curfew.active": "Actif",
  "home.curfew.scheduled": "Programmé",
  "home.curfew.off": "Désactivé",
  "home.curfew.offSub": "À activer dans Protection",

  // Carte box.
  "home.routersSection": "Votre réseau",
  "home.routersCount": "{n} routeur{p}",
  "home.router.model": "Modèle",
  "home.router.uptime": "Allumé depuis",
  "home.router.lastSeen": "Dernier contact",
  "home.router.devices": "Appareils en ligne",
  "home.router.protectionCta": "Gérer la protection",
  "home.router.devicesCta": "Voir les appareils",

  // N°102 — enseigne « mode agent requis » : des routeurs existent mais
  // aucun en mode agent → l'inventaire (et la pause dîner) ne peut pas vivre.
  "home.agentHint": "Les appareils du foyer sont découverts par votre box MikroTik connectée à MikCloud en mode agent — l'inventaire et la pause dîner vivent de ses bails DHCP.",
  "home.agentHintCta": "Voir mes routeurs",

  // Aucun routeur — l'invitation honnête (le reste de la console est vide
  // tant que la box n'est pas connectée : pas de faux zéro).
  "home.empty.title": "Votre maison attend son routeur",
  "home.empty.desc": "Ajoutez votre box MikroTik : MikCloud surveillera votre réseau, vos appareils et votre protection familiale.",
  "home.empty.cta": "Ajouter mon routeur",

  // — devices (appareils de la maison — N°101 : baux DHCP) —
  "devices.title": "Appareils",
  "devices.description": "Les appareils de votre maison — nommés, suivis, sous contrôle",
  "devices.searchPlaceholder": "Nom, appareil, IP ou MAC…",
  "devices.device": "Appareil",
  "devices.status": "Statut",
  "devices.lease": "Bail",
  "devices.actions": "Actions",

  // KPIs.
  "devices.kpi.total": "Appareils",
  "devices.kpi.totalSub": "connus de votre box",
  "devices.kpi.online": "En ligne",
  "devices.kpi.onlineSub": "bail DHCP actif",
  "devices.kpi.paused": "En pause",
  "devices.kpi.pausedSub": "internet coupé",

  // Statuts (chip).
  "devices.status.online": "En ligne",
  "devices.status.offline": "Hors ligne",
  "devices.status.paused": "En pause",

  // Pause dîner.
  "devices.pause": "Mettre en pause",
  "devices.pause.30": "30 minutes",
  "devices.pause.60": "1 heure",
  "devices.pause.120": "2 heures",
  "devices.pause.forever": "Jusqu'à réactivation",
  "devices.pauseFootnote": "La pause coupe tout l'internet de l'appareil",
  "devices.resume": "Rétablir internet",
  "devices.pauseHint": "La pause s'applique à la prochaine synchronisation de votre box (environ une minute) — le temps restant est indiqué dans le statut.",
  "devices.pauseToast": "Internet de {device} coupé pendant {duration}.",
  "devices.pauseForeverToast": "Internet de {device} coupé jusqu'à réactivation.",
  "devices.resumeToast": "Internet de {device} rétabli.",

  // Renommage.
  "devices.rename": "Renommer",
  "devices.renameTitle": "Nommer l'appareil",
  "devices.renameDesc": "Un nom familier (« TV du salon », « Tel de mama ») pour repérer chaque appareil d'un coup d'œil.",
  "devices.renamePlaceholder": "TV du salon",
  "devices.renameCurrent": "Actuellement",
  "devices.renameSave": "Enregistrer",
  "devices.renameToast": "Appareil renommé",

  // États vides.
  "devices.empty.title": "Aucun appareil pour l'instant",
  "devices.empty.desc": "Les appareils qui se connectent au WiFi de votre box apparaissent ici automatiquement.",
  "devices.empty.routerTitle": "L'inventaire attend votre box",
  "devices.empty.routerDesc": "Les appareils sont découverts par votre routeur MikroTik connecté à MikCloud (mode agent).",
  "devices.noMatch": "Aucun appareil ne correspond",
  "devices.noMatchDesc": "Modifiez votre recherche pour retrouver un appareil.",
};
