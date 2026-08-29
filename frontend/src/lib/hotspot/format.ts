// Formateurs partagés MikCloud

import type { Lang } from "./i18n";

function intlLocale(lang: Lang): string {
  return lang === "fr" ? "fr-FR" : "en-GB";
}

export function formatBytes(bytes: number, lang: Lang = "fr"): string {
  const units = lang === "en" ? ["B", "KB", "MB", "GB", "TB"] : ["o", "Ko", "Mo", "Go", "To"];
  if (!bytes || bytes <= 0) return `0 ${units[0]}`;
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const v = bytes / Math.pow(1024, i);
  const num = v >= 100 ? String(Math.round(v)) : v.toFixed(1);
  // Séparateur décimal localisé : « 12,5 Mo » / « 12.5 MB ».
  return `${lang === "en" ? num : num.replace(".", ",")} ${units[i]}`;
}

export function formatCurrency(amount: number, currency = "FCFA", lang: Lang = "fr"): string {
  const rounded = Math.round(amount);
  const formatted = new Intl.NumberFormat(intlLocale(lang)).format(rounded);
  if (currency === "FCFA" || currency === "XOF" || currency === "XAF") return `${formatted} ${currency}`;
  return `${formatted} ${currency}`;
}

export function formatDuration(totalSec: number): string {
  if (!totalSec || totalSec <= 0) return "—";
  const d = Math.floor(totalSec / 86400);
  const h = Math.floor((totalSec % 86400) / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  const s = Math.floor(totalSec % 60);
  if (d > 0) return `${d}j ${h}h`;
  if (h > 0) return `${h}h ${String(m).padStart(2, "0")}m`;
  if (m > 0) return `${m}m ${String(s).padStart(2, "0")}s`;
  return `${s}s`;
}

export function formatDate(iso: string, lang: Lang = "fr"): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "—";
  return new Intl.DateTimeFormat(intlLocale(lang), { day: "2-digit", month: "short", year: "numeric" }).format(d);
}

export function formatDateTime(iso: string, lang: Lang = "fr"): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "—";
  return new Intl.DateTimeFormat(intlLocale(lang), {
    day: "2-digit",
    month: "2-digit",
    year: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(d);
}

export function timeAgo(iso: string, lang: Lang = "fr"): string {
  if (!iso) return "—";
  const d = new Date(iso).getTime();
  if (isNaN(d)) return "—";
  const diff = Math.max(0, Date.now() - d);
  const sec = Math.floor(diff / 1000);
  if (lang === "en") {
    if (sec < 5) return "just now";
    if (sec < 60) return `${sec} s ago`;
    const min = Math.floor(diff / 60000);
    if (min < 60) return `${min} min ago`;
    const h = Math.floor(min / 60);
    if (h < 24) return `${h} h ago`;
    const days = Math.floor(h / 24);
    if (days < 30) return `${days} d ago`;
    return formatDate(iso, lang);
  }
  if (sec < 5) return "à l'instant";
  if (sec < 60) return `il y a ${sec} s`;
  const min = Math.floor(diff / 60000);
  if (min < 60) return `il y a ${min} min`;
  const h = Math.floor(min / 60);
  if (h < 24) return `il y a ${h} h`;
  const days = Math.floor(h / 24);
  if (days < 30) return `il y a ${days} j`;
  return formatDate(iso, lang);
}

/** Débit en bits/s → « 512 Kbps », « 43,5 Mbps », « 1,2 Gbps » (0 → « — »). */
export function formatBitsPerSec(bps: number): string {
  if (!bps || bps <= 0) return "—";
  const fr = (v: number) => v.toFixed(1).replace(".", ",").replace(",0", "");
  if (bps < 1_000_000) {
    const kbps = bps / 1_000;
    return `${kbps < 100 ? fr(kbps) : Math.round(kbps).toLocaleString("fr-FR")} Kbps`;
  }
  if (bps < 1_000_000_000) return `${fr(bps / 1_000_000)} Mbps`;
  return `${fr(bps / 1_000_000_000)} Gbps`;
}

/** Méga-octets (champs serveur en Mo) → « 128 Mo », « 16,0 Go ». */
export function formatMb(mb: number | undefined | null): string {
  if (!mb || mb <= 0) return "—";
  return formatBytes(mb * 1024 * 1024);
}

export function formatRateLimit(rate: string): string {
  // "2M/2M" -> "2M ↓ / 2M ↑"
  if (!rate) return "—";
  const parts = rate.split("/");
  if (parts.length < 2) return rate;
  return `${parts[1]} ↓ / ${parts[0]} ↑`;
}

/** Initiales d'affichage (avatar) à partir du nom complet. */
export function userInitials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "SC";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
}

/** Libellé lisible d'un rôle brut (admin → Administrateur / Administrator). */
// N°7 — libellés des rôles d'équipe (+ super-admin plateforme historique).
export function roleLabel(role: string, lang: Lang = "fr"): string {
  const labels: Record<string, [string, string]> = {
    manager: ["Gérant", "Manager"],
    owner: ["Propriétaire", "Owner"],
    platform_admin: ["Admin plateforme", "Platform admin"],
    admin: ["Admin plateforme", "Platform admin"],
    administrator: ["Admin plateforme", "Platform admin"],
  };
  const hit = labels[role.toLowerCase()];
  if (hit) return lang === "en" ? hit[1] : hit[0];
  if (!role) return lang === "en" ? "User" : "Utilisateur";
  return role.charAt(0).toUpperCase() + role.slice(1);
}
