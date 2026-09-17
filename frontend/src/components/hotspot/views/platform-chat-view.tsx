"use client";

// N°127 — Conversations (console plateforme, super-admin uniquement).
// L'inbox de l'assistant conversationnel de la vitrine : la FAQ du landing
// est devenue un chatbot — les visiteurs posent leurs questions au bot, et
// peuvent demander un humain. Les conversations arrivent ici :
//   • liste (toutes / avec un humain / bot / fermées) avec dernier message,
//     non-lus et activité récente ;
//   • fil complet (visiteur / assistant / support) en lecture + réponse ;
//   • clôture (message de fin côté visiteur, purge 30 jours plus tard).
// Répondre à une conversation bot la fait passer « human » : après une
// intervention humaine, le bot ne reprend jamais la main.
// Poll adaptatif : liste 8 s ; fil ouvert 4 s tant que la conversation est
// vivante (human) — la réponse du visiteur arrive quelques secondes plus tard.

import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Bot,
  CheckCheck,
  CircleOff,
  Loader2,
  MessagesSquare,
  Send,
  UserRound,
  XCircle,
} from "lucide-react";
import { toast } from "sonner";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingCards } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import {
  closeChatConversation,
  fetchChatConversation,
  fetchChatConversations,
  replyChatConversation,
} from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import { timeAgo } from "@/lib/hotspot/format";
import type { ChatConversationRow } from "@/lib/hotspot/types";

type ChatFilter = "all" | "human" | "bot" | "closed";

const FILTERS: ChatFilter[] = ["all", "human", "bot", "closed"];

/** Pastille de statut d'une conversation. */
function statusBadgeClass(status: string): string {
  if (status === "human") return "bg-amber-100 text-amber-800 border-amber-300";
  if (status === "closed") return "bg-muted text-muted-foreground border-border";
  return "bg-emerald-100 text-emerald-800 border-emerald-300";
}

export default function PlatformChatView() {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [filter, setFilter] = useState<ChatFilter>("all");
  const [reply, setReply] = useState("");
  const [confirmClose, setConfirmClose] = useState(false);
  const threadRef = useRef<HTMLDivElement>(null);

  const nf = useMemo(() => new Intl.NumberFormat(localeOf(lang)).format, [lang]);

  /* Liste des conversations (poll 8 s : les transmissions arrivent seules). */
  const listQuery = useQuery({
    queryKey: ["/api/admin/chat/conversations"],
    queryFn: fetchChatConversations,
    refetchInterval: 8_000,
  });

  /* Fil de la conversation sélectionnée (poll 4 s si vivante). */
  const threadQuery = useQuery({
    queryKey: ["/api/admin/chat/conversations", selectedId],
    queryFn: () => fetchChatConversation(selectedId as string),
    enabled: !!selectedId,
    refetchInterval: (query) =>
      query.state.data?.conversation.status === "human" ? 4_000 : false,
  });

  /* Réponse du support. */
  const replyMutation = useMutation({
    mutationFn: () => replyChatConversation(selectedId as string, reply.trim()),
    onSuccess: () => {
      setReply("");
      toast.success(t("platformChat.replySent"));
      void queryClient.invalidateQueries({ queryKey: ["/api/admin/chat/conversations"] });
    },
  });

  /* Clôture. */
  const closeMutation = useMutation({
    mutationFn: () => closeChatConversation(selectedId as string),
    onSuccess: () => {
      setConfirmClose(false);
      toast.success(t("platformChat.closed"));
      void queryClient.invalidateQueries({ queryKey: ["/api/admin/chat/conversations"] });
    },
  });

  /* Auto-scroll du fil en bas (nouveau message / ouverture). */
  useEffect(() => {
    const el = threadRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [threadQuery.data, selectedId]);

  if (listQuery.isLoading) {
    return (
      <div className="space-y-6">
        <PageHeader title={t("platformChat.title")} description={t("platformChat.description")} />
        <LoadingCards cards={4} />
      </div>
    );
  }

  if (listQuery.isError || !listQuery.data) {
    return (
      <Card>
        <EmptyState
          icon={MessagesSquare}
          title={t("platformChat.loadError")}
          description={t("platformChat.loadErrorDesc")}
          action={
            <Button variant="outline" onClick={() => void listQuery.refetch()}>
              {t("common.retry")}
            </Button>
          }
        />
      </Card>
    );
  }

  const { conversations, summary } = listQuery.data;
  const filtered =
    filter === "all" ? conversations : conversations.filter((c) => c.status === filter);
  const thread = threadQuery.data;
  const convStatus = thread?.conversation.status ?? "bot";
  const unreadLabel = (n: number) => tf("platformChat.badge.unread", { n, p: n > 1 ? "s" : "" });

  const sendReply = () => {
    const body = reply.trim();
    if (!body) {
      toast.error(t("platformChat.replyEmpty"));
      return;
    }
    if (replyMutation.isPending) return;
    replyMutation.mutate();
  };

  return (
    <div className="space-y-6">
      <PageHeader title={t("platformChat.title")} description={t("platformChat.description")} />

      {/* Synthèse */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4" aria-live="polite">
        <StatCard
          title={t("platformChat.kpi.total")}
          value={nf(summary.total)}
          sub={t("platformChat.kpi.totalSub")}
          icon={MessagesSquare}
        />
        <StatCard
          title={t("platformChat.kpi.human")}
          value={nf(summary.human)}
          sub={t("platformChat.kpi.humanSub")}
          icon={UserRound}
          valueClassName={summary.human > 0 ? "text-amber-600 dark:text-amber-400" : undefined}
        />
        <StatCard
          title={t("platformChat.kpi.unread")}
          value={nf(summary.unread)}
          sub={t("platformChat.kpi.unreadSub")}
          icon={XCircle}
          valueClassName={summary.unread > 0 ? "text-amber-600 dark:text-amber-400" : undefined}
        />
        <StatCard
          title={t("platformChat.kpi.bot")}
          value={nf(summary.bot)}
          sub={t("platformChat.kpi.botSub")}
          icon={Bot}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,400px)_minmax(0,1fr)]">
        {/* Liste des conversations */}
        <Card className="gap-0 py-0">
          <CardContent className="flex flex-col gap-3 p-4">
            <div
              className="flex flex-wrap gap-1.5"
              role="group"
              aria-label={t("platformChat.filter.all")}
            >
              {FILTERS.map((f) => {
                const on = filter === f;
                return (
                  <Button
                    key={f}
                    size="sm"
                    variant={on ? "default" : "outline"}
                    aria-pressed={on}
                    onClick={() => setFilter(f)}
                  >
                    {t(`platformChat.filter.${f}`)}
                  </Button>
                );
              })}
            </div>
            {filtered.length === 0 ? (
              <div className="py-4">
                <EmptyState
                  icon={MessagesSquare}
                  title={t("platformChat.empty")}
                  description={t("platformChat.emptyDesc")}
                />
              </div>
            ) : (
              <div className="max-h-[32rem] space-y-1.5 overflow-y-auto pr-1" role="list">
                {filtered.map((c) => (
                  <ConversationRow
                    key={c.id}
                    row={c}
                    selected={c.id === selectedId}
                    onSelect={() => {
                      setSelectedId(c.id);
                      setReply("");
                      setConfirmClose(false);
                    }}
                    statusLabel={t(`platformChat.status.${c.status}`)}
                    unreadLabel={c.unread > 0 ? unreadLabel(c.unread) : ""}
                  />
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {/* Fil de la conversation sélectionnée */}
        <Card className="gap-0 py-0">
          {!selectedId || !thread ? (
            <CardContent className="p-6">
              <EmptyState
                icon={MessagesSquare}
                title={t("platformChat.selectPrompt")}
                description={t("platformChat.selectPromptDesc")}
              />
            </CardContent>
          ) : (
            <CardContent className="flex min-h-[28rem] flex-col gap-3 p-4">
              {/* En-tête du fil */}
              <div className="flex flex-wrap items-center gap-2 border-b pb-3">
                <span
                  className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-semibold ${statusBadgeClass(
                    convStatus,
                  )}`}
                >
                  {convStatus === "human" ? (
                    <UserRound className="size-3.5" />
                  ) : convStatus === "closed" ? (
                    <CircleOff className="size-3.5" />
                  ) : (
                    <Bot className="size-3.5" />
                  )}
                  {t(`platformChat.status.${convStatus}`)}
                </span>
                <Badge variant="outline" className="uppercase">
                  {thread.conversation.lang === "en" ? "EN" : "FR"}
                </Badge>
                <span className="text-xs text-muted-foreground">
                  {tf("platformChat.thread.started", {
                    ago: timeAgo(thread.conversation.createdAt, lang),
                  })}
                </span>
                <span className="ml-auto text-xs text-muted-foreground">
                  {tf("platformChat.thread.activity", {
                    ago: timeAgo(thread.conversation.updatedAt, lang),
                  })}
                </span>
              </div>

              {/* Notes de contexte */}
              {convStatus === "bot" ? (
                <p className="rounded-lg bg-emerald-500/10 px-3 py-2 text-xs text-emerald-700 dark:text-emerald-300">
                  {t("platformChat.note.bot")}
                </p>
              ) : null}
              {convStatus === "closed" ? (
                <p className="rounded-lg bg-muted px-3 py-2 text-xs text-muted-foreground">
                  {t("platformChat.note.closed")}
                </p>
              ) : null}

              {/* Le fil */}
              <div
                ref={threadRef}
                className="flex max-h-[26rem] min-h-[14rem] flex-1 flex-col gap-2.5 overflow-y-auto pr-1"
                role="log"
                aria-live="polite"
              >
                {thread.messages.map((m) =>
                  m.sender === "visitor" ? (
                    <div key={m.id} className="flex flex-col items-end">
                      <span className="mb-0.5 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">
                        {t("platformChat.thread.visitor")}
                      </span>
                      <div className="max-w-[85%] rounded-2xl rounded-br-sm bg-primary px-3.5 py-2 text-sm text-primary-foreground">
                        {m.body}
                      </div>
                      <span className="mt-0.5 text-[0.65rem] text-muted-foreground">
                        {timeAgo(m.at, lang)}
                      </span>
                    </div>
                  ) : (
                    <div key={m.id} className="flex flex-col items-start">
                      <span className="mb-0.5 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">
                        {m.sender === "agent"
                          ? t("platformChat.thread.you")
                          : t("platformChat.thread.bot")}
                      </span>
                      <div
                        className={`max-w-[85%] rounded-2xl rounded-bl-sm px-3.5 py-2 text-sm ${
                          m.sender === "agent"
                            ? "border border-emerald-300 bg-emerald-500/10 text-foreground"
                            : "bg-muted text-foreground"
                        }`}
                      >
                        {m.body}
                      </div>
                      <span className="mt-0.5 text-[0.65rem] text-muted-foreground">
                        {timeAgo(m.at, lang)}
                      </span>
                    </div>
                  ),
                )}
                {threadQuery.isFetching && thread.messages.length === 0 ? (
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Loader2 className="size-4 animate-spin" />
                  </div>
                ) : null}
              </div>

              {/* Réponse / clôture */}
              <div className="space-y-2 border-t pt-3">
                <Textarea
                  value={reply}
                  onChange={(e) => setReply(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && !e.shiftKey) {
                      e.preventDefault();
                      sendReply();
                    }
                  }}
                  placeholder={t("platformChat.replyPlaceholder")}
                  rows={2}
                  maxLength={2000}
                  disabled={replyMutation.isPending || convStatus === "closed"}
                  aria-label={t("platformChat.reply")}
                />
                <div className="flex items-center justify-between gap-2">
                  <Button
                    size="sm"
                    onClick={sendReply}
                    disabled={replyMutation.isPending || !reply.trim() || convStatus === "closed"}
                  >
                    {replyMutation.isPending ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : (
                      <Send className="size-4" />
                    )}
                    {t("platformChat.reply")}
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => setConfirmClose(true)}
                    disabled={closeMutation.isPending || convStatus === "closed"}
                  >
                    <CheckCheck className="size-4" />
                    {t("platformChat.close")}
                  </Button>
                </div>
              </div>
            </CardContent>
          )}
        </Card>
      </div>

      {/* Confirmation de clôture */}
      <AlertDialog open={confirmClose} onOpenChange={setConfirmClose}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("platformChat.closeConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("platformChat.closeConfirmDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault();
                closeMutation.mutate();
              }}
            >
              {closeMutation.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : null}
              {t("platformChat.closeConfirmAction")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

/** Ligne de la liste des conversations. */
function ConversationRow({
  row,
  selected,
  onSelect,
  statusLabel,
  unreadLabel,
}: {
  row: ChatConversationRow;
  selected: boolean;
  onSelect: () => void;
  statusLabel: string;
  unreadLabel: string;
}) {
  const { lang } = useI18n();
  return (
    <button
      type="button"
      role="listitem"
      onClick={onSelect}
      aria-current={selected ? "true" : undefined}
      className={`w-full rounded-xl border p-3 text-left transition-colors ${
        selected
          ? "border-primary bg-primary/5"
          : "border-border hover:border-primary/40 hover:bg-muted/50"
      }`}
    >
      <div className="flex items-center gap-2">
        <span
          className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[0.65rem] font-semibold ${statusBadgeClass(
            row.status,
          )}`}
        >
          {row.status === "human" ? (
            <UserRound className="size-3" />
          ) : row.status === "closed" ? (
            <CircleOff className="size-3" />
          ) : (
            <Bot className="size-3" />
          )}
          {statusLabel}
        </span>
        <span className="text-[0.65rem] font-bold uppercase text-muted-foreground">
          {row.lang === "en" ? "EN" : "FR"}
        </span>
        {row.unread > 0 ? (
          <span className="ml-auto inline-flex items-center rounded-full bg-amber-500 px-2 py-0.5 text-[0.65rem] font-bold text-white">
            {unreadLabel}
          </span>
        ) : null}
      </div>
      <p className="mt-1.5 line-clamp-2 text-sm">{row.lastMessage || "—"}</p>
      <p className="mt-1 text-[0.65rem] text-muted-foreground">
        {row.lastSender === "visitor"
          ? "Visiteur"
          : row.lastSender === "agent"
            ? "Support"
            : "Assistant"}{" "}
        · {timeAgo(row.updatedAt, lang)}
      </p>
    </button>
  );
}
