//
// @file users.go
// @brief People who sign in: a name, a password hash, a role and grants.
//
// The store is one YAML file the keeper keeps owner-only:
//
//	users:
//	  ana:
//	    display: "Ana"
//	    role: "operator"
//	    grants: "mesh:greenhouse,aquarium:*"
//	    hash: "pbkdf2-sha256$600000$<salt>$<key>"
//	    created: "2026-11-13T09:00:00Z"
//
// A password is never stored, only what PBKDF2-HMAC-SHA256 makes of it with a
// salt of its own; checking one costs the same whatever the answer, and a name
// that is not there costs as much as a wrong password, so neither the time nor
// the message says which names exist.
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The roles a person may have. An admin does everything; an operator runs and
// changes the applications they are granted; a viewer reads.
var Roles = []string{"admin", "operator", "viewer"}

const (
	passwordIterations = 600000
	passwordKeyLength  = 32
	// MinPasswordLength is the shortest password accepted.
	MinPasswordLength = 10
)

// User is one person.
type User struct {
	Name    string
	Display string
	Role    string
	Grants  []string
	Hash    string
	Created string
}

// ErrBadCredentials is the one answer to a wrong name or a wrong password.
var ErrBadCredentials = errors.New("that name and password do not match")

// HashPassword makes the stored form of a password.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, passwordKeyLength)
	if err != nil {
		return "", err
	}
	return "pbkdf2-sha256$" + strconv.Itoa(passwordIterations) + "$" +
		base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}

// CheckPassword is whether a password is the one a hash was made from.
func CheckPassword(hash, password string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash is checked against when a name is not there, so a missing name takes
// as long as a wrong password.
var dummyHash = func() string {
	hash, _ := HashPassword("not a password anybody has")
	return hash
}()

// ValidUserName is whether a name can be a person's name here: lowercase
// letters, digits, '.', '_' and '-', 1 to 48 characters, starting with a letter.
func ValidUserName(name string) bool {
	if len(name) == 0 || len(name) > 48 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// ValidRole is whether a role is one of Roles.
func ValidRole(role string) bool {
	for _, one := range Roles {
		if one == role {
			return true
		}
	}
	return false
}

// validGrant is whether a grant is a scope: `app:*`, `app:read`, `app:thing`.
func validGrant(grant string) bool {
	if grant == "" || len(grant) > 96 {
		return false
	}
	for _, r := range grant {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune(":._-*", r)) {
			return false
		}
	}
	return true
}

// validDisplay is whether a display name can be one quoted YAML scalar.
func validDisplay(display string) bool {
	return len(display) <= 80 && !strings.ContainsAny(display, "\"\\\n\r\t")
}

// LoadUsers reads a store; a store that is not there holds nobody.
func LoadUsers(path string) (map[string]User, error) {
	users := map[string]User{}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return users, nil
	}
	if err != nil {
		return nil, err
	}
	current := ""
	for _, raw := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		line := strings.TrimSpace(raw)
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), "\"")
		switch indent {
		case 2:
			current = key
			users[current] = User{Name: current}
		case 4:
			if current == "" {
				continue
			}
			user := users[current]
			switch key {
			case "display":
				user.Display = value
			case "role":
				user.Role = value
			case "grants":
				user.Grants = splitList(value)
			case "hash":
				user.Hash = value
			case "created":
				user.Created = value
			}
			users[current] = user
		}
	}
	return users, nil
}

// saveUsers writes a store owner-only, in one step.
func saveUsers(path string, users map[string]User) error {
	names := make([]string, 0, len(users))
	for name := range users {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("# The people who sign in to the RoboHub keeper. Written by rho auth user.\n")
	b.WriteString("users:\n")
	for _, name := range names {
		user := users[name]
		fmt.Fprintf(&b, "  %s:\n    display: %q\n    role: %q\n    grants: %q\n    hash: %q\n    created: %q\n",
			name, user.Display, user.Role, strings.Join(user.Grants, ","), user.Hash, user.Created)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	next := path + ".next"
	if err := os.WriteFile(next, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(next, path)
}

// AddUser adds a person. The first person in an empty store must be an admin,
// so a store is never only people who cannot manage it.
func AddUser(path string, user User, password string) error {
	if !ValidUserName(user.Name) {
		return fmt.Errorf("a name is lowercase letters, digits, '.', '_' or '-', starting with a letter")
	}
	if !ValidRole(user.Role) {
		return fmt.Errorf("the roles are %s", strings.Join(Roles, ", "))
	}
	if !validDisplay(user.Display) {
		return fmt.Errorf("a display name is one line of at most 80 characters, without quotes")
	}
	for _, grant := range user.Grants {
		if !validGrant(grant) {
			return fmt.Errorf("%q is not a grant; a grant is like mesh:* or aquarium:read", grant)
		}
	}
	if len(password) < MinPasswordLength {
		return fmt.Errorf("a password is at least %d characters", MinPasswordLength)
	}
	users, err := LoadUsers(path)
	if err != nil {
		return err
	}
	if _, taken := users[user.Name]; taken {
		return fmt.Errorf("%s is already a user", user.Name)
	}
	if len(users) == 0 && user.Role != "admin" {
		return fmt.Errorf("the first user is an admin")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	user.Hash = hash
	user.Created = time.Now().UTC().Format(time.RFC3339)
	if user.Display == "" {
		user.Display = user.Name
	}
	users[user.Name] = user
	return saveUsers(path, users)
}

// VerifyUser answers the person a name and password belong to.
func VerifyUser(path, name, password string) (User, error) {
	users, err := LoadUsers(path)
	if err != nil {
		return User{}, err
	}
	user, found := users[name]
	if !found {
		CheckPassword(dummyHash, password)
		return User{}, ErrBadCredentials
	}
	if !CheckPassword(user.Hash, password) {
		return User{}, ErrBadCredentials
	}
	return user, nil
}

// SetPassword gives a person a new password.
func SetPassword(path, name, password string) error {
	if len(password) < MinPasswordLength {
		return fmt.Errorf("a password is at least %d characters", MinPasswordLength)
	}
	users, err := LoadUsers(path)
	if err != nil {
		return err
	}
	user, found := users[name]
	if !found {
		return fmt.Errorf("%s is not a user", name)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	user.Hash = hash
	users[name] = user
	return saveUsers(path, users)
}

// UpdateUser changes a person's display name, role or grants; an empty value
// leaves that one as it is, and grants of "-" clears them. The last admin cannot
// stop being one.
func UpdateUser(path, name, display, role, grants string) error {
	users, err := LoadUsers(path)
	if err != nil {
		return err
	}
	user, found := users[name]
	if !found {
		return fmt.Errorf("%s is not a user", name)
	}
	if display != "" {
		if !validDisplay(display) {
			return fmt.Errorf("a display name is one line of at most 80 characters, without quotes")
		}
		user.Display = display
	}
	if role != "" {
		if !ValidRole(role) {
			return fmt.Errorf("the roles are %s", strings.Join(Roles, ", "))
		}
		if user.Role == "admin" && role != "admin" && countAdmins(users) == 1 {
			return fmt.Errorf("%s is the last admin", name)
		}
		user.Role = role
	}
	if grants == "-" {
		user.Grants = nil
	} else if grants != "" {
		list := splitList(grants)
		for _, grant := range list {
			if !validGrant(grant) {
				return fmt.Errorf("%q is not a grant; a grant is like mesh:* or aquarium:read", grant)
			}
		}
		user.Grants = list
	}
	users[name] = user
	return saveUsers(path, users)
}

// RemoveUser removes a person; the last admin stays.
func RemoveUser(path, name string) error {
	users, err := LoadUsers(path)
	if err != nil {
		return err
	}
	user, found := users[name]
	if !found {
		return fmt.Errorf("%s is not a user", name)
	}
	if user.Role == "admin" && countAdmins(users) == 1 {
		return fmt.Errorf("%s is the last admin", name)
	}
	delete(users, name)
	return saveUsers(path, users)
}

func countAdmins(users map[string]User) int {
	count := 0
	for _, user := range users {
		if user.Role == "admin" {
			count++
		}
	}
	return count
}
