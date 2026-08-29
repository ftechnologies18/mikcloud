// Formateurs partagés MikCloud

export function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return "0 o";
  const units = ["o", "Ko", "Mo", "Go", "To"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const v = bytes / Math.pow(1024, i);
  return `${v >= 100 ? Math.round(v) : v.toFixed(1).replace(".", ",")} ${units[i]}`;
}

export function formatCurrency(amount: number, currency = "FCFA"): string {
  const rounded = Math.round(amount);
  const formatted = new Intl.NumberFormat("fr-FR").format(rounded);
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

export function formatDate(iso: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "—";
  return new Intl.DateTimeFormat("fr-FR", { day: "2-digit", month: "short", year: "numeric" }).format(d);
}

export function formatDateTime(iso: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "—";
  return new Intl.DateTimeFormat("fr-FR", {
    day: "2-digit",
    month: "2-digit",
    year: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(d);
}

export function timeAgo(iso: string): string {
  if (!iso) return "—";
  const d = new Date(iso).getTime();
  if (isNaN(d)) return "—";
  const diff = Math.max(0, Date.now() - d);
  const min = Math.floor(diff / 60000);
  if (min < 1) return "à l'instant";
  if (min < 60) return `il y a ${min} min`;
  const h = Math.floor(min / 60);
  if (h < 24) return `il y a ${h} h`;
  const days = Math.floor(h / 24);
  if (days < 30) return `il y a ${days} j`;
  return formatDate(iso);
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

/** Libellé francophone lisible d'un rôle brut (admin → Administrateur). */
export function roleLabel(role: string): string {
  if (!role) return "Utilisateur";
  if (role.toLowerCase() === "admin" || role.toLowerCase() === "administrator") return "Administrateur";
  return role.charAt(0).toUpperCase() + role.slice(1);
}
