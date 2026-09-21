// Package bundle writes and reads signed transfer bundles: observations as
// a message from one Gyst to another.
//
// A bundle is a JSON Lines file. The first line is a header naming the
// sender and their Ed25519 public key, the declared egress level, the
// sources the observations belong to, the SHA-256 of everything after the
// first newline, and a signature over the header and that digest. Every
// following line is one observation envelope in the v0 schema, exactly as
// the sender's log held it. The format is inspectable with a text editor
// and verifiable with nothing but the public key.
//
// Nothing in a bundle is executed, and nothing in it is believed about
// visibility: the receiver relabels every observation to name the sender.
package bundle

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/location"
	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

const Schema = "gyst.bundle/0.1.0"

// Egress levels, in order of how far a record may travel. An observation
// recorded at one level may be exported to a destination at that level or
// nearer, never further.
var egressRank = map[string]int{"device": 0, "facility": 1, "connected-server": 2}

// Permits reports whether an observation recorded at have may go to a
// destination at want.
func Permits(have, want string) bool {
	h, ok1 := egressRank[have]
	w, ok2 := egressRank[want]
	return ok1 && ok2 && h >= w
}

// ValidEgress reports whether s names an egress level.
func ValidEgress(s string) bool { _, ok := egressRank[s]; return ok }

// Source describes one source the bundle carries observations for.
type Source struct {
	SourceID string            `json:"source_id"`
	Kind     string            `json:"kind"`
	Root     string            `json:"root"`
	Location location.Location `json:"location"`
}

// Header is the first line of a bundle.
type Header struct {
	Schema     string    `json:"schema"`
	BundleID   string    `json:"bundle_id"`
	CreatedAt  time.Time `json:"created_at"`
	Sender     Sender    `json:"sender"`
	Egress     string    `json:"egress"`
	Sources    []Source  `json:"sources"`
	Counts     Counts    `json:"counts"`
	BodySHA256 string    `json:"body_sha256"`
	Signature  string    `json:"signature,omitempty"` // base64 Ed25519 over signingInput
}

type Sender struct {
	ID        string `json:"id"`
	PublicKey string `json:"public_key"` // base64
}

type Counts struct {
	Observations int `json:"observations"`
	Withheld     int `json:"withheld_by_egress"`
}

// signingInput is what the signature covers: the header with its
// signature field empty, canonically marshalled, which includes the body
// digest. Go marshals a struct deterministically.
func signingInput(h Header) ([]byte, error) {
	h.Signature = ""
	return json.Marshal(h)
}

// ---------------------------------------------------------------------------
// Keys
// ---------------------------------------------------------------------------

// Key is a sender identity: a name and an Ed25519 keypair.
type Key struct {
	ID      string
	Public  ed25519.PublicKey
	private ed25519.PrivateKey
}

func (k Key) PublicBase64() string { return base64.StdEncoding.EncodeToString(k.Public) }

// Generate makes a new key and writes it under dir as <id>.key (private,
// owner-only) and <id>.pub. It refuses to overwrite.
func Generate(dir, id string) (Key, error) {
	if id == "" || strings.ContainsAny(id, "/\\ ") {
		return Key{}, fmt.Errorf("key id must be a single word")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Key{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Key{}, err
	}
	kp := filepath.Join(dir, id+".key")
	if _, err := os.Stat(kp); err == nil {
		return Key{}, fmt.Errorf("key %s already exists; not overwriting", kp)
	}
	if err := os.WriteFile(kp, []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
		return Key{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, id+".pub"), []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644); err != nil {
		return Key{}, err
	}
	return Key{ID: id, Public: pub, private: priv}, nil
}

// Load reads a key written by Generate.
func Load(dir, id string) (Key, error) {
	raw, err := os.ReadFile(filepath.Join(dir, id+".key"))
	if err != nil {
		return Key{}, err
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return Key{}, fmt.Errorf("key %s: not an Ed25519 seed", id)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	return Key{ID: id, Public: priv.Public().(ed25519.PublicKey), private: priv}, nil
}

// ParsePublic decodes a base64 public key.
func ParsePublic(b64 string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("not an Ed25519 public key")
	}
	return ed25519.PublicKey(b), nil
}

// ---------------------------------------------------------------------------
// Writing
// ---------------------------------------------------------------------------

// Writer accumulates a bundle body, then writes header and body.
type Writer struct {
	key     Key
	egress  string
	sources []Source
	body    strings.Builder
	counts  Counts
}

func NewWriter(key Key, egress string, sources []Source) (*Writer, error) {
	if !ValidEgress(egress) {
		return nil, fmt.Errorf("egress %q: one of device, facility, connected-server", egress)
	}
	return &Writer{key: key, egress: egress, sources: sources}, nil
}

// Add includes an observation if its recorded egress permits the bundle's
// destination; otherwise it is counted as withheld.
func (w *Writer) Add(o observe.Observation) error {
	if !Permits(o.Policy.Egress, w.egress) {
		w.counts.Withheld++
		return nil
	}
	line, err := json.Marshal(o)
	if err != nil {
		return err
	}
	w.body.Write(line)
	w.body.WriteByte('\n')
	w.counts.Observations++
	return nil
}

// Counts reports what was added and withheld so far.
func (w *Writer) Counts() Counts { return w.counts }

// Finish signs and writes the bundle.
func (w *Writer) Finish(out io.Writer, now time.Time) (Header, error) {
	body := w.body.String()
	sum := sha256.Sum256([]byte(body))
	h := Header{
		Schema: Schema, CreatedAt: now.UTC(), Egress: w.egress, Sources: w.sources, Counts: w.counts,
		Sender:     Sender{ID: w.key.ID, PublicKey: w.key.PublicBase64()},
		BodySHA256: hex.EncodeToString(sum[:]),
	}
	idInput := sha256.Sum256([]byte(w.key.ID + "\x00" + h.BodySHA256 + "\x00" + h.CreatedAt.Format(time.RFC3339Nano)))
	h.BundleID = "bnd_" + hex.EncodeToString(idInput[:])[:24]
	in, err := signingInput(h)
	if err != nil {
		return h, err
	}
	h.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(w.key.private, in))
	head, err := json.Marshal(h)
	if err != nil {
		return h, err
	}
	if _, err := out.Write(append(head, '\n')); err != nil {
		return h, err
	}
	_, err = io.WriteString(out, body)
	return h, err
}

// ---------------------------------------------------------------------------
// Reading
// ---------------------------------------------------------------------------

var (
	ErrBadSignature = errors.New("bundle signature does not verify")
	ErrBadDigest    = errors.New("bundle body does not match its digest")
)

// Read verifies a bundle and returns its header and observations. It
// checks the body digest first and the signature second, against the
// public key in the header. Whether that key is trusted is the caller's
// decision; Read only proves the bundle was signed by whoever holds it.
func Read(r io.Reader) (Header, []observe.Observation, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	first, err := br.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return Header{}, nil, err
	}
	var h Header
	if err := json.Unmarshal(first, &h); err != nil {
		return Header{}, nil, fmt.Errorf("bundle header: %w", err)
	}
	if h.Schema != Schema {
		return h, nil, fmt.Errorf("bundle schema %q; this Gyst reads %s", h.Schema, Schema)
	}
	body, err := io.ReadAll(br)
	if err != nil {
		return h, nil, err
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != h.BodySHA256 {
		return h, nil, ErrBadDigest
	}
	pub, err := ParsePublic(h.Sender.PublicKey)
	if err != nil {
		return h, nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(h.Signature)
	if err != nil {
		return h, nil, ErrBadSignature
	}
	in, err := signingInput(h)
	if err != nil {
		return h, nil, err
	}
	if !ed25519.Verify(pub, in, sig) {
		return h, nil, ErrBadSignature
	}
	var obs []observe.Observation
	sc := bufio.NewScanner(strings.NewReader(string(body)))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var o observe.Observation
		if err := json.Unmarshal(line, &o); err != nil {
			return h, nil, fmt.Errorf("bundle observation %d: %w", len(obs)+1, err)
		}
		obs = append(obs, o)
	}
	if err := sc.Err(); err != nil {
		return h, nil, err
	}
	if len(obs) != h.Counts.Observations {
		return h, nil, fmt.Errorf("bundle says %d observations, holds %d", h.Counts.Observations, len(obs))
	}
	return h, obs, nil
}

// Namespace rewrites an observation for a receiver: the sender's source
// ids are prefixed with the sender's id, so two Gysts that both call a
// source "engineering" do not collide, and visibility labels are replaced
// by one naming the sender, because the sender's labels mean nothing here.
// The observation id is kept: it is the sender's evidence id, and anything
// the sender cites by it must still resolve.
func Namespace(o observe.Observation, senderID string) observe.Observation {
	o.Source.SourceID = senderID + "/" + o.Source.SourceID
	o.Subject.Location.SourceID = senderID + "/" + o.Subject.Location.SourceID
	o.Visibility.Labels = []string{"bundle:" + senderID}
	o.Visibility.SourceACLComplete = false
	return o
}
