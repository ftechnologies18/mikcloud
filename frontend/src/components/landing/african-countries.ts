// Liste des 54 pays africains + indicatifs téléphoniques internationaux.
//
// Source : ITU-T E.164 (codes pays), ISO 3166-1 alpha-2 (codes pays).
// Triés par nom français pour le dropdown FR, par nom anglais pour EN.
// Le code "other" (Autre) est ajouté pour la diaspora hors Afrique.

export interface AfricanCountry {
  /** Code ISO 3166-1 alpha-2 (clé primaire, stocké en DB). */
  code: string;
  /** Indicatif téléphonique international sans le + (ex : "225" pour CI). */
  dial: string;
  /** Nom FR. */
  fr: string;
  /** Nom EN. */
  en: string;
}

export const AFRICAN_COUNTRIES: AfricanCountry[] = [
  { code: "DZ", dial: "213", fr: "Algérie", en: "Algeria" },
  { code: "AO", dial: "244", fr: "Angola", en: "Angola" },
  { code: "BJ", dial: "229", fr: "Bénin", en: "Benin" },
  { code: "BW", dial: "267", fr: "Botswana", en: "Botswana" },
  { code: "BF", dial: "226", fr: "Burkina Faso", en: "Burkina Faso" },
  { code: "BI", dial: "257", fr: "Burundi", en: "Burundi" },
  { code: "CM", dial: "237", fr: "Cameroun", en: "Cameroon" },
  { code: "CV", dial: "238", fr: "Cap-Vert", en: "Cape Verde" },
  { code: "CF", dial: "236", fr: "Centrafrique", en: "Central African Republic" },
  { code: "KM", dial: "269", fr: "Comores", en: "Comoros" },
  { code: "CG", dial: "242", fr: "Congo", en: "Congo" },
  { code: "CD", dial: "243", fr: "Congo (RDC)", en: "DR Congo" },
  { code: "CI", dial: "225", fr: "Côte d'Ivoire", en: "Côte d'Ivoire" },
  { code: "DJ", dial: "253", fr: "Djibouti", en: "Djibouti" },
  { code: "EG", dial: "20", fr: "Égypte", en: "Egypt" },
  { code: "GQ", dial: "240", fr: "Guinée équatoriale", en: "Equatorial Guinea" },
  { code: "ER", dial: "291", fr: "Érythrée", en: "Eritrea" },
  { code: "SZ", dial: "268", fr: "Eswatini", en: "Eswatini" },
  { code: "ET", dial: "251", fr: "Éthiopie", en: "Ethiopia" },
  { code: "GA", dial: "241", fr: "Gabon", en: "Gabon" },
  { code: "GM", dial: "220", fr: "Gambie", en: "Gambia" },
  { code: "GH", dial: "233", fr: "Ghana", en: "Ghana" },
  { code: "GN", dial: "224", fr: "Guinée", en: "Guinea" },
  { code: "GW", dial: "245", fr: "Guinée-Bissau", en: "Guinea-Bissau" },
  { code: "KE", dial: "254", fr: "Kenya", en: "Kenya" },
  { code: "LS", dial: "266", fr: "Lesotho", en: "Lesotho" },
  { code: "LR", dial: "231", fr: "Libéria", en: "Liberia" },
  { code: "LY", dial: "218", fr: "Libye", en: "Libya" },
  { code: "MG", dial: "261", fr: "Madagascar", en: "Madagascar" },
  { code: "MW", dial: "265", fr: "Malawi", en: "Malawi" },
  { code: "ML", dial: "223", fr: "Mali", en: "Mali" },
  { code: "MR", dial: "222", fr: "Mauritanie", en: "Mauritania" },
  { code: "MU", dial: "230", fr: "Maurice", en: "Mauritius" },
  { code: "MA", dial: "212", fr: "Maroc", en: "Morocco" },
  { code: "MZ", dial: "258", fr: "Mozambique", en: "Mozambique" },
  { code: "NA", dial: "264", fr: "Namibie", en: "Namibia" },
  { code: "NE", dial: "227", fr: "Niger", en: "Niger" },
  { code: "NG", dial: "234", fr: "Nigeria", en: "Nigeria" },
  { code: "RW", dial: "250", fr: "Rwanda", en: "Rwanda" },
  { code: "ST", dial: "239", fr: "São Tomé-et-Príncipe", en: "São Tomé and Príncipe" },
  { code: "SN", dial: "221", fr: "Sénégal", en: "Senegal" },
  { code: "SC", dial: "248", fr: "Seychelles", en: "Seychelles" },
  { code: "SL", dial: "232", fr: "Sierra Leone", en: "Sierra Leone" },
  { code: "SO", dial: "252", fr: "Somalie", en: "Somalia" },
  { code: "ZA", dial: "27", fr: "Afrique du Sud", en: "South Africa" },
  { code: "SS", dial: "211", fr: "Soudan du Sud", en: "South Sudan" },
  { code: "SD", dial: "249", fr: "Soudan", en: "Sudan" },
  { code: "TZ", dial: "255", fr: "Tanzanie", en: "Tanzania" },
  { code: "TD", dial: "235", fr: "Tchad", en: "Chad" },
  { code: "TG", dial: "228", fr: "Togo", en: "Togo" },
  { code: "TN", dial: "216", fr: "Tunisie", en: "Tunisia" },
  { code: "UG", dial: "256", fr: "Ouganda", en: "Uganda" },
  { code: "ZM", dial: "260", fr: "Zambie", en: "Zambia" },
  { code: "ZW", dial: "263", fr: "Zimbabwe", en: "Zimbabwe" },
];

/** Pays "Autre" pour la diaspora hors Afrique. */
export const OTHER_COUNTRY: AfricanCountry = {
  code: "other",
  dial: "",
  fr: "Autre",
  en: "Other",
};

/** Tous les pays triés par nom (FR ou EN selon la langue). */
export function countriesByLang(lang: "fr" | "en"): AfricanCountry[] {
  const list = [...AFRICAN_COUNTRIES];
  list.sort((a, b) => a[lang].localeCompare(b[lang], lang === "fr" ? "fr-FR" : "en-GB"));
  return [...list, OTHER_COUNTRY];
}

/** Retrouver un pays par son code ISO. */
export function countryByCode(code: string): AfricanCountry | undefined {
  if (code === "other") return OTHER_COUNTRY;
  return AFRICAN_COUNTRIES.find((c) => c.code === code);
}

/** Formate un numéro WhatsApp pour l'affichage : "+225 07 00 00 00 00". */
export function formatPhone(phone: string, dial: string): string {
  if (!phone) return "";
  if (!dial) return phone;
  return `+${dial} ${phone}`;
}
