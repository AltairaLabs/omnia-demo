package main

import (
	"embed"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
)

// site is the design-system export. Embedded rather than mounted so the image
// runs on distroless/static with no volume and no filesystem assumptions —
// the same shape as the mortgage stub next door.
//
// The tree is excluded by the repo-root .dockerignore's blanket `**` unless it
// is explicitly re-included there. When it is not, this directive fails the
// BUILD with "contains no embeddable files" rather than failing at runtime.
//
//go:embed all:site
var site embed.FS

// portalIndex is where the kit's own entry point lives inside the export.
//
// The DIRECTORY, not index.html: http.FileServer 301s a request ending in
// index.html back to the directory, so naming the file here would cost every
// visitor two redirects to reach the same page.
const portalIndex = "/ui_kits/portal/"

// siteIndex is larkmere.com — the public marketing site, and what "/" serves.
//
// The root used to redirect straight into the signed-in portal, which is not
// how anyone reaches a bank. Opening on the public site and then moving to the
// member area is both more realistic on camera and the shape a future demo
// needs if it wants to show anything happening before sign-in.
const siteIndex = "/ui_kits/marketing/"

// member identity the portal asserts to the agent.
//
// The portal is the trusted EDGE: it owns the member's signed-in session, so it
// is the thing that knows who is asking. The AgentRuntime's
// externalAuth.edgeTrust reads these headers and does not re-verify them, which
// is exactly the arrangement a real deployment has behind an API gateway — and
// it means the demo needs no API key, no secret and no dashboard-minted JWT.
//
// Flags rather than constants so a second member can be demonstrated without a
// rebuild. There is no login here and there should not be: authenticating a
// fictional member would be a page of theatre in front of the thing the demo is
// actually about.
type member struct {
	id    string
	email string
	roles string
}

// assistantPath is where the browser opens its WebSocket. The portal proxies
// it to the agent rather than letting the page dial the facade directly: the
// facade's external listener rejects an unauthenticated browser, and a page
// that held a credential would be handing it to every visitor.
const assistantPath = "/api/assistant/ws"

// newServer returns the portal handler: the embedded site, the assistant proxy,
// a redirect from the root to the kit's entry point, and a health endpoint.
func newServer(agentURL string, m member) (http.Handler, error) {
	// The kit loads .jsx over the network and hands the text to Babel. Go's
	// mime package has no entry for .jsx, so without this it is served as
	// application/octet-stream — which some browsers refuse to eval, and the
	// page then renders blank with only a console error to say why.
	if err := mime.AddExtensionType(".jsx", "text/javascript; charset=utf-8"); err != nil {
		return nil, err
	}

	root, err := fs.Sub(site, "site")
	if err != nil {
		return nil, err
	}
	files := http.FileServer(http.FS(root))

	proxy, err := assistantProxy(agentURL, m)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle(assistantPath, proxy)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if path.Clean(r.URL.Path) == "/" {
			http.Redirect(w, r, siteIndex, http.StatusFound)
			return
		}
		files.ServeHTTP(w, r)
	})
	return mux, nil
}

// assistantProxy forwards the browser's WebSocket to the agent facade, adding
// the member identity headers the AgentRuntime's edgeTrust validator reads.
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
			// Origin against its own Host, so a forwarded `http://<portal>`
			// never matches `pii-chat:8080` and every browser connection is
			// refused with 403. It explicitly permits requests with NO Origin,
			// for non-browser clients — which, from the facade's side, is
			// exactly what this proxy is.
			//
			// Dropping it moves the CSRF check rather than removing it: the
			// check now happens in checkOrigin below, against the portal's own
			// host, which is the only host that can meaningfully judge it.
			// Browsers do not apply same-origin to WebSockets, so without that
			// check any site could open a socket here and talk to the agent as
			// this member.
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
