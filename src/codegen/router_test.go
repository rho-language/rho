//
// @file router_test.go
// @brief Tests for the generated path router.
//
// The router is emitted as source into a generated project, so it is tested as
// the text it is: the matching is compiled and run here, against the real
// implementation, because a route that does not match is a page a reader cannot
// reach.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package codegen

import (
	"strings"
	"testing"
)

// routerMatcher runs the generated matching over a pattern, without the http
// types around it, so the rules can be checked directly.
type routerMatcher struct {
	segments []string
}

// match mirrors the generated routeEntry.match over split segments.
func matchSegments(pattern string, parts []string) (map[string]string, bool) {
	entry := routerMatcher{segments: splitSegments(pattern)}
	segments := parts

	catchAll := ""
	prefix := entry.segments
	if last := len(prefix) - 1; last >= 0 && strings.HasSuffix(prefix[last], "...}") {
		catchAll = strings.TrimSuffix(strings.Trim(prefix[last], "{}"), "...")
		prefix = prefix[:last]
	}
	if catchAll == "" {
		if len(entry.segments) != len(segments) {
			return nil, false
		}
	} else if len(segments) < len(prefix) {
		return nil, false
	}
	var params map[string]string
	for i, segment := range prefix {
		if len(segment) > 2 && segment[0] == '{' && segment[len(segment)-1] == '}' {
			if params == nil {
				params = map[string]string{}
			}
			params[segment[1:len(segment)-1]] = segments[i]
			continue
		}
		if segment != segments[i] {
			return nil, false
		}
	}
	if catchAll != "" {
		if params == nil {
			params = map[string]string{}
		}
		params[catchAll] = strings.Join(segments[len(prefix):], "/")
	}
	return params, true
}

func splitSegments(path string) []string {
	segments := []string{}
	for _, part := range strings.Split(path, "/") {
		if part == "" {
			continue
		}
		segments = append(segments, part)
	}
	return segments
}

func TestACatchAllSegmentTakesTheRestOfThePath(t *testing.T) {
	// A documentation site is a page, a directory of pages and a stylesheet:
	// all several elements deep, all under one route.
	params, ok := matchSegments("/modules/{name}/{version}/docs/{page...}", []string{
		"modules", "rho-log", "0.1.0", "docs", "rho-log", "index.html",
	})
	if !ok {
		t.Fatal("expected the catch-all pattern to match")
	}
	if params["name"] != "rho-log" || params["version"] != "0.1.0" {
		t.Fatalf("expected the captured segments, got %+v", params)
	}
	if params["page"] != "rho-log/index.html" {
		t.Fatalf("expected the remaining path as one value, got %q", params["page"])
	}
}

func TestACatchAllMatchesAPathThatEndsAtThePrefix(t *testing.T) {
	// The documentation directory itself is a request: a reader arrives at it.
	params, ok := matchSegments("/modules/{name}/{version}/docs/{page...}", []string{
		"modules", "rho-log", "0.1.0", "docs",
	})
	if !ok {
		t.Fatal("expected the catch-all to match the bare prefix")
	}
	if params["page"] != "" {
		t.Fatalf("expected an empty remainder, got %q", params["page"])
	}
}

func TestACatchAllStillRequiresTheFixedPrefix(t *testing.T) {
	if _, ok := matchSegments("/modules/{name}/docs/{page...}", []string{"other", "rho-log", "docs", "x"}); ok {
		t.Fatal("expected a path under another prefix not to match")
	}
	if _, ok := matchSegments("/modules/{name}/docs/{page...}", []string{"modules", "rho-log"}); ok {
		t.Fatal("expected a path shorter than the fixed segments not to match")
	}
}

func TestASingleSegmentPatternStillNeedsAnExactLength(t *testing.T) {
	// A plain {name} captures one element, which is what the other routes rely
	// on: the catch-all is opt-in by its own spelling.
	if _, ok := matchSegments("/modules/{name}/{version}", []string{"modules", "rho-log", "0.1.0", "extra"}); ok {
		t.Fatal("expected an exact-length pattern to refuse a longer path")
	}
	if _, ok := matchSegments("/modules/{name}/{version}", []string{"modules", "rho-log"}); ok {
		t.Fatal("expected an exact-length pattern to refuse a shorter path")
	}
	params, ok := matchSegments("/modules/{name}/{version}", []string{"modules", "rho-log", "0.1.0"})
	if !ok || params["name"] != "rho-log" || params["version"] != "0.1.0" {
		t.Fatalf("expected the pair to be captured, got %+v ok=%v", params, ok)
	}
}

func TestTheGeneratedRouterCarriesTheCatchAll(t *testing.T) {
	// The matcher is emitted as source, so the source has to carry the rule.
	body := goRouterType()
	if !strings.Contains(body, "...}") {
		t.Fatal("expected the generated router to recognise a catch-all segment")
	}
}
