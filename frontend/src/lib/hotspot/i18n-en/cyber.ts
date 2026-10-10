// EN fragment for the "cyber" domain — Cybercafé module (N°290).
// Exact mirror of i18n-fr/cyber.ts (same keys).

export const enCyber: Record<string, string> = {
  "cyber.title": "Cybercafé",
  "cyber.description":
    "Workstations, time codes and the till — one code per machine, sold and collected in one gesture.",

  // — Module activation (opt-in, decision D6 of N°289) —
  "cyber.activate.title": "Cybercafé module is off",
  "cyber.activate.desc":
    "Turn the module on to register your workstations, hand out time codes and cut a machine in one click. Sales land in Reports and Accounting as direct sales — nothing else to configure.",
  "cyber.activate.cta": "Turn the module on",
  "cyber.activate.toast": "Cybercafé module enabled",
  "cyber.deactivate.toast": "Cybercafé module disabled (workstation pauses lifted)",
  "cyber.settingsHint":
    "Module active — the button below turns it off (workstation pauses are lifted automatically, no orphan firewall rules).",
  "cyber.deactivate.cta": "Turn the module off",

  // — KPIs + till —
  "cyber.kpi.total": "Workstations",
  "cyber.kpi.totalSub": "machines registered",
  "cyber.kpi.busy": "Busy",
  "cyber.kpi.busySub": "time code running",
  "cyber.kpi.paused": "Paused",
  "cyber.kpi.pausedSub": "internet cut",
  "cyber.kpi.caisse": "Till today",
  "cyber.kpi.caisseSub": "sales collected (all channels)",

  // — Table —
  "cyber.search": "Search a workstation…",
  "cyber.poste": "Workstation",
  "cyber.router": "Router",
  "cyber.status": "Status",
  "cyber.code": "Time code",
  "cyber.actions": "Actions",
  "cyber.status.free": "Free",
  "cyber.status.busy": "Busy",
  "cyber.status.paused": "Paused",
  "cyber.status.online": "Online",
  "cyber.status.active": "Never connected",
  "cyber.status.used": "Used",
  "cyber.status.expired": "Expired",
  "cyber.status.disabled": "Disabled",
  "cyber.code.remaining": "{used} / {limit} min",
  "cyber.assign": "Assign",
  "cyber.release": "Release",
  "cyber.pause": "Pause",
  "cyber.resume": "Resume",
  "cyber.rename": "Rename",
  "cyber.delete": "Delete",
  "cyber.more": "More actions",
  "cyber.pause.30": "30 minutes",
  "cyber.pause.60": "1 hour",
  "cyber.pause.120": "2 hours",
  "cyber.pause.forever": "Until reactivation",
  "cyber.pauseHint":
    "What you see is the DESIRED state: the box applies (or lifts) the cut at its next check-in — ≤ 45 s with the console open.",

  // — Empty states —
  "cyber.empty.title": "No workstations yet",
  "cyber.empty.desc":
    "Add your machines one by one (the MAC is printed on the PC) or import them from the box DHCP.",
  "cyber.empty.routerTitle": "No agent router",
  "cyber.empty.routerDesc":
    "Install the MikCloud agent on the cybercafé box to manage workstations and pauses.",
  "cyber.noMatch": "No workstation matches",
  "cyber.noMatchDesc": "Try another name, IP or MAC.",

  // — Add / import —
  "cyber.add": "Add a workstation",
  "cyber.import": "Import (DHCP)",
  "cyber.addTitle": "Add a workstation",
  "cyber.addDesc":
    "The MAC is the machine's stable identity (the IP rotates with leases). It is unique per router.",
  "cyber.addMac": "MAC address",
  "cyber.addMacPlaceholder": "AA:BB:CC:DD:EE:FF",
  "cyber.addName": "Workstation name (optional)",
  "cyber.addNamePlaceholder": "PC-1, Window machine…",
  "cyber.addRouter": "Router (the cybercafé box)",
  "cyber.addSave": "Add",
  "cyber.addToast": "Workstation added",
  "cyber.discoverTitle": "Discovered workstations (DHCP)",
  "cyber.discoverDesc": "DHCP leases reported by your agent boxes — one click to register a workstation.",
  "cyber.discoverEmpty":
    "No device discovered yet: the box reports its leases every 2 minutes, check back shortly.",
  "cyber.importCta": "Register",
  "cyber.imported": "Already registered",
  "cyber.importToast": "Workstation registered from DHCP",

  // — Time code assignment —
  "cyber.assignTitle": "Assign a time code",
  "cyber.assignDesc":
    "One code = one workstation: the voucher (limit-uptime) is created and bound to the machine, and the sale lands in the till.",
  "cyber.assignProfile": "Plan (profile)",
  "cyber.assignTime": "Time quota in minutes (empty = plan)",
  "cyber.assignTimePlaceholder": "Inherit from plan",
  "cyber.assignCta": "Assign and collect",
  "cyber.assignedTitle": "Code assigned",
  "cyber.assignedDesc":
    "The code is created on the router at the next check-in (≤ 45 s). Type it on the workstation login screen.",
  "cyber.assignedCode": "Workstation code",
  "cyber.assignedPrice": "Collected",
  "cyber.assignedHint": "The code stays printable from Vouchers (today's batch).",
  "cyber.assignedCopy": "Copy the code",
  "cyber.assignedCopied": "Code copied",
  "cyber.releaseToast": "Workstation released (the code stays manageable in Vouchers)",
  "cyber.releaseConfirmTitle": "Release this workstation?",
  "cyber.releaseConfirmDesc":
    "The workstation becomes assignable again. The linked time code stays active until it runs out — extend it or let it run from Vouchers.",
  "cyber.deleteConfirmTitle": "Delete this workstation?",
  "cyber.deleteConfirmDesc":
    "The workstation leaves the registry. If it was paused, the cut is lifted at the box's next check-in.",
  "cyber.renameTitle": "Rename the workstation",
  "cyber.renamePlaceholder": "Workstation name",
  "cyber.renameSave": "Save",
  "cyber.renameToast": "Workstation renamed",
  "cyber.deleteToast": "Workstation deleted",
  "cyber.assignToast": "Code assigned to the workstation",
};
