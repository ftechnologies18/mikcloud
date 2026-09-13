"use client";

// Onglet F7 — IP bindings : liste (bypassed/blocked), bascule, suppression
// avec confirmation, ajout via dialog (MAC validée, IP, commentaire).
// Transfert pur depuis router-tools.tsx.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Network, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from "@/components/ui/table";
import { EmptyState } from "@/components/hotspot/empty-state";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { IPBinding, IPBindingType, RouterDevice } from "@/lib/hotspot/types";
import { MAC_RE, ToolError, ToolSkeleton, UnsupportedState } from "./shared";

// ─── F7 — IP bindings ───

const BINDING_TYPE_BADGES: Record<IPBindingType, { labelKey: string; className: string }> = {
  bypassed: { labelKey: "tools.bindings.bypassed", className: "border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400" },
  blocked: { labelKey: "tools.bindings.blocked", className: "border-destructive/25 bg-destructive/10 text-destructive" },
};

function AddBindingDialog({ router, onClose }: { router: RouterDevice; onClose: () => void }) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const [mac, setMac] = useState("");
  const [address, setAddress] = useState("");
  const [comment, setComment] = useState("");
  const [type, setType] = useState<IPBindingType>("bypassed");

  const macOk = MAC_RE.test(mac.trim());
  const valid = macOk;

  const createMutation = useMutation({
    mutationFn: () =>
      api<IPBinding & { queued?: boolean }>(`/api/routers/${router.id}/ipbindings`, {
        method: "POST",
        body: {
          mac: mac.trim(),
          address: address.trim() || undefined,
          comment: comment.trim(),
          type,
        },
      }),
    onSuccess: (res) => {
      toast.success(
        res.queued ? t("tools.bindings.queuedCreate") : t("tools.bindings.addedToast"),
        {
          description: `${mac.trim()} · ${t(BINDING_TYPE_BADGES[type].labelKey).toLowerCase()}`,
        },
      );
      onClose();
      void queryClient.invalidateQueries({ queryKey: ["/api/routers", router.id, "ipbindings"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("tools.bindings.addTitle")}</DialogTitle>
          <DialogDescription>
            {tf("tools.bindings.addDesc", { name: router.name })}
          </DialogDescription>
        </DialogHeader>

        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (!valid || createMutation.isPending) return;
            createMutation.mutate();
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="binding-mac">{t("tools.bindings.mac")}</Label>
            <Input
              id="binding-mac"
              className="font-mono"
              placeholder="AA:BB:CC:DD:EE:FF"
              value={mac}
              onChange={(e) => setMac(e.target.value)}
              aria-invalid={mac.length > 0 && !macOk}
              disabled={createMutation.isPending}
              autoFocus
            />
            {mac.length > 0 && !macOk && (
              <p className="text-xs text-destructive" role="alert">
                {t("tools.bindings.macFormat")}
              </p>
            )}
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="binding-address">{t("tools.bindings.address")}</Label>
              <Input
                id="binding-address"
                className="font-mono"
                placeholder="192.168.88.10"
                value={address}
                onChange={(e) => setAddress(e.target.value)}
                disabled={createMutation.isPending}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="binding-type">{t("tools.bindings.type")}</Label>
              <Select value={type} onValueChange={(v) => setType(v as IPBindingType)} disabled={createMutation.isPending}>
                <SelectTrigger id="binding-type" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="bypassed">{t("tools.bindings.typeBypassed")}</SelectItem>
                  <SelectItem value="blocked">{t("tools.bindings.typeBlocked")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="binding-comment">{t("tools.bindings.comment")}</Label>
            <Input
              id="binding-comment"
              placeholder={t("tools.bindings.commentPlaceholder")}
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              disabled={createMutation.isPending}
            />
          </div>

          {createMutation.isError && (
            <p className="text-sm text-destructive" role="alert">
              {createMutation.error.message}
            </p>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose} disabled={createMutation.isPending}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!valid || createMutation.isPending}>
              {createMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("common.add")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function IpBindingsTab({ router }: { router: RouterDevice }) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const [adding, setAdding] = useState(false);
  const [deleting, setDeleting] = useState<IPBinding | null>(null);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["/api/routers", router.id, "ipbindings"],
    queryFn: () => api<IPBinding[]>(`/api/routers/${router.id}/ipbindings`),
    enabled: router.mode !== "real",
  });

  const bindings = data ?? [];

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["/api/routers", router.id, "ipbindings"] });

  const toggleMutation = useMutation({
    mutationFn: (binding: IPBinding) =>
      api<IPBinding>(`/api/ipbindings/${binding.id}`, { method: "PUT", body: { disabled: !binding.disabled } }),
    onSuccess: (updated) => {
      toast.success(updated.disabled ? t("tools.bindings.disabledToast") : t("tools.bindings.enabledToast"));
      void invalidate();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (binding: IPBinding) => api<{ ok: boolean }>(`/api/ipbindings/${binding.id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success(t("tools.bindings.deletedToast"));
      setDeleting(null);
      void invalidate();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  if (router.mode === "real") {
    return <UnsupportedState />;
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold">{t("tools.bindings.title")}</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">{t("tools.bindings.desc")}</p>
        </div>
        <Button size="sm" className="h-9" onClick={() => setAdding(true)}>
          <Plus className="size-4" />
          {t("common.add")}
        </Button>
      </div>

      {isLoading ? (
        <ToolSkeleton rows={4} />
      ) : isError ? (
        <ToolError error={error} onRetry={() => void refetch()} />
      ) : bindings.length === 0 ? (
        <EmptyState
          icon={Network}
          title={t("tools.bindings.empty")}
          description={t("tools.bindings.emptyDesc")}
          action={
            <Button variant="outline" onClick={() => setAdding(true)}>
              <Plus className="size-4" />
              {t("tools.bindings.addBinding")}
            </Button>
          }
        />
      ) : (
        <div className="max-h-80 overflow-y-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className="pl-4 text-muted-foreground">{t("common.mac")}</TableHead>
                <TableHead className="text-muted-foreground">{t("tools.bindings.addressCol")}</TableHead>
                <TableHead className="text-muted-foreground">{t("common.type")}</TableHead>
                <TableHead className="hidden text-muted-foreground md:table-cell">{t("tools.bindings.commentCol")}</TableHead>
                <TableHead className="text-muted-foreground">{t("common.status")}</TableHead>
                <TableHead className="pr-4 text-right text-muted-foreground">{t("common.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {bindings.map((binding) => (
                <TableRow key={binding.id}>
                  <TableCell className="pl-4 font-mono text-[13px] font-medium">{binding.mac}</TableCell>
                  <TableCell className="font-mono text-[13px] text-muted-foreground">{binding.address || "—"}</TableCell>
                  <TableCell>
                    <Badge variant="outline" className={BINDING_TYPE_BADGES[binding.type]?.className ?? ""}>
                      {BINDING_TYPE_BADGES[binding.type] ? t(BINDING_TYPE_BADGES[binding.type].labelKey) : binding.type}
                    </Badge>
                  </TableCell>
                  <TableCell className="hidden max-w-40 truncate text-muted-foreground md:table-cell" title={binding.comment}>
                    {binding.comment || "—"}
                  </TableCell>
                  <TableCell>
                    {binding.disabled ? (
                      <Badge variant="outline" className="border-border bg-muted text-muted-foreground">{t("tools.bindings.inactive")}</Badge>
                    ) : (
                      <Badge variant="outline" className="border-primary/25 bg-primary/10 text-primary">{t("tools.bindings.active")}</Badge>
                    )}
                  </TableCell>
                  <TableCell className="pr-4">
                    <div className="flex items-center justify-end gap-2">
                      <Switch
                        checked={!binding.disabled}
                        disabled={toggleMutation.isPending && toggleMutation.variables?.id === binding.id}
                        onCheckedChange={() => toggleMutation.mutate(binding)}
                        aria-label={tf("tools.bindings.toggleAria", {
                          action: binding.disabled ? t("common.activate") : t("common.deactivate"),
                          mac: binding.mac,
                        })}
                      />
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-8 text-muted-foreground hover:text-destructive"
                        onClick={() => setDeleting(binding)}
                        aria-label={tf("tools.bindings.deleteAria", { mac: binding.mac })}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {adding && <AddBindingDialog router={router} onClose={() => setAdding(false)} />}

      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("tools.bindings.deleteTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {deleting
                ? `${tf("tools.bindings.deleteDesc", { mac: deleting.mac, router: router.name })}${
                    deleting.type === "bypassed" ? t("tools.bindings.deleteExtra") : ""
                  }.`
                : ""}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={deleteMutation.isPending}
              onClick={(e) => {
                e.preventDefault();
                if (deleting) deleteMutation.mutate(deleting);
              }}
            >
              {deleteMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
