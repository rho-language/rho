// Client credentials: how one service proves to the issuer that it is itself,
// and what it may ask for.
//
// A service that calls another holds an id and a secret the issuer registered for
// it. It exchanges them for a short-lived token for one audience and some scopes,
// and the callee verifies that token offline. The issuer keeps a hash of the
// secret, never the secret, together with the audiences and scopes the client may
// be granted; a request for anything else is refused or narrowed, never widened.

package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Client is one registered service.
type Client struct {
	// ID names the client, such as `aquarium` or `mesh`.
	ID string
	// SecretHash is the hex SHA-256 of the secret. The secret is 32 random bytes,
	// so a plain hash is enough: there is nothing to guess.
	SecretHash string
	// Audiences are the services it may get a token for. A `*` segment matches
	// any one segment, so `gateway:*` is every gateway.
	Audiences []string
	// Scopes are the scopes it may be granted, with the same rule; a final `*`
	// matches the rest of a name.
	Scopes []string
}

// ErrClientDenied is what every failed authentication returns, so a caller
// cannot tell an unknown client from a wrong secret.
var ErrClientDenied = errors.New("client authentication failed")

// DefaultClientsPath is where an installation keeps its registered clients.
func DefaultClientsPath(rhoHome string) string {
	return filepath.Join(rhoHome, "auth", "clients")
}

// AddClient registers a client and returns its secret, which is shown once and
// never stored. Registering an id that exists replaces it, which is how a secret
// is rotated.
func AddClient(path, id string, audiences, scopes []string) (string, error) {
	if !validClientField(id) || strings.ContainsAny(id, ",*") {
		return "", fmt.Errorf("client id %q must be one token with no '|', ',' or '*'", id)
	}
	for _, value := range append(append([]string{}, audiences...), scopes...) {
		if !validClientField(value) || strings.Contains(value, ",") {
			return "", fmt.Errorf("%q cannot be an audience or a scope", value)
		}
	}
	if len(audiences) == 0 {
		return "", errors.New("a client needs at least one audience")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	clients, err := LoadClients(path)
	if err != nil {
		return "", err
	}
	clients[id] = Client{ID: id, SecretHash: hashSecret(secret), Audiences: audiences, Scopes: scopes}
	return secret, saveClients(path, clients)
}

// RemoveClient deletes a client; its tokens expire on their own.
func RemoveClient(path, id string) error {
	clients, err := LoadClients(path)
	if err != nil {
		return err
	}
	if _, ok := clients[id]; !ok {
		return fmt.Errorf("no client %q", id)
	}
	delete(clients, id)
	return saveClients(path, clients)
}

// LoadClients reads the registry; a missing file is an empty one.
func LoadClients(path string) (map[string]Client, error) {
	clients := map[string]Client{}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return clients, nil
	}
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 4 {
			return nil, fmt.Errorf("%s: malformed client line", path)
		}
		clients[parts[0]] = Client{ID: parts[0], SecretHash: parts[1],
			Audiences: splitList(parts[2]), Scopes: splitList(parts[3])}
	}
	return clients, nil
}

// Authenticate checks a client's secret in constant time.
func Authenticate(path, id, secret string) (Client, error) {
	clients, err := LoadClients(path)
	if err != nil {
		return Client{}, err
	}
	client, ok := clients[id]
	// The comparison runs whether or not the client exists, against a hash of
	// the right length, so timing does not say which of the two was wrong.
	expected := client.SecretHash
	if !ok {
		expected = strings.Repeat("0", sha256.Size*2)
	}
	if subtle.ConstantTimeCompare([]byte(hashSecret(secret)), []byte(expected)) != 1 || !ok || secret == "" {
		return Client{}, ErrClientDenied
	}
	return client, nil
}

// Grant is what a client gets when it asks for an audience and scopes: the
// audience must be one it is registered for, and the scopes are those it asked
// for that it may hold. Asking for none grants the concrete scopes it may hold.
func (c Client) Grant(audience string, requested []string) ([]string, error) {
	if !matchesAny(audience, c.Audiences) {
		return nil, fmt.Errorf("client %q may not ask for audience %q", c.ID, audience)
	}
	if len(requested) == 0 {
		var concrete []string
		for _, scope := range c.Scopes {
			if !strings.Contains(scope, "*") {
				concrete = append(concrete, scope)
			}
		}
		return concrete, nil
	}
	var granted []string
	for _, scope := range requested {
		if scope == "" {
			continue
		}
		if !matchesAny(scope, c.Scopes) {
			return nil, fmt.Errorf("client %q may not hold scope %q", c.ID, scope)
		}
		granted = append(granted, scope)
	}
	return granted, nil
}

func matchesAny(value string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchPattern(pattern, value) {
			return true
		}
	}
	return false
}

// matchPattern compares two `:`-separated names. A `*` segment matches exactly
// one segment, except as the last segment, where it matches one or more: so
// `gateway:*:read` matches `gateway:edge-1:read`, and `mesh:*` matches
// `mesh:home` and `mesh:home:actuate`.
func matchPattern(pattern, value string) bool {
	p := strings.Split(pattern, ":")
	v := strings.Split(value, ":")
	for i, segment := range p {
		if i >= len(v) {
			return false
		}
		if segment == "*" {
			if i == len(p)-1 {
				return true
			}
			continue
		}
		if segment != v[i] {
			return false
		}
	}
	return len(p) == len(v)
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func splitList(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func validClientField(value string) bool {
	return value != "" && !strings.ContainsAny(value, "|\n\r \t")
}

// saveClients writes the registry readable by its owner only, replacing it
// atomically so a reader never sees half of it.
func saveClients(path string, clients map[string]Client) error {
	ids := make([]string, 0, len(clients))
	for id := range clients {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	b.WriteString("# id|sha256(secret)|audiences|scopes\n")
	for _, id := range ids {
		c := clients[id]
		fmt.Fprintf(&b, "%s|%s|%s|%s\n", c.ID, c.SecretHash, strings.Join(c.Audiences, ","), strings.Join(c.Scopes, ","))
	}
	return WriteSecretFile(path, b.String())
}

// WriteSecretFile writes a file only its owner can read, creating its directory
// readable by its owner only, and replaces any file that was there atomically.
func WriteSecretFile(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".secret-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}
