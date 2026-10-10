// Fragment EN of the "ppp" domain — PPPoE subscribers (WISP console, N°294).
// Keys prefixed "ppp." ; exact mirror of i18n-fr/ppp.ts.

export const enPpp: Record<string, string> = {
  "ppp.title": "PPPoE Subscribers",
  "ppp.description":
    "Manage the subscribers of your routers' PPPoE servers: creation, suspension, renewal and disconnection — driven by the MikCloud agent (no public port opened).",

  // — Router selection (the whole view is scoped to ONE router) —
  "ppp.routerLabel": "Router",
  "ppp.routerHint": "PPPoE is driven by the MikCloud agent: real-mode routers are not eligible.",
  "ppp.noRouter.title": "No agent-mode router",
  "ppp.noRouter.desc":
    "PPPoE subscribers live on your MikroTik routers. Add a router and install the MikCloud agent (agent mode) to drive it from this console.",
  "ppp.noRouter.cta": "Go to routers",

  // — KPIs —
  "ppp.kpi.subscribers": "Subscribers",
  "ppp.kpi.subscribersSub": "registry of the selected router",
  "ppp.kpi.online": "Online",
  "ppp.kpi.onlineSub": "active sessions (agent cache)",
  "ppp.kpi.onlineUnknown": "queued read…",
  "ppp.kpi.suspended": "Suspended",
  "ppp.kpi.suspendedSub": "auto: {n}",
  "ppp.kpi.expiring": "Expiring ≤ 7 d",
  "ppp.kpi.expiringSub": "renew soon",

  // — Tabs —
  "ppp.tab.subscribers": "Subscribers",
  "ppp.tab.sessions": "Active sessions",
  "ppp.tab.discover": "Discovery",

  // — Subscribers table —
  "ppp.search": "Search a subscriber…",
  "ppp.name": "Name",
  "ppp.profile": "Profile",
  "ppp.staticIp": "Static IP",
  "ppp.pool": "pool",
  "ppp.expires": "Expiry",
  "ppp.expiresNever": "Unlimited",
  "ppp.expired": "expired",
  "ppp.expiresIn": "D-{n}",
  "ppp.status": "Status",
  "ppp.state.active": "Active",
  "ppp.state.pending": "Pending",
  "ppp.state.error": "Error",
  "ppp.state.disabled": "Suspended",
  "ppp.state.disabledAuto": "Suspended (auto)",
  "ppp.parity": "Parity",
  "ppp.lastSeen": "seen {time}",
  "ppp.neverSeen": "never seen",
  "ppp.actions": "Actions",
  "ppp.more": "More actions",
  "ppp.selectAll": "Select all",
  "ppp.errorRetry": "Retry",

  // — Multi-selection —
  "ppp.bulk.selected": "{n} selected",
  "ppp.bulk.renew": "Renew selection",
  "ppp.bulk.renewDone": "{n} subscriber(s) renewed",
  "ppp.bulk.renewPartial": "{n} renewed, {m} failed",

  // — Empty states —
  "ppp.empty.title": "No subscriber on this router",
  "ppp.empty.desc":
    "Create the first subscriber: the secret is pushed to the router at the next agent check-in (≤ 45 s). The PPPoE server must already exist on the router — otherwise use assisted provisioning.",
  "ppp.noMatch": "No subscriber matches",
  "ppp.noMatchDesc": "Try another name, profile or comment.",

  // — Creation —
  "ppp.add": "New subscriber",
  "ppp.addTitle": "Create a PPPoE subscriber",
  "ppp.addDesc":
    "The secret is stored in the cloud then pushed to the router by the agent (queued command — confirmation at the next check-in, ≤ 45 s).",
  "ppp.addName": "Subscriber name (login)",
  "ppp.addNamePlaceholder": "e.g. subscriber1, jean.dupont@isp…",
  "ppp.addPassword": "PPP password",
  "ppp.addPasswordPlaceholder": "connection secret",
  "ppp.addProfile": "PPP profile (must exist on the router)",
  "ppp.addProfilePlaceholder": "e.g. mikcloud-ppp",
  "ppp.addProfileHint": "The cloud does not create PPP profiles: the name must already exist on the router (or come from assisted provisioning).",
  "ppp.addComment": "Comment (optional)",
  "ppp.addExpires": "Expiry (optional)",
  "ppp.addExpiresHint": "Empty = unlimited. Past the expiry: automatic suspension, reminder or auto-renewal depending on the subscriber's settings.",
  "ppp.addStatic": "Static IP (optional)",
  "ppp.addStaticHint": "Empty = address assigned from the profile pool. The IP must be free on this router.",
  "ppp.addCta": "Create subscriber",
  "ppp.addPending": "Creating…",
  "ppp.addToast": "Subscriber created (command queued to the router)",

  // — Renewal (F4) —
  "ppp.renew": "Renew",
  "ppp.renewTitle": "Renew {name}",
  "ppp.renewCurrent": "Current expiry: {date}.",
  "ppp.renewNoDate": "Unlimited subscriber.",
  "ppp.renewAuto": "The new expiry starts from the furthest one (current or today).",
  "ppp.renewDays": "Renewal length (days)",
  "ppp.renewDaysHint": "1 to 3650 days — the expiry reminder is re-armed for the new date.",
  "ppp.renewAutoNote": "Re-enables a subscriber suspended automatically.",
  "ppp.renewSubmit": "Renew",
  "ppp.renewToast": "Subscriber renewed",

  // — Edition —
  "ppp.edit": "Edit",
  "ppp.editTitle": "Edit {name}",
  "ppp.editDesc": "Only modified fields are sent to the router (partial set — confirmation at the next check-in).",
  "ppp.editPassword": "PPP password",
  "ppp.editPasswordHint": "Leave empty to keep the current password.",
  "ppp.editProfile": "PPP profile",
  "ppp.editComment": "Comment",
  "ppp.editExpires": "Expiry",
  "ppp.editExpiresHint": "Empty = unlimited.",
  "ppp.editStatic": "Static IP",
  "ppp.editStaticHint": "Empty = profile pool.",
  "ppp.editExpMode": "At expiry",
  "ppp.editExpMode.disable": "Suspend the subscriber (default)",
  "ppp.editExpMode.none": "Do nothing (router parity only)",
  "ppp.editAutoRenew": "Automatic renewal (recurring, no billing)",
  "ppp.editRenewDays": "Days added at each expiry",
  "ppp.editRemind": "Expiry reminder",
  "ppp.editRemind.off": "Off",
  "ppp.editRemind.days": "{n} d before",
  "ppp.editSubmit": "Save",
  "ppp.editToast": "Subscriber updated",

  // — Suspension / resume —
  "ppp.suspend": "Suspend",
  "ppp.resume": "Resume",
  "ppp.suspendConfirmTitle": "Suspend this subscriber?",
  "ppp.suspendConfirmDesc":
    "The secret is disabled on the router (session cut, new logins refused). Resume at any time — the subscriber and their expiry are kept.",
  "ppp.suspendToast": "Subscriber suspended",
  "ppp.resumeToast": "Subscriber re-enabled",

  // — Disconnection —
  "ppp.kick": "Disconnect (agent)",
  "ppp.kickLive": "Disconnect (real time)",
  "ppp.kickToast": "Disconnection queued to the router",
  "ppp.kickLiveToast": "Session disconnected (real time)",

  // — Deletion —
  "ppp.delete": "Delete",
  "ppp.deleteConfirmTitle": "Delete this subscriber?",
  "ppp.deleteConfirmDesc":
    "The secret is removed from the router upon agent confirmation (≤ 45 s). The row leaves the registry ONLY after confirmation — never before.",
  "ppp.deleteToast": "Deletion requested",

  // — Active sessions tab —
  "ppp.sessions.empty": "No active PPPoE session",
  "ppp.sessions.emptyDesc": "The cache fills at the next agent report (2 min TTL) — or run a real-time read.",
  "ppp.sessions.queued": "Read queued — the report arrives at the agent's next check-in…",
  "ppp.sessions.updated": "Agent cache: {time}",
  "ppp.sessions.col.name": "Subscriber",
  "ppp.sessions.col.service": "Service",
  "ppp.sessions.col.address": "IP address",
  "ppp.sessions.col.caller": "Caller ID",
  "ppp.sessions.col.uptime": "Uptime",
  "ppp.sessions.live": "Real time via tunnel",
  "ppp.sessions.livePending": "Direct read in progress…",
  "ppp.sessions.liveCount": "{n} session(s) · {ms} ms through the tunnel",
  "ppp.sessions.liveErrorTitle": "Real-time read failed",

  // — Discovery tab —
  "ppp.discover.title": "Secrets seen by the router",
  "ppp.discover.empty": "No secret discovered",
  "ppp.discover.emptyDesc": "The router has not reported its pppoe-server yet (or it is empty) — the read starts automatically.",
  "ppp.discover.queued": "Read queued — report in progress…",
  "ppp.discover.updated": "Agent cache: {time}",
  "ppp.discover.note":
    "MikCloud does not modify discovered secrets: they were created outside MikCloud (Winbox, Mikhmon…). Create a subscriber in the Subscribers tab only if it does not already exist on the router.",
  "ppp.discover.col.name": "Name",
  "ppp.discover.col.profile": "Profile",
  "ppp.discover.col.disabled": "Suspended",
  "ppp.discover.col.service": "Service",
  "ppp.discover.col.comment": "Comment (router)",

  // — Real-time reinforcement (phase B) —
  "ppp.renfort.title": "Real time via tunnel",
  "ppp.renfort.desc": "Instant reads and disconnections through the WireGuard tunnel — the agent channel (≤ 45 s) remains the bedrock.",
  "ppp.renfort.credsOk": "API credentials set",
  "ppp.renfort.credsMissing": "API credentials missing",
  "ppp.renfort.tunnelOk": "WireGuard tunnel active",
  "ppp.renfort.tunnelMissing": "No tunnel (WireGuard reinforcement required)",
  "ppp.renfort.configure": "Configure",
  "ppp.renfort.remove": "Remove credentials",
  "ppp.renfort.removeConfirmTitle": "Remove the API credentials?",
  "ppp.renfort.removeConfirmDesc":
    "Real time via tunnel becomes unavailable. Agent-based control (≤ 45 s) is never affected.",
  "ppp.renfort.removeToast": "Credentials removed",
  "ppp.renfort.dialogTitle": "RouterOS API credentials",
  "ppp.renfort.dialogDesc":
    "Reuses the router's API account: credentials are encrypted at rest and never leave the backend. Enable the API service on the router (/ip service enable api).",
  "ppp.renfort.username": "API username",
  "ppp.renfort.usernamePlaceholder": "e.g. mikcloud-api",
  "ppp.renfort.password": "API password",
  "ppp.renfort.passwordPlaceholder": "API account password",
  "ppp.renfort.save": "Save credentials",
  "ppp.renfort.saveToast": "Credentials saved (encrypted at rest)",
  "ppp.renfort.note":
    "Prerequisites: RouterOS API enabled on the router (/ip service enable api) and the router tunneled (WireGuard reinforcement) — the backend dials 10.8.0.N:8728 through wg0.",

  // — Assisted provisioning (phase B) —
  "ppp.provision": "Provision the PPPoE server",
  "ppp.provisionTitle": "Assisted provisioning of the PPPoE server",
  "ppp.provisionDesc":
    "Generates an IDEMPOTENT .rsc script (each block only creates its object if absent): pool, profile and PPPoE server. No secret, no queued command — you paste the script yourself.",
  "ppp.provisionInterface": "LAN interface to bridge",
  "ppp.provisionInterfacePlaceholder": "e.g. ether2",
  "ppp.provisionService": "Service-name",
  "ppp.provisionProfile": "Default profile",
  "ppp.provisionPoolStart": "Address pool start",
  "ppp.provisionPoolEnd": "Address pool end",
  "ppp.provisionLocal": "Local address (optional)",
  "ppp.provisionDns": "DNS servers (optional — comma separated)",
  "ppp.provisionDnsPlaceholder": "e.g. 1.1.1.1, 8.8.8.8",
  "ppp.provisionCta": "Generate script",
  "ppp.provisionPending": "Generating…",
  "ppp.provisionCopy": "Copy script",
  "ppp.provisionCopied": "Script copied",
  "ppp.provisionDownload": "Download (.rsc)",
  "ppp.provisionScriptNote": "Paste into the router's terminal — idempotent: re-running duplicates nothing.",
  "ppp.provisionWarnings": "Server warnings",

  // — Misc —
  "ppp.errorToast": "Operation failed",
};
