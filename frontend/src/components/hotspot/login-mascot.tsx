"use client";

// N°143 — « Miko », la mascotte flat design de l'écran de connexion.
//
// Personnage vectoriel (SVG pur, zéro dépendance) qui PORTE le formulaire :
// ses mains reposent sur le bord supérieur de la carte de verre (le stage
// du login chevauche la carte de ~14 px, z au-dessus), ses pupilles suivent
// la saisie de l'identifiant (prop gaze normalisée -1..1), il se cache les
// yeux pendant la frappe d'un secret (mot de passe / PIN — prop covering),
// ne « triche » qu'un œil quand l'utilisateur affiche le mot de passe
// (peeking), s'étonne à l'échec (shocked), exulte pendant la soumission et
// au succès (excited / happy), penche la tête vers la bulle du code 2FA
// (curious + totpActive/totpDots) et salue de la main droite quand on
// bascule de rôle (waving). La tenue suit le toggle Admin/Revendeur :
// casque opérateur avec micro (console) ou casquette terrain avec visière
// et marque-nuage (Mode Vente).
//
// Discipline N°78 : le bundle du login reste sans framer-motion — toutes les
// animations vivent en CSS (globals.css, classes mik-mascot-* / mik-eye /
// mik-hand / mik-bubble…) et les poses sont des `style.transform` transitions
// CSS. Piège SVG assumé : `transform-box: view-box` (classe .mik-org) fait
// résoudre les transform-origin dans le repère du viewBox 240×210, pour que
// rotations et scales restent justes à toutes les tailles d'affichage.
// Accessibilité : role="img" + aria-label, le reste est décoratif ;
// prefers-reduced-motion coupe les boucles (globals.css).

import { useId } from "react";

import { cn } from "@/lib/utils";

export type MascotMood = "idle" | "happy" | "excited" | "shocked" | "curious";
export type MascotMode = "admin" | "reseller";

export interface LoginMascotProps {
  mode: MascotMode;
  mood?: MascotMood;
  /** Regard normalisé (-1..1 en x et y) — pupilles + inclinaison de la tête. */
  gaze?: { x: number; y: number };
  /** Vrai pendant la frappe d'un secret : les mains montent sur les yeux. */
  covering?: boolean;
  /** Œillo : la main droite s'abaisse et l'œil droit plisse (mot de passe affiché). */
  peeking?: boolean;
  /** Salut de la main droite (changement de rôle). */
  waving?: boolean;
  /** Étape 2FA : bulle flottante dont les points se remplissent avec le code. */
  totpActive?: boolean;
  totpDots?: number;
  label?: string;
  className?: string;
}

/* Palette flat fixe (le personnage garde ses couleurs quel que soit le
   thème — une mascotte ne change pas de peau en mode nuit ; ses teintes
   reprennent la famille émeraude/menthe « Aurora Emerald »). */
const C = {
  body: "#F4FBF7",
  bodyEdge: "#8FC3AD",
  bodyShade: "#DCEDE3",
  screen: "#0B322A",
  screenEdge: "#07231D",
  glow: "#8BF3CD",
  pupil: "#04201A",
  glint: "#EFFFF9",
  ink: "#0D3A31",
  mint: "#7CF0C9",
  halo: "#2FBF8F",
  blush: "rgba(124, 240, 201, 0.25)",
  capA: "#11805F",
  capB: "#0A5743",
  cloud: "#F4FBF7",
} as const;

const EYE_L = { x: 95, y: 96 };
const EYE_R = { x: 145, y: 96 };

/* Poses des mains (translations dans le repère viewBox, origine = centre
   de la main au repos). Cover cale chaque moufle PILE sur un œil. */
const POSE = {
  restL: { x: 0, y: 0, r: -5 },
  restR: { x: 0, y: 0, r: 5 },
  coverL: { x: 27, y: -76, r: -2 },
  coverR: { x: -27, y: -76, r: 2 },
  peekR: { x: -15, y: -40, r: 24 },
  waveR: { x: -6, y: -96, r: 0 },
} as const;

/** Moufle flat : capsule + poignet ombré + doigts suggérés. */
function Mitten({ x }: { x: number }) {
  return (
    <>
      <rect x={x} y={150} width={36} height={50} rx={17} fill={C.body} stroke={C.bodyEdge} strokeWidth={3} />
      <rect x={x + 2.5} y={150} width={31} height={9} rx={4.5} fill={C.bodyShade} />
      <path
        d={`M${x + 10},186 v9 M${x + 18},188 v10 M${x + 26},186 v9`}
        stroke={C.bodyEdge}
        strokeWidth={2.2}
        strokeLinecap="round"
        fill="none"
      />
    </>
  );
}

export function LoginMascot({
  mode,
  mood = "idle",
  gaze = { x: 0, y: 0 },
  covering = false,
  peeking = false,
  waving = false,
  totpActive = false,
  totpDots = 0,
  label,
  className,
}: LoginMascotProps) {
  // Les id React contiennent « : » — nettoyés pour l' référence url(#…).
  const shadowId = `mik-shadow-${useId().replace(/[^a-zA-Z0-9]/g, "")}`;

  const gx = Math.max(-1, Math.min(1, gaze.x));
  const gy = Math.max(-1, Math.min(1, gaze.y));
  const px = gx * 5.5;
  const py = gy * 4;

  // Humeur → expressions (œils, sourcils, bouche, joues, penché).
  const eyeScale = mood === "shocked" ? 1.22 : mood === "excited" ? 1.1 : mood === "curious" ? 1.04 : 1;
  const pupilScale = mood === "shocked" ? 0.5 : 1;
  const browY = mood === "shocked" ? -7 : mood === "curious" ? -6 : mood === "excited" ? -4 : mood === "happy" ? -2 : 0;
  const browRotL = mood === "curious" ? -10 : mood === "excited" ? -6 : 0;
  const browRotR = mood === "curious" ? 0 : mood === "excited" ? 6 : 0;
  const tilt = mood === "curious" ? -5 : gx * 2.2;
  const blushOn = mood === "happy" || mood === "excited" || peeking;
  const eyesOpen = mood !== "happy";
  const mouth: "smile" | "grin" | "o" | "flat" = covering
    ? "flat"
    : mood === "happy" || mood === "excited"
      ? "grin"
      : mood === "shocked"
        ? "o"
        : "smile";

  // Poses des mains : la gauche couvre, la droite sait tricher (œillo) et saluer.
  const poseL = covering ? POSE.coverL : POSE.restL;
  const poseR = waving ? POSE.waveR : covering ? (peeking ? POSE.peekR : POSE.coverR) : POSE.restR;
  // Œil droit plissé quand Miko ne regarde « que d'un petit œil ».
  const eyeScaleR = peeking ? `${eyeScale}, 0.55` : `${eyeScale}`;

  const totpDone = totpDots >= 6;

  return (
    <svg
      viewBox="0 0 240 210"
      role="img"
      aria-label={label}
      className={cn("mik-mascot h-auto select-none", className)}
    >
      <defs>
        <filter id={shadowId} x="-30%" y="-30%" width="160%" height="170%">
          <feDropShadow dx="0" dy="7" stdDeviation="9" floodColor="#2A6B58" floodOpacity="0.22" />
        </filter>
      </defs>

      <g className="mik-mascot-bob">
        {/* ——— Tête (penche avec le regard / l'humeur) ——— */}
        <g
          className="mik-org"
          style={{
            transform: `rotate(${tilt}deg)`,
            transformOrigin: "120px 110px",
            transition: "transform 0.4s cubic-bezier(0.34, 1.3, 0.4, 1)",
          }}
        >
          {/* Antenne + marque-nuée (Admin : flotte au-dessus du casque). */}
          {mode === "admin" && (
            <g>
              <path d="M120,40 L120,26" stroke={C.bodyEdge} strokeWidth={5} strokeLinecap="round" fill="none" />
              <circle
                className="mik-halo"
                cx={120}
                cy={13}
                r={13}
                fill="none"
                stroke={C.halo}
                strokeWidth={2}
                opacity={0.55}
                style={{ transformOrigin: "120px 13px" }}
              />
              <circle cx={113} cy={15} r={5.5} fill={C.mint} />
              <circle cx={120} cy={11} r={7} fill={C.mint} />
              <circle cx={127} cy={15} r={5.5} fill={C.mint} />
              <rect x={109} y={14} width={22} height={8} rx={4} fill={C.mint} />
            </g>
          )}

          {/* Tête + oreilles + écran visage (le filtre donne le relief clay). */}
          <g filter={`url(#${shadowId})`}>
            <rect x={28} y={92} width={16} height={28} rx={8} fill={C.bodyShade} stroke={C.bodyEdge} strokeWidth={3} />
            <rect x={196} y={92} width={16} height={28} rx={8} fill={C.bodyShade} stroke={C.bodyEdge} strokeWidth={3} />
            <rect x={42} y={36} width={156} height={126} rx={42} fill={C.body} stroke={C.bodyEdge} strokeWidth={3.5} />
            <rect x={62} y={60} width={116} height={84} rx={26} fill={C.screen} stroke={C.screenEdge} strokeWidth={3} />
          </g>

          {/* Joues (gaies / œillo). */}
          <ellipse
            className="mik-swap"
            cx={80}
            cy={115}
            rx={8}
            ry={4.6}
            fill={C.blush}
            opacity={blushOn ? 1 : 0}
            style={{ transformOrigin: "80px 115px" }}
          />
          <ellipse
            className="mik-swap"
            cx={160}
            cy={115}
            rx={8}
            ry={4.6}
            fill={C.blush}
            opacity={blushOn ? 1 : 0}
            style={{ transformOrigin: "160px 115px" }}
          />

          {/* Sourcils. */}
          <g
            className="mik-org"
            style={{
              transform: `translate(0px, ${browY}px) rotate(${browRotL}deg)`,
              transformOrigin: "95px 72px",
              transition: "transform 0.3s ease",
            }}
          >
            <rect x={85} y={70} width={20} height={4.6} rx={2.3} fill={C.mint} />
          </g>
          <g
            className="mik-org"
            style={{
              transform: `translate(0px, ${browY}px) rotate(${browRotR}deg)`,
              transformOrigin: "145px 72px",
              transition: "transform 0.3s ease",
            }}
          >
            <rect x={135} y={70} width={20} height={4.6} rx={2.3} fill={C.mint} />
          </g>

          {/* Yeux ouverts (pupilles vivantes + clignement CSS). */}
          <g
            className="mik-org"
            style={{
              transform: eyesOpen ? `scale(${eyeScale})` : "scale(1)",
              transformOrigin: `${EYE_L.x}px ${EYE_L.y}px`,
              transition: "transform 0.25s ease",
              opacity: eyesOpen ? 1 : 0,
            }}
          >
            <g className="mik-eye" style={{ transformOrigin: `${EYE_L.x}px ${EYE_L.y}px` }}>
              <circle cx={EYE_L.x} cy={EYE_L.y} r={11} fill={C.glow} />
              <g
                className="mik-org"
                style={{
                  transform: `translate(${px}px, ${py}px) scale(${pupilScale})`,
                  transformOrigin: `${EYE_L.x}px ${EYE_L.y}px`,
                  transition: "transform 0.16s ease-out",
                }}
              >
                <circle cx={EYE_L.x} cy={EYE_L.y} r={4.6} fill={C.pupil} />
                <circle cx={EYE_L.x - 3.2} cy={EYE_L.y - 3.4} r={1.7} fill={C.glint} opacity={0.85} />
              </g>
            </g>
          </g>
          <g
            className="mik-org"
            style={{
              transform: eyesOpen ? `scale(${eyeScaleR})` : "scale(1)",
              transformOrigin: `${EYE_R.x}px ${EYE_R.y}px`,
              transition: "transform 0.25s ease",
              opacity: eyesOpen ? 1 : 0,
            }}
          >
            <g className="mik-eye" style={{ transformOrigin: `${EYE_R.x}px ${EYE_R.y}px` }}>
              <circle cx={EYE_R.x} cy={EYE_R.y} r={11} fill={C.glow} />
              <g
                className="mik-org"
                style={{
                  transform: `translate(${px}px, ${py}px) scale(${pupilScale})`,
                  transformOrigin: `${EYE_R.x}px ${EYE_R.y}px`,
                  transition: "transform 0.16s ease-out",
                }}
              >
                <circle cx={EYE_R.x} cy={EYE_R.y} r={4.6} fill={C.pupil} />
                <circle cx={EYE_R.x - 3.2} cy={EYE_R.y - 3.4} r={1.7} fill={C.glint} opacity={0.85} />
              </g>
            </g>
          </g>

          {/* Yeux plissés de joie (succès) — remplacent les yeux ouverts. */}
          <g
            className="mik-swap"
            opacity={mood === "happy" ? 1 : 0}
            stroke={C.glow}
            strokeWidth={5}
            strokeLinecap="round"
            fill="none"
          >
            <path d="M84,98 Q95,87 106,98" />
            <path d="M134,98 Q145,87 156,98" />
          </g>

          {/* Bouches (fondu entre humeurs). */}
          <g className="mik-swap">
            <path
              d="M106,121 Q120,131 134,121"
              fill="none"
              stroke={C.glow}
              strokeWidth={4}
              strokeLinecap="round"
              opacity={mouth === "smile" ? 1 : 0}
            />
            <path
              d="M103,118 Q120,141 137,118 Q120,124 103,118 Z"
              fill={C.glow}
              opacity={mouth === "grin" ? 1 : 0}
            />
            <ellipse
              cx={120}
              cy={123}
              rx={5.5}
              ry={7}
              fill="none"
              stroke={C.glow}
              strokeWidth={3.4}
              opacity={mouth === "o" ? 1 : 0}
            />
            <path
              d="M111,123 L129,123"
              stroke={C.glow}
              strokeWidth={4}
              strokeLinecap="round"
              opacity={mouth === "flat" ? 1 : 0}
            />
          </g>

          {/* Tenue Admin : casque opérateur (arceau + écouteurs + micro). */}
          {mode === "admin" && (
            <g className="mik-acc-pop">
              <path
                d="M34,100 C34,8 206,8 206,100"
                fill="none"
                stroke={C.ink}
                strokeWidth={8}
                strokeLinecap="round"
              />
              <rect x={24} y={86} width={24} height={42} rx={11} fill={C.ink} />
              <rect x={192} y={86} width={24} height={42} rx={11} fill={C.ink} />
              <circle cx={36} cy={107} r={3.5} fill={C.mint} />
              <circle cx={204} cy={107} r={3.5} fill={C.mint} />
              <path
                d="M36,128 C44,146 66,152 84,148"
                fill="none"
                stroke={C.ink}
                strokeWidth={5}
                strokeLinecap="round"
              />
              <circle cx={87} cy={148} r={6} fill={C.mint} stroke={C.ink} strokeWidth={2.5} />
            </g>
          )}

          {/* Tenue Revendeur : casquette terrain (visière + marque-nuée). */}
          {mode === "reseller" && (
            <g transform="rotate(-6 120 44)">
              <g className="mik-acc-pop">
                <path
                  d="M64,48 C64,14 176,14 176,48 L176,52 Q120,58 64,52 Z"
                  fill={C.capA}
                  stroke={C.capB}
                  strokeWidth={3}
                  strokeLinejoin="round"
                />
                <path
                  d="M148,49 C182,46 198,52 201,60 C176,63 152,58 146,53 Z"
                  fill={C.capB}
                  stroke={C.capB}
                  strokeWidth={2}
                  strokeLinejoin="round"
                />
                <circle cx={120} cy={19.5} r={3.4} fill={C.capB} />
                <circle cx={114} cy={33} r={3.4} fill={C.cloud} />
                <circle cx={120} cy={30.5} r={4.2} fill={C.cloud} />
                <circle cx={126} cy={33} r={3.4} fill={C.cloud} />
                <rect x={111} y={32} width={18} height={6} rx={3} fill={C.cloud} />
              </g>
            </g>
          )}
        </g>

        {/* Étincelles (soumission / succès). */}
        {(mood === "excited" || mood === "happy") && (
          <>
            <g className="mik-sparkle" style={{ transformOrigin: "72px 76px" }}>
              <path d="M72,68 L74,74 L80,76 L74,78 L72,84 L70,78 L64,76 L70,74 Z" fill={C.mint} />
            </g>
            <g className="mik-sparkle" style={{ transformOrigin: "168px 76px", animationDelay: "0.18s" }}>
              <path d="M168,68 L170,74 L176,76 L170,78 L168,84 L166,78 L160,76 L166,74 Z" fill={C.mint} />
            </g>
          </>
        )}

        {/* Bulle 2FA : six points qui se remplissent avec le code tapé. */}
        {totpActive && (
          <g className="mik-float-b" style={{ transformOrigin: "200px 34px" }}>
            <g transform="rotate(8 200 34)">
              <rect
                x={172}
                y={16}
                width={56}
                height={37}
                rx={11}
                fill={C.body}
                stroke={totpDone ? C.halo : C.bodyEdge}
                strokeWidth={3}
              />
              {[0, 1, 2, 3, 4, 5].map((i) => (
                <circle
                  key={i}
                  cx={186 + (i % 3) * 14}
                  cy={i < 3 ? 28 : 41}
                  r={2.8}
                  fill={i < totpDots ? C.mint : "rgba(11, 50, 42, 0.16)"}
                />
              ))}
            </g>
          </g>
        )}

        {/* ——— Mains (par-dessus tout : elles couvrent les yeux) ——— */}
        <g
          className="mik-org mik-hand"
          style={{
            transform: `translate(${poseL.x}px, ${poseL.y}px) rotate(${poseL.r}deg)`,
            transformOrigin: "70px 175px",
          }}
        >
          <Mitten x={52} />
        </g>
        <g
          className="mik-org mik-hand"
          style={{
            transform: `translate(${poseR.x}px, ${poseR.y}px) rotate(${poseR.r}deg)`,
            transformOrigin: "170px 175px",
          }}
        >
          <g className={waving ? "mik-wave" : undefined} style={{ transformOrigin: "170px 175px" }}>
            <Mitten x={152} />
          </g>
        </g>
      </g>
    </svg>
  );
}

export default LoginMascot;
