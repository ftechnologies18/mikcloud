"use client";

import { useState, useRef, useEffect } from "react";
import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  ChevronDown,
  ArrowRight,
  ShieldCheck,
  Wifi,
  Waypoints,
  Router,
  Monitor,
  Server,
  Globe,
  Building2,
  Users,
} from "lucide-react";
import { landingCopy, type Lang } from "./landing-copy";

export interface HeaderProps {
  lang: Lang;
  toggleLang: () => void;
  onSignIn: () => void;
  onSignUp: () => void;
}

export function EnterpriseHeader({ lang, toggleLang, onSignIn, onSignUp }: HeaderProps) {
  const copy = landingCopy[lang];
  const pathname = usePathname();
  const [activeMenu, setActiveMenu] = useState<"products" | "solutions" | null>(null);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const timeoutRef = useRef<NodeJS.Timeout | null>(null);

  const handleMouseEnter = (menu: "products" | "solutions") => {
    if (timeoutRef.current) clearTimeout(timeoutRef.current);
    setActiveMenu(menu);
  };

  const handleMouseLeave = () => {
    timeoutRef.current = setTimeout(() => {
      setActiveMenu(null);
    }, 150);
  };

  useEffect(() => {
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  const closeMenus = () => {
    setActiveMenu(null);
    setMobileMenuOpen(false);
  };

  const getItemIcon = (title: string) => {
    if (title.includes("Hotspot") || title.includes("WiFi")) return Wifi;
    if (title.includes("Cybercafé")) return Monitor;
    if (title.includes("PPPoE") || title.includes("ISP")) return Server;
    if (title.includes("Protection") || title.includes("Shield")) return ShieldCheck;
    if (title.includes("WireGuard") || title.includes("VPN")) return Waypoints;
    if (title.includes("Flotte") || title.includes("MikroTik")) return Router;
    if (title.includes("Hôtellerie") || title.includes("Hospitality")) return Building2;
    if (title.includes("Nomades") || title.includes("Nomads")) return Globe;
    return Users;
  };

  return (
    <header className="mkl-topbar relative z-50">
      <div className="mkl-wrap flex items-center justify-between py-4">
        {/* Brand */}
        <Link href="/" className="mkl-brand flex items-center gap-2.5" aria-label={copy.header.homeLink} onClick={closeMenus}>
          <Image
            src="/logo.png"
            alt="Logo 403Shield"
            width={76}
            height={76}
            priority
            className="mkl-brand-logo size-10 object-contain"
          />
          <span className="font-extrabold text-xl tracking-tight text-[#123b3a]">{copy.header.brand}</span>
        </Link>

        {/* Navigation Desktop Mega-Menu */}
        <nav className="hidden md:flex items-center gap-1.5" aria-label="Main Navigation">
          <Link
            href="/"
            onClick={closeMenus}
            className={`mkl-link-quiet px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
              pathname === "/" ? "mkl-active text-[#123b3a]" : "text-[#123b3a]/80 hover:text-[#123b3a]"
            }`}
          >
            {copy.header.nav.home}
          </Link>

          {/* Menu Produit Dropdown */}
          <div
            className="relative"
            onMouseEnter={() => handleMouseEnter("products")}
            onMouseLeave={handleMouseLeave}
          >
            <button
              type="button"
              className={`mkl-link-quiet px-3 py-2 rounded-lg text-sm font-medium inline-flex items-center gap-1 transition-colors ${
                activeMenu === "products" ? "text-[#123b3a] bg-[#123b3a]/5" : "text-[#123b3a]/80 hover:text-[#123b3a]"
              }`}
              onClick={() => setActiveMenu(activeMenu === "products" ? null : "products")}
              aria-expanded={activeMenu === "products"}
            >
              {copy.header.nav.products}
              <ChevronDown className={`size-4 transition-transform duration-200 ${activeMenu === "products" ? "rotate-180" : ""}`} />
            </button>

            {/* Mega Menu Dropdown Panel */}
            {activeMenu === "products" && (
              <div className="absolute top-full left-1/2 -translate-x-1/2 mt-2 w-[720px] bg-[#fbf9f1] border border-[#123b3a]/15 rounded-2xl shadow-2xl p-6 grid grid-cols-2 gap-6 backdrop-blur-xl animate-in fade-in slide-in-from-top-2 duration-200">
                {copy.header.megaMenu.products.map((cat) => (
                  <div key={cat.title} className="flex flex-col gap-3">
                    <span className="text-xs font-extrabold tracking-wider uppercase text-[#123b3a]/60 px-2">
                      {cat.title}
                    </span>
                    <div className="flex flex-col gap-1">
                      {cat.items.map((item) => {
                        const Icon = getItemIcon(item.title);
                        return (
                          <Link
                            key={item.title}
                            href={item.href}
                            onClick={closeMenus}
                            className="p-2.5 rounded-xl hover:bg-[#123b3a]/5 transition-all group flex items-start gap-3"
                          >
                            <div className="p-2 rounded-lg bg-[#123b3a]/10 text-[#123b3a] group-hover:bg-[#123b3a] group-hover:text-white transition-colors mt-0.5">
                              <Icon className="size-4" />
                            </div>
                            <div className="flex flex-col gap-0.5">
                              <div className="flex items-center gap-2">
                                <span className="font-bold text-sm text-[#123b3a] group-hover:text-[#1a5351]">
                                  {item.title}
                                </span>
                                {item.badge && (
                                  <span className="text-[10px] font-bold px-1.5 py-0.5 rounded-md bg-[#238b81]/15 text-[#1a5351]">
                                    {item.badge}
                                  </span>
                                )}
                              </div>
                              <p className="text-xs text-[#123b3a]/70 line-clamp-2 leading-relaxed">
                                {item.desc}
                              </p>
                            </div>
                          </Link>
                        );
                      })}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Menu Solutions Dropdown */}
          <div
            className="relative"
            onMouseEnter={() => handleMouseEnter("solutions")}
            onMouseLeave={handleMouseLeave}
          >
            <button
              type="button"
              className={`mkl-link-quiet px-3 py-2 rounded-lg text-sm font-medium inline-flex items-center gap-1 transition-colors ${
                activeMenu === "solutions" ? "text-[#123b3a] bg-[#123b3a]/5" : "text-[#123b3a]/80 hover:text-[#123b3a]"
              }`}
              onClick={() => setActiveMenu(activeMenu === "solutions" ? null : "solutions")}
              aria-expanded={activeMenu === "solutions"}
            >
              {copy.header.nav.solutions}
              <ChevronDown className={`size-4 transition-transform duration-200 ${activeMenu === "solutions" ? "rotate-180" : ""}`} />
            </button>

            {/* Mega Menu Dropdown Panel */}
            {activeMenu === "solutions" && (
              <div className="absolute top-full left-1/2 -translate-x-1/2 mt-2 w-[480px] bg-[#fbf9f1] border border-[#123b3a]/15 rounded-2xl shadow-2xl p-6 backdrop-blur-xl animate-in fade-in slide-in-from-top-2 duration-200">
                {copy.header.megaMenu.solutions.map((cat) => (
                  <div key={cat.title} className="flex flex-col gap-3">
                    <span className="text-xs font-extrabold tracking-wider uppercase text-[#123b3a]/60 px-2">
                      {cat.title}
                    </span>
                    <div className="flex flex-col gap-1">
                      {cat.items.map((item) => {
                        const Icon = getItemIcon(item.title);
                        return (
                          <Link
                            key={item.title}
                            href={item.href}
                            onClick={closeMenus}
                            className="p-2.5 rounded-xl hover:bg-[#123b3a]/5 transition-all group flex items-start gap-3"
                          >
                            <div className="p-2 rounded-lg bg-[#123b3a]/10 text-[#123b3a] group-hover:bg-[#123b3a] group-hover:text-white transition-colors mt-0.5">
                              <Icon className="size-4" />
                            </div>
                            <div className="flex flex-col gap-0.5">
                              <span className="font-bold text-sm text-[#123b3a] group-hover:text-[#1a5351]">
                                {item.title}
                              </span>
                              <p className="text-xs text-[#123b3a]/70 leading-relaxed">
                                {item.desc}
                              </p>
                            </div>
                          </Link>
                        );
                      })}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          <Link
            href="/hotspot"
            onClick={closeMenus}
            className={`mkl-link-quiet px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
              pathname === "/hotspot" ? "mkl-active text-[#123b3a]" : "text-[#123b3a]/80 hover:text-[#123b3a]"
            }`}
          >
            {copy.header.nav.hotspot}
          </Link>

          <Link
            href="/tunnels"
            onClick={closeMenus}
            className={`mkl-link-quiet px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
              pathname === "/tunnels" || pathname === "/vpn" ? "mkl-active text-[#123b3a]" : "text-[#123b3a]/80 hover:text-[#123b3a]"
            }`}
          >
            {copy.header.nav.tunnels}
          </Link>

          <a
            href="/#tarifs"
            onClick={closeMenus}
            className="mkl-link-quiet px-3 py-2 rounded-lg text-sm font-medium text-[#123b3a]/80 hover:text-[#123b3a] transition-colors"
          >
            {copy.header.nav.pricing}
          </a>
        </nav>

        {/* Actions Header */}
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="mkl-lang font-semibold text-xs px-2.5 py-1.5 rounded-lg border border-[#123b3a]/20 hover:bg-[#123b3a]/10 transition-colors"
            onClick={toggleLang}
            aria-label="Switch language"
          >
            {copy.header.langLabel}
          </button>

          <button
            type="button"
            className="mkl-link-quiet hidden sm:inline-flex text-sm font-semibold px-3 py-1.5 text-[#123b3a] hover:opacity-80 transition-opacity"
            onClick={onSignIn}
          >
            {copy.header.signIn}
          </button>

          <button
            type="button"
            className="mkl-btn mkl-btn-teal mkl-btn-sm text-xs sm:text-sm font-bold flex items-center gap-1.5 px-4 py-2"
            onClick={onSignUp}
          >
            {copy.header.signUp} <ArrowRight className="size-4" />
          </button>

          {/* Toggle Mobile Menu Button */}
          <button
            type="button"
            className="md:hidden p-2 rounded-lg text-[#123b3a] hover:bg-[#123b3a]/10"
            onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            aria-label="Toggle Navigation Menu"
          >
            <div className="w-5 h-4 flex flex-col justify-between">
              <span className={`w-full h-0.5 bg-[#123b3a] rounded transition-transform ${mobileMenuOpen ? "rotate-45 translate-y-1.5" : ""}`} />
              <span className={`w-full h-0.5 bg-[#123b3a] rounded transition-opacity ${mobileMenuOpen ? "opacity-0" : ""}`} />
              <span className={`w-full h-0.5 bg-[#123b3a] rounded transition-transform ${mobileMenuOpen ? "-rotate-45 -translate-y-1.5" : ""}`} />
            </div>
          </button>
        </div>
      </div>

      {/* Mobile Navigation Drawer */}
      {mobileMenuOpen && (
        <div className="md:hidden bg-[#fbf9f1] border-b border-[#123b3a]/15 px-4 py-6 flex flex-col gap-4 animate-in slide-in-from-top-2 duration-200">
          <div className="flex flex-col gap-2">
            <Link
              href="/"
              className="px-3 py-2 text-base font-bold text-[#123b3a]"
              onClick={closeMenus}
            >
              {copy.header.nav.home}
            </Link>
            <Link
              href="/hotspot"
              className="px-3 py-2 text-base font-bold text-[#123b3a]"
              onClick={closeMenus}
            >
              {copy.header.nav.hotspot}
            </Link>
            <Link
              href="/tunnels"
              className="px-3 py-2 text-base font-bold text-[#123b3a]"
              onClick={closeMenus}
            >
              {copy.header.nav.tunnels}
            </Link>
            <a
              href="/#tarifs"
              className="px-3 py-2 text-base font-bold text-[#123b3a]"
              onClick={closeMenus}
            >
              {copy.header.nav.pricing}
            </a>
          </div>

          <div className="pt-4 border-t border-[#123b3a]/10 flex flex-col gap-3">
            <button
              type="button"
              className="w-full text-center py-2.5 font-bold text-[#123b3a] bg-[#123b3a]/5 rounded-xl"
              onClick={() => {
                closeMenus();
                onSignIn();
              }}
            >
              {copy.header.signIn}
            </button>
          </div>
        </div>
      )}
    </header>
  );
}
