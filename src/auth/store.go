//
// @file store.go
// @brief Credential storage in the Rho home.
//
// A client keeps the credentials it has been issued under ~/.rho/auth, and a
// registry keeps its signing secret beside them. The store is keyed by
// mechanism and by registry, so several registries and several kinds of
// credential can coexist.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ErrNoCredentialStored means the store has nothing for that key.
var ErrNoCredentialStored = errors.New("auth: no credential stored for that registry")

/* -------------------------------------------------------------------------- */
/*  Records                                                                   */
/* -------------------------------------------------------------------------- */

// Credential is one issued credential, as the client remembers it.
//
// The token itself is kept so it can be sent without a round trip; the claims
// are kept so the client can show who it is and when it runs out without
// parsing the token.
type Credential struct {
	// Method is which mechanism issued it, for example "token".
	Method string `json:"method"`
	// Registry is the audience it was issued for.
	Registry string `json:"registry"`
	// Secret is the credential, which for a token is the signed string.
	Secret string `json:"secret"`
	// Subject is who it belongs to.
	Subject string `json:"subject"`
	// Host is where it was issued from.
	Host string `json:"host,omitempty"`
	// Scopes are what it may do.
	Scopes []string `json:"scopes,omitempty"`
	// IssuedAt is when it was created.
	IssuedAt time.Time `json:"issued_at"`
	// ExpiresAt is when it runs out. Zero means it does not expire.
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// Expired reports whether a credential is past its expiry.
func (c Credential) Expired(now time.Time) bool {
	return !c.ExpiresAt.IsZero() && !now.Before(c.ExpiresAt)
}

// Display renders a credential for a listing.
func (c Credential) Display() string {
	out := c.Method + " " + c.Registry
	if c.Subject != "" {
		out += " as " + c.Subject
	}
	if !c.ExpiresAt.IsZero() {
		out += " until " + c.ExpiresAt.Format("2006-01-02")
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  Store                                                                     */
/* -------------------------------------------------------------------------- */

// Store is the credential file under the Rho home.
type Store struct {
	// Path is the file backing the store.
	Path string
}

// DefaultStorePath returns the store path under a Rho home.
func DefaultStorePath(rhoHome string) string {
	return filepath.Join(rhoHome, "auth", "credentials.json")
}

// DefaultSecretPath returns the signing secret path under a Rho home.
func DefaultSecretPath(rhoHome string) string {
	return filepath.Join(rhoHome, "auth", "signing.key")
}

// Open returns a store at the given path.
func Open(path string) *Store { return &Store{Path: path} }

// file is the on-disk shape: a list, so a future mechanism can add fields
// without a migration.
type file struct {
	Credentials []Credential `json:"credentials"`
}

func (s *Store) load() (*file, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &file{}, nil
		}
		return nil, err
	}
	var out file
	if err := json.Unmarshal(data, &out); err != nil {
		// A corrupt store is reported rather than silently discarded, so a
		// credential is never lost without notice.
		return nil, err
	}
	return &out, nil
}

func (s *Store) save(contents *file) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(contents, "", "  ")
	if err != nil {
		return err
	}
	// The file holds credentials, so it is owner-only.
	return os.WriteFile(s.Path, append(data, '\n'), 0o600)
}

// Put stores a credential, replacing any earlier one for the same registry and
// mechanism.
func (s *Store) Put(credential Credential) error {
	contents, err := s.load()
	if err != nil {
		return err
	}
	method := credential.Method
	if method == "" {
		method = TokenMethod
	}
	credential.Method = method

	kept := make([]Credential, 0, len(contents.Credentials)+1)
	for _, existing := range contents.Credentials {
		if existing.Registry == credential.Registry && existing.Method == method {
			continue
		}
		kept = append(kept, existing)
	}
	contents.Credentials = append(kept, credential)
	return s.save(contents)
}

// Get returns the credential for a registry, preferring the named method and
// otherwise the most recently issued one.
func (s *Store) Get(registry, method string) (Credential, error) {
	contents, err := s.load()
	if err != nil {
		return Credential{}, err
	}
	var best Credential
	found := false
	for _, credential := range contents.Credentials {
		if !strings.EqualFold(credential.Registry, registry) {
			continue
		}
		if method != "" && credential.Method != method {
			continue
		}
		if !found || credential.IssuedAt.After(best.IssuedAt) {
			best = credential
			found = true
		}
	}
	if !found {
		return Credential{}, ErrNoCredentialStored
	}
	return best, nil
}

// List returns every stored credential, newest first.
func (s *Store) List() ([]Credential, error) {
	contents, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]Credential, len(contents.Credentials))
	copy(out, contents.Credentials)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Registry != out[j].Registry {
			return out[i].Registry < out[j].Registry
		}
		return out[i].IssuedAt.After(out[j].IssuedAt)
	})
	return out, nil
}

// Revoke removes the credential for a registry, and reports whether there was
// one to remove.
func (s *Store) Revoke(registry string) (bool, error) {
	contents, err := s.load()
	if err != nil {
		return false, err
	}
	kept := make([]Credential, 0, len(contents.Credentials))
	removed := false
	for _, credential := range contents.Credentials {
		if strings.EqualFold(credential.Registry, registry) {
			removed = true
			continue
		}
		kept = append(kept, credential)
	}
	if !removed {
		return false, nil
	}
	contents.Credentials = kept
	return true, s.save(contents)
}
