"use client";

/* ============================================================
   403SHIELD « CLAY » — Dedicated Product Pages
   ------------------------------------------------------------
   Reusable client component for marketing routes /hotspot,
   /vpn and /tunnels (Cloudflare / OVHcloud style).
   Integrates EnterpriseHeader with MegaMenu, unified Clay styling,
   and clean CTA navigation flow.
   ============================================================ */

import Image from "next/image";
import Link from "next/link";
import type { ComponentType } from "react";
import {
  ArrowRight,
  BarChart3,
  Cctv,
  CircleDollarSign,
  Cloud,
  Globe,
  KeyRound,
  Lock,
  Palette,
  Router,
  ShieldCheck,
  Store,
  Ticket,
  type LucideProps,
} from "lucide-react";

import { useHotspotStore } from "@/lib/hotspot/store";
import { landingCopy, type Lang, type ProductPageCopy } from "./landing-copy";
import { EnterpriseHeader } from "./enterprise-header";
import { LandingFooter } from "./landing-footer";
import { Reveal } from "./reveal";
import { fraunces, manrope } from "./landing-fonts";
import "./landing-clay.css";

export type ProductPageKey = "hotspot" | "vpn" | "tunnels";

const PAGE_ICONS: Record<ProductPageKey, ComponentType<LucideProps>[]> = {
  hotspot: [Palette, Ticket, Store, BarChart3],
  vpn: [Globe, Lock, CircleDollarSign, KeyRound],
  tunnels: [Globe, Router, Cctv, KeyRound],
};

function PageCta({
  href,
  label,
  variant,
  onSignIn,
  onSignUp,
  arrow = false,
}: {
  href: string;
  label: string;
  variant: "teal" | "cream";
  onSignIn: () => void;
  onSignUp: () => void;
  arrow?: boolean;
}) {
  const cls = `mkl-btn mkl-btn-${variant}`;
  const arrowIcon = arrow ? <ArrowRight className="size-4" /> : null;
  if (href === "#signup") {
    return (
      <button className={cls} onClick={onSignUp}>
        {label}
        {arrowIcon}
      </button>
    );
  }
  if (href === "#signin") {
    return (
      <button className={cls} onClick={onSignIn}>
        {label}
        {arrowIcon}
      </button>
    );
  }
  if (href.startsWith("http")) {
    return (
      <a href={href} target="_blank" rel="noopener noreferrer" className={cls}>
        {label}
        {arrowIcon}
        <span className="sr-only"> — nouvel onglet</span>
      </a>
    );
  }
  return (
    <Link href={href} className={cls}>
      {label}
      {arrowIcon}
    </Link>
  );
}

export interface ProductPageProps {
  page: ProductPageKey;
  onSignIn: () => void;
  onSignUp: () => void;
}

export default function ProductPage({ page, onSignIn, onSignUp }: ProductPageProps) {
  const lang = useHotspotStore((s) => s.lang) as Lang;
  const setLang = useHotspotStore((s) => s.setLang);
  const copy = landingCopy[lang];
  const pageCopy: ProductPageCopy = copy.pages[page];
  const icons = PAGE_ICONS[page];

  const toggleLang = () => setLang(lang === "fr" ? "en" : "fr");

  return (
    <div className={`mkl-page ${fraunces.variable} ${manrope.variable}`}>
      <div className="mkl-grain" aria-hidden="true" />
      <div className="mkl-blob mkl-blob-1" aria-hidden="true" />
      <div className="mkl-blob mkl-blob-2" aria-hidden="true" />

      <div className="mkl-main">
        {/* Unified Enterprise Header with MegaMenu */}
        <EnterpriseHeader
          lang={lang}
          toggleLang={toggleLang}
          onSignIn={onSignIn}
          onSignUp={onSignUp}
        />

        {/* HERO SECTION */}
        <section id="top" className="mkl-hero">
          <div className="mkl-wrap mkl-hero-grid">
            <div>
              <Reveal>
                <span className="mkl-eyebrow">
                  <span className="mkl-pulse" aria-hidden="true" />
                  {pageCopy.hero.badge}
                </span>
              </Reveal>
              <Reveal delay={0.06}>
                <h1>
                  {pageCopy.hero.title1}
                  <br />
                  <em>{pageCopy.hero.titleAccent}</em> {pageCopy.hero.title2}
                </h1>
              </Reveal>
              <Reveal delay={0.12}>
                <p className="mkl-lead">{pageCopy.hero.subtitle}</p>
              </Reveal>
              <Reveal delay={0.18}>
                <div className="mkl-hero-actions">
                  <PageCta
                    href={pageCopy.hero.ctaPrimaryHref}
                    label={pageCopy.hero.ctaPrimary}
                    variant="teal"
                    onSignIn={onSignIn}
                    onSignUp={onSignUp}
                    arrow
                  />
                  <PageCta
                    href={pageCopy.hero.ctaSecondaryHref}
                    label={pageCopy.hero.ctaSecondary}
                    variant="cream"
                    onSignIn={onSignIn}
                    onSignUp={onSignUp}
                  />
                </div>
              </Reveal>
            </div>
            <Reveal delay={0.15}>
              <div className="mkl-cloud-stage" aria-hidden="true">
                <div className="mkl-ring mkl-ring-1" />
                <div className="mkl-ring mkl-ring-2" />
                <div className="mkl-cloud-body">
                  <Image
                    src="/logo.png"
                    alt=""
                    width={280}
                    height={280}
                    priority
                    className="mkl-hero-logo"
                  />
                </div>
              </div>
            </Reveal>
          </div>
        </section>

        {/* FEATURES GRID */}
        <section id="features">
          <div className="mkl-wrap">
            <Reveal className="mkl-sec-head mkl-center">
              <span className="mkl-kicker">{pageCopy.features.eyebrow}</span>
              <h2>{pageCopy.features.title}</h2>
            </Reveal>
            <div className="grid gap-6 md:grid-cols-2">
              {pageCopy.features.cards.map((card, i) => {
                const Icon = icons[i] ?? ShieldCheck;
                return (
                  <Reveal key={card.title} delay={i * 0.08} className="h-full">
                    <article className={`mkl-power mkl-p${(i % 3) + 1} h-full`}>
                      <div className="mkl-icon" aria-hidden="true">
                        <Icon className="size-7" />
                      </div>
                      <h3>{card.title}</h3>
                      <p>{card.desc}</p>
                    </article>
                  </Reveal>
                );
              })}
            </div>
          </div>
        </section>

        {/* NOTE SECTION (VPN Page) */}
        {pageCopy.note ? (
          <section style={{ paddingTop: 0 }}>
            <div className="mkl-wrap">
              <Reveal>
                <div className="mkl-panel">
                  <div className="mkl-panel-top" aria-hidden="true">
                    <i /><i /><i />
                  </div>
                  <h3 style={{ fontSize: "1.25rem", margin: "0 0 8px" }}>
                    {pageCopy.note.title}
                  </h3>
                  <p style={{ color: "rgba(18, 59, 58, 0.72)", margin: 0 }}>
                    {pageCopy.note.body}
                  </p>
                </div>
              </Reveal>
            </div>
          </section>
        ) : null}

        {/* CTA FINAL */}
        <section id="cta" style={{ paddingTop: 0 }}>
          <Reveal>
            <div className="mkl-cta">
              <Cloud className="mkl-mini-cloud mkl-mc1 size-10" aria-hidden="true" />
              <Cloud className="mkl-mini-cloud mkl-mc2 size-12" aria-hidden="true" />
              <Cloud className="mkl-mini-cloud mkl-mc3 size-7" aria-hidden="true" />
              <span className="mkl-kicker">{pageCopy.cta.kicker}</span>
              <h2>{pageCopy.cta.title}</h2>
              <p>{pageCopy.cta.body}</p>
              <div className="mkl-cta-actions">
                <PageCta
                  href={pageCopy.cta.primaryHref}
                  label={pageCopy.cta.primary}
                  variant="teal"
                  onSignIn={onSignIn}
                  onSignUp={onSignUp}
                  arrow
                />
                <PageCta
                  href={pageCopy.cta.secondaryHref}
                  label={pageCopy.cta.secondary}
                  variant="cream"
                  onSignIn={onSignIn}
                  onSignUp={onSignUp}
                />
              </div>
            </div>
          </Reveal>
        </section>

        {/* FOOTER */}
        <LandingFooter copy={copy} variant="product" />
      </div>
    </div>
  );
}
