// Fragment EN du domaine « announcements » — N°152 : annonces de la
// plateforme MikCloud (diffusion super-admin, bandeau + cloche côté clients).
// Ne pas importer directement : passer par l'agrégateur.

export const enAnnouncements: Record<string, string> = {
  // — navigation console plateforme —
  "nav.platformAnnouncements": "Announcements",

  // — vue console plateforme —
  "ann.title": "Client announcements",
  "ann.subtitle": "Broadcast a message to every MikCloud account — maintenance, releases, incidents. The announcement shows as a banner in each recipient's console and stays in their bell.",
  "ann.new": "New announcement",
  "ann.empty": "No announcement broadcast yet.",
  "ann.emptyHint": "Your first announcement will reach every active client account within seconds.",
  "ann.col.title": "Announcement",
  "ann.col.level": "Level",
  "ann.col.audience": "Audience",
  "ann.col.created": "Broadcast",
  "ann.col.expires": "Visible until",
  "ann.col.status": "Status",
  "ann.level.info": "Information",
  "ann.level.warning": "Action recommended",
  "ann.level.critical": "Incident",
  "ann.audience.all": "All accounts",
  "ann.audience.hotspot": "Hotspot accounts",
  "ann.audience.homenet": "HomeNet accounts",
  "ann.status.active": "Visible",
  "ann.status.expired": "Expired",
  "ann.noExpiry": "Until removed",
  "ann.reach": "{count} recipient account(s)",
  "ann.delete": "Remove",
  "ann.deleteTitle": "Remove this announcement?",
  "ann.deleteBody":
    "“{title}” will immediately disappear from the banner and bell of every client account. This action is permanent.",  "ann.deleteCancel": "Cancel",
  "ann.deleteConfirm": "Remove announcement",

  // — formulaire de création —
  "ann.form.title": "Broadcast an announcement",
  "ann.form.description": "The announcement appears as a banner in each recipient's console (dismissible by the manager) and in their notification bell.",
  "ann.form.titleLabel": "Title",
  "ann.form.titlePlaceholder": "e.g. Planned maintenance Saturday night",
  "ann.form.bodyLabel": "Message (optional)",
  "ann.form.bodyPlaceholder": "e.g. The MikCloud cloud will be unavailable from 10pm to 11pm. Routers keep delivering access.",
  "ann.form.bodyHint": "2000 characters max.",
  "ann.form.levelLabel": "Level",
  "ann.form.levelHint": "Drives the banner color: green (information), amber (action recommended), red (incident).",
  "ann.form.audienceLabel": "Audience",
  "ann.form.expiryLabel": "Visibility window",
  "ann.form.expiry.days": "{n} days",
  "ann.form.expiry.none": "Until manual removal",
  "ann.form.expiryHint": "After this window the announcement disappears from consoles on its own.",
  "ann.form.email": "Also email the owners",
  "ann.form.emailHint": "One email per recipient account (account address) — best-effort, never blocking.",
  "ann.form.submit": "Broadcast announcement",
  "ann.form.cancel": "Cancel",
  "ann.created": "Announcement broadcast",
  "ann.deleted": "Announcement removed",

  // — bandeau côté client —
  "ann.banner.dismiss": "Dismiss",
  "ann.banner.platform": "MikCloud announcement",
};
