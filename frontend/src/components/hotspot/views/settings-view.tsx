"use client";

// Vue Paramètres — organisation, abonnement, connexion routeur réel, zone sensible.

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Building2, Database, Eye, EyeOff, KeyRound, Router as RouterIcon, TriangleAlert } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/hotspot/api";
import type { AppSettings } from "@/lib/hotspot/types";
import { useHotspotStore } from "@/lib/hotspot/store";
import { PageHeader } from "@/components/hotspot/page-header";
import { SETTINGS_QUERY_KEY, useSettings } from "@/components/hotspot/parts/sd-currency";
import { SubscriptionCard } from "@/components/hotspot/parts/sa-subscription-card";
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
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";

const CURRENCIES = ["FCFA", "EUR", "USD", "MAD", "XOF", "GBP", "CDF", "GNF"];
const TIMEZONES = ["UTC", "Africa/Abidjan", "Africa/Dakar", "Africa/Casablanca", "Europe/Paris", "Europe/Brussels"];

const MIKROTIK_STEPS = [
  {
    title: "Activez le service API",
    text: "Ouvrez Winbox → IP → Services, puis activez « api » (port 8728).",
  },
  {
    title: "Créez un utilisateur API",
    text: "Dans Système → Utilisateurs → Groupes, créez un compte API en lecture/écriture (groupe full).",
  },
  {
    title: "Ajoutez le routeur",
    text: "Ajoutez le routeur dans la vue Routeurs avec son IP, le port 8728, ses identifiants API et le mode « Réel ».",
  },
];

interface SettingsForm {
  name: string;
  currency: string;
  timezone: string;
  waveLink?: string;
}

// Réponse de POST /api/admin/reload — résumé de l'état réimporté.
interface ReloadStats {
  ok: boolean;
  accounts: number;
  users: number;
  hotspotUsers: number;
  routers: number;
  sessions: number;
}

export default function SettingsView() {
  const queryClient = useQueryClient();
  const { data, isLoading } = useSettings();
  const [resetOpen, setResetOpen] = useState(false);
  const user = useHotspotStore((s) => s.user);
  // La réinitialisation des données devient admin-only côté serveur.
  const isAdmin = user?.role === "admin";

  const resetMutation = useMutation({
    mutationFn: () => api<{ ok: boolean }>("/api/admin/reset", { method: "POST" }),
    onSuccess: () => {
      toast.success("Données réinitialisées");
      setResetOpen(false);
      queryClient.invalidateQueries();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const reloadMutation = useMutation({
    mutationFn: () => api<ReloadStats>("/api/admin/reload", { method: "POST" }),
    onSuccess: (stats) => {
      toast.success(
        `Données rechargées — ${stats.accounts} compte(s), ${stats.hotspotUsers} utilisateurs hotspot, ${stats.routers} routeur(s)`,
      );
      queryClient.invalidateQueries();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  if (isLoading || !data) {
    return (
      <div className="space-y-4 sm:space-y-6">
        <PageHeader title="Paramètres" description="Configuration de votre espace MikCloud" />
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-3 lg:gap-6">
          <Skeleton className="h-96 rounded-xl lg:col-span-2" />
          <Skeleton className="h-96 rounded-xl" />
          <Skeleton className="h-64 rounded-xl lg:col-span-2" />
          <Skeleton className="h-64 rounded-xl" />
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader title="Paramètres" description="Configuration de votre espace MikCloud" />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3 lg:gap-6">
        {/* Organisation */}
        <OrganizationCard settings={data} />

        {/* Abonnement SaaS — formules Essentiel (1 250 F/mois/routeur) et
            Illimité (12 000 F/an, routeurs illimités), paiement via Wave. */}
        <SubscriptionCard />

        {/* Guide connexion routeur réel */}
        <Card className="gap-4 border-primary/20 bg-primary/5 py-4 sm:py-6 lg:col-span-2">
          <CardHeader className="px-4 sm:px-6">
            <CardTitle className="flex items-center gap-2 text-base">
              <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
                <RouterIcon className="size-4" />
              </span>
              Connecter un vrai routeur MikroTik
            </CardTitle>
            <CardDescription>Trois étapes pour brancher MikCloud sur votre matériel.</CardDescription>
          </CardHeader>
          <CardContent className="px-4 sm:px-6">
            <ol className="grid gap-4 sm:grid-cols-3">
              {MIKROTIK_STEPS.map((step, index) => (
                <li key={step.title} className="rounded-lg border bg-card p-3">
                  <p className="flex items-center gap-2 text-sm font-medium">
                    <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/15 text-xs font-semibold text-primary">
                      {index + 1}
                    </span>
                    {step.title}
                  </p>
                  <p className="mt-1.5 text-xs leading-relaxed text-muted-foreground">{step.text}</p>
                </li>
              ))}
            </ol>
            <p className="mt-4 text-xs text-muted-foreground">
              Le mode Simulé permet de découvrir MikCloud sans matériel.
            </p>
          </CardContent>
        </Card>

        {/* Sécurité — changement de mot de passe (POST /api/auth/password, tout utilisateur connecté) */}
        <SecurityCard />

        {/* Base de données — admin plateforme uniquement (POST /api/admin/reload admin-only) */}
        {isAdmin && (
          <Card className="gap-4 py-4 sm:py-6">
            <CardHeader className="px-4 sm:px-6">
              <CardTitle className="flex items-center gap-2 text-base">
                <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
                  <Database className="size-4" />
                </span>
                Base de données
              </CardTitle>
              <CardDescription>
                Réimporte l'intégralité des données depuis la base persistée sans redémarrer le service — utile
                après une modification SQL directe ou un changement de mot de passe admin.
              </CardDescription>
            </CardHeader>
            <CardFooter className="px-4 sm:px-6">
              <Button
                variant="outline"
                className="h-10"
                onClick={() => reloadMutation.mutate()}
                disabled={reloadMutation.isPending}
              >
                {reloadMutation.isPending ? "Rechargement…" : "Recharger depuis la base"}
              </Button>
            </CardFooter>
          </Card>
        )}

        {/* Zone sensible — admin plateforme uniquement (endpoint /api/admin/reset admin-only) */}
        {isAdmin && (
          <Card className="gap-4 border-destructive/30 py-4 sm:py-6">
            <CardHeader className="px-4 sm:px-6">
              <CardTitle className="flex items-center gap-2 text-base text-destructive">
                <TriangleAlert className="size-4" />
                Zone sensible
              </CardTitle>
              <CardDescription>
                Réinitialiser toutes les données de démonstration (utilisateurs, vouchers, sessions, revendeurs seront
                régénérés).
              </CardDescription>
            </CardHeader>
            <CardFooter className="px-4 sm:px-6">
              <Button variant="destructive" className="h-10" onClick={() => setResetOpen(true)}>
                Réinitialiser les données
              </Button>
            </CardFooter>
          </Card>
        )}
      </div>

      {/* Double confirmation de réinitialisation */}
      <AlertDialog open={resetOpen} onOpenChange={setResetOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Réinitialiser toutes les données ?</AlertDialogTitle>
            <AlertDialogDescription>
              Utilisateurs, vouchers, sessions, revendeurs et transactions seront régénérés à partir du jeu de
              démonstration. Cette action est irréversible.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Annuler</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={(event) => {
                event.preventDefault();
                resetMutation.mutate();
              }}
            >
              {resetMutation.isPending ? "Réinitialisation…" : "Réinitialiser les données"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

// Champ mot de passe avec bascule de visibilité.
function PasswordInput({
  id,
  value,
  onChange,
  placeholder,
  autoComplete,
}: {
  id: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  autoComplete?: string;
}) {
  const [visible, setVisible] = useState(false);
  return (
    <div className="relative">
      <Input
        id={id}
        type={visible ? "text" : "password"}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        autoComplete={autoComplete}
        className="h-10 pr-10"
      />
      <button
        type="button"
        onClick={() => setVisible((v) => !v)}
        aria-label={visible ? "Masquer le mot de passe" : "Afficher le mot de passe"}
        className="absolute right-1.5 top-1/2 flex size-7 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        {visible ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
      </button>
    </div>
  );
}

// Carte Sécurité — changement du mot de passe de connexion de l'utilisateur courant.
// Le backend exige le mot de passe actuel (une session ouverte ne suffit pas).
function SecurityCard() {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");

  const changeMutation = useMutation({
    mutationFn: () =>
      api<{ ok: boolean }>("/api/auth/password", {
        method: "POST",
        body: { currentPassword, newPassword },
      }),
    onSuccess: () => {
      toast.success("Mot de passe modifié", {
        description: "Utilisez-le à votre prochaine connexion.",
      });
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const canSubmit =
    currentPassword !== "" && newPassword !== "" && confirmPassword !== "" && !changeMutation.isPending;

  function submitPassword() {
    if (newPassword.length < 8) {
      toast.error("Le nouveau mot de passe doit faire au moins 8 caractères.");
      return;
    }
    if (newPassword === currentPassword) {
      toast.error("Le nouveau mot de passe doit être différent de l'actuel.");
      return;
    }
    if (newPassword !== confirmPassword) {
      toast.error("Les deux nouveaux mots de passe ne correspondent pas.");
      return;
    }
    changeMutation.mutate();
  }

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <KeyRound className="size-4" />
          </span>
          Sécurité
        </CardTitle>
        <CardDescription>Modifier le mot de passe de votre compte MikCloud.</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">
        <div className="grid gap-2">
          <Label htmlFor="pwd-current">Mot de passe actuel</Label>
          <PasswordInput
            id="pwd-current"
            value={currentPassword}
            onChange={setCurrentPassword}
            autoComplete="current-password"
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="pwd-new">Nouveau mot de passe</Label>
          <PasswordInput
            id="pwd-new"
            value={newPassword}
            onChange={setNewPassword}
            autoComplete="new-password"
          />
          <p className="text-xs text-muted-foreground">8 caractères minimum, différent de l'actuel.</p>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="pwd-confirm">Confirmer le nouveau mot de passe</Label>
          <PasswordInput
            id="pwd-confirm"
            value={confirmPassword}
            onChange={setConfirmPassword}
            autoComplete="new-password"
          />
        </div>
      </CardContent>
      <CardFooter className="px-4 sm:px-6">
        <Button className="h-10" onClick={submitPassword} disabled={!canSubmit}>
          {changeMutation.isPending ? "Modification…" : "Modifier le mot de passe"}
        </Button>
      </CardFooter>
    </Card>
  );
}

// Formulaire Organisation — état local initialisé depuis les paramètres serveur.
function OrganizationCard({ settings }: { settings: AppSettings }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<SettingsForm>(() => ({
    name: settings.tenant.name,
    currency: settings.tenant.currency,
    timezone: settings.tenant.timezone,
    waveLink: settings.tenant.waveLink ?? "",
  }));

  const saveMutation = useMutation({
    mutationFn: (payload: SettingsForm) =>
      api<AppSettings>("/api/settings", {
        method: "PUT",
        body: {
          name: payload.name,
          currency: payload.currency,
          timezone: payload.timezone,
          waveLink: payload.waveLink?.trim() ?? "",
        },
      }),
    onSuccess: () => {
      toast.success("Paramètres enregistrés");
      queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
      // La devise se propage sur toutes les vues (dashboard, revendeurs, rapports…).
      queryClient.invalidateQueries();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const submitSettings = () => {
    if (!form.name.trim()) {
      toast.error("Le nom de l'organisation est obligatoire.");
      return;
    }
    saveMutation.mutate({ ...form, name: form.name.trim() });
  };

  return (
    <Card className="gap-4 py-4 sm:py-6 lg:col-span-2">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <Building2 className="size-4 text-primary" />
          Organisation
        </CardTitle>
        <CardDescription>Nom affiché, devise et fuseau horaire de votre espace.</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:grid-cols-2 sm:px-6">
        <div className="grid gap-2 sm:col-span-2">
          <Label htmlFor="org-name">Nom de l'organisation</Label>
          <Input
            id="org-name"
            value={form.name}
            onChange={(event) => setForm((f) => ({ ...f, name: event.target.value }))}
            placeholder="Ex. ProMax Wifi"
          />
        </div>
        <div className="grid gap-2 sm:col-span-2">
          <Label htmlFor="org-wave">Lien marchand Wave (paiement mobile)</Label>
          <Input
            id="org-wave"
            value={form.waveLink ?? ""}
            onChange={(event) => setForm((f) => ({ ...f, waveLink: event.target.value }))}
            placeholder="https://pay.wave.com/m/M_xxxxx/c/ci/"
          />
          <p className="text-xs text-muted-foreground">
            Collez l'adresse de votre boutique Wave : MikCloud compose les demandes de
            paiement par montant (Wave CI n'a pas d'API publique). Vide = désactivé.
          </p>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="org-currency">Devise</Label>
          <Select value={form.currency} onValueChange={(value) => setForm((f) => ({ ...f, currency: value }))}>
            <SelectTrigger id="org-currency" className="h-10 w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CURRENCIES.map((currency) => (
                <SelectItem key={currency} value={currency}>
                  {currency}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="org-timezone">Fuseau horaire</Label>
          <Select value={form.timezone} onValueChange={(value) => setForm((f) => ({ ...f, timezone: value }))}>
            <SelectTrigger id="org-timezone" className="h-10 w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TIMEZONES.map((timezone) => (
                <SelectItem key={timezone} value={timezone}>
                  {timezone}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </CardContent>
      <CardFooter className="justify-end px-4 sm:px-6">
        <Button className="h-10" onClick={submitSettings} disabled={saveMutation.isPending || !form.name.trim()}>
          {saveMutation.isPending ? "Enregistrement…" : "Enregistrer"}
        </Button>
      </CardFooter>
    </Card>
  );
}
