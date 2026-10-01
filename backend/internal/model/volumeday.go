// N°199 — Accumulateur JOURNALIER de volume de données servi (P1 de la
// refonte app/reports, suite de l'audit « les données ne reflètent pas la
// réalité »).
//
// PROBLÈME (constat C2 de l'audit) : le volume de données n'était mesurable
// que sur les sessions ENCORE VIVANTES — les sessions fermées quittent le
// store, donc le KPI « Volume » de l'aperçu de période sous-comptait tout ce
// qui n'était plus connecté à l'instant de la lecture. Aucune comparaison
// honnête n'était possible : le badge Δ% restait masqué (N°198).
//
// MOYEN : chaque delta d'octets observé (deltas entre read_state successifs
// en mode agent — addUserBytes ; progression simulée — store.Tick) verse EN
// LIVE dans un agrégat (compte, routeur, jour). Le jour est découpé AU
// FUSEAU DU COMPTE (même règle que les fenêtres calendaires de l'aperçu) ;
// un histogramme horaire « h0,…,h23 » (somme in+out par heure locale) porte
// la granularité nécessaire à la comparaison « même durée écoulée » : la
// fenêtre précédente coupe à l'heure en cours de la ligne frontière, sans
// garder l'historique fin des sessions.
//
// DÉMARRAGE À ZÉRO (décision D2 de l'opérateur) : pas de rétrofabrication —
// les compteurs d'avant déploiement ne sont pas rejoués. Le Δ% du volume
// n'apparaît qu'une fois une base de comparaison réellement observée.
//
// RÉTENTION : 730 jours (PruneVolumeDays) — l'aperçu « Année » compare
// l'année en cours à la même durée écoulée de l'année PRÉCÉDENTE : le
// 31 décembre, il faut relire janvier N-1, soit deux années pleines.
// Au-delà, les journaux mensuels gelés (N°200) prendront le relais.
//
// ATTRIBUTION PAR OBSERVATION : un delta est versé au jour/heure de SON
// OBSERVATION (pas de réattribution rétroactive au début de session). Une
// session à cheval sur minuit verse ses octets d'après-minuit au nouveau
// jour — convention identique aux compteurs cumulés RouterOS.
package model

import (
	"strconv"
	"strings"
	"time"
)

// VolumeDay (struct) — déclarée dans entities.go, à côté de LineQualityDay.

// VolumeDayID — identifiant DÉTERMINISTE « vd-<compte>:<routeur>:<jour> » :
// une clé naturelle, pas un tirage aléatoire. Une ligne perdue puis recréée
// (redémarrage entre création et synchro PostgreSQL) retrouve le MÊME id →
// upsert au lieu d'un doublon qui doublerait le comptage ; la fusion de
// récupération (mergeSlice, N°164) fusionne ainsi par clé réelle.
func VolumeDayID(accountID, routerID, day string) string {
	return "vd-" + accountID + ":" + routerID + ":" + day
}

// AccumulateVolumeDay — verse un DELTA d'octets observé dans l'agrégat du
// jour, au fuseau du compte. À appeler sous le verrou du store, par deltas
// uniquement (jamais un cumul absolu — cf. addUserBytes : une session déjà
// comptée ne doit pas l'être deux fois). Les deltas négatifs sont clampés à 0
// par direction (agrégat monotone) ; identifiants vides ignorés.
func AccumulateVolumeDay(db *DB, accountID, routerID string, dIn, dOut int64, now time.Time) {
	// Garde défensive : un agrégat est MONOTONE — chaque direction est clampée
	// à 0 (les appelants appliquent déjà max(0, …) contre les resets de
	// compteurs, le modèle n'en dépend pas).
	if dIn < 0 {
		dIn = 0
	}
	if dOut < 0 {
		dOut = 0
	}
	if dIn == 0 && dOut == 0 || accountID == "" || routerID == "" {
		return
	}
	local := now.In(AccountTimezone(db, accountID))
	day := local.Format("2006-01-02")
	hour := local.Hour()

	// La ligne du jour est presque toujours la plus récemment créée : parcours
	// À REBOURS (la table croît jour après jour, le jour courant vit en queue).
	var row *VolumeDay
	for i := len(db.VolumeDays) - 1; i >= 0; i-- {
		if db.VolumeDays[i].AccountID == accountID && db.VolumeDays[i].RouterID == routerID && db.VolumeDays[i].Day == day {
			row = &db.VolumeDays[i]
			break
		}
	}
	if row == nil {
		if db.VolumeDays == nil {
			db.VolumeDays = []VolumeDay{}
		}
		db.VolumeDays = append(db.VolumeDays, VolumeDay{
			ID:        VolumeDayID(accountID, routerID, day),
			AccountID: accountID,
			RouterID:  routerID,
			Day:       day,
			UpdatedAt: now.UTC().Format(time.RFC3339),
		})
		row = &db.VolumeDays[len(db.VolumeDays)-1]
	}
	row.BytesIn += dIn
	row.BytesOut += dOut
	row.Hours = VolumeHoursAdd(row.Hours, hour, dIn+dOut)
	row.UpdatedAt = now.UTC().Format(time.RFC3339)
}

// VolumeHoursAdd — ajoute des octets à l'heure locale h d'un histogramme
// sérialisé « h0,…,h23 ». Heure hors bornes ou delta nul : forme inchangée
// (recanonisée). Une forme vide ou corrompue est traitée comme vierge — la
// colonne ne reçoit que des formes produites ici, le parse reste tolérant.
func VolumeHoursAdd(hist string, hour int, delta int64) string {
	counts := parseVolumeHours(hist)
	if hour >= 0 && hour < 24 && delta > 0 {
		counts[hour] += delta
	}
	return formatVolumeHours(counts)
}

// VolumeHoursSumThrough — somme des seaux 0..uptoHour INCLUS. C'est la
// lecture « même durée écoulée » de la fenêtre précédente : la ligne
// frontière est coupée à l'heure en cours — les deux fenêtres partagent la
// même plage d'heures, seule la dernière est complète côté précédent (biais
// borné à < 1 h de trafic, la fenêtre courante étant observée en direct).
func VolumeHoursSumThrough(hist string, uptoHour int) int64 {
	counts := parseVolumeHours(hist)
	var sum int64
	for h := 0; h <= uptoHour && h < 24; h++ {
		sum += counts[h]
	}
	return sum
}

// parseVolumeHours — « h0,…,h23 » → compteurs TOUJOURS de longueur 24
// (forme vide, champs non numériques ou tronqués : 0 — jamais de panique
// sur une donnée de base).
func parseVolumeHours(s string) []int64 {
	counts := make([]int64, 24)
	if s == "" {
		return counts
	}
	for i, part := range strings.Split(s, ",") {
		if i >= 24 {
			break
		}
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.ParseInt(part, 10, 64); err == nil && n > 0 {
			counts[i] = n
		}
	}
	return counts
}

// formatVolumeHours — compteurs → « h0,…,h23 » (longueur canonique 24 ;
// comptes négatifs sérialisés 0 — l'histogramme reste un histogramme).
func formatVolumeHours(counts []int64) string {
	fixed := make([]int64, 24)
	copy(fixed, counts)
	parts := make([]string, 24)
	for i, c := range fixed {
		if c < 0 {
			c = 0
		}
		parts[i] = strconv.FormatInt(c, 10)
	}
	return strings.Join(parts, ",")
}

// VolumeDayRetentionDays — rétention des agrégats journaliers : 730 jours
// (l'aperçu « Année » relit l'année précédente au 31 décembre — cf. en-tête
// de fichier). Les clés « 2006-01-02 » se comparent lexicographiquement.
const VolumeDayRetentionDays = 730

// PruneVolumeDays — rétention : retire les agrégats de plus de 730 jours.
// La borne est calculée en UTC (approximation d'un jour près pour les
// comptes très à l'est/l'ouest — sans effet à l'échelle de deux années).
// À appeler sous verrou (moteur commun applyExpiry) ; renvoie le nombre de
// lignes purgées.
func PruneVolumeDays(db *DB, now time.Time) int {
	if len(db.VolumeDays) == 0 {
		return 0
	}
	deadline := now.UTC().AddDate(0, 0, -VolumeDayRetentionDays).Format("2006-01-02")
	before := len(db.VolumeDays)
	kept := db.VolumeDays[:0]
	for _, v := range db.VolumeDays {
		if v.Day >= deadline {
			kept = append(kept, v)
		}
	}
	db.VolumeDays = kept
	return before - len(db.VolumeDays)
}
