"use client";

// Console plateforme — JOURNAL TRANSVERSE (admin plateforme uniquement).
// Activité de TOUS les comptes SaaS : support, audit, investigation.
// Filtres : compte et type ; historique borné côté serveur (max 300).

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Building2,
  Cog,
  CreditCard,
  Radio,
  Router as RouterIcon,
  ScrollText,
  Store,
  Ticket,
  Users,
  UsersRound,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { EmptyState } from "@/components/hotspot/empty-state";
import { PageHeader } from "@/components/hotspot/page-header";
import { fetchAccounts, fetchPlatformActivity } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatDate, timeAgo } from "@/lib/hotspot/format";

const TYPE_ICONS: Record<string, LucideIcon> = {
  router: RouterIcon,
  user: Users,
  voucher: Ticket,
  reseller: Store,
  session: Radio,
  system: Cog,
  team: UsersRound,
  compte: Building2,
  billing: CreditCard,
};

const TYPE_FILTERS = ["", "system", "compte", "router", "user", "voucher", "reseller", "session", "team"] as const;

export default function PlatformLogsView() {
  const { t, lang } = useI18n();
  const [accountFilter, setAccountFilter] = useState<string>("all");
  const [typeFilter, setTypeFilter] = useState<string>("all");

  const { data: accounts } = useQuery({
    queryKey: ["/api/admin/accounts"],
    queryFn: fetchAccounts,
  });

  const { data, isLoading, isError } = useQuery({
    queryKey: ["/api/admin/activity", accountFilter, typeFilter],
    queryFn: () =>
      fetchPlatformActivity({
        accountId: accountFilter === "all" ? undefined : accountFilter,
        limit: 300,
      }),
    refetchInterval: 30_000,
  });

  const rows = useMemo(() => {
    if (!data) return [];
    if (typeFilter === "all") return data;
    return data.filter((row) => row.type === typeFilter);
  }, [data, typeFilter]);

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader title={t("platformLogs.title")} description={t("platformLogs.description")} />

      {/* Filtres */}
      <div className="flex flex-wrap items-center gap-2">
        <Select value={accountFilter} onValueChange={setAccountFilter}>
          <SelectTrigger className="h-10 w-full min-w-44 sm:w-56" aria-label={t("platformLogs.filterAccount")}>
            <Building2 className="size-4 text-muted-foreground" />
            <SelectValue placeholder={t("platformLogs.filterAccount")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("platformLogs.allAccounts")}</SelectItem>
            {(accounts ?? []).map((a) => (
              <SelectItem key={a.id} value={a.id}>
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={typeFilter} onValueChange={setTypeFilter}>
          <SelectTrigger className="h-10 w-full min-w-40 sm:w-48" aria-label={t("platformLogs.filterType")}>
            <ScrollText className="size-4 text-muted-foreground" />
            <SelectValue placeholder={t("platformLogs.filterType")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("platformLogs.allTypes")}</SelectItem>
            {TYPE_FILTERS.filter((v) => v !== "").map((v) => (
              <SelectItem key={v} value={v}>
                {t(`platformLogs.type.${v}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {isLoading ? (
        <Card className="gap-0 py-0">
          <div className="space-y-3 p-4 sm:p-6">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-12 rounded-lg" />
            ))}
          </div>
        </Card>
      ) : isError || !rows ? (
        <Card className="gap-0 py-0">
          <EmptyState icon={ScrollText} title={t("platformLogs.loadError")} />
        </Card>
      ) : rows.length === 0 ? (
        <Card className="gap-0 py-0">
          <EmptyState icon={ScrollText} title={t("platformLogs.empty")} description={t("platformLogs.emptyDesc")} />
        </Card>
      ) : (
        <Card className="gap-0 py-0">
          <CardContent className="p-0">
            <ul className="max-h-[32rem] divide-y overflow-y-auto" role="list" aria-label={t("platformLogs.title")}>
              {rows.map((row) => {
                const Icon = TYPE_ICONS[row.type] ?? Cog;
                return (
                  <li key={row.id} className="flex items-start gap-3 px-4 py-3 transition-colors hover:bg-accent/40 sm:px-6">
                    <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                      <Icon className="size-4" />
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="text-sm leading-snug">{row.message}</p>
                      <p className="mt-0.5 flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
                        <span className="inline-flex items-center gap-1 font-medium text-foreground/80">
                          <Building2 className="size-3" />
                          {row.accountName || row.accountId}
                        </span>
                        {row.actorName && <span>· {row.actorName}</span>}
                        <span>· {formatDate(row.at, lang)}</span>
                        <span>· {timeAgo(row.at, lang)}</span>
                      </p>
                    </div>
                    <span className="shrink-0 rounded-md bg-muted px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
                      {t(`platformLogs.type.${row.type}`, row.type)}
                    </span>
                  </li>
                );
              })}
            </ul>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
