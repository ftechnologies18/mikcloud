// Sémantique trafic MikCloud — UNE seule source de vérité.
//
// ⚠️ Piège RouterOS (piègement confirmé par la doc officielle
// https://help.mikrotik.com/docs/spaces/ROS/pages/56459266/HotSpot+-+Captive+portal) :
// les compteurs hotspot sont du point de vue du ROUTEUR, pas du client :
//   - `bytes-in`  = bytes UPLOADED    (le routeur REÇOIT ce que le client envoie)
//   - `bytes-out` = bytes DOWNLOADED  (le routeur ENVOIE ce que le client télécharge)
//
// Le backend (agent → gateway → DB → API) transporte ces compteurs BRUTS
// (`bytesIn`/`bytesOut`) — c'est le bon choix : fidélité au routeur, somme
// invariante pour les quotas. L'étiquetage client (upload/download) se fait
// UNIQUEMENT ici, via ces accesseurs — aucun composant ne doit consommer
// `.bytesIn`/`.bytesOut` directement pour un affichage directionnel.

interface BytesCarrier {
  bytesIn: number;
  bytesOut: number;
}

/** Trafic MONTANT (upload) — compteur RouterOS `bytes-in`. */
export function upBytes(s: BytesCarrier): number {
  return s.bytesIn;
}

/** Trafic DESCENDANT (download) — compteur RouterOS `bytes-out`. */
export function downBytes(s: BytesCarrier): number {
  return s.bytesOut;
}
