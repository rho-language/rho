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
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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
	case "key":
		err = boardKey(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho bootstrap:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: rho bootstrap <init|status|token|key> [-config file] [-dir dir] [-token-file file|-]")
	os.Exit(2)
}

func bootstrap(args []string) error {
	fs := flag.NewFlagSet("rho bootstrap init", flag.ExitOnError)
	configPath := fs.String("config", "", "provisioning contract")
	dir := fs.String("dir", "", "identity directory")
	legacyToken := fs.String("token", "", "refused: a token on the command line is visible to every user of the machine")
	tokenFile := fs.String("token-file", "", "file holding the one-time Keeper enrollment token, or - for standard input")
	force := fs.Bool("force", false, "regenerate an existing identity")
	offline := fs.Bool("offline", false, "do not enroll with Keeper")
	fetchKey := fs.Bool("board-key", false, "after enrolling, fetch this node's board key from Mesh")
	meshURL := fs.String("mesh-url", "", "where Mesh's node-key endpoint is (default: the keeper's /app/mesh)")
	fs.Parse(args)
	if *legacyToken != "" {
		return errors.New("-token is refused: a command line is visible to every user of the machine (ps, /proc) " +
			"and is kept in shell history; pass the token with -token-file FILE, or -token-file - on standard input")
	}
	token := ""
	if *tokenFile != "" && !*offline {
		value, err := readToken(*tokenFile, os.Stdin)
		if err != nil {
			return err
		}
		token = value
	}
	after := func(contract schema.Provisioning, dir string, record identity) error {
		if err := enroll(contract, dir, record, token, *offline); err != nil {
			return err
		}
		if *fetchKey && token != "" && !*offline {
			return fetchBoardKey(contract, dir, record, *meshURL)
		}
		return nil
	}

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
				return after(contract, identityDir, existing)
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
	return after(contract, identityDir, record)
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
	if err := checkKeeperURL(contract.Keeper.URL); err != nil {
		return err
	}
	form := url.Values{
		"device_id":  {record.DeviceID},
		"public_key": {record.PublicKey},
		"token":      {token},
		"project":    {record.Project},
		"area":       {fmt.Sprint(record.Area)},
		"node":       {fmt.Sprint(record.Node)},
	}
	response, err := httpClient.PostForm(strings.TrimRight(contract.Keeper.URL, "/")+"/api/enroll", form)
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
// line the keeper's /api/enroll endpoint reads:
//
//	token|device|project|area|node|expires|flags
//
// The keeper marks lines used in the same file, so the line is added under the
// keeper's lock (a directory beside the file) and the file is replaced whole,
// through a temporary file and a rename: a token added while the keeper marks
// another is not lost, and a reader never sees half a file.
func token(args []string) error {
	fs := flag.NewFlagSet("rho bootstrap token", flag.ExitOnError)
	device := fs.String("device", "", "device id this token is for")
	project := fs.String("project", "", "project the device is placed in")
	area := fs.String("area", "0", "area id")
	node := fs.String("node", "0", "node id")
	ttl := fs.Duration("ttl", 15*time.Minute, "token lifetime")
	reenroll := fs.Bool("reenroll", false, "allow the token to replace a device that is already enrolled")
	file := fs.String("file", expandHome("~/.rho/keeper/enrollment.tokens"), "token file")
	fs.Parse(args)
	if *device == "" {
		return fmt.Errorf("token needs -device")
	}
	if !validName(*device) {
		return fmt.Errorf("-device must be letters, digits, '.', '-' or '_'")
	}
	if strings.ContainsAny(*project, "|\r\n") {
		return fmt.Errorf("-project cannot hold '|' or a line break")
	}
	if !placementNumber(*area) || !placementNumber(*node) {
		return fmt.Errorf("-area and -node are numbers of one to five digits")
	}
	if *ttl <= 0 {
		return fmt.Errorf("-ttl must be positive: a token without expiry is refused by the keeper")
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	value := hex.EncodeToString(raw)
	expires := time.Now().Add(*ttl).Unix()
	flags := ""
	if *reenroll {
		flags = "reenroll"
	}
	line := value + "|" + *device + "|" + *project + "|" + *area + "|" + *node + "|" +
		fmt.Sprint(expires) + "|" + flags
	if err := appendTokenLine(*file, line); err != nil {
		return err
	}
	fmt.Printf("token=%s\nexpires=%d\ndevice=%s\nproject=%s\narea=%s\nnode=%s\n", value, expires, *device, *project, *area, *node)
	return nil
}

// tokenLockTries and tokenLockWait are how long a token waits for the keeper's
// lock before taking it as left behind by a process that died holding it; the
// keeper waits as long.
const (
	tokenLockTries = 50
	tokenLockWait  = 100 * time.Millisecond
)

// appendTokenLine adds one line to the token file under the keeper's lock.
func appendTokenLine(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lock := path + ".lock"
	held := false
	for try := 0; try < tokenLockTries; try++ {
		if err := os.Mkdir(lock, 0700); err == nil {
			held = true
			break
		} else if !errors.Is(err, os.ErrExist) {
			return err
		}
		time.Sleep(tokenLockWait)
	}
	if !held {
		fmt.Fprintln(os.Stderr, "rho bootstrap: the token file's lock was held too long; taking it")
		_ = os.RemoveAll(lock)
		if err := os.Mkdir(lock, 0700); err != nil {
			return err
		}
	}
	defer os.Remove(lock)
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		existing = append(existing, '\n')
	}
	return writeAtomic(path, append(existing, []byte(line+"\n")...), 0600)
}

// writeAtomic replaces a file whole: a temporary file in the same directory,
// made with its mode before anything is written to it, then renamed over it.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validName is a device id the keeper accepts: it names the file its record is
// kept in.
func validName(name string) bool {
	return namePattern.MatchString(name) && name != "." && !strings.Contains(name, "..")
}

func placementNumber(text string) bool {
	if len(text) == 0 || len(text) > 5 {
		return false
	}
	for _, ch := range text {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// readToken reads a token from a file, or from standard input for "-". Only the
// first line counts, and a token is hex.
func readToken(path string, stdin io.Reader) (string, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(io.LimitReader(stdin, 4096))
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("reading the token: %w", err)
	}
	value := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	if value == "" {
		return "", fmt.Errorf("the token file is empty")
	}
	if strings.ContainsAny(value, "|& \t") {
		return "", fmt.Errorf("the token file does not hold a token")
	}
	return value, nil
}

// checkKeeperURL refuses to send a token or a credential where it could be read
// on the way: https, or http to this machine only.
func checkKeeperURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("keeper URL %q is not a URL", raw)
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		host := parsed.Hostname()
		if host == "localhost" {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
		return fmt.Errorf("keeper URL %s is plain http: the token and the credential would cross the network "+
			"in the clear; use https (http is accepted only for 127.0.0.1 or localhost)", raw)
	default:
		return fmt.Errorf("keeper URL %s: only https is accepted", raw)
	}
}

// httpClient is what the bootstrap talks to the keeper and Mesh with: bounded,
// and never following a redirect, which could send a credential elsewhere.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

/* -------------------------------------------------------------------------- */
/*  The board key                                                             */
/* -------------------------------------------------------------------------- */

var (
	uidPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)
	keyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// boardKey fetches this node's board key from Mesh with the credential the
// keeper issued at enrollment (`rho bootstrap key`).
func boardKey(args []string) error {
	fs := flag.NewFlagSet("rho bootstrap key", flag.ExitOnError)
	configPath := fs.String("config", "", "provisioning contract")
	dir := fs.String("dir", "", "identity directory")
	meshURL := fs.String("mesh-url", "", "where Mesh's node-key endpoint is (default: the keeper's /app/mesh)")
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
	record, err := readIdentity(filepath.Join(identityDir, "identity.json"))
	if err != nil {
		return err
	}
	return fetchBoardKey(contract, identityDir, record, *meshURL)
}

// meshKeyURL is Mesh's node-key endpoint: the one given, or Mesh behind the
// keeper's origin, which is where a node already trusts the TLS certificate.
func meshKeyURL(contract schema.Provisioning, given string) string {
	if given != "" {
		return strings.TrimRight(given, "/") + "/mesh/api/nodes/key"
	}
	return strings.TrimRight(contract.Keeper.URL, "/") + "/app/mesh/mesh/api/nodes/key"
}

// fetchBoardKey asks Mesh for the board id and key of the node this machine was
// enrolled as, and keeps them where the host node reads them: board.key (the
// key as `<epoch>:<32 hex digits>`, the key epoch being the one Mesh answers;
// the hex alone when an older Mesh names no epoch) and board.env (RH_SECURE_UID
// and RH_SECURE_KEY_FILE), both 0600. Neither is printed. A key the node's
// keystore kept beside the file (`board.key.keystore`, robohub-linux) is removed:
// the key Mesh hands out is the current one, and a stored key from another mesh
// must not stand in for it.
func fetchBoardKey(contract schema.Provisioning, dir string, record identity, given string) error {
	credential, err := os.ReadFile(filepath.Join(dir, "credential"))
	if err != nil {
		return fmt.Errorf("no keeper credential: enroll first (%w)", err)
	}
	endpoint := meshKeyURL(contract, given)
	if err := checkKeeperURL(endpoint); err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Node "+record.DeviceID+":"+strings.TrimSpace(string(credential)))
	response, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("fetching the board key failed: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<16))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var refusal struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &refusal)
		if refusal.Error == "" {
			refusal.Error = response.Status
		}
		return fmt.Errorf("Mesh refused the board key: %s", refusal.Error)
	}
	var answer struct {
		UID   string  `json:"uid"`
		Key   string  `json:"key"`
		Epoch *uint32 `json:"epoch"`
	}
	if err := json.Unmarshal(body, &answer); err != nil || !uidPattern.MatchString(answer.UID) ||
		!keyPattern.MatchString(answer.Key) {
		return fmt.Errorf("Mesh returned no board id and key")
	}
	keyText := answer.Key + "\n"
	if answer.Epoch != nil {
		keyText = fmt.Sprintf("%d:%s\n", *answer.Epoch, answer.Key)
	}
	keyPath, err := filepath.Abs(filepath.Join(dir, "board.key"))
	if err != nil {
		return err
	}
	if err := writeAtomic(keyPath, []byte(keyText), 0600); err != nil {
		return err
	}
	if err := os.Remove(keyPath + ".keystore"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("the old keystore beside the key could not be removed: %w", err)
	}
	env := "RH_SECURE_UID=" + answer.UID + "\nRH_SECURE_KEY_FILE=" + keyPath + "\n"
	if err := writeAtomic(filepath.Join(dir, "board.env"), []byte(env), 0600); err != nil {
		return err
	}
	fmt.Printf("board key stored: %s (uid %s)\n", keyPath, answer.UID)
	return nil
}

func loadContract(path string) (schema.Provisioning, error) {
	contract := schema.Provisioning{}
	if path == "" {
		for _, candidate := range []string{"/etc/rho/provisioning.json", expandHome("~/.rho/provisioning.json")} {
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

// writePrivate writes a file only its owner can read, with that mode from the
// moment it exists.
func writePrivate(path string, data []byte) error {
	return writeAtomic(path, data, 0600)
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
