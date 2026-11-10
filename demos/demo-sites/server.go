package main

import (
	"bytes"
	"compress/gzip"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// site holds every brand's design-system export, plus the scripts they share.
// Embedded rather than mounted so the image runs on distroless/static with no
// volume and no filesystem assumptions — the same shape as the stub next door.
//
//	site/shared/    vendored React, ReactDOM and lucide, and the React shim
//	                the compiled kits import. Served beneath EVERY brand.
//	site/<brand>/   one brand's tokens, components, assets and ui_kits.
//
// One binary and one image for every brand: a brand is a directory and a
// --site flag, not a build.
//
//go:embed all:site
var site embed.FS

// sharedDir is the layer every brand sits on top of.
const sharedDir = "shared"

// member is the identity the site asserts to the agent.
//
// The site is the trusted EDGE: it owns the visitor's signed-in session, so it
// is the thing that knows who is asking. The AgentRuntime's
// externalAuth.edgeTrust reads these headers and does not re-verify them, which
// is exactly the arrangement a real deployment has behind an API gateway — and
// it means the demo needs no API key, no secret and no dashboard-minted JWT.
//
// There is no login here and there should not be: authenticating a fictional
// customer would be a page of theatre in front of the thing the demo is
// actually about.
type member struct {
	id    string
	email string
	roles string
}

// brand is one company's site.
type brand struct {
	// dir is the directory under site/, and the --site value.
	dir string
	// index is where "/" sends a visitor, relative to the site root: the
	// public marketing site, not the signed-in area, because that is how
	// anyone reaches a real one.
	//
	// A DIRECTORY, not index.html: http.FileServer 301s a request ending in
	// index.html back to the directory, so naming the file would cost every
	// visitor two redirects to reach the same page.
	index string
	// who is asserted to the agent when no --member-* flag overrides it.
	who member
}

// brands is every site the binary serves.
var brands = map[string]brand{
	"larkmere-mutual": {
		dir:   "larkmere-mutual",
		index: "ui_kits/marketing/",
		who:   member{id: "4471-889", email: "ellen.whitcomb@example.com", roles: "member"},
	},
	"wrenmoor-energy": {
		dir:   "wrenmoor-energy",
		index: "ui_kits/marketing/",
		// Account 40021187 is the account the stub's read_bill, read_usage
		// and switch_tariff know, and the one the intranet's session record
		// shows.
		who: member{id: "40021187", email: "amira.saleem@example.com", roles: "customer"},
	},
	"halvern-partners": {
		dir:   "halvern-partners",
		index: "ui_kits/marketing/",
		// The Practice Desk's signed-in delivery partner. No Halvern page
		// talks to an agent yet; this is who it will be.
		who: member{id: "hp-0042", email: "rhiannon.vale@example.com", roles: "staff"},
	},
}

// brandNames lists the valid --site values, sorted, for error messages.
func brandNames() []string {
	names := make([]string, 0, len(brands))
	for n := range brands {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// lookupBrand returns the named brand, with each non-empty field of override
// replacing its default identity.
func lookupBrand(name string, override member) (brand, error) {
	b, ok := brands[name]
	if !ok {
		return brand{}, fmt.Errorf("unknown site %q (known: %s)", name, strings.Join(brandNames(), ", "))
	}
	if override.id != "" {
		b.who.id = override.id
	}
	if override.email != "" {
		b.who.email = override.email
	}
	if override.roles != "" {
		b.who.roles = override.roles
	}
	return b, nil
}

// layered serves the brand's own tree first and the shared tree beneath it.
// A brand can override a shared file by shipping its own copy, and every URL a
// page uses stays relative to that page: the shared scripts appear at
// <site>/vendor/ exactly as if each brand had its own.
type layered struct {
	top, base fs.FS
}

func (l layered) Open(name string) (fs.File, error) {
	f, err := l.top.Open(name)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return f, err
	}
	return l.base.Open(name)
}

// siteFS returns the brand's served tree: its own directory over the shared one.
func siteFS(b brand) (fs.FS, error) {
	top, err := fs.Sub(site, path.Join("site", b.dir))
	if err != nil {
		return nil, err
	}
	// fs.Sub does not fail on a missing directory, so check: a brand with no
	// embedded tree would otherwise serve only the shared scripts and 404 its
	// own entry point.
	if _, err := fs.Stat(top, "."); err != nil {
		return nil, fmt.Errorf("site %q has no embedded tree: %w", b.dir, err)
	}
	base, err := fs.Sub(site, path.Join("site", sharedDir))
	if err != nil {
		return nil, err
	}
	return layered{top: top, base: base}, nil
}

// assistantPath is where the browser opens its WebSocket. The site proxies it
// to the agent rather than letting the page dial the facade directly: the
// facade's external listener rejects an unauthenticated browser, and a page
// that held a credential would be handing it to every visitor.
const assistantPath = "/api/assistant/ws"

// The two account-system routes the assistant panel uses besides the socket,
// proxied to --backend-url (the stub that also backs the agent's tools):
//
//	consentPath   the customer clicked "Switch me": record that consent and
//	              hand back the consent_id the agent must quote to switch.
//	receiptsPath  the tool calls the account system served this session, so
//	              the panel can show what the agent actually read.
//
// Both exist because the facade sends the browser CLIENT tool calls only:
// server-side tool calls never cross the websocket, so neither the receipt
// nor the proof of a click can come from the conversation itself.
const (
	consentPath  = "/api/assistant/consent"
	receiptsPath = "/api/assistant/receipts"
)

// newServer returns the site handler: the embedded site, the assistant proxy,
// a redirect from the root to the brand's public site, and a health endpoint.
// backendURL may be empty, in which case the consent and receipt routes 404.
func newServer(b brand, agentURL, backendURL string) (http.Handler, error) {
	// A .jsx is served as source (the pages load the compiled app.js). Go's
	// mime package has no entry for .jsx, so without this it is served as
	// application/octet-stream — which some browsers refuse to eval, and the
	// page then renders blank with only a console error to say why.
	if err := mime.AddExtensionType(".jsx", "text/javascript; charset=utf-8"); err != nil {
		return nil, err
	}
	// Not in Go's built-in table on every platform; pinned so a font is never
	// served as octet-stream.
	if err := mime.AddExtensionType(".woff2", "font/woff2"); err != nil {
		return nil, err
	}

	root, err := siteFS(b)
	if err != nil {
		return nil, err
	}
	files := gzipped(root, http.FileServer(http.FS(root)))

	proxy, err := assistantProxy(agentURL, b.who)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle(assistantPath, proxy)
	if backendURL != "" {
		account, err := accountProxy(backendURL, b.who)
		if err != nil {
			return nil, err
		}
		mux.Handle(consentPath, account)
		mux.Handle(receiptsPath, account)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if path.Clean(r.URL.Path) == "/" {
			// A RELATIVE Location, set by hand: http.Redirect would make it
			// absolute. Behind a path prefix the browser resolves it within
			// the prefix, where "/ui_kits/..." would escape it.
			w.Header().Set("Location", b.index)
			w.WriteHeader(http.StatusFound)
			return
		}
		files.ServeHTTP(w, r)
	})
	return mux, nil
}

// compressible are the extensions worth gzipping: text. Images and fonts are
// already compressed.
var compressible = map[string]bool{
	".html": true, ".js": true, ".jsx": true, ".css": true, ".svg": true, ".json": true, ".md": true,
}

// gzipped serves a compressible file gzipped to a client that accepts it, and
// everything else through next. The site is embedded and never changes, so
// each file is compressed once, on first request, and kept: the container's
// CPU limit is a tenth of a core, and compressing the 640 KB icon library on
// every request would cost more than sending it.
//
// A directory (".../") is served by next, uncompressed: its index.html is a
// few hundred bytes.
func gzipped(root fs.FS, next http.Handler) http.Handler {
	var cache sync.Map // path -> []byte
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		// Embedded files carry no modification time, so without this the
		// browser fetches everything again on every visit. The vendored
		// libraries, fonts and photos change only with a new image.
		if strings.HasPrefix(name, "vendor/") || strings.HasPrefix(name, "assets/") || strings.HasPrefix(name, "tokens/") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		}
		if strings.HasSuffix(r.URL.Path, "/") || !compressible[path.Ext(name)] ||
			!strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		body, ok := cache.Load(name)
		if !ok {
			raw, err := fs.ReadFile(root, name)
			if err != nil {
				next.ServeHTTP(w, r) // the file server's own 404
				return
			}
			var buf bytes.Buffer
			zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
			_, _ = zw.Write(raw)
			_ = zw.Close()
			body, _ = cache.LoadOrStore(name, buf.Bytes())
		}
		b := body.([]byte)
		h := w.Header()
		h.Set("Content-Type", mime.TypeByExtension(path.Ext(name)))
		h.Set("Content-Encoding", "gzip")
		h.Set("Vary", "Accept-Encoding")
		h.Set("Content-Length", strconv.Itoa(len(b)))
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(b)
	})
}

// assistantProxy forwards the browser's WebSocket to the agent facade, adding
// the identity headers the AgentRuntime's edgeTrust validator reads.
//
// httputil.ReverseProxy handles the 101 upgrade itself, so this needs no
// WebSocket library and no framing code — which matters, because a hand-rolled
// relay is where you get to invent your own bugs about close codes and
// half-open connections.
//
// The header names are edgeTrust's defaults (x-user-id, x-user-email,
// x-user-roles). The roles header name is NOT configurable on the CRD side, so
// don't rename it here expecting the facade to follow.
func assistantProxy(agentURL string, m member) (http.Handler, error) {
	target, err := url.Parse(agentURL)
	if err != nil {
		return nil, err
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			// The facade serves the conversation at /ws; the browser asks for
			// assistantPath. Rewritten rather than exposed, so the page never
			// learns the agent's own routing.
			pr.Out.URL.Path = "/ws"

			// Drop the browser's Origin.
			//
			// The facade same-origin-checks WebSocket upgrades: it compares
			// Origin against its own Host, so a forwarded `http://<site>`
			// never matches `<agent>:8080` and every browser connection is
			// refused with 403. It explicitly permits requests with NO Origin,
			// for non-browser clients — which, from the facade's side, is
			// exactly what this proxy is.
			//
			// Dropping it moves the CSRF check rather than removing it: the
			// check now happens in sameOrigin below, against the site's own
			// host, which is the only host that can meaningfully judge it.
			// Browsers do not apply same-origin to WebSockets, so without that
			// check any site could open a socket here and talk to the agent as
			// this customer.
			pr.Out.Header.Del("Origin")

			// The identity the edge asserts, set on the OUTBOUND request only.
			// Whatever the browser sent under these names is overwritten: a
			// visitor must not be able to choose who the agent thinks they are.
			pr.Out.Header.Set("x-user-id", m.id)
			pr.Out.Header.Set("x-user-email", m.email)
			pr.Out.Header.Set("x-user-roles", m.roles)
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			// Loud, and on the way out. A silent proxy failure looks exactly
			// like a model that declined to answer — which is the single most
			// misleading thing this demo could do.
			log.Printf("assistant proxy failed: %v", err)
			http.Error(w, "assistant unavailable", http.StatusBadGateway)
		},
	}

	// Same-origin gate, in front of the proxy. This is the check the facade
	// would have done, relocated to the only place that can do it correctly
	// now that Origin is stripped on the way out.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			log.Printf("assistant: refused cross-origin upgrade from %q", r.Header.Get("Origin"))
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		proxy.ServeHTTP(w, r)
	}), nil
}

// accountProxy forwards the panel's consent and receipt requests to the account
// back end, asserting the customer's identity the same way the runtime does on
// a tool call (x-omnia-user-id), so the back end files a consent against — and
// shows receipts for — the signed-in customer and nobody else.
//
//	POST <site>/api/assistant/consent   -> POST <backend>/wren/consent
//	GET  <site>/api/assistant/receipts  -> GET  <backend>/wren/receipts
func accountProxy(backendURL string, m member) (http.Handler, error) {
	target, err := url.Parse(backendURL)
	if err != nil {
		return nil, err
	}
	routes := map[string]struct{ method, path string }{
		consentPath:  {http.MethodPost, "/wren/consent"},
		receiptsPath: {http.MethodGet, "/wren/receipts"},
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = routes[pr.In.URL.Path].path
			pr.Out.Header.Del("Origin")
			pr.Out.Header.Del("Cookie")
			// Overwritten, never passed through: the browser does not get to
			// say whose consent this is.
			pr.Out.Header.Set("X-Omnia-User-Id", m.id)
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			log.Printf("account proxy failed: %v", err)
			http.Error(w, "account system unavailable", http.StatusBadGateway)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, ok := routes[r.URL.Path]
		if !ok || r.Method != route.method {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// A consent is a state change made from a browser, so the same CSRF
		// gate as the socket: another site must not be able to consent on the
		// customer's behalf.
		if !sameOrigin(r) {
			log.Printf("account: refused cross-origin %s from %q", r.URL.Path, r.Header.Get("Origin"))
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		proxy.ServeHTTP(w, r)
	}), nil
}

// sameOrigin reports whether the request's Origin, if any, matches the host it
// was sent to.
//
// An ABSENT Origin passes: non-browser callers (curl, a test harness) do not
// send one, and the demo is reachable from both. A browser always sends it, so
// the case this exists to refuse — another site opening a socket here — is
// always covered.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}
