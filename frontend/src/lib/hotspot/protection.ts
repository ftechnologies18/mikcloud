// N°83 — Protection : helpers purs de l'état de sécurité d'un routeur.
//
// La vue Protection (sidebar principale), le bandeau du tableau de bord et
// le résumé de l'onglet Système de la fiche routeur partagent le MÊME
// calcul de verdict — ces fonctions en sont la source unique. Elles ne
// lisent QUE les champs exposés par GET /api/routers (safeWifiLevel,
// shieldLevel, familyGuardSpec, antiVpnLevel — N°80/81/82/88) : zéro nouvel
// endpoint, zéro octet supplémentaire pour le parc.
//
// La logique de fenêtre FamilyGuard (parse du spec canonique + « en cours
// maintenant ? ») est déplacée ici depuis router-tools.tsx : miroir exact
// de model.FamilyGuardConfig.ActiveAt côté Go — UTC (== heure d'Abidjan
// GMT), passage de minuit, jour = jour de DÉBUT de la fenêtre.

import type { FamilyGuardWindow } from "./api";
import type { RouterDevice } from "./types";

/** Niveau SafeWiFi effectif d'un routeur ("" ou inconnu → off, N°80). */
export function safeWifiLevelOf(r: RouterDevice): "off" | "threats" | "family" {
  return r.safeWifiLevel === "threats" || r.safeWifiLevel === "family" ? r.safeWifiLevel : "off";
}

/** Bouclier Shield actif ? ("" ou inconnu → off, N°81). */
export function shieldOn(r: RouterDevice): boolean {
  return r.shieldLevel === "on";
}

/** Bloque-VPN actif ? ("" ou inconnu → off, N°88). */
export function antiVpnOn(r: RouterDevice): boolean {
  return r.antiVpnLevel === "on";
}

/** Parse le spec canonique FamilyGuard "<enabled>|<HH:MM>|<HH:MM>|<1111111>" —
 * toute forme invalide retombe sur les défauts (22:00 → 06:00, tous les
 * jours, désactivé). */
export function parseFamilyGuardSpec(spec: string | undefined): FamilyGuardWindow {
  const w: FamilyGuardWindow = { enabled: false, start: "22:00", end: "06:00", days: "1111111" };
  if (!spec) return w;
  const parts = spec.split("|");
  if (parts.length !== 4) return w;
  const timeOk = (s: string) => /^([01]\d|2[0-3]):[0-5]\d$/.test(s);
  if (timeOk(parts[1]) && timeOk(parts[2]) && parts[1] !== parts[2] && /^[01]{7}$/.test(parts[3]) && parts[3].includes("1")) {
    w.enabled = parts[0] === "1";
    w.start = parts[1];
    w.end = parts[2];
    w.days = parts[3];
  }
  return w;
}

/** Vrai si le couvre-feu est EN COURS à `now` — calculé en UTC (heure
 * d'Abidjan GMT, comme le cloud : le couvre-feu vit à l'heure du site,
 * pas à celle du navigateur). Miroir exact de model.FamilyGuardConfig.ActiveAt :
 * passage de minuit, jour = jour de DÉBUT de la fenêtre. */
export function familyGuardActiveNow(w: FamilyGuardWindow, now: Date): boolean {
  if (!w.enabled) return false;
  const [sh, sm] = w.start.split(":").map(Number);
  const [eh, em] = w.end.split(":").map(Number);
  const s = sh * 60 + sm;
  const e = eh * 60 + em;
  if (s === e) return false;
  const day = (now.getUTCDay() + 6) % 7; // lundi = 0 … dimanche = 6
  const m = now.getUTCHours() * 60 + now.getUTCMinutes();
  if (s < e) return m >= s && m < e && w.days[day] === "1";
  const prev = (day + 6) % 7; // portion après minuit = fenêtre partie la veille
  return (m >= s && w.days[day] === "1") || (m < e && w.days[prev] === "1");
}

/** Nombre de protections actives sur ce routeur (0 à 4). */
export function protectionScore(r: RouterDevice): number {
  let n = 0;
  if (safeWifiLevelOf(r) !== "off") n++;
  if (shieldOn(r)) n++;
  if (parseFamilyGuardSpec(r.familyGuardSpec).enabled) n++;
  if (antiVpnOn(r)) n++;
  return n;
}

/** Verdict de protection d'un routeur — le vocabulaire vendeur N°83. */
export type ProtectionVerdict = "protected" | "partial" | "unprotected";

/** Verdict d'un routeur : 4/4 = bien protégé, 1-3 = à renforcer, 0 = non
 * protégé. Calculé depuis les seuls champs existants de GET /api/routers.
 * N°88 : le bloque-VPN complète le quatuor — sans lui, la footnote SafeWiFi
 * (N°85 : « seul un VPN contourne ») reste une porte ouverte : le verdict
 * l'exige pour « Bien protégé ». */
export function protectionVerdict(r: RouterDevice): ProtectionVerdict {
  const n = protectionScore(r);
  if (n >= 4) return "protected";
  if (n >= 1) return "partial";
  return "unprotected";
}
