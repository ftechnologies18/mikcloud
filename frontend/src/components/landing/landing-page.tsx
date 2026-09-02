"use client";

import { useState, type ComponentType } from "react";
import { motion, useReducedMotion, type Variants } from "framer-motion";
import { FtciCredit } from "@/components/ftci-credit";
import {
  Activity,
  BarChart3,
  Bell,
  Cloud,
  Clock,
  Coffee,
  GraduationCap,
  Globe,
  Headphones,
  Hotel,
  Lock,
  Menu,
  Monitor,
  MonitorSmartphone,
  Network,
  Rocket,
  Server,
  ShieldCheck,
  ShieldOff,
  Store,
  Ticket,
  Timer,
  TrendingDown,
  Wallet,
  Wifi,
  X,
  type LucideProps,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { Separator } from "@/components/ui/separator";
import { useHotspotStore } from "@/lib/hotspot/store";
import { landingCopy, type Lang } from "./landing-copy";

/* ─── Mappeur d'icônes (nom string → composant lucide) ─── */
const ICONS: Record<string, ComponentType<LucideProps>> = {
  Activity, BarChart3, Bell, Cloud, Clock, Coffee, GraduationCap, Globe,
  Headphones, Hotel, Lock, Monitor, MonitorSmartphone, Network, Rocket,
  Server, ShieldCheck, ShieldOff, Store, Ticket, Timer, TrendingDown, Wallet,
};

function Icon({ name, className }: { name: string; className?: string }) {
  const Cmp = ICONS[name] ?? Globe;
  return <Cmp className={className} />;
}

/* ─── Animations (respect prefers-reduced-motion) ─── */
const fadeUp: Variants = {
  hidden: { opacity: 0, y: 24 },
  show: { opacity: 1, y: 0, transition: { duration: 0.5, ease: "easeOut" } },
};

const stagger: Variants = {
  hidden: {},
  show: { transition: { staggerChildren: 0.08 } },
};

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
      initial="hidden"
      whileInView="show"
      viewport={{ once: true, margin: "-80px" }}
      variants={fadeUp}
      transition={{ delay }}
    >
      {children}
    </motion.div>
  );
}

/* ─── Container ─── */
function Container({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={`mx-auto max-w-7xl px-4 sm:px-6 lg:px-8 ${className ?? ""}`}>{children}</div>;
}

/* ─── Eyebrow (petit titre de section coloré) ─── */
function Eyebrow({ children }: { children: React.ReactNode }) {
  return (
    <span className="inline-block text-xs font-semibold uppercase tracking-widest text-primary mb-3">
      {children}
    </span>
  );
}

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
  const [mobileOpen, setMobileOpen] = useState(false);

  const toggleLang = () => setLang(lang === "fr" ? "en" : "fr");

  return (
    <div className="min-h-screen bg-background text-foreground">
      {/* ─── HEADER ─── */}
      <header className="sticky top-0 z-50 border-b border-border/60 bg-background/80 backdrop-blur-xl">
        <Container className="flex h-16 items-center justify-between">
          <a href="#top" className="flex items-center gap-2 font-bold text-lg">
            <span className="grid size-8 place-items-center rounded-lg bg-aurora text-primary-foreground shadow-lg shadow-primary/30">
              <Wifi className="size-5" />
            </span>
            {copy.header.brand}
          </a>

          <nav className="hidden md:flex items-center gap-1">
            <Button variant="ghost" size="sm" asChild>
              <a href="#benefits">{copy.header.nav.benefits}</a>
            </Button>
            <Button variant="ghost" size="sm" asChild>
              <a href="#features">{copy.header.nav.features}</a>
            </Button>
            <Button variant="ghost" size="sm" asChild>
              <a href="#pricing">{copy.header.nav.pricing}</a>
            </Button>
            <Button variant="ghost" size="sm" asChild>
              <a href="#faq">{copy.header.nav.faq}</a>
            </Button>
          </nav>

          <div className="flex items-center gap-2">
            <Button variant="ghost" size="sm" onClick={toggleLang} aria-label="Toggle language">
              <Globe className="size-4" />
              {copy.header.langLabel}
            </Button>
            <Button variant="ghost" size="sm" onClick={onSignIn} className="hidden sm:inline-flex">
              {copy.header.signIn}
            </Button>
            <Button size="sm" onClick={onSignUp} className="hidden sm:inline-flex">
              {copy.header.signUp}
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="md:hidden"
              onClick={() => setMobileOpen((v) => !v)}
              aria-label="Menu"
            >
              {mobileOpen ? <X className="size-5" /> : <Menu className="size-5" />}
            </Button>
          </div>
        </Container>

        {mobileOpen && (
          <div className="md:hidden border-t border-border/60 bg-background">
            <Container className="flex flex-col gap-1 py-3">
              <Button variant="ghost" size="sm" asChild className="justify-start">
                <a href="#benefits" onClick={() => setMobileOpen(false)}>{copy.header.nav.benefits}</a>
              </Button>
              <Button variant="ghost" size="sm" asChild className="justify-start">
                <a href="#features" onClick={() => setMobileOpen(false)}>{copy.header.nav.features}</a>
              </Button>
              <Button variant="ghost" size="sm" asChild className="justify-start">
                <a href="#pricing" onClick={() => setMobileOpen(false)}>{copy.header.nav.pricing}</a>
              </Button>
              <Button variant="ghost" size="sm" asChild className="justify-start">
                <a href="#faq" onClick={() => setMobileOpen(false)}>{copy.header.nav.faq}</a>
              </Button>
              <Button variant="ghost" size="sm" onClick={() => { setMobileOpen(false); onSignIn(); }} className="mt-2">
                {copy.header.signIn}
              </Button>
              <Button size="sm" onClick={() => { setMobileOpen(false); onSignUp(); }} className="mt-2">
                {copy.header.signUp}
              </Button>
            </Container>
          </div>
        )}
      </header>

      {/* ─── HERO ─── */}
      <section id="top" className="relative overflow-hidden">
        {/* Halos aurora en fond */}
        <div aria-hidden className="pointer-events-none absolute inset-0 -z-10">
          <div className="absolute -top-32 left-1/4 size-96 rounded-full bg-primary/20 blur-[120px]" />
          <div className="absolute top-20 right-1/4 size-80 rounded-full bg-accent/30 blur-[100px]" />
        </div>

        <Container className="py-20 sm:py-28 lg:py-32 text-center">
          <Reveal>
            <Badge variant="secondary" className="mb-6 px-3 py-1 text-xs font-medium">
              <span className="mr-1.5 inline-block size-1.5 rounded-full bg-primary animate-pulse" />
              {copy.hero.badge}
            </Badge>
          </Reveal>

          <Reveal delay={0.05}>
            <h1 className="mx-auto max-w-4xl text-4xl font-bold tracking-tight sm:text-5xl lg:text-6xl">
              {copy.hero.title1}{" "}
              <span className="bg-gradient-to-r from-primary to-accent-foreground bg-clip-text text-transparent">
                {copy.hero.titleAccent}
              </span>{" "}
              {copy.hero.title2}
            </h1>
          </Reveal>

          <Reveal delay={0.1}>
            <p className="mx-auto mt-6 max-w-2xl text-base text-muted-foreground sm:text-lg">
              {copy.hero.subtitle}
            </p>
          </Reveal>

          <Reveal delay={0.15}>
            <div className="mt-8 flex flex-col items-center gap-3 sm:flex-row sm:justify-center">
              <Button size="lg" onClick={onSignUp} className="w-full sm:w-auto">
                {copy.hero.ctaSignUp}
              </Button>
              <Button size="lg" variant="outline" onClick={onSignIn} className="w-full sm:w-auto">
                {copy.hero.ctaPrimary}
              </Button>
            </div>
          </Reveal>

          <Reveal delay={0.2}>
            <p className="mt-3 text-xs text-muted-foreground">{copy.hero.trialHint}</p>
          </Reveal>

          {/* Stat bar */}
          <Reveal delay={0.25}>
            <div className="mt-16 grid grid-cols-2 gap-4 sm:grid-cols-4 sm:gap-8">
              {copy.hero.statBar.map((stat) => (
                <div key={stat.label} className="text-center">
                  <div className="text-3xl font-bold text-primary sm:text-4xl">{stat.value}</div>
                  <div className="mt-1 text-xs text-muted-foreground sm:text-sm">{stat.label}</div>
                </div>
              ))}
            </div>
          </Reveal>
        </Container>
      </section>

      {/* ─── TRUST BAR ─── */}
      <section className="border-y border-border/60 bg-card/30">
        <Container className="py-6">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {copy.trust.items.map((item) => (
              <div key={item.label} className="flex items-center gap-3">
                <Icon name={item.icon} className="size-5 shrink-0 text-primary" />
                <span className="text-sm text-muted-foreground">{item.label}</span>
              </div>
            ))}
          </div>
        </Container>
      </section>

      {/* ─── BENEFITS ─── */}
      <section id="benefits" className="py-16 sm:py-24">
        <Container>
          <Reveal className="mx-auto max-w-2xl text-center">
            <Eyebrow>{copy.benefits.eyebrow}</Eyebrow>
            <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">{copy.benefits.title}</h2>
            <p className="mt-4 text-muted-foreground">{copy.benefits.subtitle}</p>
          </Reveal>

          <motion.div
            className="mt-12 grid gap-6 sm:grid-cols-2 lg:grid-cols-4"
            initial="hidden"
            whileInView="show"
            viewport={{ once: true, margin: "-80px" }}
            variants={stagger}
          >
            {copy.benefits.items.map((item) => (
              <motion.div key={item.title} variants={fadeUp}>
                <Card className="h-full border-border/60 bg-card/50 backdrop-blur transition-colors hover:border-primary/40">
                  <CardHeader>
                    <div className="mb-3 grid size-11 place-items-center rounded-xl bg-primary/10 text-primary">
                      <Icon name={item.icon} className="size-5" />
                    </div>
                    <CardTitle className="text-lg">{item.title}</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <p className="text-sm text-muted-foreground">{item.description}</p>
                  </CardContent>
                </Card>
              </motion.div>
            ))}
          </motion.div>
        </Container>
      </section>

      {/* ─── FEATURES ─── */}
      <section id="features" className="py-16 sm:py-24 bg-card/20">
        <Container>
          <Reveal className="mx-auto max-w-2xl text-center">
            <Eyebrow>{copy.features.eyebrow}</Eyebrow>
            <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">{copy.features.title}</h2>
            <p className="mt-4 text-muted-foreground">{copy.features.subtitle}</p>
          </Reveal>

          <motion.div
            className="mt-12 grid gap-6 md:grid-cols-2 lg:grid-cols-3"
            initial="hidden"
            whileInView="show"
            viewport={{ once: true, margin: "-80px" }}
            variants={stagger}
          >
            {copy.features.items.map((item) => (
              <motion.div key={item.title} variants={fadeUp}>
                <Card className="group h-full border-border/60 bg-card/50 backdrop-blur transition-all hover:border-primary/40 hover:shadow-lg hover:shadow-primary/5">
                  <CardHeader>
                    <div className="mb-4 grid size-12 place-items-center rounded-xl bg-gradient-to-br from-primary/15 to-accent/30 text-primary transition-transform group-hover:scale-110">
                      <Icon name={item.icon} className="size-6" />
                    </div>
                    <CardTitle className="text-xl">{item.title}</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <p className="text-sm text-muted-foreground">{item.description}</p>
                    <button
                      onClick={onSignIn}
                      className="mt-4 inline-flex items-center gap-1 text-sm font-medium text-primary hover:underline"
                    >
                      {item.cta}
                      <span aria-hidden>→</span>
                    </button>
                  </CardContent>
                </Card>
              </motion.div>
            ))}
          </motion.div>
        </Container>
      </section>

      {/* ─── HOW IT WORKS ─── */}
      <section className="py-16 sm:py-24">
        <Container>
          <Reveal className="mx-auto max-w-2xl text-center">
            <Eyebrow>{copy.how.eyebrow}</Eyebrow>
            <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">{copy.how.title}</h2>
            <p className="mt-4 text-muted-foreground">{copy.how.subtitle}</p>
          </Reveal>

          <motion.div
            className="mt-12 grid gap-8 md:grid-cols-3"
            initial="hidden"
            whileInView="show"
            viewport={{ once: true, margin: "-80px" }}
            variants={stagger}
          >
            {copy.how.steps.map((step) => (
              <motion.div key={step.num} variants={fadeUp} className="relative">
                <div className="text-5xl font-bold text-primary/20">{step.num}</div>
                <h3 className="mt-2 text-xl font-semibold">{step.title}</h3>
                <p className="mt-3 text-sm text-muted-foreground">{step.description}</p>
              </motion.div>
            ))}
          </motion.div>
        </Container>
      </section>

      {/* ─── USE CASES ─── */}
      <section id="use-cases" className="py-16 sm:py-24 bg-card/20">
        <Container>
          <Reveal className="mx-auto max-w-2xl text-center">
            <Eyebrow>{copy.useCases.eyebrow}</Eyebrow>
            <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">{copy.useCases.title}</h2>
            <p className="mt-4 text-muted-foreground">{copy.useCases.subtitle}</p>
          </Reveal>

          <motion.div
            className="mt-12 grid gap-6 sm:grid-cols-2 lg:grid-cols-3"
            initial="hidden"
            whileInView="show"
            viewport={{ once: true, margin: "-80px" }}
            variants={stagger}
          >
            {copy.useCases.items.map((item) => (
              <motion.div key={item.title} variants={fadeUp}>
                <Card className="h-full border-border/60 bg-card/50">
                  <CardContent className="pt-6">
                    <div className="mb-4 grid size-11 place-items-center rounded-xl bg-primary/10 text-primary">
                      <Icon name={item.icon} className="size-5" />
                    </div>
                    <h3 className="font-semibold">{item.title}</h3>
                    <p className="mt-2 text-sm text-muted-foreground">{item.description}</p>
                  </CardContent>
                </Card>
              </motion.div>
            ))}
          </motion.div>
        </Container>
      </section>

      {/* ─── HARDWARE ─── */}
      <section className="py-16 sm:py-24">
        <Container>
          <Reveal className="mx-auto max-w-3xl text-center">
            <Eyebrow>{copy.hardware.eyebrow}</Eyebrow>
            <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">{copy.hardware.title}</h2>
            <p className="mt-4 text-muted-foreground">{copy.hardware.subtitle}</p>
          </Reveal>

          <Reveal delay={0.1} className="mt-10 flex justify-center">
            <div className="inline-flex flex-col items-center gap-3 rounded-2xl border border-border/60 bg-card/50 px-10 py-8">
              <div className="grid size-14 place-items-center rounded-xl bg-aurora text-primary-foreground shadow-lg shadow-primary/30">
                <Server className="size-7" />
              </div>
              <div className="text-2xl font-bold">{copy.hardware.primaryVendor}</div>
              <p className="max-w-xs text-center text-sm text-muted-foreground">
                {copy.hardware.primaryVendorNote}
              </p>
            </div>
          </Reveal>

          <Reveal delay={0.15} className="mt-6 text-center">
            <p className="mx-auto max-w-xl text-xs text-muted-foreground">{copy.hardware.roadmapNote}</p>
          </Reveal>
        </Container>
      </section>

      {/* ─── PRICING ─── */}
      <section id="pricing" className="py-16 sm:py-24 bg-card/20">
        <Container>
          <Reveal className="mx-auto max-w-2xl text-center">
            <Eyebrow>{copy.pricing.eyebrow}</Eyebrow>
            <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">{copy.pricing.title}</h2>
            <p className="mt-4 text-muted-foreground">{copy.pricing.subtitle}</p>
          </Reveal>

          <motion.div
            className="mx-auto mt-12 grid max-w-4xl gap-6 md:grid-cols-2"
            initial="hidden"
            whileInView="show"
            viewport={{ once: true, margin: "-80px" }}
            variants={stagger}
          >
            {copy.pricing.plans.map((plan) => (
              <motion.div key={plan.name} variants={fadeUp}>
                <Card
                  className={`relative h-full ${
                    plan.highlight
                      ? "border-primary/50 bg-card shadow-xl shadow-primary/10 ring-1 ring-primary/30"
                      : "border-border/60 bg-card/50"
                  }`}
                >
                  {plan.badge && (
                    <div className="absolute -top-3 left-1/2 -translate-x-1/2">
                      <Badge className="bg-aurora text-primary-foreground shadow-lg">
                        {plan.badge}
                      </Badge>
                    </div>
                  )}
                  <CardHeader className="text-center pb-0">
                    <h3 className="text-xl font-semibold">{plan.name}</h3>
                    <p className="text-sm text-muted-foreground">{plan.tagline}</p>
                    <div className="mt-4">
                      <span className="text-4xl font-bold">{plan.price}</span>
                      <span className="ml-1 text-sm text-muted-foreground">{plan.period}</span>
                    </div>
                  </CardHeader>
                  <CardContent className="mt-6">
                    <Button
                      className="w-full"
                      variant={plan.highlight ? "default" : "outline"}
                      onClick={onSignIn}
                    >
                      {plan.cta}
                    </Button>
                    <Separator className="my-6" />
                    <ul className="space-y-3">
                      {plan.features.map((f) => (
                        <li key={f} className="flex items-start gap-2 text-sm">
                          <ShieldCheck className="mt-0.5 size-4 shrink-0 text-primary" />
                          <span className="text-muted-foreground">{f}</span>
                        </li>
                      ))}
                    </ul>
                  </CardContent>
                </Card>
              </motion.div>
            ))}
          </motion.div>

          <Reveal delay={0.1} className="mt-8 text-center">
            <p className="mx-auto max-w-3xl text-xs text-muted-foreground">{copy.pricing.currencyNote}</p>
          </Reveal>
        </Container>
      </section>

      {/* ─── TESTIMONIALS / VALUE PROPS ─── */}
      <section className="py-16 sm:py-24">
        <Container>
          <Reveal className="mx-auto max-w-2xl text-center">
            <Eyebrow>{copy.testimonials.eyebrow}</Eyebrow>
            <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">{copy.testimonials.title}</h2>
            <p className="mt-4 text-muted-foreground">{copy.testimonials.subtitle}</p>
          </Reveal>

          <motion.div
            className="mt-12 grid gap-6 sm:grid-cols-2 lg:grid-cols-4"
            initial="hidden"
            whileInView="show"
            viewport={{ once: true, margin: "-80px" }}
            variants={stagger}
          >
            {copy.testimonials.valueProps.map((item) => (
              <motion.div key={item.title} variants={fadeUp}>
                <Card className="h-full border-border/60 bg-card/50 text-center">
                  <CardContent className="pt-8">
                    <div className="mx-auto mb-4 grid size-12 place-items-center rounded-xl bg-primary/10 text-primary">
                      <Icon name={item.icon} className="size-6" />
                    </div>
                    <div className="text-2xl font-bold text-primary">{item.title}</div>
                    <p className="mt-2 text-sm text-muted-foreground">{item.description}</p>
                  </CardContent>
                </Card>
              </motion.div>
            ))}
          </motion.div>
        </Container>
      </section>

      {/* ─── FAQ ─── */}
      <section id="faq" className="py-16 sm:py-24 bg-card/20">
        <Container>
          <Reveal className="mx-auto max-w-2xl text-center">
            <Eyebrow>{copy.faq.eyebrow}</Eyebrow>
            <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">{copy.faq.title}</h2>
            <p className="mt-4 text-muted-foreground">{copy.faq.subtitle}</p>
          </Reveal>

          <Reveal delay={0.1} className="mx-auto mt-12 max-w-3xl">
            <Accordion type="single" collapsible className="w-full">
              {copy.faq.items.map((item, i) => (
                <AccordionItem key={i} value={`item-${i}`}>
                  <AccordionTrigger className="text-left text-base font-medium">
                    {item.q}
                  </AccordionTrigger>
                  <AccordionContent className="text-sm text-muted-foreground leading-relaxed">
                    {item.a}
                  </AccordionContent>
                </AccordionItem>
              ))}
            </Accordion>
          </Reveal>
        </Container>
      </section>

      {/* ─── FINAL CTA ─── */}
      <section className="py-16 sm:py-24">
        <Container>
          <Reveal>
            <div className="relative overflow-hidden rounded-3xl bg-aurora px-6 py-16 text-center shadow-2xl shadow-primary/20 sm:px-12 sm:py-20">
              <div aria-hidden className="pointer-events-none absolute inset-0">
                <div className="absolute -top-20 -left-20 size-60 rounded-full bg-white/10 blur-3xl" />
                <div className="absolute -bottom-20 -right-20 size-60 rounded-full bg-white/10 blur-3xl" />
              </div>
              <div className="relative">
                <h2 className="mx-auto max-w-2xl text-3xl font-bold tracking-tight text-primary-foreground sm:text-4xl">
                  {copy.finalCta.title}
                </h2>
                <p className="mx-auto mt-4 max-w-xl text-primary-foreground/80">
                  {copy.finalCta.subtitle}
                </p>
                <div className="mt-8 flex flex-col items-center gap-3 sm:flex-row sm:justify-center">
                  <Button
                    size="lg"
                    variant="secondary"
                    onClick={onSignIn}
                    className="w-full bg-background text-foreground hover:bg-background/90 sm:w-auto"
                  >
                    {copy.finalCta.primary}
                  </Button>
                  <Button
                    size="lg"
                    onClick={onSignIn}
                    className="w-full border border-primary-foreground/30 bg-primary-foreground/10 text-primary-foreground hover:bg-primary-foreground/20 sm:w-auto"
                  >
                    {copy.finalCta.secondary}
                  </Button>
                </div>
              </div>
            </div>
          </Reveal>
        </Container>
      </section>

      {/* ─── FOOTER ─── */}
      <footer className="border-t border-border/60 bg-card/30">
        <Container className="py-12">
          <div className="grid gap-8 md:grid-cols-4">
            <div className="md:col-span-1">
              <a href="#top" className="flex items-center gap-2 font-bold text-lg">
                <span className="grid size-8 place-items-center rounded-lg bg-aurora text-primary-foreground">
                  <Wifi className="size-5" />
                </span>
                {copy.footer.tagline && copy.header.brand}
              </a>
              <p className="mt-3 text-sm text-muted-foreground">{copy.footer.tagline}</p>
            </div>

            {copy.footer.columns.map((col) => (
              <div key={col.title}>
                <h4 className="text-sm font-semibold">{col.title}</h4>
                <ul className="mt-3 space-y-2">
                  {col.links.map((link) => (
                    <li key={link.label}>
                      <a
                        href={link.href}
                        className="text-sm text-muted-foreground hover:text-foreground transition-colors"
                      >
                        {link.label}
                      </a>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>

          <Separator className="my-8" />

          <div className="flex flex-col items-center justify-between gap-4 text-center sm:flex-row sm:text-left">
            <FtciCredit className="text-xs text-muted-foreground" />
            <div className="flex flex-col items-center gap-1 text-xs text-muted-foreground sm:items-end">
              <a href={`mailto:${copy.footer.contact}`} className="hover:text-foreground">
                {copy.footer.contact}
              </a>
              <span>{copy.footer.location}</span>
            </div>
          </div>
        </Container>
      </footer>
    </div>
  );
}
