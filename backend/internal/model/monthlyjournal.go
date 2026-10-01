// N°200 — journaux MENSUELS GELÉS (P2 de la refonte app/reports, décision D3
// de l'opérateur : « gel auto au 1er + bouton Clôturer le mois maintenant »).
//
// PROBLÈME (constat C4 de l'audit « les données ne reflètent pas la réalité »)
// : tous les chiffres des rapports sont recalculés à la volée depuis des
// sources VIVANTES — hotspot_users (purge selon la politique d'expiration),
// user_logs (rétention 30/60/90 j + plafond 5 000), transactions. Les courbes
// « 12 derniers mois » pourrissent donc avec le temps : le mois de février
// affiché en décembre n'est plus celui de février.
//
// MOYEN : au bascule de mois, un JOURNAL est gelé — une ligne par (compte,
// mois calendaire au fuseau du compte) portant les cinq KPI de l'aperçu
// (ventes écoulées, trésorerie réelle, panier payé, connexions, volume de
// données) + la répartition par canal. La ligne est IMMUABLE une fois écrite
// (clé naturelle « mj-<compte>:<YYYY-MM> ») : les archives ne bougent plus,
// quelle que soit la rétention des journaux. La table volume_days (N°199,
// rétention 730 j) alimente la partie volume ; au-delà de deux ans, SEULS les
// journaux restent — c'est leur raison d'être.
//
// DEUX CHEMINS D'ÉCRITURE, jamais de réécriture :
//   - AUTO   : le balayage horaire (RunRetentionSweep — rattrapage au boot
//     Render inclus) constate au fil de l'eau que le mois PRÉCÉDENT n'a pas
//     de journal et le crée pour le mois PLEIN. Un compte dormant, jamais
//     consulté, est couvert aussi (même argument que la rétention N°64) :
//     les sources décayant, attendre une visite console perdrait des
//     données. Seul le mois immédiatement précédent est couvert — un trou
//     plus ancien reste un trou visible (honnête) plutôt qu'un journal
//     reconstruit de sources déjà purgées ;
//   - MANUEL : le bouton « Clôturer le mois maintenant » gèle le mois EN
//     COURS à l'instant du clic (partial=true, fenêtre couverte explicite).
//     Acte comptable délibéré : l'activité restante du mois ne sera PAS
//     journalisée — le dialogue de confirmation le dit noir sur blanc. Un
//     mois déjà gelé ne l'est JAMAIS deux fois (409) : gelé = gelé.
//
// DÉMARRAGE : au premier déploiement, le balayage crée le journal du mois
// précédent à partir des sources survivantes — les chiffres réels de
// septembre (ventes, trésorerie, connexions) sont archivés tels quels ; le
// volume data vaut 0 pour tout mois antérieur au déploiement de
// l'accumulateur (décision D2 : pas de rétrofabrication).
//
// RÉTENTION : AUCUNE — une douzaine de lignes par compte et par an, c'est la
// mémoire comptable définitive du compte (contrairement aux agrégats
// quotidiens volume_days, purgés à 730 j).
package model

// MonthlyJournal (struct) — déclarée dans entities.go, à côté de VolumeDay.

// MonthlyJournalID — identifiant DÉTERMINISTE « mj-<compte>:<YYYY-MM> » :
// clé naturelle du mois calendaire, pas un tirage aléatoire — un journal
// n'existe qu'une fois (l'upsert PostgreSQL et la fusion de récupération
// mergeSlice, N°164, raisonnent par clé réelle).
func MonthlyJournalID(accountID, month string) string {
	return "mj-" + accountID + ":" + month
}
