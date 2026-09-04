import AppRoute from "@/components/hotspot/routes/app-route";

// /app/[[...vue]] — console : /app (vue courante) et /app/<vue> (lien
// direct : /app/users, /app/vouchers…). La garde et la synchronisation
// URL ↔ store vivent dans AppRoute.
export default function AppPage() {
  return <AppRoute />;
}
