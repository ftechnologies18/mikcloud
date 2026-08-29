"use client";

// Vue Utilisateurs réguliers (kind=regular) — comptes hotspot personnalisés :
// filtres (recherche, profil, statut), table paginée, création / édition / activation / suppression.

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ChevronLeft,
  ChevronRight,
  ClipboardCopy,
  Eye,
  EyeOff,
  Loader2,
  MoreHorizontal,
  Pencil,
  Power,
  Search,
  Trash2,
  UserPlus,
  Users,
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { copyToClipboard } from "@/components/hotspot/parts/uc-clipboard";
import { PasswordCell } from "@/components/hotspot/parts/uc-password-cell";
import { api } from "@/lib/hotspot/api";
import { formatBytes, formatCurrency, formatDate } from "@/lib/hotspot/format";
import type { HotspotUser, PagedUsers, Profile, RouterDevice } from "@/lib/hotspot/types";

const PAGE_SIZE = 10;

const STATUS_OPTIONS = [
  { value: "all", label: "Tous les statuts" },
  { value: "active", label: "Actifs" },
  { value: "disabled", label: "Désactivés" },
  { value: "used", label: "Utilisés" },
  { value: "expired", label: "Expirés" },
];

interface UserForm {
  username: string;
  password: string;
  profileId: string;
  routerId: string;
  comment: string;
}

const EMPTY_FORM: UserForm = { username: "", password: "", profileId: "", routerId: "", comment: "" };

export default function UsersView() {
  const currency = useCurrency();
  const queryClient = useQueryClient();

  // Filtres (recherche avec debounce ~400 ms)
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [profileFilter, setProfileFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState("all");
  const [page, setPage] = useState(1);

  useEffect(() => {
    const timer = setTimeout(() => {
      setSearch(searchInput.trim());
      setPage(1);
    }, 400);
    return () => clearTimeout(timer);
  }, [searchInput]);

  // Révélation des mots de passe (par ligne)
  const [revealed, setRevealed] = useState<Set<string>>(new Set());

  // Dialogues
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<HotspotUser | null>(null);
  const [form, setForm] = useState<UserForm>(EMPTY_FORM);
  const [deleting, setDeleting] = useState<HotspotUser | null>(null);

  const { data: profiles } = useQuery({
    queryKey: ["/api/profiles"],
    queryFn: () => api<Profile[]>("/api/profiles"),
  });

  const { data: routers } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
  });

  const statusParam = statusFilter === "all" ? undefined : statusFilter;
  const profileParam = profileFilter === "all" ? undefined : profileFilter;

  // La borne réelle vient de pagedData : safePage évite une page hors bornes après suppression/filtrage.
  const { data: pagedData, isLoading, isFetching } = useQuery({
    queryKey: ["/api/users", { kind: "regular", search, status: statusParam, profileId: profileParam, page }],
    queryFn: () =>
      api<PagedUsers>("/api/users", {
        params: { kind: "regular", search, status: statusParam, profileId: profileParam, page, pageSize: PAGE_SIZE },
      }),
    placeholderData: (previous) => previous,
  });

  const users = pagedData?.data ?? [];
  const totalCount = pagedData?.total ?? 0;
  const maxPage = Math.max(1, Math.ceil(totalCount / PAGE_SIZE));
  const safePage = Math.min(page, maxPage);
  const rangeStart = totalCount === 0 ? 0 : (safePage - 1) * PAGE_SIZE + 1;
  const rangeEnd = Math.min(safePage * PAGE_SIZE, totalCount);

  function invalidateUsers() {
    void queryClient.invalidateQueries({ queryKey: ["/api/users"] });
    void queryClient.invalidateQueries({ queryKey: ["/api/dashboard"] });
  }

  function toggleReveal(id: string) {
    setRevealed((previous) => {
      const next = new Set(previous);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function openCreate() {
    setEditing(null);
    setForm(EMPTY_FORM);
    setDialogOpen(true);
  }

  function openEdit(user: HotspotUser) {
    setEditing(user);
    setForm({
      username: user.username,
      password: "",
      profileId: user.profileId,
      routerId: user.routerId,
      comment: user.comment || "",
    });
    setDialogOpen(true);
  }

  function closeDialog(open: boolean) {
    setDialogOpen(open);
    if (!open) setEditing(null);
  }

  async function copyCredentials(user: HotspotUser) {
    const ok = await copyToClipboard(`${user.username} / ${user.password}`);
    if (ok) toast.success("Identifiants copiés");
    else toast.error("Copie impossible dans le presse-papiers");
  }

  const saveMutation = useMutation({
    mutationFn: async (payload: { id: string | null; form: UserForm }) => {
      if (payload.id) {
        // PUT : seuls username / password / profileId / comment sont modifiables côté API.
        const body: Record<string, unknown> = {
          profileId: payload.form.profileId,
          comment: payload.form.comment.trim(),
        };
        if (payload.form.username.trim()) body.username = payload.form.username.trim();
        if (payload.form.password) body.password = payload.form.password;
        return api<HotspotUser>(`/api/users/${payload.id}`, { method: "PUT", body });
      }
      return api<HotspotUser>("/api/users", {
        method: "POST",
        body: {
          kind: "regular",
          username: payload.form.username.trim() || undefined,
          password: payload.form.password || undefined,
          profileId: payload.form.profileId,
          routerId: payload.form.routerId,
          comment: payload.form.comment.trim() || undefined,
        },
      });
    },
    onSuccess: (user, variables) => {
      toast.success(variables.id ? `Utilisateur ${user.username} modifié` : `Utilisateur ${user.username} créé`);
      closeDialog(false);
      invalidateUsers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const toggleMutation = useMutation({
    mutationFn: (user: HotspotUser) =>
      api<HotspotUser>(`/api/users/${user.id}/${user.status === "disabled" ? "enable" : "disable"}`, {
        method: "POST",
      }),
    onSuccess: (user) => {
      toast.success(user.status === "disabled" ? `Utilisateur ${user.username} désactivé` : `Utilisateur ${user.username} activé`);
      invalidateUsers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (user: HotspotUser) => api<{ ok: boolean }>(`/api/users/${user.id}`, { method: "DELETE" }),
    onSuccess: (_res, user) => {
      toast.success(`Utilisateur ${user.username} supprimé`);
      setDeleting(null);
      invalidateUsers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const selectedProfile = profiles?.find((p) => p.id === form.profileId);
  const formValid = form.profileId !== "" && (editing !== null || form.routerId !== "");

  function submitUser() {
    if (!formValid || saveMutation.isPending) return;
    saveMutation.mutate({ id: editing?.id ?? null, form });
  }

  const hasFilters = search !== "" || profileFilter !== "all" || statusFilter !== "all";

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title="Utilisateurs"
        description="Comptes hotspot personnalisés (abonnés, staff, illimités)"
        actions={
          <Button className="h-10" onClick={openCreate}>
            <UserPlus className="size-4" />
            Nouvel utilisateur
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
              placeholder="Rechercher un utilisateur…"
              value={searchInput}
              onChange={(event) => setSearchInput(event.target.value)}
              aria-label="Rechercher un utilisateur"
            />
          </div>
          <div className="flex flex-1 flex-wrap gap-3 sm:justify-end">
            <Select
              value={profileFilter}
              onValueChange={(value) => {
                setProfileFilter(value);
                setPage(1);
              }}
            >
              <SelectTrigger className="h-10 w-full sm:w-48" aria-label="Filtrer par profil">
                <SelectValue placeholder="Profil" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Tous les profils</SelectItem>
                {profiles?.map((profile) => (
                  <SelectItem key={profile.id} value={profile.id}>
                    {profile.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={statusFilter}
              onValueChange={(value) => {
                setStatusFilter(value);
                setPage(1);
              }}
            >
              <SelectTrigger className="h-10 w-full sm:w-44" aria-label="Filtrer par statut">
                <SelectValue placeholder="Statut" />
              </SelectTrigger>
              <SelectContent>
                {STATUS_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {/* Table des utilisateurs */}
      <Card className="gap-0 py-0">
        {isLoading ? (
          <LoadingRows rows={8} />
        ) : users.length === 0 ? (
          <EmptyState
            icon={Users}
            title="Aucun utilisateur"
            description={
              hasFilters
                ? "Aucun compte ne correspond à ces filtres."
                : "Créez votre premier compte hotspot personnalisé."
            }
            action={
              !hasFilters && (
                <Button onClick={openCreate}>
                  <UserPlus className="size-4" />
                  Nouvel utilisateur
                </Button>
              )
            }
          />
        ) : (
          <>
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="pl-4 text-muted-foreground sm:pl-6">Utilisateur</TableHead>
                    <TableHead className="text-muted-foreground">Profil</TableHead>
                    <TableHead className="hidden text-muted-foreground md:table-cell">Routeur</TableHead>
                    <TableHead className="text-muted-foreground">Statut</TableHead>
                    <TableHead className="text-muted-foreground">Données</TableHead>
                    <TableHead className="hidden text-muted-foreground sm:table-cell">Expire le</TableHead>
                    <TableHead className="hidden text-muted-foreground lg:table-cell">Créé</TableHead>
                    <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {users.map((user) => (
                    <TableRow key={user.id}>
                      <TableCell className="pl-4 sm:pl-6">
                        <div className="flex flex-col gap-0.5 py-1">
                          <span className="font-mono text-sm font-medium">{user.username}</span>
                          <PasswordCell
                            password={user.password}
                            visible={revealed.has(user.id)}
                            onToggle={() => toggleReveal(user.id)}
                            label={
                              revealed.has(user.id)
                                ? `Masquer le mot de passe de ${user.username}`
                                : `Afficher le mot de passe de ${user.username}`
                            }
                          />
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline" className="max-w-36 truncate">
                          {user.profileName}
                        </Badge>
                      </TableCell>
                      <TableCell className="hidden max-w-40 truncate text-muted-foreground md:table-cell">
                        {user.routerName}
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={user.status} dot />
                      </TableCell>
                      <TableCell className="tabular-nums text-muted-foreground">
                        {formatBytes(user.bytesIn + user.bytesOut)}
                      </TableCell>
                      <TableCell className="hidden tabular-nums text-muted-foreground sm:table-cell">
                        {formatDate(user.expiresAt)}
                      </TableCell>
                      <TableCell className="hidden tabular-nums text-muted-foreground lg:table-cell">
                        {formatDate(user.createdAt)}
                      </TableCell>
                      <TableCell className="pr-4 text-right sm:pr-6">
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-10 text-muted-foreground hover:text-foreground"
                              aria-label={`Actions pour ${user.username}`}
                            >
                              <MoreHorizontal className="size-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end" className="w-56">
                            <DropdownMenuItem className="min-h-10" onClick={() => openEdit(user)}>
                              <Pencil className="size-4" />
                              Modifier
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              className="min-h-10"
                              disabled={toggleMutation.isPending && toggleMutation.variables?.id === user.id}
                              onClick={() => toggleMutation.mutate(user)}
                            >
                              {toggleMutation.isPending && toggleMutation.variables?.id === user.id ? (
                                <Loader2 className="size-4 animate-spin" />
                              ) : (
                                <Power className="size-4" />
                              )}
                              {user.status === "disabled" ? "Activer" : "Désactiver"}
                            </DropdownMenuItem>
                            <DropdownMenuItem className="min-h-10" onClick={() => void copyCredentials(user)}>
                              <ClipboardCopy className="size-4" />
                              Copier identifiants
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              variant="destructive"
                              className="min-h-10"
                              onClick={() => setDeleting(user)}
                            >
                              <Trash2 className="size-4" />
                              Supprimer
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>

            {/* Pagination */}
            <div className="flex flex-wrap items-center justify-between gap-3 border-t px-4 py-3 sm:px-6">
              <p className="text-xs text-muted-foreground">
                {isFetching ? "Actualisation…" : `${rangeStart}–${rangeEnd} sur ${totalCount}`}
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
                  Précédent
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-10"
                  onClick={() => setPage((p) => Math.min(maxPage, p + 1))}
                  disabled={safePage >= maxPage}
                >
                  Suivant
                  <ChevronRight className="size-4" />
                </Button>
              </div>
            </div>
          </>
        )}
      </Card>

      {/* Dialogue création / édition */}
      <Dialog open={dialogOpen} onOpenChange={closeDialog}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{editing ? `Modifier ${editing.username}` : "Nouvel utilisateur"}</DialogTitle>
            <DialogDescription>
              {editing
                ? "Ajustez le compte hotspot. Laissez le mot de passe vide pour le conserver."
                : "Laissez l'identifiant et le mot de passe vides pour les générer automatiquement."}
            </DialogDescription>
          </DialogHeader>

          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              submitUser();
            }}
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="user-username">Utilisateur</Label>
                <Input
                  id="user-username"
                  placeholder="Auto-généré si vide"
                  value={form.username}
                  onChange={(event) => setForm((f) => ({ ...f, username: event.target.value }))}
                  disabled={saveMutation.isPending}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="user-password">Mot de passe</Label>
                <Input
                  id="user-password"
                  type="text"
                  autoComplete="new-password"
                  placeholder={editing ? "Inchangé si vide" : "Auto-généré si vide"}
                  value={form.password}
                  onChange={(event) => setForm((f) => ({ ...f, password: event.target.value }))}
                  disabled={saveMutation.isPending}
                />
              </div>
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="user-profile">Profil</Label>
                <Select
                  value={form.profileId}
                  onValueChange={(value) => setForm((f) => ({ ...f, profileId: value }))}
                  disabled={saveMutation.isPending}
                >
                  <SelectTrigger id="user-profile" className="h-10 w-full">
                    <SelectValue placeholder="Sélectionner un profil" />
                  </SelectTrigger>
                  <SelectContent>
                    {profiles?.map((profile) => (
                      <SelectItem key={profile.id} value={profile.id}>
                        {profile.name} — {formatCurrency(profile.price, currency)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="user-router">Routeur</Label>
                <Select
                  value={form.routerId}
                  onValueChange={(value) => setForm((f) => ({ ...f, routerId: value }))}
                  disabled={saveMutation.isPending || editing !== null}
                >
                  <SelectTrigger id="user-router" className="h-10 w-full">
                    <SelectValue placeholder="Sélectionner un routeur" />
                  </SelectTrigger>
                  <SelectContent>
                    {routers?.map((router) => (
                      <SelectItem key={router.id} value={router.id}>
                        {router.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {editing && <p className="text-xs text-muted-foreground">Le routeur n'est pas modifiable.</p>}
              </div>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="user-comment">Commentaire (optionnel)</Label>
              <Textarea
                id="user-comment"
                placeholder="Ex. Abonnement mensuel — paiement Mobile Money"
                value={form.comment}
                onChange={(event) => setForm((f) => ({ ...f, comment: event.target.value }))}
                disabled={saveMutation.isPending}
              />
            </div>

            {selectedProfile && (
              <p className="rounded-lg border bg-muted/40 px-3 py-2 text-sm">
                Forfait : <span className="font-semibold text-primary">{selectedProfile.name}</span> —{" "}
                {formatCurrency(selectedProfile.price, currency)} · validité {selectedProfile.validityDays} j
              </p>
            )}

            {saveMutation.isError && (
              <p className="text-sm text-destructive" role="alert">
                {saveMutation.error.message}
              </p>
            )}

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => closeDialog(false)} disabled={saveMutation.isPending}>
                Annuler
              </Button>
              <Button type="submit" disabled={!formValid || saveMutation.isPending}>
                {saveMutation.isPending && <Loader2 className="size-4 animate-spin" />}
                {editing ? "Enregistrer" : "Créer l'utilisateur"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Confirmation suppression */}
      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Supprimer {deleting?.username} ?</AlertDialogTitle>
            <AlertDialogDescription>
              Cette action est irréversible. Le compte hotspot sera retiré du routeur.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>Annuler</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={deleteMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                if (deleting) deleteMutation.mutate(deleting);
              }}
            >
              {deleteMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              Supprimer
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
