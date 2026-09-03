// mikbackup — outil de sauvegarde chiffrée de la base Neon (sécurité S4,
// P1 de l'audit pré-lancement commercial : « sauvegardes testées »).
//
// Deux sous-commandes :
//
//	mikbackup export        -dsn URL -out FILE -key HEX64
//	mikbackup restore-check -dsn URL -file FILE -key HEX64
//
// export        — déverse TOUTES les tables du schéma public sous forme de
//
//	lignes JSON textuelles (row_to_json : le typage Postgres
//	est préservé tel quel — dates, bytea, jsonb, arrays), puis
//	chiffre le tout AES-256-GCM (clé hex 32 octets) dans un
//	fichier manipulable (base64). Le déchiffrement authentifie
//	le contenu (GCM) : un fichier altéré est rejeté.
//
// restore-check — VÉRITABLE test de restauration : déchiffre le fichier, puis
//
//	recrée chaque table sous le préfixe s4check_ (LIKE,
//	structure identique), y réinsère TOUTES les lignes du
//	fichier via json_populate_record (Postgres re-typage
//	natif — la moindre incompatibilité de type est une erreur
//	bloquante), vérifie les comptages et DROP les tables
//	miroir. Verdict imprimé table par table.
//
// L'outil vit dans le module Go existant (aucune dépendance nouvelle) :
// pgx pour l'accès base, crypto/aes + crypto/cipher pour le chiffrement.
// La clé de chiffrement n'apparaît JAMAIS dans le dépôt (variable
// d'environnement / flag) — cf. docs/RUNBOOK-SECRETS.md.
//
// Limites assumées : sauvegarde cohérente à l'instant T (une transaction
// par table ; Neon conserve de son côté un historique point-in-time selon
// le plan, cf. runbook) ; les fichiers exportés contiennent des données
// sensibles chiffrées : les stocker hors du service (artefact CI chiffré,
// coffre de l'opérateur).
package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// magic — en-tête du fichier de sauvegarde (version du format).
const magic = "MIKCLOUD-BACKUP-v1"

// backupFile — structure logique du fichier (avant chiffrement).
type backupFile struct {
	Version     int                    `json:"version"`
	GeneratedAt string                 `json:"generated_at"`
	Tables      map[string]backupTable `json:"tables"`
}

// backupTable — lignes JSON textuelles d'une table (row_to_json).
type backupTable struct {
	Rows []string `json:"rows"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "export":
		exportCmd(os.Args[2:])
	case "restore-check":
		restoreCheckCmd(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "sous-commande inconnue : %s\n", os.Args[1])
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage : mikbackup export|restore-check [flags]")
	fmt.Fprintln(os.Stderr, "  export        -dsn URL -out FILE -key HEX64  — sauvegarde chiffrée de toutes les tables")
	fmt.Fprintln(os.Stderr, "  restore-check -dsn URL -file FILE -key HEX64 — test de restauration (tables s4check_*)")
	os.Exit(2)
}

// ---------------------------------------------------------------------------
// export
// ---------------------------------------------------------------------------

func exportCmd(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	dsn := fs.String("dsn", "", "DSN PostgreSQL (Neon)")
	out := fs.String("out", "", "fichier de sauvegarde chiffré")
	key := fs.String("key", "", "clé de chiffrement (64 hex = 32 octets)")
	_ = fs.Parse(args)
	if *dsn == "" || *out == "" || *key == "" {
		fs.Usage()
		os.Exit(2)
	}
	bk, err := doExport(*dsn)
	if err != nil {
		fatal("export : %v", err)
	}
	if err := writeEncrypted(*out, *key, bk); err != nil {
		fatal("écriture chiffrée : %v", err)
	}
	total := 0
	names := sortedTables(bk.Tables)
	for _, n := range names {
		fmt.Printf("  %-22s %d lignes\n", n, len(bk.Tables[n].Rows))
		total += len(bk.Tables[n].Rows)
	}
	fmt.Printf("Sauvegarde OK : %d tables, %d lignes → %s (chiffré AES-256-GCM)\n", len(names), total, *out)
}

// doExport — connexion, liste des tables du schéma public, déversement.
func doExport(dsn string) (*backupFile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connexion impossible : %w", err)
	}
	defer conn.Close(ctx)

	var tables []string
	rows, err := conn.Query(ctx, `SELECT table_name FROM information_schema.tables
		WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name`)
	if err != nil {
		return nil, fmt.Errorf("liste des tables : %w", err)
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		tables = append(tables, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	bk := &backupFile{Version: 1, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Tables: map[string]backupTable{}}
	for _, t := range tables {
		if strings.HasPrefix(t, "s4check_") {
			continue // restes d'un test antérieur : hors sauvegarde
		}
		r, err := conn.Query(ctx, fmt.Sprintf(`SELECT coalesce(row_to_json(x)::text, 'null') FROM public.%s x`, pgx.Identifier{t}.Sanitize()))
		if err != nil {
			return nil, fmt.Errorf("lecture %s : %w", t, err)
		}
		bt := backupTable{}
		for r.Next() {
			var line string
			if err := r.Scan(&line); err != nil {
				r.Close()
				return nil, fmt.Errorf("scan %s : %w", t, err)
			}
			bt.Rows = append(bt.Rows, line)
		}
		r.Close()
		if err := r.Err(); err != nil {
			return nil, fmt.Errorf("itération %s : %w", t, err)
		}
		bk.Tables[t] = bt
	}
	return bk, nil
}

// ---------------------------------------------------------------------------
// restore-check
// ---------------------------------------------------------------------------

func restoreCheckCmd(args []string) {
	fs := flag.NewFlagSet("restore-check", flag.ExitOnError)
	dsn := fs.String("dsn", "", "DSN PostgreSQL (Neon)")
	file := fs.String("file", "", "fichier de sauvegarde chiffré")
	key := fs.String("key", "", "clé de chiffrement (64 hex = 32 octets)")
	_ = fs.Parse(args)
	if *dsn == "" || *file == "" || *key == "" {
		fs.Usage()
		os.Exit(2)
	}
	bk, err := readEncrypted(*file, *key)
	if err != nil {
		fatal("lecture sauvegarde : %v", err)
	}
	names := sortedTables(bk.Tables)
	fmt.Printf("Fichier authentique (AES-GCM) — %d tables, généré le %s\n", len(names), bk.GeneratedAt)
	if err := doRestoreCheck(*dsn, bk); err != nil {
		fatal("restauration : %v", err)
	}
	fmt.Println("RESTAURATION TESTÉE : OK — toutes les lignes du fichier réinsérées sans erreur, tables miroir nettoyées.")
}

// doRestoreCheck — recrée chaque table en miroir s4check_*, y réinsère les
// lignes du fichier, vérifie les comptages, DROP les miroirs. Une transaction
// par table : tout échec de typage/contrainte casse la table concernée et
// remonte en erreur.
func doRestoreCheck(dsn string, bk *backupFile) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connexion impossible : %w", err)
	}
	defer conn.Close(ctx)

	for _, t := range sortedTables(bk.Tables) {
		want := len(bk.Tables[t].Rows)
		mirror := "s4check_" + t
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		err = func() error {
			if _, err := tx.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS public.%s CASCADE`, pgx.Identifier{mirror}.Sanitize())); err != nil {
				return fmt.Errorf("drop %s : %w", mirror, err)
			}
			// Structure identique à la table d'origine.
			if _, err := tx.Exec(ctx, fmt.Sprintf(`CREATE TABLE public.%s (LIKE public.%s INCLUDING ALL)`,
				pgx.Identifier{mirror}.Sanitize(), pgx.Identifier{t}.Sanitize())); err != nil {
				return fmt.Errorf("create %s : %w", mirror, err)
			}
			for _, line := range bk.Tables[t].Rows {
				// Postgres re-typage natif : populate_record rejette toute
				// valeur incompatible (dates, bytea, contraintes, types).
				if _, err := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO public.%s SELECT * FROM json_populate_record(NULL::public.%s, $1::json)`,
					pgx.Identifier{mirror}.Sanitize(), pgx.Identifier{t}.Sanitize()), line); err != nil {
					return fmt.Errorf("insert %s : %w", mirror, err)
				}
			}
			var got int
			if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM public.%s`, pgx.Identifier{mirror}.Sanitize())).Scan(&got); err != nil {
				return err
			}
			if got != want {
				return fmt.Errorf("%s : %d/%d lignes restaurées", mirror, got, want)
			}
			// Nettoyage immédiat (dans la transaction) : la prod ne garde
			// aucune table miroir.
			if _, err := tx.Exec(ctx, fmt.Sprintf(`DROP TABLE public.%s CASCADE`, pgx.Identifier{mirror}.Sanitize())); err != nil {
				return fmt.Errorf("drop final %s : %w", mirror, err)
			}
			return nil
		}()
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("table %s : %w", t, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s : %w", t, err)
		}
		fmt.Printf("  %-22s %d/%d lignes restaurées ✓\n", t, want, want)
	}
	return nil
}

// ---------------------------------------------------------------------------
// chiffrement AES-256-GCM
// ---------------------------------------------------------------------------

// parseKey — clé hex 64 caractères = 32 octets (AES-256).
func parseKey(hexKey string) ([]byte, error) {
	k, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil || len(k) != 32 {
		return nil, fmt.Errorf("clé invalide : attendu 64 caractères hex (32 octets), obtenu %d octets", len(k))
	}
	return k, nil
}

// writeEncrypted — sérialise, chiffre (AES-256-GCM, nonce aléatoire) et
// écrit un fichier texte : magic + base64(nonce||ciphertext).
func writeEncrypted(path, hexKey string, bk *backupFile) error {
	key, err := parseKey(hexKey)
	if err != nil {
		return err
	}
	plain, err := json.Marshal(bk)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	blob := append(nonce, gcm.Seal(nil, nonce, plain, nil)...)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(magic + "\n" + base64.StdEncoding.EncodeToString(blob) + "\n"); err != nil {
		return err
	}
	return nil
}

// readEncrypted — lit, authentifie (GCM : fichier altéré ou mauvaise clé =
// erreur) et dé-sérialise.
func readEncrypted(path, hexKey string) (*backupFile, error) {
	key, err := parseKey(hexKey)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.SplitN(strings.TrimSpace(string(raw)), "\n", 2)
	if len(lines) != 2 || lines[0] != magic {
		return nil, fmt.Errorf("format de fichier inconnu")
	}
	blob, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[1]))
	if err != nil {
		return nil, fmt.Errorf("contenu illisible : %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, fmt.Errorf("fichier tronqué")
	}
	nonce, ct := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("déchiffrement impossible (clé erronée ou fichier altéré)")
	}
	var bk backupFile
	if err := json.Unmarshal(plain, &bk); err != nil {
		return nil, err
	}
	return &bk, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func sortedTables(m map[string]backupTable) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	// Tri insertion simple (peu de tables, aucune dépendance externe).
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}

func fatal(f string, args ...any) {
	fmt.Fprintf(os.Stderr, "mikbackup : "+f+"\n", args...)
	os.Exit(1)
}
