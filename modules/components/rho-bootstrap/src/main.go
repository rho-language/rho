//
// @file main.go
// @brief rho-bootstrap — first boot identity and credential bootstrap.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rho/src/schema"
)

type identity struct {
	InstanceID string `json:"instance_id"`
	DeviceID   string `json:"device_id"`
	Kind       string `json:"kind"`
	Project    string `json:"project"`
	Area       uint32 `json:"area"`
	Node       uint32 `json:"node"`
	PublicKey  string `json:"public_key"`
	Created    string `json:"created"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "init", "bootstrap":
		err = bootstrap(os.Args[2:])
	case "status":
		err = status(os.Args[2:])
	case "token":
		err = token(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho bootstrap:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: rho bootstrap <init|status|token> [-config file] [-dir dir] [-token token]")
	os.Exit(2)
}

func bootstrap(args []string) error {
	fs := flag.NewFlagSet("rho bootstrap init", flag.ExitOnError)
	configPath := fs.String("config", "", "provisioning contract")
	dir := fs.String("dir", "", "identity directory")
	token := fs.String("token", "", "one-time Keeper enrollment token")
	force := fs.Bool("force", false, "regenerate an existing identity")
	offline := fs.Bool("offline", false, "do not enroll with Keeper")
	fs.Parse(args)

	contract, err := loadContract(*configPath)
	if err != nil {
		return err
	}
	if err := schema.ValidateProvisioning(contract); err != nil {
		return err
	}
	identityDir := *dir
	if identityDir == "" {
		identityDir = expandHome(contract.Secrets.Store)
	}
	if identityDir == "" {
		identityDir = expandHome("~/.rho/identity")
	}
	if err := os.MkdirAll(identityDir, 0700); err != nil {
		return err
	}
	_ = os.Chmod(identityDir, 0700)

	identityPath := filepath.Join(identityDir, "identity.json")
	keyPath := filepath.Join(identityDir, "identity.key")
	if !*force {
		if existing, err := readIdentity(identityPath); err == nil {
			if _, err := os.Stat(keyPath); err == nil {
				fmt.Printf("identity exists: %s (%s)\n", existing.InstanceID, existing.DeviceID)
				return enroll(contract, identityDir, existing, *token, *offline)
			}
		}
	}

	instanceID, err := newUUID()
	if err != nil {
		return err
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	record := identity{
		InstanceID: instanceID,
		DeviceID:   contract.Device.ID,
		Kind:       contract.Device.Kind,
		Project:    contract.Placement.Project,
		Area:       contract.Placement.Area,
		Node:       contract.Placement.Node,
		PublicKey:  base64.StdEncoding.EncodeToString(public),
		Created:    time.Now().UTC().Format(time.RFC3339),
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := writePrivate(identityPath, append(encoded, '\n')); err != nil {
		return err
	}
	if err := writePrivate(keyPath, []byte(base64.StdEncoding.EncodeToString(private.Seed())+"\n")); err != nil {
		return err
	}
	if err := writePublic(filepath.Join(identityDir, "identity.pub"), []byte(record.PublicKey+"\n")); err != nil {
		return err
	}
	fmt.Printf("identity created: %s (%s)\n", record.InstanceID, record.DeviceID)
	return enroll(contract, identityDir, record, *token, *offline)
}

func status(args []string) error {
	fs := flag.NewFlagSet("rho bootstrap status", flag.ExitOnError)
	dir := fs.String("dir", expandHome("~/.rho/identity"), "identity directory")
	fs.Parse(args)
	record, err := readIdentity(filepath.Join(*dir, "identity.json"))
	if err != nil {
		return err
	}
	fmt.Printf("instance=%s\ndevice=%s\nkind=%s\nproject=%s\narea=%d\nnode=%d\npublic_key=%s\ncreated=%s\n",
		record.InstanceID, record.DeviceID, record.Kind, record.Project,
		record.Area, record.Node, record.PublicKey, record.Created)
	return nil
}

func enroll(contract schema.Provisioning, dir string, record identity, token string, offline bool) error {
	if offline || token == "" {
		return nil
	}
	if contract.Keeper.URL == "" {
		return fmt.Errorf("a token was given but the contract has no keeper URL")
	}
	form := url.Values{
		"device_id":  {record.DeviceID},
		"public_key": {record.PublicKey},
		"token":      {token},
		"project":    {record.Project},
		"area":       {fmt.Sprint(record.Area)},
		"node":       {fmt.Sprint(record.Node)},
	}
	response, err := http.PostForm(strings.TrimRight(contract.Keeper.URL, "/")+"/api/enroll", form)
	if err != nil {
		return fmt.Errorf("keeper enrollment failed: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<16))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("keeper enrollment refused: %s", strings.TrimSpace(string(body)))
	}
	var answer struct {
		Credential string `json:"credential"`
	}
	if err := json.Unmarshal(body, &answer); err != nil || answer.Credential == "" {
		return fmt.Errorf("keeper enrollment returned no credential")
	}
	if err := writePrivate(filepath.Join(dir, "credential"), []byte(answer.Credential+"\n")); err != nil {
		return err
	}
	fmt.Println("enrolled with keeper")
	return nil
}

// token creates a one-time enrollment token on the keeper host. It writes the
// line the keeper's /api/enroll endpoint reads.
func token(args []string) error {
	fs := flag.NewFlagSet("rho bootstrap token", flag.ExitOnError)
	device := fs.String("device", "", "device id this token is for")
	project := fs.String("project", "", "project the device may claim")
	area := fs.String("area", "0", "area id")
	node := fs.String("node", "0", "node id")
	ttl := fs.Duration("ttl", 15*time.Minute, "token lifetime")
	file := fs.String("file", expandHome("~/.rho/keeper/enrollment.tokens"), "token file")
	fs.Parse(args)
	if *device == "" {
		return fmt.Errorf("token needs -device")
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	value := hex.EncodeToString(raw)
	expires := time.Now().Add(*ttl).Unix()
	line := value + "|" + *device + "|" + *project + "|" + *area + "|" + *node + "|" +
		fmt.Sprint(expires) + "|"
	if err := os.MkdirAll(filepath.Dir(*file), 0700); err != nil {
		return err
	}
	existing, _ := os.ReadFile(*file)
	if err := writePrivate(*file, append(existing, []byte(line+"\n")...)); err != nil {
		return err
	}
	fmt.Printf("token=%s\nexpires=%d\ndevice=%s\nproject=%s\n", value, expires, *device, *project)
	return nil
}

func loadContract(path string) (schema.Provisioning, error) {
	contract := schema.Provisioning{}
	if path == "" {
		for _, candidate := range []string{"/etc/robohub/provisioning.json", expandHome("~/.rho/provisioning.json")} {
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
		}
	}
	if path == "" {
		return contract, fmt.Errorf("no provisioning contract found")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return contract, err
	}
	if err := json.Unmarshal(data, &contract); err != nil {
		return contract, fmt.Errorf("provisioning contract is not JSON: %w", err)
	}
	return contract, nil
}

func readIdentity(path string) (identity, error) {
	record := identity{}
	data, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, err
	}
	if record.InstanceID == "" || record.DeviceID == "" {
		return record, fmt.Errorf("identity is incomplete")
	}
	return record, nil
}

func writePrivate(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

func writePublic(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	return os.Chmod(path, 0644)
}

func newUUID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	raw := hex.EncodeToString(bytes[:])
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32], nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
