"use client";

// Vue Journal utilisateurs (F3) — connexions, déconnexions, expirations et kicks
// capturés par le moteur cloud (Tick / agent). Filtres recherche + routeur +
// action, table paginée rafraîchie toutes les 10 s, export CSV.

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Download, Loader2, ScrollText, Search } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { api, apiDownload } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatDateTime } from "@/lib/hotspot/format";
import type { PagedUserLogs, RouterDevice, UserLogAction } from "@/lib/hotspot/types";

const PAGE_SIZE = 20;

const ACTION_OPTIONS = [
  { value: "all", labelKey: "logs.allActions" },
  { value: "login", labelKey: "logs.logins" },
  { value: "logout", labelKey: "logs.logouts" },
  { value: "expire", labelKey: "logs.expirations" },
  { value: "kick", labelKey: "logs.kicks" },
];

/** Clés i18n des badges par action : login=vert, logout=neutre, expire=orange, kick=rouge. */
const ACTION_BADGES: Record<UserLogAction, { labelKey: string; className: string }> = {
  login: { labelKey: "logs.login", className: "border-primary/25 bg-primary/10 text-primary" },
  logout: { labelKey: "logs.logout", className: "border-border bg-muted text-muted-foreground" },
  expire: { labelKey: "logs.expire", className: "border-orange-500/25 bg-orange-500/10 text-orange-500" },
  kick: { labelKey: "logs.kick", className: "border-destructive/25 bg-destructive/10 text-destructive" },
};

function ActionBadge({ action }: { action: UserLogAction }) {
  const { t } = useI18n();
  const badge = ACTION_BADGES[action] ?? ACTION_BADGES.logout;
  return (
    <Badge variant="outline" className={badge.className}>
      {t(badge.labelKey)}
    </Badge>
  );
}

export default function LogsView() {
  const { t, tf, lang } = useI18n();
  // Filtres (recherche avec debounce ~400 ms)
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [routerFilter, setRouterFilter] = useState("all");
  const [actionFilter, setActionFilter] = useState("all");
  const [page, setPage] = useState(1);
  const [exporting, setExporting] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => {
      setSearch(searchInput.trim());
      setPage(1);
    }, 400);
    return () => clearTimeout(timer);
  }, [searchInput]);

  const { data: routers } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
  });

  const routerParam = routerFilter === "all" ? undefined : routerFilter;
  const actionParam = actionFilter === "all" ? undefined : actionFilter;

  const { data: pagedData, isLoading, isFetching } = useQuery({
    queryKey: ["/api/user-logs", { search, routerId: routerParam, action: actionParam, page }],
    queryFn: () =>
      api<PagedUserLogs>("/api/user-logs", {
        params: { search, routerId: routerParam, action: actionParam, page, pageSize: PAGE_SIZE },
      }),
    refetchInterval: 10_000,
    placeholderData: (previous) => previous,
  });

  const logs = pagedData?.data ?? [];
  const totalCount = pagedData?.total ?? 0;
  const maxPage = Math.max(1, Math.ceil(totalCount / PAGE_SIZE));
  const safePage = Math.min(page, maxPage);
  const rangeStart = totalCount === 0 ? 0 : (safePage - 1) * PAGE_SIZE + 1;
  const rangeEnd = Math.min(safePage * PAGE_SIZE, totalCount);

  async function handleExportCsv() {
    try {
      setExporting(true);
      const date = new Date().toISOString().slice(0, 10);
      await apiDownload(
        "/api/user-logs/export",
        `${lang === "fr" ? "journal-utilisateurs" : "user-log"}-${date}.csv`,
        {
          search,
          routerId: routerParam,
          action: actionParam,
        },
      );
      toast.success(t("common.exportDownloaded"));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("common.exportFailed"));
    } finally {
      setExporting(false);
    }
  }

  const hasFilters = search !== "" || routerFilter !== "all" || actionFilter !== "all";

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("logs.title")}
        description={t("logs.description")}
        actions={
          <Button variant="outline" className="h-10" onClick={() => void handleExportCsv()} disabled={exporting}>
            {exporting ? <Loader2 className="size-4 animate-spin" /> : <Download className="size-4" />}
            {t("common.exportCsv")}
          </Button>
        }
      />

      {/* Barre de filtres */}
      <Card className="gap-0 py-0">
        <CardContent className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center">
          <div className="relative w-full sm:max-w-xs sm:flex-1">
            <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
            <Input
              className="h-10 pl-9"
              placeholder={t("logs.searchPlaceholder")}
              value={searchInput}
              onChange={(event) => setSearchInput(event.target.value)}
              aria-label={t("logs.searchLabel")}
            />
          </div>
          <div className="flex flex-1 flex-wrap gap-3 sm:justify-end">
            <Select
              value={routerFilter}
              onValueChange={(value) => {
                setRouterFilter(value);
                setPage(1);
              }}
            >
              <SelectTrigger className="h-10 w-full sm:w-48" aria-label={t("common.filterByRouter")}>
                <SelectValue placeholder={t("common.router")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("common.allRouters")}</SelectItem>
                {routers?.map((router) => (
                  <SelectItem key={router.id} value={router.id}>
                    {router.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={actionFilter}
              onValueChange={(value) => {
                setActionFilter(value);
                setPage(1);
              }}
            >
              <SelectTrigger className="h-10 w-full sm:w-48" aria-label={t("logs.filterByAction")}>
                <SelectValue placeholder={t("logs.action")} />
              </SelectTrigger>
              <SelectContent>
                {ACTION_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {t(option.labelKey)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {/* Table du journal (hauteur limitée + défilement) */}
      <Card className="gap-0 py-0">
        {isLoading ? (
          <LoadingRows rows={8} />
        ) : logs.length === 0 ? (
          <EmptyState
            icon={ScrollText}
            title={t("logs.empty")}
            description={hasFilters ? t("logs.emptyFiltered") : t("logs.emptyDesc")}
          />
        ) : (
          <>
            <div className="max-h-[65vh] overflow-auto">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("common.date")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("common.user")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("logs.action")}</TableHead>
                    <TableHead className="hidden text-muted-foreground md:table-cell">{t("common.router")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("common.ip")}</TableHead>
                    <TableHead className="hidden pr-4 text-muted-foreground lg:table-cell sm:pr-6">{t("common.mac")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {logs.map((log) => (
                    <TableRow key={log.id}>
                      <TableCell className="whitespace-nowrap pl-4 tabular-nums text-muted-foreground sm:pl-6">
                        {formatDateTime(log.at, lang)}
                      </TableCell>
                      <TableCell className="font-mono text-sm font-medium">{log.username}</TableCell>
                      <TableCell>
                        <ActionBadge action={log.action} />
                      </TableCell>
                      <TableCell className="hidden max-w-40 truncate text-muted-foreground md:table-cell">
                        {log.routerName}
                      </TableCell>
                      <TableCell className="font-mono text-[13px] text-muted-foreground">
                        {log.ip || "—"}
                      </TableCell>
                      <TableCell className="hidden pr-4 font-mono text-[13px] text-muted-foreground lg:table-cell sm:pr-6">
                        {log.mac || "—"}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>

            {/* Pagination */}
            <div className="flex flex-wrap items-center justify-between gap-3 border-t px-4 py-3 sm:px-6">
              <p className="text-xs text-muted-foreground">
                {isFetching
                  ? t("common.refreshing")
                  : tf("common.range", { start: rangeStart, end: rangeEnd, total: totalCount })}
              </p>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="h-10"
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={safePage <= 1}
                >
                  <ChevronLeft className="size-4" />
                  {t("common.previous")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-10"
                  onClick={() => setPage((p) => Math.min(maxPage, p + 1))}
                  disabled={safePage >= maxPage}
                >
                  {t("common.next")}
                  <ChevronRight className="size-4" />
                </Button>
              </div>
            </div>
          </>
        )}
      </Card>
    </div>
  );
}
