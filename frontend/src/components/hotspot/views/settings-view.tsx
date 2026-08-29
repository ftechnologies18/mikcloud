"use client";

// Vue Paramètres — organisation, abonnement, connexion routeur réel, zone sensible.

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Building2, Check, Crown, Router as RouterIcon, TriangleAlert } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/hotspot/api";
import type { AppSettings } from "@/lib/hotspot/types";
import { PageHeader } from "@/components/hotspot/page-header";
import { SETTINGS_QUERY_KEY, useSettings } from "@/components/hotspot/parts/sd-currency";
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
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";

const CURRENCIES = ["FCFA", "EUR", "USD", "MAD", "XOF", "GBP", "CDF", "GNF"];
const TIMEZONES = ["UTC", "Africa/Abidjan", "Africa/Dakar", "Africa/Casablanca", "Europe/Paris", "Europe/Brussels"];

const PLAN_FEATURES = ["Routeurs illimités", "Utilisateurs illimités", "Revendeurs illimités", "Support prioritaire"];

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
}

export default function SettingsView() {
  const queryClient = useQueryClient();
  const { data, isLoading } = useSettings();
  const [resetOpen, setResetOpen] = useState(false);

  const resetMutation = useMutation({
    mutationFn: () => api<{ ok: boolean }>("/api/admin/reset", { method: "POST" }),
    onSuccess: () => {
      toast.success("Données réinitialisées");
      setResetOpen(false);
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

        {/* Abonnement */}
        <Card className="gap-4 py-4 sm:py-6">
          <CardHeader className="px-4 sm:px-6">
            <CardTitle className="flex items-center justify-between gap-2 text-base">
              Abonnement
              <Badge className="gap-1 border-primary/25 bg-primary/15 text-primary" variant="outline">
                <Crown className="size-3" />
                PRO
              </Badge>
            </CardTitle>
            <CardDescription>Toutes les fonctionnalités, sans limite.</CardDescription>
          </CardHeader>
          <CardContent className="px-4 sm:px-6">
            <ul className="space-y-3">
              {PLAN_FEATURES.map((feature) => (
                <li key={feature} className="flex items-center gap-2.5 text-sm">
                  <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary">
                    <Check className="size-3" />
                  </span>
                  {feature}
                </li>
              ))}
            </ul>
            <p className="mt-5 text-xs text-muted-foreground">Plan actuel — MikCloud Pro</p>
          </CardContent>
        </Card>

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

        {/* Zone sensible */}
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

// Formulaire Organisation — état local initialisé depuis les paramètres serveur.
function OrganizationCard({ settings }: { settings: AppSettings }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<SettingsForm>(() => ({
    name: settings.tenant.name,
    currency: settings.tenant.currency,
    timezone: settings.tenant.timezone,
  }));

  const saveMutation = useMutation({
    mutationFn: (payload: SettingsForm) =>
      api<AppSettings>("/api/settings", {
        method: "PUT",
        body: { name: payload.name, currency: payload.currency, timezone: payload.timezone },
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
            placeholder="Ex. SpotNet WiFi"
          />
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
