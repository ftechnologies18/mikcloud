// pg_settings_sql_test.go — N°56 : garde-fou statique de l'UPSERT settings.
//
// Contexte (incident découvert pendant N°56) : l'ajout des colonnes
// hospitalité (N°55) avait étendu la liste des colonnes SANS dupliquer le
// premier placeholder — 31 colonnes pour 30 expressions → chaque Exec
// échouait côté PostgreSQL (« INSERT has more target columns than
// expressions »), donc CHAQUE Sync échouait (rollback total) et Neon ne
// recevait plus rien depuis le déploiement N°55 (régression silencieuse :
// le store logue et retente, tout le système tournant sur la mémoire).
//
// Le pattern maison de syncSettings est : id et account_id partagent le
// MÊME paramètre ($1 = accID), soit N colonnes → N expressions → N-1
// paramètres distincts ($1..$N-1) → N-1 arguments Go. Ce test lit le
// littéral SQL du fichier source et verrouille cet invariant pour empêcher
// toute récidive lors du prochain ajout de colonne settings.

package store

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestSyncSettingsSQLConsistency(t *testing.T) {
	src, err := readSourceFile("pg.go")
	if err != nil {
		t.Fatalf("lecture du source impossible : %v", err)
	}
	i := strings.Index(src, "INSERT INTO settings")
	if i < 0 {
		t.Fatal("bloc INSERT INTO settings introuvable dans pg.go")
	}
	// Le littéral SQL s'arrête au prochain backtick (la clause SET EXCLUDED
	// en fait partie — elle précède la fermeture du littéral).
	close := strings.Index(src[i:], "`")
	if close < 0 {
		t.Fatal("littéral SQL non refermé")
	}
	frag := src[i : i+close]
	if !strings.Contains(frag, "ON CONFLICT") {
		t.Fatal("clause ON CONFLICT introuvable dans le littéral")
	}

	// 1) Colonnes : la liste entre la première parenthèse et VALUES.
	colsPart := frag[strings.Index(frag, "(")+1 : strings.Index(frag, "VALUES")]
	cols := []string{}
	for _, c := range strings.Split(colsPart, ",") {
		c = strings.TrimSpace(c)
		if c != "" && !strings.HasPrefix(c, "--") {
			cols = append(cols, c)
		}
	}

	// 2) Expressions : la liste entre VALUES ( et le fragment suivant.
	valsPart := frag[strings.Index(frag, "VALUES")+6:]
	valsPart = strings.TrimSpace(valsPart)
	valsPart = strings.TrimPrefix(valsPart, "(")
	valsPart = valsPart[:strings.Index(valsPart, ")")]
	exprs := []string{}
	for _, e := range strings.Split(valsPart, ",") {
		if strings.TrimSpace(e) != "" {
			exprs = append(exprs, strings.TrimSpace(e))
		}
	}

	if len(cols) == 0 || len(exprs) == 0 {
		t.Fatalf("parsing du bloc impossible (cols=%d exprs=%d)", len(cols), len(exprs))
	}
	if len(cols) != len(exprs) {
		t.Fatalf("RÉGRESSION : %d colonnes pour %d expressions VALUES — l'Exec échouera et TOUTE la synchro Neon sera annulée (rollback) — cf. incident N°55", len(cols), len(exprs))
	}

	// 3) Paramètres distincts : id et account_id partagent $1 → N-1 params.
	maxPh := 0
	phRe := regexp.MustCompile(`\$(\d+)`)
	for _, e := range exprs {
		m := phRe.FindStringSubmatch(e)
		if m == nil {
			t.Fatalf("expression sans placeholder : %q", e)
		}
		n, _ := strconv.Atoi(m[1])
		if n > maxPh {
			maxPh = n
		}
	}
	if maxPh != len(cols)-1 {
		t.Fatalf("invariant du pattern maison violé : %d colonnes attendent $1..$%d (id et account_id partagent $1), trouvé max $%d", len(cols), len(cols)-1, maxPh)
	}

	// 4) La clause SET couvre toutes les colonnes sauf id (clé du conflit).
	setCount := strings.Count(frag, "= EXCLUDED.")
	if setCount != len(cols)-1 {
		t.Fatalf("clause SET : %d lignes EXCLUDED pour %d colonnes (attendu %d = colonnes - id)", setCount, len(cols), len(cols)-1)
	}
}

// readSourceFile — lit un fichier du package (les tests tournent avec le
// répertoire du package comme working directory).
func readSourceFile(name string) (string, error) {
	b, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
