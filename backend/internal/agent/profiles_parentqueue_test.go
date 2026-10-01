package agent

import (
	"strings"
	"testing"
)

// TestProfileEnsureParentQueue — N°201 — les trois régimes du parent-queue
// dans la génération du script (le cloud choisit le régime via la présence
// de la clé parentQueue dans le payload — cf. plProfile/HasQueue) :
//
//   - HasQueue + ParentQueue rempli  → add ET set portent parent-queue="X"
//     (la file existe sur la box — le gérant l'a vérifiée ou le cloud l'a
//     confirmée par sig) ;
//   - HasQueue + ParentQueue vide     → le set aligne parent-queue=none
//     (détachement explicite, sémantique historique) ;
//   - clé OMISE (HasQueue=false)      → NI le add NI le set ne touchent au
//     parent-queue : c'est le régime N°201 pour une box où la file managée
//     mikcloud-qos n'existe pas — le profil doit être créé SANS elle (fin
//     de l'incident Zikisso : l'add échouait silencieusement sur la
//     référence à une file inexistante, tous les user_add en cascade).
func TestProfileEnsureParentQueue(t *testing.T) {
	wired := ProfileRef{
		Name: "3heures-pq", RateLimit: "4M/4M", SessionTimeoutMin: 180, SharedUsers: 1,
		HasRate: true, HasTimeout: true, HasShared: true,
		HasQueue: true, ParentQueue: "mikcloud-qos",
	}
	script := profileEnsureLine(wired)
	if !strings.Contains(script, `parent-queue="mikcloud-qos"`) {
		t.Fatal("file managée présente sur la box : l'add doit porter parent-queue=\"mikcloud-qos\"")
	}
	setLine := profileSetLine(wired.Name, wired)
	if !strings.Contains(setLine, `parent-queue="mikcloud-qos"`) {
		t.Fatal("file managée présente sur la box : le set doit aligner parent-queue")
	}

	detached := wired
	detached.ParentQueue = ""
	scriptDetached := profileEnsureLine(detached)
	if strings.Contains(scriptDetached, `parent-queue="mikcloud-qos"`) {
		t.Fatal("détachement : plus aucune référence à la file managée")
	}
	if !strings.Contains(profileSetLine(detached.Name, detached), "parent-queue=none") {
		t.Fatal("détachement : le set doit aligner parent-queue=none (sémantique historique)")
	}

	omitted := wired
	omitted.HasQueue = false // clé absente du payload — régime N°201
	scriptOmitted := profileEnsureLine(omitted)
	if strings.Contains(scriptOmitted, "parent-queue") {
		t.Fatalf("clé omise : le script ne doit PAS toucher au parent-queue, obtenu :\n%s", scriptOmitted)
	}
	if !strings.Contains(scriptOmitted, "/ip hotspot user profile add") {
		t.Fatal("clé omise : l'add du profil doit rester émis (c'est tout l'enjeu — le créer SANS la file)")
	}

	// File CUSTOM du gérant : passée telle quelle (le cloud ne filtre que
	// la file managée mikcloud-qos).
	custom := wired
	custom.ParentQueue = "ma-file-agg"
	if !strings.Contains(profileEnsureLine(custom), `parent-queue="ma-file-agg"`) {
		t.Fatal("file custom : le choix du gérant passe tel quel")
	}
}
