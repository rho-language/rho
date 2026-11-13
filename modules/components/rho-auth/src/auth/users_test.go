package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPasswordIsCheckedAgainstItsHashOnly(t *testing.T) {
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hash, "correct") || !strings.HasPrefix(hash, "pbkdf2-sha256$600000$") {
		t.Fatalf("the hash says too much or too little: %s", hash)
	}
	if !CheckPassword(hash, "correct horse battery") || CheckPassword(hash, "correct horse batterY") {
		t.Fatal("a password and its hash do not agree")
	}
	again, _ := HashPassword("correct horse battery")
	if again == hash {
		t.Fatal("two hashes of one password are the same: the salt is not random")
	}
}

func TestTheStoreKeepsPeopleOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.yml")
	if err := AddUser(path, User{Name: "ana", Role: "operator"}, "a long password"); err == nil {
		t.Fatal("the first user may be other than an admin")
	}
	if err := AddUser(path, User{Name: "aurel", Display: "Aurel", Role: "admin"}, "short"); err == nil {
		t.Fatal("a short password was accepted")
	}
	if err := AddUser(path, User{Name: "aurel", Display: "Aurel", Role: "admin"}, "a long password"); err != nil {
		t.Fatal(err)
	}
	if err := AddUser(path, User{Name: "ana", Role: "operator", Grants: []string{"mesh:greenhouse", "aquarium:*"}}, "another long one"); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("the store is %v, not owner-only", info.Mode().Perm())
	}
	ana, err := VerifyUser(path, "ana", "another long one")
	if err != nil || ana.Role != "operator" || strings.Join(ana.Grants, ",") != "mesh:greenhouse,aquarium:*" || ana.Display != "ana" {
		t.Fatalf("ana is %+v, %v", ana, err)
	}
	if _, err := VerifyUser(path, "ana", "a long password"); err != ErrBadCredentials {
		t.Fatalf("a wrong password: %v", err)
	}
	if _, err := VerifyUser(path, "nobody", "a long password"); err != ErrBadCredentials {
		t.Fatalf("a missing name says something else: %v", err)
	}
	if err := UpdateUser(path, "aurel", "", "viewer", ""); err == nil {
		t.Fatal("the last admin stopped being one")
	}
	if err := RemoveUser(path, "aurel"); err == nil {
		t.Fatal("the last admin was removed")
	}
	if err := UpdateUser(path, "ana", "Ana", "", "energy:read"); err != nil {
		t.Fatal(err)
	}
	if err := SetPassword(path, "ana", "a new long password"); err != nil {
		t.Fatal(err)
	}
	ana, err = VerifyUser(path, "ana", "a new long password")
	if err != nil || ana.Display != "Ana" || strings.Join(ana.Grants, ",") != "energy:read" {
		t.Fatalf("after the changes ana is %+v, %v", ana, err)
	}
	if err := RemoveUser(path, "ana"); err != nil {
		t.Fatal(err)
	}
	users, _ := LoadUsers(path)
	if len(users) != 1 {
		t.Fatalf("%d users left", len(users))
	}
}
