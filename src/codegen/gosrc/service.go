//go:build ignore

// The host half of a service that calls other services: its identity, an HTTP
// client that carries it, and a follower for another service's event stream.
//
// This file is not compiled into the generator. Everything after the marker line
// is written into a generated Go program that calls rho_service_token,
// rho_http_request or rho_sse_follow, with the imports the generator declares
// for it. It is a real Go file so that it is read, formatted and vetted as one.
//
// A service learns who it is from its environment, set by whoever started it:
//
//	RHO_CLIENT_ID           the client id it was registered under
//	RHO_CLIENT_SECRET_FILE  a file, readable by its owner only, holding the secret
//	RHO_AUTH_URL            the issuer's base URL; tokens come from POST /auth/service-token
//
// No secret or token is ever put on a command line or in a URL.

package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// --- emitted below ---

// rhoServiceTokens caches one token per audience and scope set until it is 80 %
// through its lifetime.
var rhoServiceTokens = struct {
	sync.Mutex
	held map[string]rhoHeldToken
}{held: map[string]rhoHeldToken{}}

type rhoHeldToken struct {
	token   string
	renewAt time.Time
}

var rhoServiceClient = &http.Client{Timeout: 10 * time.Second}

// rhoServiceToken is a token for audience with scopes, from the issuer, or ""
// when the service has no credential or the issuer refused it.
func rhoServiceToken(audience string, scopes string) string {
	key := audience + "|" + scopes
	rhoServiceTokens.Lock()
	defer rhoServiceTokens.Unlock()
	if held, ok := rhoServiceTokens.held[key]; ok && time.Now().Before(held.renewAt) {
		return held.token
	}
	id := os.Getenv("RHO_CLIENT_ID")
	issuer := strings.TrimRight(os.Getenv("RHO_AUTH_URL"), "/")
	secretFile := os.Getenv("RHO_CLIENT_SECRET_FILE")
	if id == "" || issuer == "" || secretFile == "" {
		return ""
	}
	raw, err := os.ReadFile(secretFile)
	if err != nil {
		return ""
	}
	form := url.Values{
		"client_id":     {id},
		"client_secret": {strings.TrimSpace(string(raw))},
		"audience":      {audience},
	}
	if scopes != "" {
		form.Set("scopes", scopes)
	}
	response, err := rhoServiceClient.PostForm(issuer+"/auth/service-token", form)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ""
	}
	var answer struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expires_in"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&answer) != nil || answer.Token == "" {
		return ""
	}
	lifetime := time.Duration(answer.ExpiresIn) * time.Second
	if lifetime <= 0 {
		lifetime = time.Minute
	}
	rhoServiceTokens.held[key] = rhoHeldToken{token: answer.Token, renewAt: time.Now().Add(lifetime * 8 / 10)}
	return answer.Token
}

// rhoHttpRequest sends one request and writes the response body to outputPath.
// With an audience, it carries a service token for it. The answer is the HTTP
// status, or 0 when no response came back (or a token was needed and could not
// be had).
func rhoHttpRequest(method string, address string, body string, contentType string,
	audience string, scopes string, outputPath string) uint32 {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, address, reader)
	if err != nil {
		return 0
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	request.Header.Set("Accept", "application/json")
	if audience != "" {
		token := rhoServiceToken(audience, scopes)
		if token == "" {
			return 0
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := rhoServiceClient.Do(request)
	if err != nil {
		return 0
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if outputPath != "" {
		_ = os.MkdirAll(filepath.Dir(outputPath), 0o700)
		_ = os.WriteFile(outputPath, data, 0o600)
	}
	return uint32(response.StatusCode)
}

// rhoSseFollowers are the streams already followed, so asking twice for the
// same stream into the same file starts one follower, not two.
var rhoSseFollowers = struct {
	sync.Mutex
	started map[string]bool
}{started: map[string]bool{}}

// rhoSseFollowLimit is the size a followed feed reaches before it starts again;
// a reader follows it by offset and starts over when it shrinks.
const rhoSseFollowLimit = 256 << 10

// rhoSseFollow follows another service's event stream into a local file, one
// {"event","data"} line per event, the shape sse_events serves. It returns at
// once; the follower reconnects for as long as the program runs, with a fresh
// token each time and the last event id it saw.
func rhoSseFollow(address string, audience string, scopes string, path string) bool {
	key := address + "\x00" + path
	rhoSseFollowers.Lock()
	if rhoSseFollowers.started[key] {
		rhoSseFollowers.Unlock()
		return true
	}
	rhoSseFollowers.started[key] = true
	rhoSseFollowers.Unlock()
	go rhoSseFollowLoop(address, audience, scopes, path)
	return true
}

func rhoSseFollowLoop(address string, audience string, scopes string, path string) {
	client := &http.Client{} // a stream has no deadline; a dead peer is noticed by its end
	lastID := ""
	backoff := time.Second
	for {
		connected := rhoSseFollowOnce(client, address, audience, scopes, path, &lastID)
		if connected {
			backoff = time.Second
		} else if backoff < time.Minute {
			backoff *= 2
		}
		time.Sleep(backoff)
	}
}

// rhoSseFollowOnce holds one connection, and reports whether it got as far as a
// stream, which is what decides whether the next attempt waits longer.
func rhoSseFollowOnce(client *http.Client, address, audience, scopes, path string, lastID *string) bool {
	request, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		return false
	}
	request.Header.Set("Accept", "text/event-stream")
	if *lastID != "" {
		request.Header.Set("Last-Event-ID", *lastID)
	}
	if audience != "" {
		token := rhoServiceToken(audience, scopes)
		if token == "" {
			return false
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusUnauthorized {
			// The token may have been revoked or expired early; the next attempt
			// asks for a new one.
			rhoServiceTokens.Lock()
			delete(rhoServiceTokens.held, audience+"|"+scopes)
			rhoServiceTokens.Unlock()
		}
		return false
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	reader := bufio.NewReaderSize(response.Body, 64<<10)
	event, id := "", ""
	var data []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return true
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if len(data) > 0 {
				if event == "" {
					event = "message"
				}
				rhoSseFollowAppend(path, event, strings.Join(data, "\n"))
				if id != "" {
					*lastID = id
				}
			}
			event, id, data = "", "", nil
		case strings.HasPrefix(line, ":"):
			// A comment, which a server sends to keep the connection open.
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "id:"):
			id = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
}

func rhoSseFollowAppend(path string, event string, data string) {
	line, err := json.Marshal(map[string]string{"event": event, "data": data})
	if err != nil {
		return
	}
	if info, err := os.Stat(path); err == nil && info.Size() > rhoSseFollowLimit {
		_ = os.Truncate(path, 0)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = file.Write(append(line, '\n'))
	_ = file.Close()
}
