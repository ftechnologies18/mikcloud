"use client";

/* ============================================================
   MIKCLOUD « CLAY » — Widget chat de la vitrine (N°127)
   ------------------------------------------------------------
   L'assistant conversationnel remplace la section « Questions
   fréquentes » : un bouton flottant clay ouvre un panneau de
   discussion (style Claymorphisme, palette FreeTech). Le bot
   répond depuis le backend (api/chatbot.go) ; à tout moment le
   visiteur peut demander un humain — la conversation part dans
   l'inbox de la console plateforme et les réponses du support
   reviennent ici par polling (4 s quand le panneau est ouvert).

   Contrat de fil (append-only) : le front suit la conversation
   par OFFSET (nombre de messages connus) — jamais par horloge.
   Le message en vol s'affiche en bulle « pending » semi-
   transparente ; la réponse serveur (message confirmé + réponse
   bot) la remplace — une seule source de vérité.
   ============================================================ */

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import { MessageCircle, Send, UserRound, X } from "lucide-react";

import { apiAnon } from "@/lib/hotspot/api";
import { useHotspotStore } from "@/lib/hotspot/store";
import { landingCopy, type Lang } from "./landing-copy";

type ChatStatus = "bot" | "human" | "closed";

interface ChatMsg {
  id: string;
  sender: "visitor" | "bot" | "agent";
  body: string;
  at: string;
}

interface ChatSessionResp {
  id: string;
  token: string;
  status: ChatStatus;
  total: number;
  messages: ChatMsg[];
}

interface ChatPollResp {
  status: ChatStatus;
  total: number;
  messages: ChatMsg[];
}

/** Clé localStorage : { id, token } — la conversation survit au rechargement. */
const STORAGE_KEY = "mikcloud-chat";
/** Cadence du polling visiteur (panneau ouvert). */
const POLL_MS = 4000;

function timeOf(at: string): string {
  try {
    return new Date(at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  } catch {
    return "";
  }
}

export function LandingChatWidget() {
  const lang = useHotspotStore((s) => s.lang) as Lang;
  const copy = landingCopy[lang].chat;

  /* N°128 — la langue suit le visiteur : chaque POST au backend emporte la
     langue courante de l'interface (ref → callbacks stables, pas de
     re-création au changement de langue). */
  const langRef = useRef<Lang>(lang);
  useEffect(() => {
    langRef.current = lang;
  }, [lang]);

  const [mounted, setMounted] = useState(false);
  const [open, setOpen] = useState(false);
  const [messages, setMessages] = useState<ChatMsg[]>([]);
  const [status, setStatus] = useState<ChatStatus>("bot");
  const [session, setSession] = useState<{ id: string; token: string } | null>(null);
  const [input, setInput] = useState("");
  const [pending, setPending] = useState<string | null>(null);
  const [sendError, setSendError] = useState(false);
  const [loading, setLoading] = useState(false);

  const sessionRef = useRef<{ id: string; token: string } | null>(null);
  const messagesRef = useRef<ChatMsg[]>([]);
  const busyRef = useRef(false);
  const bodyRef = useRef<HTMLDivElement>(null);

  useEffect(() => setMounted(true), []);
  useEffect(() => {
    messagesRef.current = messages;
  }, [messages]);

  /* Auto-scroll en bas à chaque nouveau message / ouverture. */
  useEffect(() => {
    const el = bodyRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages, pending, open]);

  /* ensureSession — restaure la conversation (localStorage) ou en ouvre
     une nouvelle ; garde module contre les doubles créations. */
  const ensureSession = useCallback(async (): Promise<void> => {
    if (sessionRef.current || busyRef.current) return;
    busyRef.current = true;
    setLoading(true);
    try {
      let saved: { id: string; token: string } | null = null;
      try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (raw) saved = JSON.parse(raw) as { id: string; token: string };
      } catch {
        /* stockage illisible → nouvelle conversation */
      }
      if (saved?.token) {
        try {
          const r = await apiAnon<ChatPollResp>("/api/chat/messages", {
            params: { token: saved.token, offset: "0" },
          });
          sessionRef.current = saved;
          setSession(saved);
          setMessages(r.messages);
          setStatus(r.status);
          return;
        } catch {
          /* conversation purgée ou token invalide → nouvelle */
        }
      }
      const r = await apiAnon<ChatSessionResp>("/api/chat/session", {
        method: "POST",
        body: { lang },
      });
      const s = { id: r.id, token: r.token };
      localStorage.setItem(STORAGE_KEY, JSON.stringify(s));
      sessionRef.current = s;
      setSession(s);
      setMessages(r.messages);
      setStatus(r.status);
    } catch {
      setSendError(true);
    } finally {
      busyRef.current = false;
      setLoading(false);
    }
  }, [lang]);

  /* Polling (panneau ouvert) : nouvelles réponses du support + statut. */
  useEffect(() => {
    if (!open || !session) return;
    let alive = true;
    const poll = async () => {
      try {
        const r = await apiAnon<ChatPollResp>("/api/chat/messages", {
          params: { token: session.token, offset: String(messagesRef.current.length) },
        });
        if (!alive) return;
        if (r.messages.length > 0) setMessages((prev) => [...prev, ...r.messages]);
        setStatus(r.status);
      } catch {
        /* silencieux : le prochain tick réessaie */
      }
    };
    void poll();
    const id = setInterval(poll, POLL_MS);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, [open, session]);

  /* Envoi d'un message (saisie ou suggestion). */
  const sendBody = useCallback(async (raw: string) => {
    const body = raw.trim();
    if (!body || busyRef.current) return;
    const tok = sessionRef.current?.token;
    if (!tok) return;
    busyRef.current = true;
    setInput("");
    setPending(body);
    setSendError(false);
    try {
      const r = await apiAnon<ChatPollResp>("/api/chat/message", {
        method: "POST",
        body: { token: tok, body, offset: String(messagesRef.current.length), lang: langRef.current },
      });
      setMessages((prev) => [...prev, ...r.messages]);
      setStatus(r.status);
      setPending(null);
    } catch {
      setPending(null);
      setInput(body);
      setSendError(true);
    } finally {
      busyRef.current = false;
    }
  }, []);

  /* Transmission à un humain. */
  const askHuman = useCallback(async () => {
    const tok = sessionRef.current?.token;
    if (!tok || busyRef.current) return;
    busyRef.current = true;
    try {
      const r = await apiAnon<ChatPollResp>("/api/chat/handoff", {
        method: "POST",
        body: { token: tok, offset: String(messagesRef.current.length), lang: langRef.current },
      });
      setMessages((prev) => [...prev, ...r.messages]);
      setStatus(r.status);
    } catch {
      setSendError(true);
    } finally {
      busyRef.current = false;
    }
  }, []);

  /* Nouvelle conversation (après clôture). */
  const newConversation = useCallback(async () => {
    if (busyRef.current) return;
    busyRef.current = true;
    try {
      const r = await apiAnon<ChatSessionResp>("/api/chat/session", {
        method: "POST",
        body: { lang },
      });
      const s = { id: r.id, token: r.token };
      localStorage.setItem(STORAGE_KEY, JSON.stringify(s));
      sessionRef.current = s;
      setSession(s);
      setMessages(r.messages);
      setStatus(r.status);
      setSendError(false);
    } catch {
      setSendError(true);
    } finally {
      busyRef.current = false;
    }
  }, [lang]);

  const toggle = () => {
    const next = !open;
    setOpen(next);
    if (next) void ensureSession();
  };

  if (!mounted) return null;

  const statusLabel =
    status === "human" ? copy.statusHuman : status === "closed" ? copy.statusClosed : copy.statusBot;

  const panel = (
    <div
      className="mkl-chat-panel"
      role="dialog"
      aria-modal="false"
      aria-label={copy.title}
    >
      <header className="mkl-chat-head">
        <Image
          src="/logo.png"
          alt=""
          width={80}
          height={80}
          className="mkl-chat-head-logo"
        />
        <div>
          <b>{copy.title}</b>
          <span>{statusLabel}</span>
        </div>
        <button className="mkl-chat-close" onClick={() => setOpen(false)} aria-label={copy.closeLabel}>
          <X className="size-4" />
        </button>
      </header>

      {status === "human" ? <div className="mkl-chat-note">{copy.handoffNote}</div> : null}
      {status === "closed" ? <div className="mkl-chat-note">{copy.closedNote}</div> : null}

      <div className="mkl-chat-body" ref={bodyRef} role="log" aria-live="polite">
        {loading && messages.length === 0 ? (
          <div className="mkl-chat-loading" aria-hidden="true">
            <i /><i /><i />
          </div>
        ) : null}
        {messages.map((m) =>
          m.sender === "visitor" ? (
            <div key={m.id} className="mkl-msg mkl-msg-visitor">
              <p>{m.body}</p>
              <span className="mkl-msg-meta">{timeOf(m.at)}</span>
            </div>
          ) : (
            <div key={m.id} className={`mkl-msg mkl-msg-${m.sender}`}>
              {m.sender === "agent" ? <span className="mkl-msg-badge">{copy.agentBadge}</span> : null}
              <p>{m.body}</p>
              <span className="mkl-msg-meta">{timeOf(m.at)}</span>
            </div>
          ),
        )}
        {pending !== null ? (
          <div className="mkl-msg mkl-msg-visitor mkl-msg-pending">
            <p>{pending}</p>
          </div>
        ) : null}
      </div>

      {status === "bot" ? (
        <div className="mkl-chat-chips">
          {copy.suggestions.map((s) => (
            <button
              key={s}
              type="button"
              className="mkl-chat-chip"
              onClick={() => void sendBody(s)}
            >
              {s}
            </button>
          ))}
        </div>
      ) : null}

      {sendError ? <div className="mkl-chat-err">{copy.sendError}</div> : null}

      {status === "closed" ? (
        <div className="mkl-chat-actions">
          <button type="button" className="mkl-chat-human" onClick={() => void newConversation()}>
            {copy.newConv}
          </button>
        </div>
      ) : (
        <form
          className="mkl-chat-inputbar"
          onSubmit={(e) => {
            e.preventDefault();
            void sendBody(input);
          }}
        >
          <input
            className="mkl-chat-input"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder={copy.placeholder}
            aria-label={copy.inputLabel}
            maxLength={1000}
            disabled={pending !== null}
            autoComplete="off"
          />
          <button
            type="submit"
            className="mkl-chat-send"
            disabled={!input.trim() || pending !== null}
            aria-label={copy.send}
          >
            <Send className="size-4" />
          </button>
        </form>
      )}

      {status === "bot" ? (
        <div className="mkl-chat-actions">
          <button type="button" className="mkl-chat-human" onClick={() => void askHuman()}>
            <UserRound className="size-4" /> {copy.humanBtn}
          </button>
        </div>
      ) : null}
    </div>
  );

  return (
    <>
      <button
        className="mkl-chat-fab"
        onClick={toggle}
        aria-label={open ? copy.closeLabel : copy.openLabel}
        aria-expanded={open}
      >
        {open ? <X className="size-6" /> : <MessageCircle className="size-6" />}
        {status === "human" && !open ? <span className="mkl-chat-fab-dot" aria-hidden="true" /> : null}
      </button>
      {open ? panel : null}
    </>
  );
}
