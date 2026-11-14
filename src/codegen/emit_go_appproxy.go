//
// @file emit_go_appproxy.go
// @brief The app_proxy middleware: a supervisor's applications behind its own origin.
//
// @project Rho Programming Language
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"fmt"
	"strings"
)

// emitAppProxyMiddleware renders the wrapper that serves /<prefix>/<name>/...
// from the application named <name>, on its loopback port.
//
// It is how one origin carries a supervisor and the applications it runs. A
// browser's cookies are scoped by host and not by port, so applications on
// ports of their own each receive every other one's cookies — the
// supervisor's admin session among them. Behind this proxy an application is
// reached through the supervisor's origin, its cookies are scoped to its own
// path, and the supervisor's own cookie (strip_cookie) is never forwarded.
//
// The applications are listed in apps_file, one `name port` per line, which the
// supervisor writes; it is read when it changes, so an application installed
// is reachable without a restart. An application writes root-relative links
// ("/water", hx-post="/rules"), so HTML, a Location header and a cookie's Path
// are rewritten onto the prefix; X-Forwarded-Prefix, -Host and -Proto tell it
// where it is reached, for what it builds itself. Responses stream, so a
// server-sent event feed is relayed as it is written.
func emitAppProxyMiddleware(name string, spec MiddlewareSpec) string {
	prefix := "/" + strings.Trim(option(spec, "prefix", "/app/"), "/") + "/"
	appsFile := option(spec, "apps_file", "~/.rho/keeper/proxy.txt")
	strip := option(spec, "strip_cookie", "")
	var b strings.Builder
	fmt.Fprintf(&b, `const (
	appProxyPrefix = %q
	appProxyFile   = %q
	appProxyStrip  = %q
)

var (
	appProxyMu      sync.Mutex
	appProxyPorts   = map[string]string{}
	appProxyStamp   time.Time
	appProxyAttrs   = regexp.MustCompile(`+"`"+`(\s(?:href|src|action|formaction|poster|hx-get|hx-post|hx-put|hx-patch|hx-delete|hx-push-url|sse-connect|data-href|data-url)=)(["'])/`+"`"+`)
	appProxyCSSURL  = regexp.MustCompile(`+"`"+`(url\()(["']?)/`+"`"+`)
)

// appProxyPort is the loopback port of a listed application, or "".
func appProxyPort(name string) string {
	path := appProxyFile
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	appProxyMu.Lock()
	defer appProxyMu.Unlock()
	if !info.ModTime().Equal(appProxyStamp) {
		ports := map[string]string{}
		if data, readErr := os.ReadFile(path); readErr == nil {
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 2 {
					if n, convErr := strconv.Atoi(fields[1]); convErr == nil && n > 0 && n < 65536 {
						ports[fields[0]] = fields[1]
					}
				}
			}
		}
		appProxyPorts, appProxyStamp = ports, info.ModTime()
	}
	return appProxyPorts[name]
}

// appProxyRewrite turns root-relative links of a page into links under base.
// A protocol-relative "//host" link is another site's and is left alone.
func appProxyRewrite(body []byte, base string) []byte {
	// A new slice for each match: appending to the match would write into the
	// page it was found in.
	swap := func(match []byte) []byte {
		return []byte(string(match[:len(match)-1]) + base + "/")
	}
	out := appProxyAttrs.ReplaceAllFunc(body, swap)
	out = appProxyCSSURL.ReplaceAllFunc(out, swap)
	// A "//host" link became base + "//host" above: it is another site's, and
	// is put back as it was.
	return bytes.ReplaceAll(out, []byte(base+"//"), []byte("//"))
}

func %[4]s(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, appProxyPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, appProxyPrefix)
		name, tail, hasTail := strings.Cut(rest, "/")
		port := appProxyPort(name)
		if name == "" || port == "" {
			http.NotFound(w, r)
			return
		}
		base := appProxyPrefix + name
		if !hasTail {
			http.Redirect(w, r, base+"/", http.StatusFound)
			return
		}
		target, _ := url.Parse("http://127.0.0.1:" + port)
		proxy := &httputil.ReverseProxy{
			FlushInterval: -1,
			Director: func(out *http.Request) {
				out.URL.Scheme = target.Scheme
				out.URL.Host = target.Host
				out.URL.Path = "/" + tail
				out.URL.RawPath = ""
				out.Host = r.Host
				proto := "http"
				if r.TLS != nil {
					proto = "https"
				}
				out.Header.Set("X-Forwarded-Host", r.Host)
				out.Header.Set("X-Forwarded-Proto", proto)
				out.Header.Set("X-Forwarded-Prefix", base)
				// The supervisor's session is the supervisor's: an application
				// behind it never sees that cookie.
				if appProxyStrip != "" {
					cookies := out.Cookies()
					out.Header.Del("Cookie")
					for _, c := range cookies {
						if c.Name != appProxyStrip {
							out.AddCookie(c)
						}
					}
				}
			},
			ModifyResponse: func(resp *http.Response) error {
				if location := resp.Header.Get("Location"); strings.HasPrefix(location, base+"/") || location == base {
					// Already under the prefix: the application built it from
					// X-Forwarded-Prefix.
				} else if strings.HasPrefix(location, "/") && !strings.HasPrefix(location, "//") {
					resp.Header.Set("Location", base+location)
				} else if strings.HasPrefix(location, "http://127.0.0.1:"+port+"/") {
					resp.Header.Set("Location", base+strings.TrimPrefix(location, "http://127.0.0.1:"+port))
				}
				// An application's cookie is its own: scoped to its path.
				if cookies := resp.Header.Values("Set-Cookie"); len(cookies) > 0 {
					resp.Header.Del("Set-Cookie")
					for _, line := range cookies {
						parts := strings.Split(line, ";")
						kept := []string{parts[0]}
						for _, attribute := range parts[1:] {
							if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(attribute)), "path=") {
								kept = append(kept, attribute)
							}
						}
						resp.Header.Add("Set-Cookie", strings.Join(kept, ";")+"; Path="+base+"/")
					}
				}
				contentType := resp.Header.Get("Content-Type")
				if !strings.HasPrefix(contentType, "text/html") && !strings.HasPrefix(contentType, "text/css") {
					return nil
				}
				if resp.Header.Get("Content-Encoding") != "" {
					return nil
				}
				body, err := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					return err
				}
				body = appProxyRewrite(body, base)
				resp.Body = io.NopCloser(bytes.NewReader(body))
				resp.ContentLength = int64(len(body))
				resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
				return nil
			},
			ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
				http.Error(w, "the application "+name+" is not answering", http.StatusBadGateway)
			},
		}
		proxy.ServeHTTP(w, r)
	})
}

`, prefix, appsFile, strip, name)
	return b.String()
}
