//
// @file index.go
// @brief The repository's own list of what it holds, kept once per machine.
//
// A search over a vendor's registers is only trustworthy if it is a search over
// what the vendor actually publishes. So the list of files is fetched once and
// cached, and every lookup afterwards is against that: a name that is not in the
// index is a name the vendor does not publish, and there is nothing to fetch for it
// and nothing to guess.
//
// The list is one request and a few hundred kilobytes, which is why the whole
// repository is indexed rather than one vendor directory at a time: asking each of
// twenty-six vendors in turn would be twenty-six requests to answer one question,
// and would be rate-limited long before it was answered.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhodbg

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// indexFile is the name the cached list is kept under, beside the files it
// describes.
const indexFile = "index.json"

// How long a cached index is trusted.
//
// Long, because it is a list of files rather than the files: a new device takes
// weeks to appear and a moved one is rare, and a machine that re-fetched this on
// every session would be doing a network request for a question that has the same
// answer. A person who wants the current list asks for it by name.
const indexAge = 30 * 24 * time.Hour

// Index is what a repository publishes: every device it has a register map for,
// by the name it is published under.
type Index struct {
	// Devices maps a device's name, lowercased, to the paths it is published at.
	//
	// Lowercased because a lookup starts from a part number somebody wrote and the
	// vendor published it in whatever case they use — `STM32H753x`, `nRF52840`,
	// `MAX32625` — and a search that missed on case would report that a chip has no
	// register map on a machine that has just been told it does.
	//
	// A list of paths rather than one, because two vendors may publish a file of
	// the same name, and which of them a name refers to is not a thing this can
	// decide.
	Devices map[string][]string `json:"devices"`
	// Ref is the ref the list was read from, so a stale list says which one it is
	// rather than pretending to be current.
	Ref string `json:"ref"`
	// Repository is where it came from.
	Repository string `json:"repository"`
	// Read is when the list was fetched, so a cache has an age rather than being
	// trusted for ever.
	Read time.Time `json:"read"`
}

// DeviceNames is every published device, as the vendor writes it, in name order.
//
// Taken from the paths rather than from the map's keys, because the keys are
// lowercased for lookup and a listing that showed `stm32h753x` where the vendor
// publishes `STM32H753x` would have a person looking for a spelling that is not
// there.
func (i Index) DeviceNames() []string {
	seen := map[string]bool{}
	names := []string{}
	for _, paths := range i.Devices {
		for _, one := range paths {
			device := strings.TrimSuffix(path.Base(one), ".svd")
			if seen[device] {
				continue
			}
			seen[device] = true
			names = append(names, device)
		}
	}
	sort.Strings(names)
	return names
}

// published returns the paths a name is published at, matched without regard to
// case, because the index was keyed that way and a caller may hold either.
func (i Index) published(name string) ([]string, bool) {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	if paths, found := i.Devices[trimmed]; found {
		return paths, true
	}
	// A list that was built rather than fetched — an index handed to this, a test's
	// own — may be keyed however it was written, so the last thing to try is a scan.
	// It costs a pass over the list per name that missed, and a name that missed the
	// map at all is about to be reported as not published, which is a slower answer
	// by a long way than the miss.
	for key, paths := range i.Devices {
		if strings.ToLower(key) == trimmed {
			return paths, true
		}
	}
	return nil, false
}

// LoadIndex is the repository's list of what it holds, read from the cache when
// there is one to read and fetched once when there is not.
//
// A cached list is trusted for `indexAge` and no longer: a device takes weeks to
// appear and a moved one is rare, so a machine should not ask again on every
// session — and a list a month old is a list that has missed a release, which is
// exactly when a project needs it to be right.
//
// `refresh` is how a person asks for the current list rather than the one this
// machine last saw, which is the only thing that makes the cache a thing anybody
// would want to forget.
func LoadIndex(settings Settings, refresh bool) (Index, error) {
	endpoint, err := treeURL(settings.Repository, settings.Ref)
	if err != nil {
		// The repository cannot be listed. A list this machine already has is still
		// better than nothing: a device does not stop existing because the machine
		// asking about it is offline.
		if index, cached := readIndex(indexPath(settings)); cached && fresh(index, indexPath(settings)) {
			return index, nil
		}
		return Index{}, err
	}
	return loadIndexFrom(settings, endpoint, refresh)
}

// loadIndexFrom is the list, read from an endpoint that is given rather than derived.
//
// The derivation — a web address to the API that answers about it — belongs to the
// caller, which is what keeps `Settings` a statement of what a person configured and
// nothing else, and what lets this be pointed at a list that is not GitHub's: a
// test, or a mirror with an API of its own.
func loadIndexFrom(settings Settings, endpoint string, refresh bool) (Index, error) {
	cached := indexPath(settings)
	if !refresh && cached != "" {
		if index, ok := readIndex(cached); ok && fresh(index, cached) {
			return index, nil
		}
	}
	index, err := fetchIndex(settings, endpoint)
	if err != nil {
		// A list that was cached before is better than nothing on a machine with no
		// network, and a device's register map does not stop existing because the
		// machine is offline.
		if !refresh && cached != "" {
			if index, ok := readIndex(cached); ok {
				return index, nil
			}
		}
		return Index{}, err
	}
	writeIndex(cached, index)
	return index, nil
}

func indexPath(settings Settings) string {
	directory := settings.CacheDirectory()
	if directory == "" {
		return ""
	}
	return filepath.Join(directory, indexFile)
}

// fresh is whether a cached list is recent enough to answer with.
func fresh(index Index, path string) bool {
	// A list that says when it was read is judged on that; one that does not — an
	// index written by something else, or before this file recorded it — is judged on
	// its own timestamp, so a cache is never kept for ever because it lacks a field.
	if !index.Read.IsZero() {
		return time.Since(index.Read) < indexAge
	}
	info, err := os.Stat(path)
	return err == nil && time.Since(info.ModTime()) < indexAge
}

func readIndex(path string) (Index, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Index{}, false
	}
	index := Index{}
	if err := json.Unmarshal(data, &index); err != nil {
		return Index{}, false
	}
	if len(index.Devices) == 0 {
		return Index{}, false
	}
	return index, true
}

func writeIndex(path string, index Index) {
	if path == "" {
		return
	}
	data, err := json.MarshalIndent(index, "", " ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	// Written beside the cache and renamed into it, like a fetched register map:
	// the list is read by every session on the machine, and half of one would be a
	// list of devices that does not exist.
	temporary, err := os.CreateTemp(filepath.Dir(path), indexFile+".*.partial")
	if err != nil {
		return
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		os.Remove(temporary.Name())
		return
	}
	temporary.Close()
	_ = os.Rename(temporary.Name(), path)
}

// treeURL is the repository's file list at a ref, read from the API that answers
// it.
//
// The API and the web are two different hosts, and a repository a person names is
// a web address: `https://github.com/cmsis-svd/cmsis-svd-data`. Turning that into
// `api.github.com/repos/cmsis-svd/cmsis-svd-data` is what makes the list
// readable, and leaving it alone is a 404 from a host that has the repository and
// is not being asked the right question.
//
// A repository that is not a GitHub one has no such API to ask, and says so rather
// than failing with a 404: the list is how a part number is resolved, and a machine
// whose mirror cannot be indexed is a machine whose projects must name their
// register maps.
func treeURL(repository string, ref string) (string, error) {
	if ref == "" {
		ref = DefaultRef
	}
	base := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(repository), "/"), ".git")
	owner, name, found := githubRepository(base)
	if !found {
		return "", fmt.Errorf(
			"%s is not a GitHub repository, and only a GitHub one can be listed: "+
				"name the register map with `svd:` or `svd_source:` instead", base)
	}
	return fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1", owner, name, ref), nil
}

// githubRepository is the owner and name in a GitHub web address.
func githubRepository(url string) (string, string, bool) {
	rest, found := strings.CutPrefix(url, "https://github.com/")
	if !found {
		rest, found = strings.CutPrefix(url, "http://github.com/")
		if !found {
			return "", "", false
		}
	}
	rest = strings.Trim(rest, "/")
	owner, name, found := strings.Cut(rest, "/")
	if !found || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", "", false
	}
	return owner, name, true
}

// fetchIndex reads the repository's own list of what it holds.
//
// The git tree API rather than a listing per vendor, because it answers the whole
// question in one request and a session cannot afford twenty-six.
func fetchIndex(settings Settings, address string) (Index, error) {
	client := &http.Client{Timeout: fetchTimeout}
	response, err := client.Get(address)
	if err != nil {
		return Index{}, fmt.Errorf("read the list of register maps from %s: %w", address, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Index{}, fmt.Errorf("read the list of register maps from %s: the repository answered %s", address, response.Status)
	}

	document := struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}{}
	if err := json.NewDecoder(io.LimitReader(response.Body, fetchLimit)).Decode(&document); err != nil {
		return Index{}, fmt.Errorf("read the list of register maps from %s: %w", address, err)
	}
	if document.Truncated {
		// A truncated list is a list that would say a device does not exist because
		// the response ran out, which is the one answer that must never be wrong.
		return Index{}, fmt.Errorf("the list of register maps from %s is incomplete, so a lookup against it cannot be trusted", address)
	}

	index := Index{
		Ref: settings.Ref, Repository: settings.Repository,
		Devices: map[string][]string{}, Read: time.Now(),
	}
	for _, entry := range document.Tree {
		if entry.Type != "blob" || !strings.HasSuffix(entry.Path, ".svd") {
			continue
		}
		// Keyed the way a lookup arrives: a part number somebody wrote, which is not
		// in the vendor's case. The path keeps theirs, because that is the file to
		// fetch.
		device := strings.ToLower(strings.TrimSuffix(path.Base(entry.Path), ".svd"))
		index.Devices[device] = append(index.Devices[device], entry.Path)
	}
	if len(index.Devices) == 0 {
		return Index{}, fmt.Errorf("the list from %s holds no register maps at all, so this is not a device data repository", address)
	}
	return index, nil
}
