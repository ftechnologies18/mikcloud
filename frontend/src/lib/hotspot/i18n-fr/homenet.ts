// Fragment FR du domaine « homenet » (N°100) — clés préfixées "home." et
// "devices." : la console du FOYER (tableau de bord maison + appareils).
// Le vocabulaire y est domestique (« box », « appareil », « couvre-feu ») :
// un parent n'y croise jamais « voucher », « revendeur » ni « client ».
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
  "home.kpi.devices": "Appareils connectés",
  "home.kpi.devicesSub": "en ce moment",
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
  "home.router.devices": "Appareils",
  "home.router.protectionCta": "Gérer la protection",
  "home.router.devicesCta": "Voir les appareils",

  // Aucun routeur — l'invitation honnête (le reste de la console est vide
  // tant que la box n'est pas connectée : pas de faux zéro).
  "home.empty.title": "Votre maison attend son routeur",
  "home.empty.desc": "Ajoutez votre box MikroTik : MikCloud surveillera votre réseau, vos appareils et votre protection familiale.",
  "home.empty.cta": "Ajouter mon routeur",

  // — devices (appareils connectés) —
  "devices.title": "Appareils",
  "devices.description": "Les appareils connectés à votre réseau, en direct",
  "devices.searchPlaceholder": "Nom, IP ou MAC…",
  "devices.device": "Appareil",
  "devices.kpi.connected": "Appareils connectés",
  "devices.kpi.connectedSub": "sur votre réseau domestique",
  "devices.kpi.download": "Trafic descendant",
  "devices.kpi.downloadSub": "tous appareils confondus",
  "devices.kpi.upload": "Trafic montant",
  "devices.kpi.uploadSub": "tous appareils confondus",
  "devices.empty.title": "Aucun appareil connecté",
  "devices.empty.desc": "Les appareils qui rejoignent votre WiFi apparaissent ici automatiquement.",
  "devices.noMatch": "Aucun appareil ne correspond",
  "devices.noMatchDesc": "Modifiez votre recherche pour retrouver un appareil.",
};
