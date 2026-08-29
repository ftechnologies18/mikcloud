// Package routeros — client du protocole binaire RouterOS API (port 8728),
// implémenté à la main avec la stdlib uniquement.
//
// Encodage des mots : longueur < 0x80 sur 1 octet, sinon continuation 7 bits
// (octets de poids fort en premier, bit haut à 1 sauf dernier octet).
// Une phrase (sentence) est une suite de mots terminée par un mot vide.
// Réponses : !re (données), !done (fin), !trap (erreur), !fatal (erreur fatale),
// les paires clé/valeur sont encodées "=clé=valeur".
package routeros

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Client — connexion authentifiée à un équipement MikroTik.
type Client struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
}

const commandTimeout = 10 * time.Second

// Dial ouvre une connexion TCP et s'authentifie :
//   - RouterOS v6.43+ : envoi direct de =name= / =password=
//   - fallback anciens firmware : challenge MD5 (!done avec =ret=)
//     réponse = "00" + hex(md5(0x00 + motdepasse + challenge))
func Dial(host string, port int, username, password string, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, fmt.Errorf("connexion à %s impossible : %w", address, err)
	}
	c := &Client{conn: conn, reader: bufio.NewReader(conn), writer: bufio.NewWriter(conn)}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if err := c.login(username, password); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return c, nil
}

// Close ferme la connexion.
func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) login(username, password string) error {
	_, done, err := c.Run("/login", "=name="+username, "=password="+password)
	if err != nil {
		return fmt.Errorf("authentification refusée : %w", err)
	}
	// Ancien firmware : le routeur renvoie un challenge MD5 dans =ret=
	if ret := done["ret"]; ret != "" {
		challenge, derr := hex.DecodeString(ret)
		if derr != nil {
			return fmt.Errorf("challenge RouterOS invalide")
		}
		h := md5.New()
		h.Write([]byte{0x00})
		h.Write([]byte(password))
		h.Write(challenge)
		response := "00" + hex.EncodeToString(h.Sum(nil))
		if _, _, err = c.Run("/login", "=name="+username, "=response="+response); err != nil {
			return fmt.Errorf("authentification refusée : %w", err)
		}
	}
	return nil
}

// Run envoie une phrase et lit les réponses jusqu'à !done.
// Retourne les lignes de données (!re) et les attributs du !done.
func (c *Client) Run(words ...string) ([]map[string]string, map[string]string, error) {
	_ = c.conn.SetDeadline(time.Now().Add(commandTimeout))
	if err := c.writeSentence(words...); err != nil {
		return nil, nil, err
	}
	rows := []map[string]string{}
	for {
		sentence, err := c.readSentence()
		if err != nil {
			return nil, nil, err
		}
		if len(sentence) == 0 {
			continue
		}
		switch sentence[0] {
		case "!re":
			rows = append(rows, parsePairs(sentence[1:]))
		case "!done":
			return rows, parsePairs(sentence[1:]), nil
		case "!trap", "!fatal":
			msg := "erreur RouterOS"
			if m := parsePairs(sentence[1:])["message"]; m != "" {
				msg = m
			}
			if sentence[0] == "!fatal" {
				msg = "erreur fatale : " + msg
			}
			return nil, nil, fmt.Errorf("%s", msg)
		}
	}
}

// Exec envoie une phrase et ignore les données de réponse.
func (c *Client) Exec(words ...string) error {
	_, _, err := c.Run(words...)
	return err
}

// ---------------------------------------------------------------------------
// Encodage / décodage bas niveau
// ---------------------------------------------------------------------------

func (c *Client) writeSentence(words ...string) error {
	for _, w := range words {
		if err := writeWord(c.writer, w); err != nil {
			return err
		}
	}
	if err := writeWord(c.writer, ""); err != nil { // mot vide = fin de phrase
		return err
	}
	return c.writer.Flush()
}

func writeWord(w *bufio.Writer, word string) error {
	if err := writeLength(w, len(word)); err != nil {
		return err
	}
	_, err := w.WriteString(word)
	return err
}

func writeLength(w *bufio.Writer, n int) error {
	if n < 0x80 {
		return w.WriteByte(byte(n))
	}
	// groupes de 7 bits, poids faible en premier
	var chunks []byte
	for n > 0 {
		chunks = append(chunks, byte(n&0x7F))
		n >>= 7
	}
	for i := len(chunks) - 1; i > 0; i-- {
		if err := w.WriteByte(0x80 | chunks[i]); err != nil {
			return err
		}
	}
	return w.WriteByte(chunks[0])
}

func (c *Client) readSentence() ([]string, error) {
	var words []string
	for {
		w, err := c.readWord()
		if err != nil {
			return nil, err
		}
		if w == "" {
			return words, nil
		}
		words = append(words, w)
	}
}

func (c *Client) readWord() (string, error) {
	n, err := readLength(c.reader)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(c.reader, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func readLength(r *bufio.Reader) (int, error) {
	n := 0
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		n = n<<7 | int(b&0x7F)
		if b&0x80 == 0 {
			return n, nil
		}
	}
}

func parsePairs(words []string) map[string]string {
	m := map[string]string{}
	for _, w := range words {
		if len(w) >= 2 && w[0] == '=' {
			kv := w[1:]
			if i := strings.IndexByte(kv, '='); i >= 0 {
				m[kv[:i]] = kv[i+1:]
			} else {
				m[kv] = ""
			}
		}
	}
	return m
}

// parseUptime convertit une durée RouterOS ("1w2d3h4m5s") en secondes.
func parseUptime(s string) int64 {
	var total, num int64
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			num = num*10 + int64(ch-'0')
			continue
		}
		switch ch {
		case 'w':
			total += num * 7 * 24 * 3600
		case 'd':
			total += num * 24 * 3600
		case 'h':
			total += num * 3600
		case 'm':
			total += num * 60
		case 's':
			total += num
		}
		num = 0
	}
	return total
}

func parseInt64(s string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
