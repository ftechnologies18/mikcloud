"use client";

// Vue Rapports v4 — DÉ-DUPLICATION DES ONGLETS (retour exploitant N°206).
//
// CONSTAT v3 : la refonte N°204 avait unifié la grammaire visuelle mais
// laissé une bande de résumé en tête de CHAQUE onglet — revenus, ventes,
// panier moyen (et marge) répétaient les cartes de l'Aperçu jusqu'à
// quatre fois à l'écran, et deux onglets montraient chacun un graphe de
// CA quotidien. Vue de l'exploitant : « la carte de l'onglet comptabilité
// affiche les mêmes revenus, ventes, panier moyen et marge » — redondance
// de fonctionnalité entre les onglets.
//
// PRINCIPE v4 — UNE MÉTRIQUE, UN FOYER :
//   · Aperçu → les SEULES grandes cartes KPI (ventes, revenus, panier,
//     connexions, volume) + les graphes de tendance intrapériode.
//   · Comptabilité → le CA par bucket (jour/semaine/mois) : le total de
//     la fenêtre vit dans l'en-tête du graphe (contexte de lecture, pas
//     une carte KPI), répartitions par site et par canal — sans marge,
//     qui a son onglet dédié.
//   · Activité → l'usage du réseau UNIQUEMENT : sessions, connexions,
//     trafic, heure de pointe. Le graphe CA quotidien a déménagé en
//     Comptabilité : une seule maison par question.
//   · Marge → la rentabilité (valeur écoulée vs coût), avec détection
//     du cas dégénéré : prix de vente = prix gros partout → marge nulle
//     PAR CONSTRUCTION (constaté en production : tous les profils sans
//     prix public distinct) → bandeau explicite + raccourci vers la
//     configuration des profils plutôt qu'un mur de zéros incompréhensible.
//   · Archives → journaux mensuels gelés (D3, N°200) — inchangés.
//
// DOCTRINES INCHANGÉES : revenus = CONSOMMÉ (tickets écoulés, pas
// générations de stock) ; périodes calendaires au fuseau du compte + Δ%
// vs période précédente au même moment (aperçu) ; vues glissantes en
// onglets (D4) ; volume démarre au déploiement de l'accumulateur, trous
// honnêtes (D2).

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ComposedChart,
  Legend,
  Line,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  AlertTriangle,
  Archive,
  CalendarRange,
  Clock3,
  Coins,
  Database,
  Download,
  LockKeyhole,
  Percent,
  Router as RouterIcon,
  ShoppingCart,
  SlidersHorizontal,
  Store,
  TrendingUp,
  Users,
  Wallet,
  Wifi,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

import { api, apiDownload } from "@/lib/hotspot/api";
import { STALE_TIME } from "@/lib/hotspot/query";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import { useChartPalette, type ChartPalette } from "@/lib/hotspot/chart-theme";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { Lang } from "@/lib/hotspot/i18n";
import type {
  AccountingData,
  AccountingPeriod,
  HourlyStats,
  JournalsResponse,
  MonthlyJournal,
  OverviewPeriod,
  ReportsData,
  RouterDevice,
  StatsOverview,
} from "@/lib/hotspot/types";
import { formatBytes, formatCurrency, formatDateTime } from "@/lib/hotspot/format";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingCards } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import { ChartTooltip } from "@/components/hotspot/parts/sd-chart-tooltip";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { cn } from "@/lib/utils";

/** Fenêtre glissante de l'onglet Activité. */
const PERIODS = [
  { value: "7", labelKey: "reports.days7" },
  { value: "14", labelKey: "reports.days14" },
  { value: "30", labelKey: "reports.days30" },
];

/** Taille de bucket de la comptabilité (à ne pas confondre avec OverviewPeriod). */
const ACCOUNTING_PERIODS: {
  value: AccountingPeriod;
  labelKey: string;
  windowKey: string;
  unitKey: string;
}[] = [
  { value: "day", labelKey: "reports.period.day", windowKey: "reports.window.day", unitKey: "reports.unit.day" },
  { value: "week", labelKey: "reports.period.week", windowKey: "reports.window.week", unitKey: "reports.unit.week" },
  { value: "month", labelKey: "reports.period.month", windowKey: "reports.window.month", unitKey: "reports.unit.month" },
];

/** Période calendaire EN COURS de l'aperçu, au fuseau du compte (N°198). */
const OVERVIEW_PERIODS: { value: OverviewPeriod; labelKey: string }[] = [
  { value: "day", labelKey: "reports.overview.today" },
  { value: "week", labelKey: "reports.overview.thisWeek" },
  { value: "month", labelKey: "reports.overview.thisMonth" },
  { value: "year", labelKey: "reports.overview.thisYear" },
];

// N°203 — l'unité des buckets de la série intrapériode suit la période :
// heures pour le jour, jours pour la semaine et le mois, mois pour l'année.
const OVERVIEW_META: Record<OverviewPeriod, { windowKey: string; vsKey: string; unitKey: string }> = {
  day: { windowKey: "reports.overview.windowDay", vsKey: "reports.overview.vsDay", unitKey: "reports.overview.unitHour" },
  week: { windowKey: "reports.overview.windowWeek", vsKey: "reports.overview.vsWeek", unitKey: "reports.overview.unitDay" },
  month: { windowKey: "reports.overview.windowMonth", vsKey: "reports.overview.vsMonth", unitKey: "reports.overview.unitDay" },
  year: { windowKey: "reports.overview.windowYear", vsKey: "reports.overview.vsYear", unitKey: "reports.overview.unitMonth" },
};

// Palette thématée (nuit/jour) — statuts de vouchers.
const voucherStatusRows = (p: ChartPalette) => [
  { key: "active", labelKey: "common.statusActive", color: p.series[0] },
  { key: "used", labelKey: "common.statusUsed", color: p.series[2] },
  { key: "expired", labelKey: "common.statusExpired", color: p.axis },
  { key: "disabled", labelKey: "common.statusDisabled", color: p.series[3] },
] as const;

/** Couleur des barres de marge : vert si positive, rouge sinon. */
const MARGIN_POS = "#10b981";
const MARGIN_NEG = "#ef4444";

// ─────────────────────────────────────────────────────────────────────────────
// Helpers partagés
// ─────────────────────────────────────────────────────────────────────────────

/** Pourcentage localisé (12,4 % en FR, 12.4% en EN). */
function fmtPct(value: number, lang: Lang): string {
  return `${value.toFixed(1).replace(".", lang === "fr" ? "," : ".")}${lang === "fr" ? " " : ""}%`;
}

/** Δ% vs période précédente → pastille de tendance (rien si pas de base). */
function deltaTrend(
  current: number,
  previous: number | undefined,
  lang: Lang,
): { value: string; up: boolean } | undefined {
  if (previous === undefined || previous <= 0) return undefined;
  const pct = ((current - previous) / previous) * 100;
  if (Math.abs(pct) < 0.05) return undefined;
  return { value: fmtPct(Math.abs(pct), lang), up: pct > 0 };
}

/** Couleur de la marge : verte si positive, rouge si négative, neutre sinon. */
function cnMargin(margin: number): string {
  if (margin > 0) return "font-semibold text-emerald-600 dark:text-emerald-400";
  if (margin < 0) return "font-semibold text-destructive";
  return "font-semibold text-muted-foreground";
}

/** Badge de taux de marge : vert positif, rouge négatif, neutre à zéro. */
function RateBadge({ rate, lang }: { rate: number; lang: Lang }) {
  return (
    <Badge
      variant="outline"
      className={
        rate > 0
          ? "border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
          : rate < 0
            ? "border-destructive/25 bg-destructive/10 text-destructive"
            : "border-border bg-muted text-muted-foreground"
      }
    >
      {fmtPct(rate, lang)}
    </Badge>
  );
}

/** Libellé localisé d'une clé « YYYY-MM » (« octobre 2026 »). */
function monthLabel(month: string, lang: Lang): string {
  const [y, m] = month.split("-");
  const d = new Date(Number(y), Number(m) - 1, 1);
  return new Intl.DateTimeFormat(localeOf(lang), { month: "long", year: "numeric" }).format(d);
}

// ─────────────────────────────────────────────────────────────────────────────
// Primitives UI — UNE grammaire pour les cinq onglets
// ─────────────────────────────────────────────────────────────────────────────

/** Pastille Δ% compacte (bande de résumé). Même code couleur que StatCard. */
function TrendPill({ trend }: { trend?: { value: string; up: boolean } }) {
  if (!trend) return null;
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center rounded-full px-1.5 py-0.5 text-[10px] font-semibold tabular-nums",
        trend.up ? "bg-primary/15 text-primary" : "bg-destructive/15 text-destructive",
      )}
    >
      {trend.up ? "▲" : "▼"} {trend.value}
    </span>
  );
}

/** Une statistique de la bande de résumé (SummaryStrip). */
function SummaryItem({
  icon: Icon,
  label,
  value,
  valueClassName,
  trend,
  sub,
  live,
  className,
}: {
  icon: LucideIcon;
  label: string;
  value: string;
  valueClassName?: string;
  trend?: { value: string; up: boolean };
  sub?: string;
  live?: boolean;
  className?: string;
}) {
  return (
    <div className={cn("flex min-w-0 flex-col justify-center px-4 py-4 sm:px-5", className)}>
      <p className="flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
        <Icon className="size-3.5 shrink-0" aria-hidden />
        <span className="truncate">{label}</span>
      </p>
      <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1">
        <span className={cn("text-lg font-semibold tracking-tight tabular-nums sm:text-xl", valueClassName)}>
          {value}
        </span>
        <TrendPill trend={trend} />
        {live && (
          <span className="live-dot size-2 shrink-0 rounded-full bg-primary" aria-hidden />
        )}
      </div>
      {sub && <p className="mt-0.5 truncate text-xs text-muted-foreground">{sub}</p>}
    </div>
  );
}

/** Bande de résumé compacte d'un onglet — ne porte QUE des métriques
 *  propres à l'onglet (v4) : jamais une répétition des cartes KPI de
 *  l'Aperçu, cause du constat « mêmes revenus/ventes/panier partout ». */
function SummaryStrip({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <Card className={cn("gap-0 py-0", className)}>
      <CardContent className="p-0">
        <div className="grid grid-cols-2 divide-border/60 lg:grid-cols-4 lg:divide-x">{children}</div>
      </CardContent>
    </Card>
  );
}

/** Barre d'outils d'onglet — MÊME structure dans les cinq onglets : période
 *  à gauche, filtre site + action à droite. */
function TabToolbar({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      {children}
    </div>
  );
}

/** Carte de graphe unifiée — titre + description + contenu, hauteur
 *  homogène. « action » porte un contexte chiffré LIÉ au graphe (total
 *  de la fenêtre v4) : un nombre ancré dans son graphique, jamais une
 *  carte KPI détachée qui concurrence l'Aperçu. */
function ChartCard({
  icon: Icon,
  title,
  description,
  action,
  children,
  contentClassName,
  className,
}: {
  icon?: LucideIcon;
  title: string;
  description?: React.ReactNode;
  action?: React.ReactNode;
  children: React.ReactNode;
  contentClassName?: string;
  className?: string;
}) {
  return (
    <Card className={cn("gap-4 py-4 sm:py-5", className)}>
      <CardHeader className="px-4 sm:px-6">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <CardTitle className="flex items-center gap-2 text-base">
              {Icon && <Icon className="size-4 shrink-0 text-muted-foreground" aria-hidden />}
              {title}
            </CardTitle>
            {description && <CardDescription>{description}</CardDescription>}
          </div>
          {action}
        </div>
      </CardHeader>
      <CardContent className={cn("px-4 sm:px-6", contentClassName)}>{children}</CardContent>
    </Card>
  );
}

/** Ligne « libellé + valeur + part » avec barre de progression — factorise
 *  les listes par site / canal / revendeurs / profils / statuts. */
function ShareRow({
  icon,
  label,
  right,
  share,
  color,
  className,
}: {
  icon?: React.ReactNode;
  label: string;
  right?: React.ReactNode;
  /** Part 0-100 (déjà normalisée par l'appelant). */
  share: number;
  color?: string;
  className?: string;
}) {
  return (
    <div className={className}>
      <div className="flex items-baseline justify-between gap-3 text-sm">
        <span className="flex min-w-0 items-center gap-2">
          {icon}
          <span className="truncate font-medium">{label}</span>
        </span>
        {right && <span className="shrink-0 text-xs text-muted-foreground">{right}</span>}
      </div>
      <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-muted">
        <div
          className={cn("h-full rounded-full transition-all", !color && "bg-primary")}
          style={{ width: `${Math.max(2, share)}%`, ...(color ? { background: color } : {}) }}
        />
      </div>
    </div>
  );
}

/** Liste des sites du compte — partagée par tous les onglets. */
function useRoutersList() {
  return useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
    // N°130 — état du parc (check-ins agents ~45 s).
    staleTime: STALE_TIME.operational,
  }).data;
}

/** Filtre site — s'applique à l'aperçu, à la comptabilité, à l'activité et
 *  à la marge (le backend borne ventes, connexions, sessions et analyse). */
function SiteFilter({
  value,
  onChange,
  routers,
}: {
  value: string;
  onChange: (value: string) => void;
  routers?: RouterDevice[];
}) {
  const { t } = useI18n();
  return (
    <div className="flex items-center gap-2">
      <RouterIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden />
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger className="h-9 w-full sm:w-52" aria-label={t("reports.filterRouter")}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{t("common.allSites")}</SelectItem>
          {routers?.map((router) => (
            <SelectItem key={router.id} value={router.id}>
              {router.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Tooltips
// ─────────────────────────────────────────────────────────────────────────────

// Tooltip comptabilité : revenus + ventes du point survolé.
function AccountingTooltip({
  active,
  payload,
  label,
  currency,
  lang,
  revenueLabel,
  salesLabel,
}: {
  active?: boolean;
  payload?: { payload?: { revenue: number; sales: number } }[];
  label?: string;
  currency: string;
  lang: Lang;
  revenueLabel: string;
  salesLabel: string;
}) {
  if (!active || !payload || payload.length === 0 || !payload[0]?.payload) return null;
  const point = payload[0].payload;
  return (
    <div className="rounded-lg border bg-popover px-3 py-2 text-xs shadow-md">
      <p className="mb-1.5 font-medium text-foreground">{label}</p>
      <p className="text-muted-foreground">
        {revenueLabel}{" "}
        <span className="font-medium text-foreground">{formatCurrency(point.revenue, currency, lang)}</span>
      </p>
      <p className="text-muted-foreground">
        {salesLabel} <span className="font-medium text-foreground">{point.sales}</span>
      </p>
    </div>
  );
}

// Tooltip heures de pointe : CA + connexions de la tranche survolée.
function PeakHoursTooltip({
  active,
  payload,
  label,
  currency,
  lang,
  revenueLabel,
  loginsLabel,
}: {
  active?: boolean;
  payload?: { dataKey?: string | number; value?: number }[];
  label?: string;
  currency: string;
  lang: Lang;
  revenueLabel: string;
  loginsLabel: string;
}) {
  if (!active || !payload || payload.length === 0) return null;
  const revenue = payload.find((p) => p.dataKey === "revenue")?.value ?? 0;
  const logins = payload.find((p) => p.dataKey === "logins")?.value ?? 0;
  return (
    <div className="rounded-lg border bg-popover px-3 py-2 text-xs shadow-md">
      <p className="mb-1.5 font-medium text-foreground">{label}</p>
      <p className="text-muted-foreground">
        {revenueLabel}{" "}
        <span className="font-medium text-foreground">{formatCurrency(revenue, currency, lang)}</span>
      </p>
      <p className="text-muted-foreground">
        {loginsLabel} <span className="font-medium text-foreground">{logins}</span>
      </p>
    </div>
  );
}

// Tooltip tendance (N°203) : CA + ventes + connexions du bucket survolé.
function OverviewActivityTooltip({
  active,
  payload,
  label,
  currency,
  lang,
  revenueLabel,
  salesLabel,
  loginsLabel,
}: {
  active?: boolean;
  payload?: { payload?: { revenue: number; sales: number; logins: number } }[];
  label?: string;
  currency: string;
  lang: Lang;
  revenueLabel: string;
  salesLabel: string;
  loginsLabel: string;
}) {
  if (!active || !payload || payload.length === 0 || !payload[0]?.payload) return null;
  const point = payload[0].payload;
  return (
    <div className="rounded-lg border bg-popover px-3 py-2 text-xs shadow-md">
      <p className="mb-1.5 font-medium text-foreground">{label}</p>
      <p className="text-muted-foreground">
        {revenueLabel}{" "}
        <span className="font-medium text-foreground">{formatCurrency(point.revenue, currency, lang)}</span>
      </p>
      <p className="text-muted-foreground">
        {salesLabel} <span className="font-medium text-foreground">{point.sales}</span>
      </p>
      <p className="text-muted-foreground">
        {loginsLabel} <span className="font-medium text-foreground">{point.logins}</span>
      </p>
    </div>
  );
}

// Tooltip volume (N°203) : trafic servi du bucket survolé (formatBytes
// binaire 1024 — la même base que le KPI et les journaux mensuels).
function OverviewVolumeTooltip({
  active,
  payload,
  label,
  lang,
  volumeLabel,
}: {
  active?: boolean;
  payload?: { payload?: { dataBytes: number } }[];
  label?: string;
  lang: Lang;
  volumeLabel: string;
}) {
  if (!active || !payload || payload.length === 0 || !payload[0]?.payload) return null;
  const point = payload[0].payload;
  return (
    <div className="rounded-lg border bg-popover px-3 py-2 text-xs shadow-md">
      <p className="mb-1.5 font-medium text-foreground">{label}</p>
      <p className="text-muted-foreground">
        {volumeLabel} <span className="font-medium text-foreground">{formatBytes(point.dataBytes, lang)}</span>
      </p>
    </div>
  );
}

/** Axe Y devise compacte localisée. */
function useCompactAxis(lang: Lang) {
  return useMemo(
    () =>
      new Intl.NumberFormat(localeOf(lang), {
        notation: "compact",
        maximumFractionDigits: 1,
      }),
    [lang],
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Onglet Aperçu — LA période calendaire en cours (N°198/N°203) : les seules
// grandes cartes KPI du module + les graphes de tendance intrapériode.
// V4 : foyer UNIQUE des métriques vedettes (ventes, revenus, panier,
// connexions, volume) — aucun autre onglet ne les répète en cartes.
// ─────────────────────────────────────────────────────────────────────────────

function OverviewTab() {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const charts = useChartPalette();
  const AXIS_TICK = { fontSize: 11, fill: charts.axis };
  const compact = useCompactAxis(lang);
  const [period, setPeriod] = useState<OverviewPeriod>("day");
  const [routerFilter, setRouterFilter] = useState("all");
  const routers = useRoutersList();

  const { data, isLoading } = useQuery({
    queryKey: ["/api/stats/overview", period, routerFilter],
    queryFn: () =>
      api<StatsOverview>("/api/stats/overview", { params: { period, routerId: routerFilter } }),
    placeholderData: (previous) => previous,
  });

  const meta = OVERVIEW_META[period];
  const kpis = data?.kpis;
  // N°203 — la série intrapériode est VISIBLE : aucun bucket actif → pas de
  // graphes vides (les KPI disent déjà zéro, honnêtement).
  const series = data?.series ?? [];
  const hasActivity = series.some(
    (pt) => pt.revenue > 0 || pt.sales > 0 || pt.logins > 0 || (pt.dataBytes ?? 0) > 0,
  );

  return (
    <div className="space-y-4 sm:space-y-6">
      {/* Barre d'outils : période calendaire + filtre site */}
      <TabToolbar>
        <Tabs value={period} onValueChange={(value) => setPeriod(value as OverviewPeriod)}>
          <TabsList>
            {OVERVIEW_PERIODS.map((p) => (
              <TabsTrigger key={p.value} value={p.value}>
                {t(p.labelKey)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <SiteFilter value={routerFilter} onChange={setRouterFilter} routers={routers} />
      </TabToolbar>

      {/* Contexte de fenêtre : la période ET sa base de comparaison. */}
      <p className="flex items-center gap-1.5 text-xs text-muted-foreground sm:text-sm">
        <CalendarRange className="size-3.5 shrink-0" aria-hidden />
        {data ? `${t(meta.windowKey)} · ${t(meta.vsKey)}` : t("reports.overview.desc")}
      </p>

      {isLoading && !data ? (
        <LoadingCards cards={5} />
      ) : !kpis ? null : (
        /* N°198 — 3+2 sur laptop (les 5 colonnes ne laissaient pas la place
           du « 5 000 XOF » à côté du badge Δ% et de l'icône : empilement
           caractère par caractère, constaté au DOM), 5 colonnes réservées
           aux écrans très larges. */
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-5">
          <StatCard
            title={t("reports.sales")}
            value={String(kpis.sales)}
            sub={t("reports.vouchersSold")}
            icon={ShoppingCart}
            trend={deltaTrend(kpis.sales, kpis.salesPrev, lang)}
          />
          <StatCard
            title={t("reports.revenue")}
            value={formatCurrency(kpis.revenue, currency, lang)}
            icon={Wallet}
            trend={deltaTrend(kpis.revenue, kpis.revenuePrev, lang)}
          />
          <StatCard
            title={t("reports.avgTicket")}
            value={formatCurrency(kpis.avgTicket, currency, lang)}
            sub={t("reports.perVoucher")}
            icon={TrendingUp}
            trend={deltaTrend(kpis.avgTicket, kpis.avgTicketPrev, lang)}
          />
          <StatCard
            title={t("reports.overview.loginsTitle")}
            value={new Intl.NumberFormat(localeOf(lang)).format(kpis.logins)}
            sub={t("reports.overview.loginsSub")}
            icon={Users}
            trend={deltaTrend(kpis.logins, kpis.loginsPrev, lang)}
          />
          {/* Volume — agrégats journaliers persistés (N°199), sessions
              fermées comprises. Le badge Δ% n'apparaît qu'une fois une base
              réellement observée (D2 : jamais de comparaison au vide). */}
          <StatCard
            title={t("reports.overview.dataVolume")}
            value={formatBytes(kpis.dataBytes, lang)}
            sub={t("reports.overview.dataSub")}
            icon={Database}
            trend={deltaTrend(kpis.dataBytes, kpis.dataBytesPrev, lang)}
            live
          />
        </div>
      )}

      {/* N°203 — GRAPHES DE TENDANCE de la série intrapériode + VOLUME dans
          les buckets (heure locale pour « Aujourd'hui », jour calendaire
          pour semaine/mois, mois pour l'année). Le dernier bucket est
          PARTIEL : accumulation live, jamais un axe qui prétend être complet. */}
      {series.length > 0 && hasActivity && (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <ChartCard
            icon={TrendingUp}
            title={t("reports.overview.trendTitle")}
            description={
              <>
                {tf("reports.overview.trendDesc", { unit: t(meta.unitKey) })} ·{" "}
                {t("reports.overview.lastPartial")}
              </>
            }
          >
            <ResponsiveContainer width="100%" height={264}>
              <ComposedChart data={series} margin={{ top: 8, right: 4, left: 0, bottom: 0 }}>
                <CartesianGrid stroke={charts.grid} strokeOpacity={0.6} vertical={false} />
                <XAxis dataKey="label" tick={AXIS_TICK} axisLine={false} tickLine={false} minTickGap={12} />
                <YAxis
                  yAxisId="revenue"
                  tick={AXIS_TICK}
                  axisLine={false}
                  tickLine={false}
                  width={48}
                  tickFormatter={(value: number) => compact.format(value)}
                />
                <YAxis
                  yAxisId="logins"
                  orientation="right"
                  tick={AXIS_TICK}
                  axisLine={false}
                  tickLine={false}
                  width={36}
                  allowDecimals={false}
                />
                <Tooltip
                  cursor={{ fill: charts.cursorFill, fillOpacity: 0.06 }}
                  content={
                    <OverviewActivityTooltip
                      currency={currency}
                      lang={lang}
                      revenueLabel={t("reports.tooltipRevenue")}
                      salesLabel={t("reports.tooltipSales")}
                      loginsLabel={t("reports.overview.loginsTitle")}
                    />
                  }
                />
                <Legend
                  iconType="circle"
                  iconSize={8}
                  formatter={(value) => <span className="text-xs text-muted-foreground">{value}</span>}
                />
                <Bar
                  yAxisId="revenue"
                  dataKey="revenue"
                  name={t("reports.revenue")}
                  fill={charts.series[0]}
                  radius={[3, 3, 0, 0]}
                  maxBarSize={14}
                />
                <Line
                  yAxisId="logins"
                  dataKey="logins"
                  name={t("reports.overview.loginsTitle")}
                  type="monotone"
                  stroke={charts.series[1]}
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4 }}
                />
              </ComposedChart>
            </ResponsiveContainer>
          </ChartCard>

          <ChartCard
            icon={Database}
            title={t("reports.overview.dataVolume")}
            description={
              <>
                {tf("reports.overview.volumeTrendDesc", { unit: t(meta.unitKey) })} ·{" "}
                {t("reports.overview.lastPartial")}
              </>
            }
          >
            <ResponsiveContainer width="100%" height={264}>
              <BarChart data={series} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                <CartesianGrid stroke={charts.grid} strokeOpacity={0.6} vertical={false} />
                <XAxis dataKey="label" tick={AXIS_TICK} axisLine={false} tickLine={false} minTickGap={12} />
                <YAxis
                  tick={AXIS_TICK}
                  axisLine={false}
                  tickLine={false}
                  width={56}
                  tickFormatter={(value: number) => formatBytes(value, lang)}
                />
                <Tooltip
                  cursor={{ fill: charts.cursorFill, fillOpacity: 0.06 }}
                  content={<OverviewVolumeTooltip lang={lang} volumeLabel={t("reports.overview.dataVolume")} />}
                />
                <Bar
                  dataKey="dataBytes"
                  name={t("reports.overview.dataVolume")}
                  fill={charts.series[2]}
                  radius={[3, 3, 0, 0]}
                  maxBarSize={14}
                />
              </BarChart>
            </ResponsiveContainer>
          </ChartCard>
        </div>
      )}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Onglet Comptabilité — « D'où vient l'argent ? » Ventes par jour/semaine/
// mois (buckets glissants, D4). V4 : PLUS de bande de résumé — le total de
// la fenêtre vit dans l'en-tête du graphe (contexte de lecture), et la
// marge a disparu ici : elle vit dans son onglet dédié. Les métriques
// vedettes (ventes, revenus, panier) restent la propriété de l'Aperçu.
// ─────────────────────────────────────────────────────────────────────────────

function AccountingTab() {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const charts = useChartPalette();
  const AXIS_TICK = { fontSize: 11, fill: charts.axis };
  const compact = useCompactAxis(lang);
  const [period, setPeriod] = useState<AccountingPeriod>("day");
  const [routerFilter, setRouterFilter] = useState("all");
  const routers = useRoutersList();

  const { data, isLoading } = useQuery({
    queryKey: ["/api/accounting", period, routerFilter],
    queryFn: () =>
      api<AccountingData>("/api/accounting", { params: { period, routerId: routerFilter } }),
    placeholderData: (previous) => previous,
  });

  const periodMeta = ACCOUNTING_PERIODS.find((p) => p.value === period) ?? ACCOUNTING_PERIODS[0];
  const byRouter = data?.byRouter ?? [];
  const maxShare = Math.max(...byRouter.map((r) => r.share), 1);
  const channel = data?.channel;
  const channelTotal = channel ? channel.directRevenue + channel.resellerRevenue : 0;
  // Pic de CA de la fenêtre (meilleur bucket de la série affichée).
  const bestBucket = useMemo(
    () => (data?.series ?? []).reduce<{ label: string; revenue: number }>(
      (best, pt) => (pt.revenue > best.revenue ? { label: pt.label, revenue: pt.revenue } : best),
      { label: "", revenue: 0 },
    ),
    [data],
  );

  return (
    <div className="space-y-4 sm:space-y-6">
      {/* Barre d'outils : taille de bucket + filtre site + export */}
      <TabToolbar>
        <Tabs value={period} onValueChange={(value) => setPeriod(value as AccountingPeriod)}>
          <TabsList>
            {ACCOUNTING_PERIODS.map((p) => (
              <TabsTrigger key={p.value} value={p.value}>
                {t(p.labelKey)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <div className="flex flex-wrap items-center gap-2">
          <SiteFilter value={routerFilter} onChange={setRouterFilter} routers={routers} />
          <Button
            variant="outline"
            className="h-9"
            onClick={() =>
              apiDownload(`/api/accounting/export`, `mikcloud-comptabilite-${period}.csv`, {
                period,
                routerId: routerFilter,
              })
                .then(() => toast.success(t("common.exportDownloaded")))
                .catch((err: Error) => toast.error(err.message))
            }
          >
            <Download className="size-4" />
            <span className="hidden sm:inline">{t("common.exportCsv")}</span>
          </Button>
        </div>
      </TabToolbar>

      {isLoading && !data ? (
        <div className="space-y-4 sm:space-y-6">
          <Skeleton className="h-80 rounded-xl" />
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Skeleton className="h-64 rounded-xl" />
            <Skeleton className="h-64 rounded-xl" />
          </div>
        </div>
      ) : !data ? null : (
        <>
          {/* CA par bucket — pleine largeur : 30 barres quotidiennes
              lisibles, jamais tassées à moitié. Le TOTAL de la fenêtre est
              ancré dans l'en-tête (v4) : le nombre que la somme des barres
              doit atteindre, pas une carte KPI de plus. */}
          <ChartCard
            icon={Wallet}
            title={tf("reports.revenueBy", { unit: t(periodMeta.unitKey) })}
            description={
              <>
                {routerFilter === "all" ? `${t("reports.allSites")} — ` : `${routers?.find((r) => r.id === routerFilter)?.name ?? ""} — `}
                {t(periodMeta.windowKey)} · {tf("reports.accounting.tickets", { n: data.totals.sales })}
                {bestBucket.revenue > 0 && (
                  <>
                    {" · "}
                    {tf("reports.bestPeriod", {
                      label: bestBucket.label,
                      amount: formatCurrency(bestBucket.revenue, currency, lang),
                    })}
                  </>
                )}
              </>
            }
            action={
              <div className="shrink-0 text-left sm:text-right">
                <p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
                  {t("reports.accounting.windowRevenue")}
                </p>
                <p className="text-lg font-semibold tracking-tight tabular-nums sm:text-xl">
                  {formatCurrency(data.totals.revenue, currency, lang)}
                </p>
              </div>
            }
          >
            <ResponsiveContainer width="100%" height={280}>
              <BarChart data={data.series} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                <CartesianGrid stroke={charts.grid} strokeOpacity={0.6} vertical={false} />
                <XAxis dataKey="label" tick={AXIS_TICK} axisLine={false} tickLine={false} minTickGap={12} />
                <YAxis
                  tick={AXIS_TICK}
                  axisLine={false}
                  tickLine={false}
                  width={48}
                  tickFormatter={(value: number) => compact.format(value)}
                />
                <Tooltip
                  cursor={{ fill: charts.cursorFill, fillOpacity: 0.06 }}
                  content={
                    <AccountingTooltip
                      currency={currency}
                      lang={lang}
                      revenueLabel={t("reports.tooltipRevenue")}
                      salesLabel={t("reports.tooltipSales")}
                    />
                  }
                />
                <Bar
                  dataKey="revenue"
                  name={t("reports.revenue")}
                  fill={charts.series[0]}
                  radius={[4, 4, 0, 0]}
                  maxBarSize={40}
                />
              </BarChart>
            </ResponsiveContainer>
          </ChartCard>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            {/* Répartition par site — CA, ventes et part (v4 : sans taux de
                marge, l'analyse de rentabilité vit dans l'onglet Marge). */}
            <ChartCard
              icon={RouterIcon}
              title={t("reports.salesBySite")}
              description={t("reports.salesBySiteDesc")}
            >
              {byRouter.length === 0 ? (
                <EmptyState
                  icon={CalendarRange}
                  title={t("reports.noSales")}
                  description={t("reports.noSalesDesc")}
                />
              ) : (
                <div className="max-h-72 space-y-5 overflow-y-auto pr-1">
                  {byRouter.map((router) => (
                    <ShareRow
                      key={router.routerId}
                      icon={<RouterIcon className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />}
                      label={router.routerName}
                      share={(router.share / maxShare) * 100}
                      right={
                        <>
                          <span className="font-medium text-foreground">
                            {formatCurrency(router.revenue, currency, lang)}
                          </span>{" "}
                          · {tf("reports.soldCount", { n: router.sales })} ·{" "}
                          {new Intl.NumberFormat(localeOf(lang), { maximumFractionDigits: 1 }).format(router.share)} %
                        </>
                      }
                    />
                  ))}
                </div>
              )}
            </ChartCard>

            {/* Canal de distribution — ventes directes vs réseau revendeurs */}
            {channel && (
              <ChartCard icon={Store} title={t("reports.channel.title")} description={t(routerFilter === "all" ? "reports.channel.desc" : "reports.channel.descSite")}>
                {channelTotal === 0 ? (
                  <p className="py-4 text-center text-sm text-muted-foreground">{t("reports.channel.empty")}</p>
                ) : (
                  <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
                    {(
                      [
                        {
                          key: "direct",
                          icon: Store,
                          nameKey: "reports.channel.direct",
                          revenue: channel.directRevenue,
                          sales: channel.directSales,
                          color: charts.series[0],
                        },
                        {
                          key: "reseller",
                          icon: Users,
                          nameKey: "reports.channel.resellers",
                          revenue: channel.resellerRevenue,
                          sales: channel.resellerSales,
                          color: charts.series[1],
                        },
                      ] as const
                    ).map((row) => {
                      const share = (row.revenue / channelTotal) * 100;
                      return (
                        <ShareRow
                          key={row.key}
                          icon={<row.icon className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />}
                          label={t(row.nameKey)}
                          share={share}
                          color={row.color}
                          right={
                            <>
                              <span className="font-medium text-foreground">
                                {formatCurrency(row.revenue, currency, lang)}
                              </span>{" "}
                              · {tf("reports.soldCount", { n: row.sales })} ·{" "}
                              {new Intl.NumberFormat(localeOf(lang), { maximumFractionDigits: 1 }).format(share)} %
                            </>
                          }
                        />
                      );
                    })}
                  </div>
                )}
              </ChartCard>
            )}
          </div>
        </>
      )}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Onglet Activité — « Comment le réseau est-il utilisé ? » Fenêtre glissante
// 7/14/30 jours (D4). V4 : bande de résumé 100 % usage (sessions, connexions,
// trafic, heure de pointe) et le graphe « revenus quotidiens » a été
// supprimé — le CA par jour vit en Comptabilité, une seule maison par
// question (fin du doublon constaté par l'exploitant).
// ─────────────────────────────────────────────────────────────────────────────

function ActivityTab() {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const charts = useChartPalette();
  const AXIS_TICK = { fontSize: 11, fill: charts.axis };
  const compact = useCompactAxis(lang);
  const [days, setDays] = useState(14);
  const [routerFilter, setRouterFilter] = useState("all");
  const routers = useRoutersList();

  const { data, isLoading } = useQuery({
    queryKey: ["/api/reports", days, routerFilter],
    queryFn: () => api<ReportsData>("/api/reports", { params: { days, routerId: routerFilter } }),
    placeholderData: (previous) => previous,
  });

  // N°10 — affluence réelle par tranche horaire (même fenêtre que l'onglet).
  // N°198 : CA sur la doctrine « consommé » + filtre site.
  const { data: hourly } = useQuery({
    queryKey: ["/api/stats/hourly", days, routerFilter],
    queryFn: () => api<HourlyStats>("/api/stats/hourly", { params: { days, routerId: routerFilter } }),
    placeholderData: (previous) => previous,
  });

  const fmtInt = useMemo(() => new Intl.NumberFormat(localeOf(lang)), [lang]);
  const salesByProfile = useMemo(
    () => [...(data?.salesByProfile ?? [])].sort((a, b) => b.revenue - a.revenue),
    [data],
  );
  const maxProfileRevenue = Math.max(...salesByProfile.map((s) => s.revenue), 1);
  const maxStatus = useMemo(
    () =>
      data
        ? Math.max(...voucherStatusRows(charts).map((row) => data.voucherStatus[row.key]), 1)
        : 1,
    [data, charts],
  );
  const topResellers = data?.topResellers ?? [];
  const maxResellerRevenue = Math.max(...topResellers.map((r) => r.revenue), 1);
  const sessions = data?.sessions;
  const sessionTraffic = sessions ? sessions.bytesIn + sessions.bytesOut : 0;
  // Connexions de la fenêtre + jour record — dérivés de la série quotidienne.
  const totalLogins = useMemo(
    () => (data?.loginsByDay ?? []).reduce((sum, d) => sum + d.count, 0),
    [data],
  );
  const bestDay = useMemo(
    () => (data?.loginsByDay ?? []).reduce<{ day: string; count: number }>(
      (best, d) => (d.count > best.count ? d : best),
      { day: "", count: 0 },
    ),
    [data],
  );
  const hourlyData = useMemo(
    () =>
      hourly
        ? hourly.salesByHour.map((revenue, i) => ({
            hour: `${String(i).padStart(2, "0")}h`,
            revenue,
            logins: hourly.loginsByHour[i] ?? 0,
          }))
        : [],
    [hourly],
  );
  const hasHourlyActivity = hourly ? hourly.totalLogins > 0 || hourly.totalSales > 0 : false;

  return (
    <div className="space-y-4 sm:space-y-6">
      {/* Barre d'outils : fenêtre glissante + filtre site */}
      <TabToolbar>
        <Tabs value={String(days)} onValueChange={(value) => setDays(Number(value))}>
          <TabsList>
            {PERIODS.map((period) => (
              <TabsTrigger key={period.value} value={period.value}>
                {t(period.labelKey)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <SiteFilter value={routerFilter} onChange={setRouterFilter} routers={routers} />
      </TabToolbar>

      {isLoading && !data ? (
        <div className="space-y-4 sm:space-y-6">
          <Skeleton className="h-24 rounded-xl" />
          <Skeleton className="h-72 rounded-xl" />
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Skeleton className="h-64 rounded-xl" />
            <Skeleton className="h-64 rounded-xl" />
          </div>
        </div>
      ) : !data ? null : (
        <>
          {/* Bande de résumé — métriques d'USAGE propres à l'onglet (v4) :
              ni revenus, ni ventes, ni panier (foyer unique : l'Aperçu). */}
          <SummaryStrip>
            <SummaryItem
              icon={Wifi}
              label={t("reports.sessions")}
              value={sessions ? fmtInt.format(sessions.count) : "—"}
              sub={tf("reports.lastDays", { n: days })}
              live
            />
            <SummaryItem
              icon={Users}
              label={t("reports.overview.loginsTitle")}
              value={fmtInt.format(totalLogins)}
              sub={t("reports.activity.loginsSub")}
            />
            <SummaryItem
              icon={Database}
              label={t("reports.overview.dataVolume")}
              value={sessions ? formatBytes(sessionTraffic, lang) : "—"}
              sub={t("reports.activity.trafficSub")}
            />
            <SummaryItem
              icon={Clock3}
              label={t("reports.peakHours.title")}
              value={hourly && hasHourlyActivity ? `${String(hourly.peakHour).padStart(2, "0")}h` : "—"}
              sub={t("reports.activity.peakSub")}
            />
          </SummaryStrip>

          {/* Connexions par jour — pleine largeur : LE rythme d'affluence de
              la fenêtre (le graphe CA quotidien vit en Comptabilité). */}
          <ChartCard
            icon={Users}
            title={t("reports.loginsPerDay")}
            description={
              <>
                {t("reports.loginsPerDayDesc")}
                {bestDay.count > 0 && (
                  <>
                    {" · "}
                    {tf("reports.activity.bestDay", { day: bestDay.day, n: bestDay.count })}
                  </>
                )}
              </>
            }
          >
            <ResponsiveContainer width="100%" height={264}>
              <BarChart data={data.loginsByDay} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                <CartesianGrid stroke={charts.grid} strokeOpacity={0.6} vertical={false} />
                <XAxis dataKey="day" tick={AXIS_TICK} axisLine={false} tickLine={false} minTickGap={12} />
                <YAxis
                  tick={AXIS_TICK}
                  axisLine={false}
                  tickLine={false}
                  width={40}
                  allowDecimals={false}
                />
                <Tooltip
                  cursor={{ fill: charts.cursorFill, fillOpacity: 0.06 }}
                  content={
                    <ChartTooltip
                      formatter={(value) => new Intl.NumberFormat(localeOf(lang)).format(value)}
                    />
                  }
                />
                <Bar
                  dataKey="count"
                  name={t("reports.loginsPerDay")}
                  fill={charts.series[1]}
                  radius={[4, 4, 0, 0]}
                  maxBarSize={40}
                />
              </BarChart>
            </ResponsiveContainer>
          </ChartCard>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            {/* Heures de pointe — CA + connexions par heure (fuseau du compte) */}
            <ChartCard
              icon={Clock3}
              title={t("reports.peakHours.title")}
              description={
                <>
                  {t("reports.peakHours.desc")}
                  {hourly && hasHourlyActivity && (
                    <>
                      {" · "}
                      {tf("reports.peakHours.peak", { h: String(hourly.peakHour).padStart(2, "0") })}
                    </>
                  )}
                </>
              }
            >
              {!hourly ? (
                <Skeleton className="h-60 rounded-lg" />
              ) : !hasHourlyActivity ? (
                <p className="py-12 text-center text-sm text-muted-foreground">
                  {t("reports.peakHours.empty")}
                </p>
              ) : (
                <ResponsiveContainer width="100%" height={264}>
                  <ComposedChart data={hourlyData} margin={{ top: 8, right: 4, left: 0, bottom: 0 }}>
                    <CartesianGrid stroke={charts.grid} strokeOpacity={0.6} vertical={false} />
                    <XAxis dataKey="hour" tick={AXIS_TICK} axisLine={false} tickLine={false} interval={2} />
                    <YAxis
                      yAxisId="revenue"
                      tick={AXIS_TICK}
                      axisLine={false}
                      tickLine={false}
                      width={48}
                      tickFormatter={(value: number) => compact.format(value)}
                    />
                    <YAxis
                      yAxisId="logins"
                      orientation="right"
                      tick={AXIS_TICK}
                      axisLine={false}
                      tickLine={false}
                      width={36}
                      allowDecimals={false}
                    />
                    <Tooltip
                      cursor={{ fill: charts.cursorFill, fillOpacity: 0.06 }}
                      content={
                        <PeakHoursTooltip
                          currency={currency}
                          lang={lang}
                          revenueLabel={t("reports.peakHours.revenue")}
                          loginsLabel={t("reports.peakHours.logins")}
                        />
                      }
                    />
                    <Legend
                      iconType="circle"
                      iconSize={8}
                      formatter={(value) => <span className="text-xs text-muted-foreground">{value}</span>}
                    />
                    <Bar
                      yAxisId="revenue"
                      dataKey="revenue"
                      name={t("reports.peakHours.revenue")}
                      fill={charts.series[0]}
                      radius={[3, 3, 0, 0]}
                      maxBarSize={14}
                    />
                    <Line
                      yAxisId="logins"
                      dataKey="logins"
                      name={t("reports.peakHours.logins")}
                      type="monotone"
                      stroke={charts.series[1]}
                      strokeWidth={2}
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                  </ComposedChart>
                </ResponsiveContainer>
              )}
            </ChartCard>

            {/* Top 5 revendeurs par CA — moteur de distribution mikCloud */}
            <ChartCard
              icon={Store}
              title={t("reports.topResellers.title")}
              description={t(routerFilter === "all" ? "reports.topResellers.desc" : "reports.topResellers.descSite")}
            >
              {topResellers.length === 0 ? (
                <EmptyState
                  icon={Users}
                  title={t("reports.topResellers.empty")}
                  description={t("reports.salesByProfileDesc")}
                />
              ) : (
                <div className="space-y-4">
                  {topResellers.map((reseller, index) => (
                    <ShareRow
                      key={reseller.name}
                      icon={
                        <Badge
                          variant="outline"
                          className={
                            index === 0
                              ? "shrink-0 border-primary/30 bg-primary/10 text-primary"
                              : "shrink-0 border-border bg-muted text-muted-foreground"
                          }
                        >
                          {tf("reports.rank", { n: index + 1 })}
                        </Badge>
                      }
                      label={reseller.name}
                      share={(reseller.revenue / maxResellerRevenue) * 100}
                      right={
                        <>
                          <span className="font-medium text-foreground">
                            {formatCurrency(reseller.revenue, currency, lang)}
                          </span>{" "}
                          · {tf("reports.soldCount", { n: reseller.sales })}
                        </>
                      }
                    />
                  ))}
                </div>
              )}
            </ChartCard>
          </div>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <ChartCard icon={TrendingUp} title={t("reports.salesByProfile")} description={t("reports.salesByProfileDesc")}>
              {salesByProfile.length === 0 ? (
                <p className="py-6 text-center text-sm text-muted-foreground">{t("reports.noSalesPeriod")}</p>
              ) : (
                <div className="space-y-4">
                  {salesByProfile.map((sale) => (
                    <ShareRow
                      key={sale.name}
                      label={sale.name}
                      share={(sale.revenue / maxProfileRevenue) * 100}
                      right={
                        <>
                          <span className="font-medium text-foreground">
                            {tf("reports.soldCount", { n: sale.count })}
                          </span>
                          {" · "}
                          {formatCurrency(sale.revenue, currency, lang)}
                        </>
                      }
                    />
                  ))}
                </div>
              )}
            </ChartCard>

            <ChartCard icon={ShoppingCart} title={t("reports.voucherStatus")} description={t("reports.voucherStatusDesc")}>
              <div className="space-y-4">
                {voucherStatusRows(charts).map((row) => {
                  const count = data.voucherStatus[row.key];
                  return (
                    <ShareRow
                      key={row.key}
                      icon={<span className="size-2 shrink-0 rounded-full" style={{ background: row.color }} aria-hidden />}
                      label={t(row.labelKey)}
                      share={(count / maxStatus) * 100}
                      color={row.color}
                      right={<span className="font-medium tabular-nums text-foreground">{count}</span>}
                    />
                  );
                })}
              </div>
            </ChartCard>
          </div>
        </>
      )}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Onglet Marge (F13) — « Quelle rentabilité ? » Prix de vente vs coût sur
// 30 jours glissants. V4 : la métrique « revenue » est rebaptisée VALEUR
// ÉCOULÉE (prix public des tickets) pour cesser de doublonner le CA
// trésorerie de la Comptabilité — et le cas dégénéré (marge nulle PAR
// CONSTRUCTION : aucun profil sans prix de vente distinct) est diagnostiqué
// et expliqué, avec raccourci vers la configuration des profils.
// ─────────────────────────────────────────────────────────────────────────────

function MarginTab() {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const charts = useChartPalette();
  const setView = useHotspotStore((s) => s.setView);
  const AXIS_TICK = { fontSize: 11, fill: charts.axis };
  const compact = useCompactAxis(lang);
  const [routerFilter, setRouterFilter] = useState("all");
  const routers = useRoutersList();

  const { data, isLoading } = useQuery({
    queryKey: ["/api/reports", 30, routerFilter],
    queryFn: () => api<ReportsData>("/api/reports", { params: { days: 30, routerId: routerFilter } }),
    placeholderData: (previous) => previous,
  });

  const margin = data?.margin;
  const pctFmt = (value: number): string => fmtPct(value, lang);

  if (isLoading && !data) {
    return (
      <div className="space-y-4 sm:space-y-6">
        <Skeleton className="h-28 rounded-xl" />
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <Skeleton className="h-72 rounded-xl" />
          <Skeleton className="h-72 rounded-xl" />
        </div>
      </div>
    );
  }

  // Backend non à jour (bloc « margin » absent) → EmptyState discret.
  if (!margin) {
    return (
      <Card className="gap-0 py-0">
        <EmptyState
          icon={Percent}
          title={t("reports.margin.unavailable")}
          description={t("reports.margin.unavailableDesc")}
        />
      </Card>
    );
  }

  // Marge NULLE PAR CONSTRUCTION : chaque ticket écoulé vaut exactement son
  // coût (aucun profil avec prix de vente public distinct) — constaté en
  // production sur des comptes entiers. Un mur de zéros sans explication
  // passait pour un bug (retour exploitant N°206) ; on nomme la cause et
  // on propose l'action correctrice.
  const degenerate = margin.margin === 0 && margin.cost > 0 && margin.cost === margin.revenue;

  const byProfile = [...(margin.byProfile ?? [])].sort((a, b) => b.margin - a.margin);
  const bySite = margin.byRouter ?? [];
  const byDay = margin.byDay ?? [];
  const prev = margin.prev;
  const maxSiteMargin = Math.max(...bySite.map((r) => Math.abs(r.margin)), 1);
  const bestProfile = byProfile[0];

  return (
    <div className="space-y-4 sm:space-y-6">
      {/* Barre d'outils : l'analyse de marge est toujours 30 j glissants —
          seul le filtre site s'applique. */}
      <TabToolbar>
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground sm:text-sm">
          <CalendarRange className="size-3.5 shrink-0" aria-hidden />
          {t("reports.margin.window")}
        </p>
        <SiteFilter value={routerFilter} onChange={setRouterFilter} routers={routers} />
      </TabToolbar>

      {/* Cas dégénéré — marge nulle par construction (prix public = prix gros
          partout) : diagnostic + action, pas un mur de zéros. */}
      {degenerate && (
        <div
          role="status"
          className="flex flex-col gap-3 rounded-xl border border-amber-500/30 bg-amber-500/10 p-4 sm:flex-row sm:items-center sm:justify-between"
        >
          <div className="flex min-w-0 items-start gap-3">
            <AlertTriangle
              className="mt-0.5 size-5 shrink-0 text-amber-600 dark:text-amber-400"
              aria-hidden
            />
            <div className="min-w-0">
              <p className="text-sm font-semibold text-amber-700 dark:text-amber-300">
                {t("reports.margin.degenerateTitle")}
              </p>
              <p className="mt-0.5 text-sm text-muted-foreground">
                {t("reports.margin.degenerateDesc")}
              </p>
            </div>
          </div>
          <Button
            variant="outline"
            className="h-9 shrink-0 border-amber-500/40 hover:bg-amber-500/10"
            onClick={() => setView("profiles")}
          >
            <SlidersHorizontal className="size-4" />
            {t("reports.margin.configure")}
          </Button>
        </div>
      )}

      {/* Bande de résumé : valeur écoulée / coût / marge / taux — le groupe
          de métriques PROPRES à la rentabilité (v4 : plus aucun doublon du
          CA trésorerie — « valeur écoulée » = prix public des tickets). */}
      <SummaryStrip>
        <SummaryItem
          icon={Wallet}
          label={t("reports.margin.revenue")}
          value={formatCurrency(margin.revenue, currency, lang)}
          sub={t("reports.margin.soldValueSub")}
          trend={deltaTrend(margin.revenue, prev?.revenue, lang)}
        />
        <SummaryItem
          icon={Coins}
          label={t("reports.margin.cost")}
          value={formatCurrency(margin.cost, currency, lang)}
          sub={t("reports.margin.window")}
        />
        <SummaryItem
          icon={TrendingUp}
          label={t("reports.margin.margin")}
          value={formatCurrency(margin.margin, currency, lang)}
          valueClassName={cnMargin(margin.margin)}
          sub={t("reports.margin.window")}
          trend={deltaTrend(margin.margin, prev?.margin, lang)}
        />
        <SummaryItem
          icon={Percent}
          label={t("reports.margin.rate")}
          value={pctFmt(margin.marginPct)}
          sub={t("reports.margin.rateSub")}
        />
      </SummaryStrip>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {/* Évolution quotidienne de la marge (vert positif / rouge négatif) */}
        <ChartCard icon={Coins} title={t("reports.margin.trendTitle")} description={t("reports.margin.trendDesc")}>
          {byDay.length === 0 ? (
            <p className="py-12 text-center text-sm text-muted-foreground">{t("reports.margin.noProfiles")}</p>
          ) : (
            <ResponsiveContainer width="100%" height={240}>
              <BarChart data={byDay} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                <CartesianGrid stroke={charts.grid} strokeOpacity={0.6} vertical={false} />
                <XAxis dataKey="day" tick={AXIS_TICK} axisLine={false} tickLine={false} minTickGap={12} />
                <YAxis
                  tick={AXIS_TICK}
                  axisLine={false}
                  tickLine={false}
                  width={48}
                  tickFormatter={(value: number) => compact.format(value)}
                />
                <Tooltip
                  cursor={{ fill: charts.cursorFill, fillOpacity: 0.06 }}
                  content={<ChartTooltip formatter={(value) => formatCurrency(value, currency, lang)} />}
                />
                <ReferenceLine y={0} stroke={charts.axis} />
                <Bar dataKey="margin" name={t("reports.margin.margin")} radius={[3, 3, 0, 0]} maxBarSize={14}>
                  {byDay.map((pt, i) => (
                    <Cell key={i} fill={pt.margin >= 0 ? MARGIN_POS : MARGIN_NEG} />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          )}
        </ChartCard>

        {/* Marge par site — quel routeur rapporte le plus ? */}
        <ChartCard icon={RouterIcon} title={t("reports.margin.bySiteTitle")} description={t("reports.margin.bySiteDesc")}>
          {bySite.length === 0 ? (
            <p className="py-12 text-center text-sm text-muted-foreground">{t("reports.margin.noProfiles")}</p>
          ) : (
            <div className="max-h-64 space-y-4 overflow-y-auto pr-1">
              {bySite.map((site) => {
                const rate = site.revenue > 0 ? (site.margin / site.revenue) * 100 : 0;
                return (
                  <ShareRow
                    key={site.routerName}
                    label={site.routerName}
                    share={(Math.abs(site.margin) / maxSiteMargin) * 100}
                    color={site.margin >= 0 ? MARGIN_POS : MARGIN_NEG}
                    right={
                      <>
                        <span className={cnMargin(site.margin)}>
                          {formatCurrency(site.margin, currency, lang)}
                        </span>
                        {site.revenue > 0 && (
                          <RateBadge rate={rate} lang={lang} />
                        )}
                      </>
                    }
                  />
                );
              })}
            </div>
          )}
        </ChartCard>
      </div>

      {/* Table par profil : ventes, valeur, coût, marge + badge taux + part */}
      <Card className="gap-4 py-4 sm:py-5">
        <CardHeader className="px-4 sm:px-6">
          <CardTitle className="text-base">{t("reports.margin.byProfileTitle")}</CardTitle>
          <CardDescription>
            {t("reports.margin.byProfileDesc")}
            {bestProfile && bestProfile.margin > 0 && (
              <>
                {" · "}
                {tf("reports.margin.bestProfile", { name: bestProfile.name })}
              </>
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className="px-0 sm:px-0">
          {byProfile.length === 0 ? (
            <p className="px-4 py-6 text-center text-sm text-muted-foreground sm:px-6">
              {t("reports.margin.noProfiles")}
            </p>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/50 hover:bg-muted/50">
                    <TableHead className="pl-4 text-muted-foreground sm:pl-6">
                      {t("reports.margin.profile")}
                    </TableHead>
                    <TableHead className="text-right text-muted-foreground">
                      {t("reports.margin.sold")}
                    </TableHead>
                    <TableHead className="text-right text-muted-foreground">
                      {t("reports.margin.revenue")}
                    </TableHead>
                    <TableHead className="hidden text-right text-muted-foreground sm:table-cell">
                      {t("reports.margin.cost")}
                    </TableHead>
                    <TableHead className="text-right text-muted-foreground">
                      {t("reports.margin.margin")}
                    </TableHead>
                    <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">
                      {t("reports.margin.share")}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {byProfile.map((row) => {
                    const rate = row.revenue > 0 ? (row.margin / row.revenue) * 100 : 0;
                    const share =
                      margin.margin > 0 ? Math.max(0, (row.margin / margin.margin) * 100) : 0;
                    return (
                      <TableRow key={row.name}>
                        <TableCell className="max-w-44 truncate pl-4 font-medium sm:pl-6">{row.name}</TableCell>
                        <TableCell className="text-right tabular-nums">{row.sold}</TableCell>
                        <TableCell className="text-right tabular-nums">
                          {formatCurrency(row.revenue, currency, lang)}
                        </TableCell>
                        <TableCell className="hidden text-right tabular-nums text-muted-foreground sm:table-cell">
                          {formatCurrency(row.cost, currency, lang)}
                        </TableCell>
                        <TableCell className="text-right">
                          <span className="inline-flex items-center gap-2">
                            <span className={cnMargin(row.margin)}>
                              {formatCurrency(row.margin, currency, lang)}
                            </span>
                            {row.revenue > 0 && <RateBadge rate={rate} lang={lang} />}
                          </span>
                        </TableCell>
                        <TableCell className="pr-4 text-right tabular-nums text-muted-foreground sm:pr-6">
                          {margin.margin > 0 ? pctFmt(share) : "—"}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Onglet Archives (N°200) — journaux MENSUELS GELÉS (décision D3). Chaque
// mois est figé au bascule (gel automatique — balayage horaire serveur) ou
// par « Clôturer le mois maintenant » (acte comptable délibéré : mois
// partiel, fenêtre couverte explicite). Immuable une fois écrit — les
// archives ne bougent plus, quelle que soit la rétention des journaux
// vivants.
// ─────────────────────────────────────────────────────────────────────────────

/** Une donnée compacte de la carte « mois en cours ». */
function LiveStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <p className="truncate text-xs text-muted-foreground">{label}</p>
      <p className="truncate text-base font-semibold tabular-nums sm:text-lg">{value}</p>
    </div>
  );
}

function ArchivesTab() {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const queryClient = useQueryClient();
  const [confirmOpen, setConfirmOpen] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ["/api/reports/journals"],
    queryFn: () => api<JournalsResponse>("/api/reports/journals"),
  });

  // Chiffres LIVE du mois courant — même source que l'aperçu (period=month,
  // tous sites) : jamais confondus avec les archives gelées.
  const { data: live } = useQuery({
    queryKey: ["/api/stats/overview", "month", "all"],
    queryFn: () =>
      api<StatsOverview>("/api/stats/overview", { params: { period: "month", routerId: "all" } }),
  });

  const closeMutation = useMutation({
    mutationFn: () =>
      api<{ journal: MonthlyJournal }>("/api/reports/journals/close", { method: "POST" }),
    onSuccess: () => {
      toast.success(t("reports.journals.toastClosed"));
      setConfirmOpen(false);
      queryClient.invalidateQueries({ queryKey: ["/api/reports/journals"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const journals = data?.journals ?? [];
  const currentMonth = data?.currentMonth ?? "";
  const currentClosed = data?.currentClosed ?? false;
  const currentJournal = journals.find((j) => j.month === currentMonth);
  const liveKpis = live?.kpis;
  // Jours écoulés du mois courant (avertissement de clôture) — dérivé de la
  // fenêtre servie par l'aperçu, fuseau du compte.
  const elapsedDays = useMemo(() => {
    if (!live?.window?.start) return 1;
    const start = new Date(live.window.start).getTime();
    const end = new Date(live.window.end).getTime();
    return Math.max(1, Math.ceil((end - start) / 86_400_000));
  }, [live]);

  const csvExport = () =>
    apiDownload("/api/reports/journals.csv", "mikcloud-journaux-mensuels.csv")
      .then(() => toast.success(t("common.exportDownloaded")))
      .catch((err: Error) => toast.error(err.message));

  return (
    <div className="space-y-4 sm:space-y-6">
      {/* Mois en cours — live tant qu'il n'est pas clôturé, gelé ensuite */}
      <Card className="gap-4 py-4 sm:py-5">
        <CardHeader className="px-4 sm:px-6">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
            <div className="min-w-0">
              <CardTitle className="flex flex-wrap items-center gap-2 text-base">
                {t("reports.journals.currentTitle")} — {currentMonth ? monthLabel(currentMonth, lang) : "…"}
                <span className="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[10px] font-medium">
                  {currentClosed ? (
                    <>
                      <LockKeyhole className="size-3" aria-hidden />
                      {t("reports.journals.frozenBadge")}
                    </>
                  ) : (
                    <>
                      <span className="live-dot size-1.5 rounded-full bg-primary" aria-hidden />
                      {t("reports.journals.liveBadge")}
                    </>
                  )}
                </span>
              </CardTitle>
              <CardDescription>
                {currentClosed
                  ? t("reports.journals.currentClosedDesc")
                  : t("reports.journals.currentLiveDesc")}
              </CardDescription>
            </div>
            {!currentClosed && currentMonth !== "" && (
              <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
                <AlertDialogTrigger asChild>
                  <Button variant="outline" className="h-9 shrink-0" disabled={closeMutation.isPending}>
                    <LockKeyhole className="size-4" />
                    <span className="hidden sm:inline">{t("reports.journals.closeNow")}</span>
                    <span className="sm:hidden">{t("reports.journals.closeConfirmAction")}</span>
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>
                      {tf("reports.journals.closeConfirmTitle", {
                        month: monthLabel(currentMonth, lang),
                      })}
                    </AlertDialogTitle>
                    <AlertDialogDescription>
                      {tf("reports.journals.closeConfirmDesc", {
                        month: monthLabel(currentMonth, lang),
                        days: elapsedDays,
                      })}
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel disabled={closeMutation.isPending}>
                      {t("common.cancel")}
                    </AlertDialogCancel>
                    <AlertDialogAction
                      disabled={closeMutation.isPending}
                      onClick={(e) => {
                        e.preventDefault(); // garde le dialogue ouvert pendant la mutation
                        closeMutation.mutate();
                      }}
                    >
                      {closeMutation.isPending ? t("reports.journals.closing") : t("reports.journals.closeConfirmAction")}
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            )}
          </div>
        </CardHeader>
        <CardContent className="px-4 sm:px-6">
          {isLoading && !data ? (
            <LoadingCards cards={1} />
          ) : currentClosed && currentJournal ? (
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
              <LiveStat label={t("reports.sales")} value={String(currentJournal.sales)} />
              <LiveStat label={t("reports.revenue")} value={formatCurrency(currentJournal.revenue, currency, lang)} />
              <LiveStat label={t("reports.avgTicket")} value={formatCurrency(currentJournal.avgTicket, currency, lang)} />
              <LiveStat label={t("reports.overview.loginsTitle")} value={new Intl.NumberFormat(localeOf(lang)).format(currentJournal.logins)} />
              <LiveStat label={t("reports.overview.dataVolume")} value={formatBytes(currentJournal.dataIn + currentJournal.dataOut, lang)} />
            </div>
          ) : (
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
              <LiveStat label={t("reports.sales")} value={liveKpis ? String(liveKpis.sales) : "—"} />
              <LiveStat label={t("reports.revenue")} value={liveKpis ? formatCurrency(liveKpis.revenue, currency, lang) : "—"} />
              <LiveStat label={t("reports.avgTicket")} value={liveKpis ? formatCurrency(liveKpis.avgTicket, currency, lang) : "—"} />
              <LiveStat label={t("reports.overview.loginsTitle")} value={liveKpis ? new Intl.NumberFormat(localeOf(lang)).format(liveKpis.logins) : "—"} />
              <LiveStat label={t("reports.overview.dataVolume")} value={liveKpis ? formatBytes(liveKpis.dataBytes, lang) : "—"} />
            </div>
          )}
        </CardContent>
      </Card>

      {/* Archives — un mois gelé par ligne, du plus récent au plus ancien */}
      <Card className="gap-4 py-4 sm:py-5">
        <CardHeader className="px-4 sm:px-6">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0">
              <CardTitle className="text-base">{t("reports.journals.title")}</CardTitle>
              <CardDescription>{t("reports.journals.desc")}</CardDescription>
            </div>
            {journals.length > 0 && (
              <Button variant="outline" className="h-9 shrink-0" onClick={csvExport}>
                <Download className="size-4" />
                <span className="hidden sm:inline">{t("common.exportCsv")}</span>
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent className="px-4 sm:px-6">
          {isLoading && !data ? (
            <LoadingCards cards={1} />
          ) : journals.length === 0 ? (
            <EmptyState
              icon={Archive}
              title={t("reports.journals.empty")}
              description={t("reports.journals.emptyDesc")}
            />
          ) : (
            <div className="max-h-96 overflow-y-auto">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/50 hover:bg-muted/50">
                    <TableHead className="pl-0">{t("reports.journals.monthCol")}</TableHead>
                    <TableHead className="text-right">{t("reports.sales")}</TableHead>
                    <TableHead className="text-right">{t("reports.revenue")}</TableHead>
                    <TableHead className="hidden text-right md:table-cell">{t("reports.avgTicket")}</TableHead>
                    <TableHead className="hidden text-right md:table-cell">{t("reports.overview.loginsTitle")}</TableHead>
                    <TableHead className="hidden text-right lg:table-cell">{t("reports.overview.dataVolume")}</TableHead>
                    <TableHead className="hidden text-right md:table-cell">{t("reports.journals.coverageCol")}</TableHead>
                    <TableHead className="pr-0 text-right">{t("reports.journals.closeCol")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {journals.map((j) => (
                    <TableRow key={j.id}>
                      <TableCell className="pl-0 font-medium">
                        <span className="capitalize">{monthLabel(j.month, lang)}</span>
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        <span className="font-medium">{new Intl.NumberFormat(localeOf(lang)).format(j.sales)}</span>
                        <span className="block text-xs text-muted-foreground">
                          {tf("reports.journals.channelSplit", { d: j.directSales, r: j.resellerSales })}
                        </span>
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatCurrency(j.revenue, currency, lang)}
                      </TableCell>
                      <TableCell className="hidden text-right tabular-nums md:table-cell">
                        {formatCurrency(j.avgTicket, currency, lang)}
                      </TableCell>
                      <TableCell className="hidden text-right tabular-nums md:table-cell">
                        {new Intl.NumberFormat(localeOf(lang)).format(j.logins)}
                      </TableCell>
                      <TableCell className="hidden text-right tabular-nums lg:table-cell">
                        {formatBytes(j.dataIn + j.dataOut, lang)}
                      </TableCell>
                      <TableCell className="hidden text-right md:table-cell">
                        <span className="tabular-nums text-muted-foreground">
                          {tf("reports.journals.daysCovered", { n: j.days })}
                        </span>
                        {j.partial && (
                          <Badge variant="outline" className="ml-1.5 border-amber-500/30 bg-amber-500/10 px-1.5 py-0 text-[10px] font-medium text-amber-600 dark:text-amber-400">
                            {t("reports.journals.partialBadge")}
                          </Badge>
                        )}
                      </TableCell>
                      <TableCell className="pr-0 text-right">
                        <Badge
                          variant="outline"
                          className={
                            j.source === "manual"
                              ? "border-primary/30 bg-primary/10 px-1.5 py-0 text-[10px] font-medium text-primary"
                              : "border-border bg-muted px-1.5 py-0 text-[10px] font-medium text-muted-foreground"
                          }
                        >
                          {j.source === "manual" ? t("reports.journals.sourceManual") : t("reports.journals.sourceAuto")}
                        </Badge>
                        <span className="block text-xs text-muted-foreground">
                          {tf("reports.journals.closedOn", { date: formatDateTime(j.closedAt, lang) })}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Vue Rapports — cinq onglets, UNE métrique par foyer (v4) :
// Aperçu (les cartes KPI, période calendaire) / Comptabilité (le CA par
// bucket + répartitions) / Activité (l'usage réseau) / Marge (la
// rentabilité, avec diagnostic du cas dégénéré) / Archives (journaux
// gelés).
// ─────────────────────────────────────────────────────────────────────────────

type ReportsTab = "overview" | "accounting" | "activity" | "margin" | "archives";

export default function ReportsView() {
  const { t } = useI18n();
  const [tab, setTab] = useState<ReportsTab>("overview");

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("reports.title")}
        description={t("reports.description")}
        actions={
          <Tabs value={tab} onValueChange={(value) => setTab(value as ReportsTab)}>
            {/* Mobile : les cinq onglets se replient sur deux lignes plutôt
                que de déborder (audit v2 : barre tassée impossible à taper).
                flex-none sur l'orphelin de seconde ligne — sinon flex-1
                l'étire sur toute la largeur (mesuré au DOM : 352 px). */}
            <TabsList className="h-auto w-full flex-wrap justify-start gap-1 sm:w-auto">
              <TabsTrigger value="overview" className="flex-none sm:flex-1">
                {t("reports.tabOverview")}
              </TabsTrigger>
              <TabsTrigger value="accounting" className="flex-none sm:flex-1">
                {t("reports.tabAccounting")}
              </TabsTrigger>
              <TabsTrigger value="activity" className="flex-none sm:flex-1">
                {t("reports.tabActivity")}
              </TabsTrigger>
              <TabsTrigger value="margin" className="flex-none sm:flex-1">
                {t("reports.tabMargin")}
              </TabsTrigger>
              <TabsTrigger value="archives" className="flex-none sm:flex-1">
                {t("reports.tabArchives")}
              </TabsTrigger>
            </TabsList>
          </Tabs>
        }
      />

      {tab === "overview" ? (
        <OverviewTab />
      ) : tab === "accounting" ? (
        <AccountingTab />
      ) : tab === "activity" ? (
        <ActivityTab />
      ) : tab === "margin" ? (
        <MarginTab />
      ) : (
        <ArchivesTab />
      )}
    </div>
  );
}
