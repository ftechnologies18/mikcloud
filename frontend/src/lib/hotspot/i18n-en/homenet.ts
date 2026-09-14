// Fragment EN du domaine « homenet » (N°100) — clés préfixées "home." et
// "devices." : la console du FOYER (tableau de bord maison + appareils).
// Miroir exact de i18n-fr/homenet.ts (mêmes clés, mêmes valeurs traduites).
// N°101 — Phase 3 : les appareils vivent des baux DHCP de la box (fin du
// pis-aller « sessions ») : noms affectés, statut Online/Offline et LA
// pause dîner (couper l'internet d'un appareil, 30 min ou plus).
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
  "home.kpi.devices": "Devices online",
  "home.kpi.devicesSub": "on your WiFi, right now",
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
  "home.router.devices": "Devices online",
  "home.router.protectionCta": "Manage protection",
  "home.router.devicesCta": "View devices",

  // No router — the honest invitation (the rest of the console stays empty
  // until the box is connected: no fake zeros).
  "home.empty.title": "Your home is waiting for its router",
  "home.empty.desc": "Add your MikroTik box: MikCloud will watch your network, your devices and your family protection.",
  "home.empty.cta": "Add my router",

  // — devices (home devices — N°101: DHCP leases) —
  "devices.title": "Devices",
  "devices.description": "The devices of your home — named, tracked, under control",
  "devices.searchPlaceholder": "Name, device, IP or MAC…",
  "devices.device": "Device",
  "devices.status": "Status",
  "devices.lease": "Lease",
  "devices.actions": "Actions",

  // KPIs.
  "devices.kpi.total": "Devices",
  "devices.kpi.totalSub": "known to your box",
  "devices.kpi.online": "Online",
  "devices.kpi.onlineSub": "active DHCP lease",
  "devices.kpi.paused": "Paused",
  "devices.kpi.pausedSub": "internet cut off",

  // Status chips.
  "devices.status.online": "Online",
  "devices.status.offline": "Offline",
  "devices.status.paused": "Paused",

  // Dinner pause.
  "devices.pause": "Pause",
  "devices.pause.30": "30 minutes",
  "devices.pause.60": "1 hour",
  "devices.pause.120": "2 hours",
  "devices.pause.forever": "Until re-enabled",
  "devices.pauseFootnote": "Pause cuts the device's entire internet",
  "devices.resume": "Restore internet",
  "devices.pauseHint": "The pause applies at your box's next sync (about a minute) — the remaining time is shown in the status.",
  "devices.pauseToast": "{device}'s internet cut for {duration}.",
  "devices.pauseForeverToast": "{device}'s internet cut until re-enabled.",
  "devices.resumeToast": "{device}'s internet restored.",

  // Renaming.
  "devices.rename": "Rename",
  "devices.renameTitle": "Name the device",
  "devices.renameDesc": "A familiar name (\"Living room TV\", \"Mama's phone\") to spot each device at a glance.",
  "devices.renamePlaceholder": "Living room TV",
  "devices.renameCurrent": "Currently",
  "devices.renameSave": "Save",
  "devices.renameToast": "Device renamed",

  // Empty states.
  "devices.empty.title": "No devices yet",
  "devices.empty.desc": "Devices joining your box's WiFi appear here automatically.",
  "devices.empty.routerTitle": "The inventory awaits your box",
  "devices.empty.routerDesc": "Devices are discovered by your MikroTik router connected to MikCloud (agent mode).",
  "devices.noMatch": "No matching device",
  "devices.noMatchDesc": "Adjust your search to find a device.",
};
