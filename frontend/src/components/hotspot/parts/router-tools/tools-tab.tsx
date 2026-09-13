"use client";

// Onglet F9 — DHCP (baux), Hôtes hotspot, Cookies, Journal routeur :
// 4 sections enveloppe {queued, data, updatedAt} via ToolSection.
// Transfert pur depuis router-tools.tsx.

import type { ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Cookie,
  FileText,
  RefreshCw,
  Router as RouterIcon,
  TriangleAlert,
  Users
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { EmptyState } from "@/components/hotspot/empty-state";
import { cn } from "@/lib/utils";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatDuration, timeAgo } from "@/lib/hotspot/format";
import type {
  DhcpLeaseRow,
  HotspotCookieRow,
  HotspotHostRow,
  RouterDevice,
  RouterLogRow
} from "@/lib/hotspot/types";
import { QueuedBanner, ToolError, ToolSkeleton, UnsupportedState, fetchToolEnvelope } from "./shared";

// ─── F9 — DHCP · Hôtes · Cookies · Journal ───

function DhcpTable({ rows }: { rows: DhcpLeaseRow[] }) {
  const { t } = useI18n();
  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="pl-4 text-muted-foreground">{t("common.ip")}</TableHead>
          <TableHead className="text-muted-foreground">{t("common.mac")}</TableHead>
          <TableHead className="text-muted-foreground">{t("tools.dhcp.host")}</TableHead>
          <TableHead className="hidden text-muted-foreground sm:table-cell">{t("tools.dhcp.expires")}</TableHead>
          <TableHead className="pr-4 text-muted-foreground">{t("common.status")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row, i) => (
          <TableRow key={`${row.mac}-${row.ip}-${i}`}>
            <TableCell className="pl-4 font-mono text-[13px] font-medium">{row.ip || "—"}</TableCell>
            <TableCell className="font-mono text-[13px] text-muted-foreground">{row.mac || "—"}</TableCell>
            <TableCell className="max-w-32 truncate" title={row.host}>
              {row.host || "—"}
            </TableCell>
            <TableCell className="hidden whitespace-nowrap text-muted-foreground sm:table-cell">{row.expires || "—"}</TableCell>
            <TableCell className="pr-4">
              <Badge
                variant="outline"
                className={
                  row.status?.toLowerCase() === "bound"
                    ? "border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                    : "border-border bg-muted text-muted-foreground"
                }
              >
                {row.status || "—"}
              </Badge>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function HostsTable({ rows }: { rows: HotspotHostRow[] }) {
  const { t } = useI18n();
  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="pl-4 text-muted-foreground">{t("common.mac")}</TableHead>
          <TableHead className="text-muted-foreground">{t("common.ip")}</TableHead>
          <TableHead className="hidden text-muted-foreground md:table-cell">{t("tools.hosts.server")}</TableHead>
          <TableHead className="text-muted-foreground">{t("tools.hosts.uptime")}</TableHead>
          <TableHead className="pr-4 text-muted-foreground">{t("tools.hosts.authorized")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row, i) => (
          <TableRow key={`${row.mac}-${row.ip}-${i}`}>
            <TableCell className="pl-4 font-mono text-[13px] font-medium">{row.mac || "—"}</TableCell>
            <TableCell className="font-mono text-[13px] text-muted-foreground">{row.ip || "—"}</TableCell>
            <TableCell className="hidden max-w-32 truncate text-muted-foreground md:table-cell" title={row.server}>
              {row.server || "—"}
            </TableCell>
            <TableCell className="whitespace-nowrap tabular-nums">{formatDuration(row.uptime)}</TableCell>
            <TableCell className="pr-4">
              {row.authorized ? (
                <Badge variant="outline" className="border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                  {t("tools.hosts.authorized")}
                </Badge>
              ) : (
                <Badge variant="outline" className="border-border bg-muted text-muted-foreground">
                  {t("tools.hosts.unauthorized")}
                </Badge>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function CookiesTable({ rows }: { rows: HotspotCookieRow[] }) {
  const { t } = useI18n();
  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="pl-4 text-muted-foreground">{t("common.user")}</TableHead>
          <TableHead className="text-muted-foreground">{t("common.mac")}</TableHead>
          <TableHead className="pr-4 text-muted-foreground">{t("tools.cookies.expires")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row, i) => (
          <TableRow key={`${row.user}-${row.mac}-${i}`}>
            <TableCell className="pl-4 font-mono text-[13px] font-medium">{row.user || "—"}</TableCell>
            <TableCell className="font-mono text-[13px] text-muted-foreground">{row.mac || "—"}</TableCell>
            <TableCell className="pr-4 whitespace-nowrap text-muted-foreground">{row.expires || "—"}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function LogTable({ rows }: { rows: RouterLogRow[] }) {
  const { t } = useI18n();
  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="pl-4 whitespace-nowrap text-muted-foreground">{t("tools.log.time")}</TableHead>
          <TableHead className="text-muted-foreground">{t("tools.log.topics")}</TableHead>
          <TableHead className="pr-4 text-muted-foreground">{t("tools.log.message")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row, i) => (
          <TableRow key={`${row.time}-${i}`}>
            <TableCell className="pl-4 whitespace-nowrap font-mono text-[13px] tabular-nums text-muted-foreground">
              {row.time || "—"}
            </TableCell>
            <TableCell>
              <div className="flex max-w-32 flex-wrap gap-1">
                {(row.topics ?? "")
                  .split(/[,; ]+/)
                  .filter(Boolean)
                  .map((topic) => (
                    <Badge key={topic} variant="outline" className="border-border bg-muted px-1.5 text-[10px] text-muted-foreground">
                      {topic}
                    </Badge>
                  ))}
              </div>
            </TableCell>
            <TableCell className="pr-4 font-mono text-xs break-all">{row.message || "—"}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function ToolSection<T>({
  router,
  kind,
  title,
  description,
  emptyTitle,
  emptyDescription,
  children,
}: {
  router: RouterDevice;
  kind: "dhcp" | "hosts" | "cookies" | "log";
  title: string;
  description: string;
  emptyTitle: string;
  emptyDescription: string;
  children: (rows: T[]) => ReactNode;
}) {
  const { t, tf, lang } = useI18n();
  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ["/api/routers", router.id, kind],
    queryFn: () => fetchToolEnvelope<T>(`/api/routers/${router.id}/${kind}`),
    enabled: router.mode !== "real",
    // Poll 3 s tant que la commande agent est en file, puis arrêt.
    refetchInterval: (query) => (query.state.data?.queued ? 3_000 : false),
  });

  const rows = data?.data ?? [];

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h3 className="text-sm font-semibold">{title}</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>
        </div>
        <div className="flex items-center gap-2">
          {data?.updatedAt && (
            <span className="hidden text-xs text-muted-foreground sm:inline">
              {tf("tools.updatedAgo", { ago: timeAgo(data.updatedAt, lang) })}
            </span>
          )}
          <Button
            variant="outline"
            size="sm"
            className="h-8"
            onClick={() => void refetch()}
            disabled={isFetching}
            aria-label={tf("tools.refreshAria", { title })}
          >
            <RefreshCw className={cn("size-3.5", isFetching && "animate-spin")} />
            {t("tools.refresh")}
          </Button>
        </div>
      </div>

      {data?.queued && <QueuedBanner />}

      {isLoading ? (
        <ToolSkeleton rows={5} />
      ) : isError ? (
        <ToolError error={error} onRetry={() => void refetch()} />
      ) : rows.length === 0 && !data?.queued ? (
        <EmptyState icon={TriangleAlert} title={emptyTitle} description={emptyDescription} />
      ) : rows.length === 0 ? (
        <ToolSkeleton rows={5} />
      ) : (
        <div className="max-h-80 overflow-y-auto rounded-lg border">{children(rows)}</div>
      )}
    </div>
  );
}

export function ToolsTab({ router }: { router: RouterDevice }) {
  const { t } = useI18n();
  if (router.mode === "real") {
    return <UnsupportedState />;
  }
  return (
    <Tabs defaultValue="dhcp" className="gap-0">
      <div className="overflow-x-auto pb-3">
        <TabsList className="w-full min-w-max sm:w-fit">
          <TabsTrigger value="dhcp">
            <RouterIcon className="size-4" />
            {t("tools.dhcp")}
          </TabsTrigger>
          <TabsTrigger value="hosts">
            <Users className="size-4" />
            {t("tools.hosts")}
          </TabsTrigger>
          <TabsTrigger value="cookies">
            <Cookie className="size-4" />
            {t("tools.cookies")}
          </TabsTrigger>
          <TabsTrigger value="log">
            <FileText className="size-4" />
            {t("tools.log")}
          </TabsTrigger>
        </TabsList>
      </div>

      <TabsContent value="dhcp" className="pt-1">
        <ToolSection<DhcpLeaseRow>
          router={router}
          kind="dhcp"
          title={t("tools.dhcp.title")}
          description={t("tools.dhcp.desc")}
          emptyTitle={t("tools.dhcp.empty")}
          emptyDescription={t("tools.dhcp.emptyDesc")}
        >
          {(rows) => <DhcpTable rows={rows} />}
        </ToolSection>
      </TabsContent>

      <TabsContent value="hosts" className="pt-1">
        <ToolSection<HotspotHostRow>
          router={router}
          kind="hosts"
          title={t("tools.hosts.title")}
          description={t("tools.hosts.desc")}
          emptyTitle={t("tools.hosts.empty")}
          emptyDescription={t("tools.hosts.emptyDesc")}
        >
          {(rows) => <HostsTable rows={rows} />}
        </ToolSection>
      </TabsContent>

      <TabsContent value="cookies" className="pt-1">
        <ToolSection<HotspotCookieRow>
          router={router}
          kind="cookies"
          title={t("tools.cookies.title")}
          description={t("tools.cookies.desc")}
          emptyTitle={t("tools.cookies.empty")}
          emptyDescription={t("tools.cookies.emptyDesc")}
        >
          {(rows) => <CookiesTable rows={rows} />}
        </ToolSection>
      </TabsContent>

      <TabsContent value="log" className="pt-1">
        <ToolSection<RouterLogRow>
          router={router}
          kind="log"
          title={t("tools.log.title")}
          description={t("tools.log.desc")}
          emptyTitle={t("tools.log.empty")}
          emptyDescription={t("tools.log.emptyDesc")}
        >
          {(rows) => <LogTable rows={rows} />}
        </ToolSection>
      </TabsContent>
    </Tabs>
  );
}
