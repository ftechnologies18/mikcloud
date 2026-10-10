// Fragment EN of the "vpn" domain — sellable WireGuard (N°291).
// Keys prefixed "vpn." ; exact mirror of i18n-fr/vpn.ts.

export const enVpn: Record<string, string> = {
  "vpn.title": "WireGuard VPN",
  "vpn.description":
    "Sell WireGuard VPN access to your clients — configs generated on your own server, delivered via QR, e-mail or Telegram, revoked in one click.",

  // — Module activation (opt-in, same D6 doctrine as Cybercafé) —
  "vpn.activate.title": "VPN module disabled",
  "vpn.activate.desc":
    "Enable the module to create WireGuard VPN access for your clients. Each access exits through your MikCloud server (fixed IP), the sale can be recorded in the cash journal, and revocation cuts the client instantly — without touching other accesses.",
  "vpn.activate.cta": "Enable module",
  "vpn.activate.toast": "VPN module enabled",
  "vpn.deactivate.toast": "VPN module disabled (active accesses keep running)",
  "vpn.settingsHint":
    "Module active — the button below disables it. No active access is cut: revocation remains a per-peer action.",
  "vpn.deactivate.cta": "Disable module",

  // — KPIs + VM health —
  "vpn.kpi.peers": "Active accesses",
  "vpn.kpi.peersSub": "VPN peers in service",
  "vpn.kpi.slots": "wg0 slots",
  "vpn.kpi.slotsSub": "{used} / {pool} addresses reserved (routers included)",
  "vpn.kpi.vm": "VPN server",
  "vpn.kpi.vmUp": "VM reachable",
  "vpn.kpi.vmUpSub": "wg-mini answers on the VM",
  "vpn.kpi.vmDown": "VM unreachable",
  "vpn.kpi.vmDownSub": "check the wg-mini service (systemctl status wg-mini)",
  "vpn.kpi.caisse": "Today's cash",
  "vpn.kpi.caisseSub": "sales recorded (all channels)",

  // — Table —
  "vpn.search": "Search an access…",
  "vpn.access": "Access",
  "vpn.name": "VM name",
  "vpn.address": "Tunnel address",
  "vpn.state": "Status",
  "vpn.created": "Created",
  "vpn.actions": "Actions",
  "vpn.state.active": "Active",
  "vpn.state.pending": "Creating…",
  "vpn.state.error": "Error",
  "vpn.kind.fulltunnel": "End client",
  "vpn.kind.remote": "Remote access",
  "vpn.conf": "Configuration",
  "vpn.deliver": "Deliver",
  "vpn.revoke": "Revoke",
  "vpn.more": "More actions",
  "vpn.noAddress": "—",

  // — Empty states —
  "vpn.empty.title": "No VPN access sold yet",
  "vpn.empty.desc":
    "Create an access for a client: the configuration is generated on your server and delivered as QR, e-mail or Telegram.",
  "vpn.noMatch": "No access matches",
  "vpn.noMatchDesc": "Try another name or label.",

  // — Creation —
  "vpn.add": "Sell an access",
  "vpn.addTitle": "Sell a VPN access",
  "vpn.addDesc":
    "Keys and configuration are generated on your server (the client private key exists nowhere else). Generation takes a few seconds.",
  "vpn.addLabel": "Label (client name, device…)",
  "vpn.addLabelPlaceholder": "Client Ali's phone, shop PC…",
  "vpn.addKind": "Access shape",
  "vpn.addKind.fulltunnel": "End client — all traffic exits through the VM (fixed IP)",
  "vpn.addKind.remote": "Owner remote access — phase B (soon)",
  "vpn.addPrice": "Recorded sale price (0 = record nothing)",
  "vpn.addPricePlaceholder": "e.g. 2000",
  "vpn.addPriceHint": "Above 0: the sale goes to the cash journal (direct channel) and reports.",
  "vpn.addWarning":
    "The client's traffic exits through YOUR server: keep an eye on usage (egress ~10 TB/month on the Always Free tier) and revoke dormant accesses.",
  "vpn.addCta": "Generate & sell",
  "vpn.addToast": "VPN access created",
  "vpn.addPending": "Generating on the VM…",

  // — Conf / delivery (D5-a) —
  "vpn.confTitle": "VPN configuration",
  "vpn.confDesc":
    "The client installs the WireGuard app, then « + » → import. Keep this configuration secret: it grants full access.",
  "vpn.confCopy": "Copy configuration",
  "vpn.confCopied": "Configuration copied",
  "vpn.confQr": "QR (direct scan)",
  "vpn.confEmail": "Send by e-mail",
  "vpn.confTelegram": "Send to Telegram",
  "vpn.emailTitle": "Send the configuration by e-mail",
  "vpn.emailDesc": "The full configuration is sent to the client (your account's e-mail provider).",
  "vpn.emailTo": "Client e-mail address",
  "vpn.emailCta": "Send",
  "vpn.emailToast": "E-mail send in progress",
  "vpn.telegramToast": "Config sent to Telegram",
  "vpn.revokeConfirmTitle": "Revoke this access?",
  "vpn.revokeConfirmDesc":
    "The client immediately loses access (server updated hot). Other accesses are untouched. This action is final.",
  "vpn.revokeToast": "Access revoked",

  // — Reconciliation (check D4) —
  "vpn.reconcile": "Check the VM",
  "vpn.reconcileOk": "VM checked: {mine} access(es) present on the server",
  "vpn.reconcileMissing": "{n} access(es) missing on the VM: {names}",
  "vpn.reconcileDown": "VM unreachable — reconciliation impossible",

  // — Misc —
  "vpn.errorToast": "Operation failed",
  "vpn.error.miniUnconfigured":
    "wg-mini mini-service not installed on the VM — see deploy/oracle/wg-mini/README.md (the operator installs it in 10 minutes).",
  "vpn.disableHint":
    "Disabling does NOT cut active accesses: the server keeps routing existing peers.",
};
