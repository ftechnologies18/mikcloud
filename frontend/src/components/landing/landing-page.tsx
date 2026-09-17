"use client";

/* ============================================================
   MIKCLOUD « CLAY » — Landing page FreeTech (N°120 / N°124)
   ------------------------------------------------------------
   Reconstruction complète de la vitrine : Claymorphisme & Flat
   Design, palette FreeTech (ivoire, jaune crème, vert sarcelle,
   vert menthe), rail vertical qui s'étend au survol (desktop) /
   barre tactile en bas (mobile), sculptures clay animées.
   Design system isolé dans landing-clay.css (préfixe mkl-,
   z-index 40 < modales shadcn 50 : la SignupModal passe devant).
   Contenu : UNIQUEMENT des fonctionnalités réelles (cf.
   landing-copy.ts) — pas de métriques d'usage inventées.
   ============================================================ */

import { useEffect, useRef, useState, type ComponentType } from "react";
import Image from "next/image";
import Link from "next/link";
import { Fraunces, Manrope } from "next/font/google";
import { motion, useInView, useReducedMotion } from "framer-motion";
import {
  Activity,
  ArrowRight,
  Ban,
  BarChart3,
  Bell,
  CircleDollarSign,
  Cloud,
  Globe,
  Home,
  Lock,
  MoonStar,
  Palette,
  RefreshCw,
  Router,
  ShieldCheck,
  Sparkles,
  Ticket,
  Wifi,
  Zap,
  type LucideProps,
} from "lucide-react";

import { FtciCredit } from "@/components/ftci-credit";
import { useHotspotStore } from "@/lib/hotspot/store";
import { landingCopy, type Lang } from "./landing-copy";
import { LandingChatWidget } from "./chat-widget";
import "./landing-clay.css";

/* ─── Polices display (serif Fraunces) & texte (Manrope) ─── */
const fraunces = Fraunces({
  subsets: ["latin"],
  variable: "--font-fraunces",
  display: "swap",
  axes: ["opsz"],
});
const manrope = Manrope({
  subsets: ["latin"],
  variable: "--font-manrope",
  display: "swap",
});

/* ─── Icônes des sections du rail (name → lucide) ─── */
const RAIL_ICONS: Record<string, ComponentType<LucideProps>> = {
  home: Home,
  powers: Sparkles,
  protection: ShieldCheck,
  hotspot: Wifi,
  fleet: Router,
  pricing: CircleDollarSign,
};
const POWER_ICONS = [Ticket, ShieldCheck, Router];
const PROTECTION_FEAT_ICONS = [Globe, Lock, MoonStar, Ban];
const HOTSPOT_FEAT_ICONS = [Palette, Ticket, BarChart3];
const FLEET_FEAT_ICONS = [RefreshCw, Activity, Bell];
const CHIP_ICONS = [ShieldCheck, Ticket, Zap];

/* ─── Reveal on scroll (respecte prefers-reduced-motion) ─── */
function Reveal({
  children,
  className,
  delay = 0,
}: {
  children: React.ReactNode;
  className?: string;
  delay?: number;
}) {
  const reduce = useReducedMotion();
  if (reduce) return <div className={className}>{children}</div>;
  return (
    <motion.div
      className={className}
      initial={{ opacity: 0, y: 34 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-60px" }}
      transition={{ duration: 0.7, delay, ease: [0.22, 1, 0.36, 1] }}
    >
      {children}
    </motion.div>
  );
}

/* ─── Compteur animé (bandeau stats) ─── */
function CountUp({ value, lang }: { value: number; lang: Lang }) {
  const ref = useRef<HTMLSpanElement>(null);
  const inView = useInView(ref, { once: true, margin: "-40px" });
  const reduce = useReducedMotion();
  const locale = lang === "fr" ? "fr-FR" : "en-US";
  const [animated, setAnimated] = useState<number | null>(null);

  useEffect(() => {
    if (!inView || reduce) return;
    const t0 = performance.now();
    const dur = 1600;
    let raf = 0;
    const tick = (t: number) => {
      const p = Math.min((t - t0) / dur, 1);
      const ease = 1 - Math.pow(1 - p, 3);
      setAnimated(Math.round(value * ease));
      if (p < 1) raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [inView, value, reduce]);

  /* Hors animation (pas encore visible ou mouvement réduit) : valeur
     finale directe — aucun setState synchrone dans l'effet. */
  const shown =
    !inView || reduce ? value : (animated ?? 0);

  return <span ref={ref}>{shown.toLocaleString(locale)}</span>;
}

/* --- N°124 — chips « clientèle cible » (mode Hotspot) : petites pills
    clay qui apparaissent en cascade ; prefers-reduced-motion rend des
    spans statiques (aucun mouvement). --- */
function AudienceChips({ items, left = false }: { items: string[]; left?: boolean }) {
  const reduce = useReducedMotion();
  return (
    <div className={`mkl-audience${left ? " mkl-audience-left" : ""}`}>
      {items.map((item, i) =>
        reduce ? (
          <span key={item} className="mkl-audience-chip">
            <i aria-hidden="true" />
            {item}
          </span>
        ) : (
          <motion.span
            key={item}
            className="mkl-audience-chip"
            initial={{ opacity: 0, y: 16, scale: 0.85 }}
            whileInView={{ opacity: 1, y: 0, scale: 1 }}
            viewport={{ once: true, margin: "-30px" }}
            transition={{ delay: i * 0.06, duration: 0.45, ease: [0.22, 1, 0.36, 1] }}
          >
            <i aria-hidden="true" />
            {item}
          </motion.span>
        ),
      )}
    </div>
  );
}

/* ─── Sculpture cloud clay du hero (les animations sont coupées par la
    media query prefers-reduced-motion du CSS) ─── */
function CloudStage({ chips }: { chips: string[] }) {
  return (
    <div className="mkl-cloud-stage" aria-hidden="true">
      <div className="mkl-ring mkl-ring-1" />
      <div className="mkl-ring mkl-ring-2" />
      <div className="mkl-clay-circle mkl-c1" />
      <div className="mkl-clay-circle mkl-c2" />
      <div className="mkl-clay-circle mkl-c3" />
      <div className="mkl-clay-circle mkl-c4" />
      <div className="mkl-cloud-body">
        {/* N°124 — le VRAI logo MikCloud (nuage blanc + routeur rouge +
            signal Wi-Fi vert, même design que le favicon) assis sur la
            sculpture clay ; décoratif dans un conteneur aria-hidden. */}
        <Image
          src="/logo.png"
          alt=""
          width={280}
          height={280}
          priority
          className="mkl-hero-logo"
        />
      </div>
      {chips.slice(0, 3).map((chip, i) => {
        const Icon = CHIP_ICONS[i] ?? ShieldCheck;
        return (
          <div key={chip} className={`mkl-chip mkl-chip-${"abc"[i] ?? "a"}`}>
            <Icon className="size-4" /> {chip}
          </div>
        );
      })}
    </div>
  );
}

/* ─── Panel « Centre de protection » (score + flux de supervision) ─── */
function ProtectionPanel({
  panel,
}: {
  panel: LandingPanel;
}) {
  const reduce = useReducedMotion();
  const total = panel.lines.length + 1; // + ligne auto-réparation
  const [active, setActive] = useState(0);

  useEffect(() => {
    if (reduce) return;
    const id = setInterval(() => setActive((i) => (i + 1) % total), 2400);
    return () => clearInterval(id);
  }, [reduce, total]);

  const ringLength = 2 * Math.PI * 36; // r=36 → périmètre

  return (
    <div className="mkl-panel mkl-dark">
      <div className="mkl-panel-top" aria-hidden="true">
        <i /><i /><i />
      </div>
      <p className="font-bold" style={{ marginBottom: 18 }}>{panel.title}</p>

      {/* Score anneau 4/4 */}
      <div className="mkl-score">
        <div className="mkl-score-ring">
          <svg viewBox="0 0 84 84" role="img" aria-label={`${panel.scoreLabel} : 4/4`}>
            <circle className="mkl-ring-track" cx="42" cy="42" r="36" />
            <motion.circle
              className="mkl-ring-value"
              cx="42"
              cy="42"
              r="36"
              strokeDasharray={ringLength}
              initial={reduce ? undefined : { strokeDashoffset: ringLength }}
              whileInView={reduce ? undefined : { strokeDashoffset: 0 }}
              viewport={{ once: true }}
              transition={{ duration: 1.4, ease: "easeOut" }}
            />
          </svg>
          <span className="mkl-score-num">4/4</span>
        </div>
        <div>
          <b>{panel.scoreVerdict}</b>
          <span>{panel.scoreLabel}</span>
        </div>
      </div>

      {/* Stat mini */}
      <div className="mkl-stat-row">
        {panel.stats.map((s) => (
          <div key={s.label} className="mkl-stat-mini">
            <b>{s.value}</b>
            <span>{s.label}</span>
          </div>
        ))}
      </div>

      {/* Flux de supervision : les protections s'illuminent à tour de rôle */}
      <div className="mkl-log-lines" aria-live="off">
        {panel.lines.map((line, i) => (
          <div key={line.name} className={`mkl-log-line${active === i ? " mkl-on" : ""}`}>
            <span>{line.name}</span>
            <span className={`mkl-ok${i === 2 ? " mkl-warn" : ""}`}>{line.state}</span>
          </div>
        ))}
        <div className={`mkl-log-line${active === panel.lines.length ? " mkl-on" : ""}`}>
          <span>{panel.repairLine.name}</span>
          <span className="mkl-ok mkl-warn">{panel.repairLine.state}</span>
        </div>
      </div>
    </div>
  );
}

type LandingPanel = ReturnType<
  () => import("./landing-copy").LandingCopy["protection"]["panel"]
>;

/* ===========================================================
   LANDING PAGE
   =========================================================== */
export interface LandingPageProps {
  /** Déclenche l'écran de connexion (au lieu de la landing). */
  onSignIn: () => void;
  /** Ouvre la modale d'inscription (SignupModal). */
  onSignUp: () => void;
}

export default function LandingPage({ onSignIn, onSignUp }: LandingPageProps) {
  const lang = useHotspotStore((s) => s.lang) as Lang;
  const setLang = useHotspotStore((s) => s.setLang);
  const copy = landingCopy[lang];
  const reduce = useReducedMotion();

  /* Section active dans le rail (IntersectionObserver) */
  const [active, setActive] = useState("top");
  /* N°122 — mode de tarifs affiché : Hotspot (défaut) ou HomeNet */
  const [priceMode, setPriceMode] = useState<"hotspot" | "homenet">("hotspot");
  useEffect(() => {
    const sections = ["top", "pouvoirs", "protection", "hotspot", "parc", "tarifs"];
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) setActive(e.target.id);
        }
      },
      { rootMargin: "-40% 0px -55% 0px" },
    );
    for (const id of sections) {
      const el = document.getElementById(id);
      if (el) io.observe(el);
    }
    return () => io.disconnect();
  }, []);

  /* Scroll doux vers les ancres (scopé à la vitrine) */
  const goTo = (id: string) => (e: React.MouseEvent) => {
    e.preventDefault();
    document
      .getElementById(id)
      ?.scrollIntoView({ behavior: reduce ? "auto" : "smooth", block: "start" });
  };

  const toggleLang = () => setLang(lang === "fr" ? "en" : "fr");

  const railKeys = ["home", "powers", "protection", "hotspot", "fleet", "pricing"] as const;
  const railIds = ["top", "pouvoirs", "protection", "hotspot", "parc", "tarifs"];

  /* N°124 — segment de tarifs actif : hint, clientèle cible et formules. */
  const activeSegment =
    copy.pricing.segments.find((s) => s.id === priceMode) ?? copy.pricing.segments[0];

  return (
    <div className={`mkl-page ${fraunces.variable} ${manrope.variable}`}>
      {/* Grain + halos FreeTech */}
      <div className="mkl-grain" aria-hidden="true" />
      <div className="mkl-blob mkl-blob-1" aria-hidden="true" />
      <div className="mkl-blob mkl-blob-2" aria-hidden="true" />

      {/* ═══ RAIL VERTICAL (desktop, s'étend au survol) ═══ */}
      <nav className="mkl-rail" aria-label={copy.rail.home}>
        {/* N°124 — signature : le logo réel coiffe le rail. */}
        <Image
          src="/logo.png"
          alt="MikCloud"
          width={88}
          height={88}
          priority
          className="mkl-rail-logo"
        />
        {railKeys.map((key, i) => {
          const Icon = RAIL_ICONS[key];
          return (
            <a
              key={key}
              href={`#${railIds[i]}`}
              onClick={goTo(railIds[i])}
              className={active === railIds[i] ? "mkl-active" : undefined}
              aria-current={active === railIds[i] ? "true" : undefined}
            >
              <span className="mkl-dot" aria-hidden="true">
                <Icon />
              </span>
              <span className="mkl-lbl">{copy.rail[key]}</span>
            </a>
          );
        })}
        <button className="mkl-rail-cta" onClick={onSignUp}>
          <span className="mkl-dot" aria-hidden="true">
            <ArrowRight />
          </span>
          <span className="mkl-lbl">{copy.rail.cta}</span>
        </button>
      </nav>

      {/* ═══ RAIL MOBILE (barre tactile en bas) ═══ */}
      <nav className="mkl-rail-mobile" aria-label={copy.rail.home}>
        <Image
          src="/logo.png"
          alt=""
          width={80}
          height={80}
          className="mkl-rail-logo"
        />
        {railKeys.map((key, i) => {
          const Icon = RAIL_ICONS[key];
          return (
            <a
              key={key}
              href={`#${railIds[i]}`}
              onClick={goTo(railIds[i])}
              className={active === railIds[i] ? "mkl-active" : undefined}
              aria-label={copy.rail[key]}
            >
              <Icon className="size-5" />
            </a>
          );
        })}
        <button
          className="mkl-rail-cta-mobile"
          onClick={onSignUp}
          aria-label={copy.rail.cta}
        >
          <ArrowRight className="size-5" />
        </button>
      </nav>

      <div className="mkl-main">
        {/* ═══ TOPBAR ═══ */}
        <div className="mkl-topbar">
          <div className="mkl-wrap flex items-center justify-between py-5">
            <a
              href="#top"
              onClick={goTo("top")}
              className="mkl-brand"
              aria-label={copy.header.homeLink}
            >
              {/* N°124 — le logo réel (favicon) remplace le badge clay. */}
              <Image
                src="/logo.png"
                alt="Logo MikCloud"
                width={84}
                height={84}
                priority
                className="mkl-brand-logo"
              />
              {copy.header.brand}
            </a>
            <div className="flex items-center gap-2">
              <button className="mkl-lang" onClick={toggleLang} aria-label="Switch language">
                {copy.header.langLabel}
              </button>
              <button className="mkl-link-quiet" onClick={onSignIn}>
                {copy.header.signIn}
              </button>
              <button className="mkl-btn mkl-btn-teal mkl-btn-sm" onClick={onSignUp}>
                {copy.header.signUp} <ArrowRight className="size-4" />
              </button>
            </div>
          </div>
        </div>

        {/* ═══ HERO ═══ */}
        <section id="top" className="mkl-hero">
          <div className="mkl-wrap mkl-hero-grid">
            <div>
              <Reveal>
                <span className="mkl-eyebrow">
                  <span className="mkl-pulse" aria-hidden="true" />
                  {copy.hero.badge}
                </span>
              </Reveal>
              <Reveal delay={0.06}>
                <h1>
                  {copy.hero.title1}
                  <br />
                  <em>{copy.hero.titleAccent}</em> {copy.hero.title2}
                </h1>
              </Reveal>
              <Reveal delay={0.12}>
                <p className="mkl-lead">{copy.hero.subtitle}</p>
              </Reveal>
              <Reveal delay={0.18}>
                <div className="mkl-hero-actions">
                  <button className="mkl-btn mkl-btn-teal" onClick={onSignUp}>
                    {copy.hero.ctaPrimary}
                  </button>
                  <a
                    href="#pouvoirs"
                    onClick={goTo("pouvoirs")}
                    className="mkl-btn mkl-btn-cream"
                  >
                    {copy.hero.ctaSecondary}
                  </a>
                </div>
                <p className="mkl-trial-hint">{copy.hero.trialHint}</p>
              </Reveal>
            </div>
            <Reveal delay={0.15}>
              <CloudStage chips={copy.hero.chips} />
            </Reveal>
          </div>
        </section>

        {/* ═══ MARQUEE ═══ */}
        <div className="mkl-marquee" aria-hidden="true">
          <div className="mkl-marquee-track">
            {[0, 1].map((dup) => (
              <span key={dup}>
                {copy.marquee.map((item) => (
                  <span key={`${dup}-${item}`}>
                    {item} <i>✦</i>
                  </span>
                ))}
              </span>
            ))}
          </div>
        </div>

        {/* ═══ SUPER-POUVOIRS ═══ */}
        <section id="pouvoirs">
          <div className="mkl-wrap">
            <Reveal className="mkl-sec-head">
              <span className="mkl-kicker">{copy.powers.eyebrow}</span>
              <h2>{copy.powers.title}</h2>
              <p>{copy.powers.subtitle}</p>
            </Reveal>
            <div className="mkl-powers">
              {copy.powers.cards.map((card, i) => {
                const Icon = POWER_ICONS[i] ?? Ticket;
                return (
                  <Reveal key={card.title} delay={i * 0.08} className="h-full">
                    <article className={`mkl-power mkl-p${i + 1} h-full`}>
                      <span className="mkl-tag">{card.tag}</span>
                      <div className="mkl-icon" aria-hidden="true">
                        <Icon className="size-7" />
                      </div>
                      <h3>{card.title}</h3>
                      <p>{card.desc}</p>
                      <ul>
                        {card.items.map((item) => (
                          <li key={item}>{item}</li>
                        ))}
                      </ul>
                    </article>
                  </Reveal>
                );
              })}
            </div>
          </div>
        </section>

        {/* ═══ PROTECTION CLOUD ═══ */}
        <section id="protection">
          <div className="mkl-wrap mkl-split">
            <Reveal>
              <span className="mkl-kicker">{copy.protection.kicker}</span>
              <h2>{copy.protection.title}</h2>
              <p className="mkl-body">{copy.protection.body}</p>
              <div className="mkl-feat-list">
                {copy.protection.feats.map((feat, i) => {
                  const Icon = PROTECTION_FEAT_ICONS[i] ?? Globe;
                  return (
                    <div key={feat.title} className="mkl-feat">
                      <div className="mkl-fi" aria-hidden="true">
                        <Icon className="size-5" />
                      </div>
                      <div>
                        <h4>{feat.title}</h4>
                        <p>{feat.desc}</p>
                      </div>
                    </div>
                  );
                })}
              </div>
            </Reveal>
            <Reveal delay={0.1} className="mkl-split-visual">
              <ProtectionPanel panel={copy.protection.panel} />
            </Reveal>
          </div>
        </section>

        {/* ═══ HOTSPOT ═══ */}
        <section id="hotspot">
          <div className="mkl-wrap mkl-split mkl-rev">
            <Reveal className="mkl-split-visual">
              <div className="mkl-panel">
                <div className="mkl-panel-top" aria-hidden="true">
                  <i /><i /><i />
                </div>
                <div
                  className="flex items-baseline justify-between"
                  style={{ marginBottom: 6 }}
                >
                  <strong>{copy.hotspot.panel.title}</strong>
                  <span style={{ color: "var(--mkl-teal)", fontWeight: 800 }}>
                    {copy.hotspot.panel.online}
                  </span>
                </div>
                <div className="mkl-bar-chart" aria-hidden="true">
                  {[45, 70, 55, 90, 62, 78, 100, 68].map((h, i) => (
                    <motion.div
                      key={i}
                      className="mkl-bar"
                      initial={reduce ? undefined : { height: 0 }}
                      whileInView={reduce ? undefined : { height: `${h}%` }}
                      viewport={{ once: true }}
                      transition={{ duration: 0.9, delay: i * 0.07, ease: [0.22, 1, 0.36, 1] }}
                    />
                  ))}
                </div>
              </div>
            </Reveal>
            <Reveal delay={0.1}>
              <span className="mkl-kicker">{copy.hotspot.kicker}</span>
              <h2>{copy.hotspot.title}</h2>
              <p className="mkl-body">{copy.hotspot.body}</p>
              {/* N°124 — clientèle cible du mode Hotspot (chips clay). */}
              {copy.hotspot.audience.length > 0 ? (
                <div className="mkl-for-who">
                  <span className="mkl-for-who-label">{copy.hotspot.audienceLabel}</span>
                  <AudienceChips items={copy.hotspot.audience} left />
                </div>
              ) : null}
              <div className="mkl-feat-list">
                {copy.hotspot.feats.map((feat, i) => {
                  const Icon = HOTSPOT_FEAT_ICONS[i] ?? Ticket;
                  return (
                    <div key={feat.title} className="mkl-feat">
                      <div className="mkl-fi" aria-hidden="true">
                        <Icon className="size-5" />
                      </div>
                      <div>
                        <h4>{feat.title}</h4>
                        <p>{feat.desc}</p>
                      </div>
                    </div>
                  );
                })}
              </div>
            </Reveal>
          </div>
        </section>

        {/* ═══ PARC & FLOTTE ═══ */}
        <section id="parc">
          <div className="mkl-wrap mkl-split">
            <Reveal>
              <span className="mkl-kicker">{copy.fleet.kicker}</span>
              <h2>{copy.fleet.title}</h2>
              <p className="mkl-body">{copy.fleet.body}</p>
              <div className="mkl-feat-list">
                {copy.fleet.feats.map((feat, i) => {
                  const Icon = FLEET_FEAT_ICONS[i] ?? RefreshCw;
                  return (
                    <div key={feat.title} className="mkl-feat">
                      <div className="mkl-fi" aria-hidden="true">
                        <Icon className="size-5" />
                      </div>
                      <div>
                        <h4>{feat.title}</h4>
                        <p>{feat.desc}</p>
                      </div>
                    </div>
                  );
                })}
              </div>
            </Reveal>
            <Reveal delay={0.1} className="mkl-split-visual">
              {/* Maquette illustrative du parc (produit réel N°115/N°117) */}
              <div className="mkl-panel">
                <div className="mkl-panel-top" aria-hidden="true">
                  <i /><i /><i />
                </div>
                <p className="font-bold" style={{ marginBottom: 16 }}>
                  {copy.fleet.panel.title}
                </p>
                <div className="flex flex-col gap-2.5">
                  {copy.fleet.panel.rows.map((row) => (
                    <div key={row.name} className="mkl-fleet-row">
                      <b>{row.name}</b>
                      <span
                        className="mkl-ver"
                        style={{ color: row.upToDate ? "var(--mkl-teal-dark)" : "#8a6d1a" }}
                      >
                        {row.version}
                      </span>
                      <span
                        className="mkl-ver"
                        style={{ fontWeight: 800, whiteSpace: "nowrap" }}
                      >
                        {row.state}
                      </span>
                    </div>
                  ))}
                </div>
                <div className="mkl-fleet-actions" aria-hidden="true">
                  <span className="mkl-btn mkl-btn-cream">{copy.fleet.panel.checkAll}</span>
                  <span className="mkl-btn mkl-btn-teal">{copy.fleet.panel.updateAll}</span>
                </div>
              </div>
            </Reveal>
          </div>
        </section>

        {/* ═══ BANDEAU STATS ═══ */}
        <section style={{ paddingTop: 0, paddingBottom: 0 }}>
          <Reveal>
            <div className="mkl-band">
              <div className="mkl-wrap mkl-band-grid">
                {copy.stats.map((stat) => (
                  <div key={stat.label}>
                    <b>
                      <CountUp value={stat.value} lang={lang} />
                    </b>
                    <span>{stat.label}</span>
                  </div>
                ))}
              </div>
            </div>
          </Reveal>
        </section>

        {/* ═══ TARIFS ═══ */}
        <section id="tarifs">
          <div className="mkl-wrap">
            <Reveal className="mkl-sec-head mkl-center">
              <span className="mkl-kicker">{copy.pricing.eyebrow}</span>
              <h2>{copy.pricing.title}</h2>
              <p>{copy.pricing.subtitle}</p>
            </Reveal>
            {/* N°122 — segmentation Hotspot / HomeNet : le sélecteur clay
                pilote les 3 formules du mode choisi (prix, essai, arguments).
                N°124 — thumb sarcelle glissant (layoutId framer-motion,
                coupé sous prefers-reduced-motion) + chips clientèle cible
                affichées uniquement pour le mode Hotspot. */}
            <Reveal delay={0.05}>
              <div
                className="mkl-mode-toggle"
                role="group"
                aria-label={copy.pricing.modesLabel}
              >
                {copy.pricing.segments.map((seg) => {
                  const on = priceMode === seg.id;
                  return (
                    <button
                      key={seg.id}
                      type="button"
                      aria-pressed={on}
                      className={`mkl-mode-btn ${on ? "is-active" : ""}`}
                      onClick={() => setPriceMode(seg.id)}
                    >
                      {on && !reduce ? (
                        <motion.span
                          className="mkl-mode-thumb"
                          layoutId="mkl-mode-thumb"
                          aria-hidden="true"
                          transition={{ type: "spring", stiffness: 500, damping: 40 }}
                        />
                      ) : null}
                      <span className="mkl-mode-lbl">{seg.label}</span>
                    </button>
                  );
                })}
              </div>
              <p className="mkl-mode-hint" role="status">
                {activeSegment.hint}
              </p>
              {activeSegment.audience?.length ? (
                <AudienceChips key={`audience-${priceMode}`} items={activeSegment.audience} />
              ) : null}
            </Reveal>
            {/* key={priceMode} : le changement de mode REJOUE l'entrée des
                cartes (Reveal remonté) — avec prefers-reduced-motion, aucun
                mouvement (Reveal rend un div statique). */}
            <div className="mkl-pricing" key={priceMode}>
              {activeSegment.plans.map((plan, i) => (
                <Reveal key={`${priceMode}-${plan.name}`} delay={i * 0.08} className="h-full">
                  <article className={`mkl-price-card mkl-pc${i + 1} h-full`}>
                    {plan.highlight && plan.badge ? <span className="mkl-pop">{plan.badge}</span> : null}
                    <h3>{plan.name}</h3>
                    <p className="mkl-tagline">{plan.tagline}</p>
                    <div className="mkl-amount">
                      {plan.price} <small>{plan.period}</small>
                    </div>
                    <ul>
                      {plan.features.map((f) => (
                        <li key={f}>{f}</li>
                      ))}
                    </ul>
                    <button
                      className={`mkl-btn ${plan.highlight ? "mkl-btn-cream" : "mkl-btn-teal"}`}
                      style={{ justifyContent: "center" }}
                      onClick={onSignUp}
                    >
                      {plan.cta}
                    </button>
                  </article>
                </Reveal>
              ))}
            </div>
            <Reveal delay={0.15}>
              <p className="mkl-currency-note">{copy.pricing.currencyNote}</p>
            </Reveal>
          </div>
        </section>

        {/* ═══ CTA FINAL ═══ */}
        <section id="cta" style={{ paddingTop: 0 }}>
          <Reveal>
            <div className="mkl-cta">
              <Cloud className="mkl-mini-cloud mkl-mc1 size-10" aria-hidden="true" />
              <Cloud className="mkl-mini-cloud mkl-mc2 size-12" aria-hidden="true" />
              <Cloud className="mkl-mini-cloud mkl-mc3 size-7" aria-hidden="true" />
              <span className="mkl-kicker">{copy.finalCta.kicker}</span>
              <h2>{copy.finalCta.title}</h2>
              <p>{copy.finalCta.subtitle}</p>
              <div className="mkl-cta-actions">
                <button className="mkl-btn mkl-btn-teal" onClick={onSignUp}>
                  {copy.finalCta.primary} <ArrowRight className="size-4" />
                </button>
                <button className="mkl-btn mkl-btn-cream" onClick={onSignIn}>
                  {copy.finalCta.secondary}
                </button>
              </div>
            </div>
          </Reveal>
        </section>

        {/* ═══ FOOTER ═══ */}
        <footer className="mkl-footer">
          <div className="mkl-wrap">
            <div className="mkl-foot-grid">
              <div>
                <a href="#top" onClick={goTo("top")} className="mkl-brand" style={{ marginBottom: 14 }}>
                  <Image
                    src="/logo.png"
                    alt="Logo MikCloud"
                    width={84}
                    height={84}
                    className="mkl-brand-logo"
                  />
                  {copy.header.brand}
                </a>
                <p className="mkl-foot-tagline">{copy.footer.tagline}</p>
              </div>
              {copy.footer.columns.map((col) => (
                <div key={col.title}>
                  <h5>{col.title}</h5>
                  {col.links.map((link) =>
                    link.href.startsWith("/") ? (
                      <Link key={link.label} href={link.href}>
                        {link.label}
                      </Link>
                    ) : (
                      <a key={link.label} href={link.href} onClick={goTo(link.href.slice(1))}>
                        {link.label}
                      </a>
                    ),
                  )}
                </div>
              ))}
            </div>
            <div className="mkl-foot-bottom">
              <FtciCredit className="text-xs" />
            </div>
          </div>
        </footer>
      </div>

      {/* âââ ASSISTANT CHAT (NÂ°125 â remplace la FAQ) âââ */}
      <LandingChatWidget />
    </div>
  );
}
