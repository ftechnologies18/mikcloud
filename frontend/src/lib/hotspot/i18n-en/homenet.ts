// Fragment EN du domaine « homenet » (N°100) — clés préfixées "home." et
// "devices." : la console du FOYER (tableau de bord maison + appareils).
// Miroir exact de i18n-fr/homenet.ts (mêmes clés, mêmes valeurs traduites).
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const enHomeNet: Record<string, string> = {

  // — home (home dashboard) —
  "home.title": "Your home",
  "home.description": "Your home network at a glance",
  "home.loadError": "Couldn't load your network",
  "home.loadErrorDesc": "Check your connection, then try again.",

  // KPIs — the household's four questions.
  "home.kpi.router": "Router",
  "home.kpi.routerSub": "Your MikroTik box",
  "home.kpi.devices": "Connected devices",
  "home.kpi.devicesSub": "right now",
  "home.kpi.protection": "Protection",
  "home.kpi.protectionNone": "Add a router first",
  "home.kpi.curfew": "Internet curfew",
  "home.curfew.active": "Active",
  "home.curfew.scheduled": "Scheduled",
  "home.curfew.off": "Off",
  "home.curfew.offSub": "Enable it in Protection",

  // Box card.
  "home.routersSection": "Your network",
  "home.routersCount": "{n} router{p}",
  "home.router.model": "Model",
  "home.router.uptime": "Uptime",
  "home.router.lastSeen": "Last seen",
  "home.router.devices": "Devices",
  "home.router.protectionCta": "Manage protection",
  "home.router.devicesCta": "View devices",

  // No router — the honest invitation (the rest of the console stays empty
  // until the box is connected: no fake zeros).
  "home.empty.title": "Your home is waiting for its router",
  "home.empty.desc": "Add your MikroTik box: MikCloud will watch your network, your devices and your family protection.",
  "home.empty.cta": "Add my router",

  // — devices (connected devices) —
  "devices.title": "Devices",
  "devices.description": "The devices connected to your network, live",
  "devices.searchPlaceholder": "Name, IP or MAC…",
  "devices.device": "Device",
  "devices.kpi.connected": "Connected devices",
  "devices.kpi.connectedSub": "on your home network",
  "devices.kpi.download": "Download traffic",
  "devices.kpi.downloadSub": "across all devices",
  "devices.kpi.upload": "Upload traffic",
  "devices.kpi.uploadSub": "across all devices",
  "devices.empty.title": "No device connected",
  "devices.empty.desc": "Devices joining your WiFi appear here automatically.",
  "devices.noMatch": "No matching device",
  "devices.noMatchDesc": "Adjust your search to find a device.",
};
