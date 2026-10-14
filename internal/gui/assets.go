package gui

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"

	"golang.org/x/net/html"
)

func (s *server) serveAssets(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	if s.devProxy != nil {
		s.devProxy.ServeHTTP(w, r)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	switch r.URL.Path {
	case "/", "/accounts", "/transfers", "/mounts", "/recycle", "/recovery", "/settings":
		name = "index.html"
	}
	if !fs.ValidPath(name) {
		http.NotFound(w, r)
		return
	}
	data, err := assets.ReadFile("web/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if name == "index.html" {
		// Keep same-origin navigation context without sending paths to other
		// origins. Activation fragments are never included in referrers.
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		nonce, err := randomToken()
		if err != nil {
			http.Error(w, "interface unavailable", http.StatusInternalServerError)
			return
		}
		data = nonceHTML(data, nonce)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' 'nonce-"+nonce+"'; style-src 'self' 'nonce-"+nonce+"'; style-src-attr 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self'; base-uri 'none'; object-src 'none'; form-action 'self'; frame-ancestors 'none'")
	} else if strings.HasPrefix(name, "_nuxt/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

// Development assets and Vite's websocket share the authenticated server's
// origin. API/control traffic never reaches Nuxt, and local credentials are
// stripped before proxying. Production has no proxy or relaxed development CSP.
func developmentProxy(address string) (http.Handler, error) {
	if address == "" {
		return nil, nil
	}
	target, err := url.Parse(address)
	if err != nil || target.User != nil || target.RawQuery != "" || target.Fragment != "" || (target.Path != "" && target.Path != "/") ||
		(target.Scheme != "http" && target.Scheme != "https") ||
		(target.Hostname() != "localhost" && target.Hostname() != "127.0.0.1" && target.Hostname() != "::1") {
		return nil, errors.New("PKUDISK_GUI_DEV_PROXY must be a loopback HTTP(S) origin")
	}
	proxy := &httputil.ReverseProxy{Rewrite: func(request *httputil.ProxyRequest) {
		request.SetURL(target)
		request.Out.Host = target.Host
		request.Out.Header.Del("Authorization")
		request.Out.Header.Del("Cookie")
		request.Out.Header.Del("Proxy-Authorization")
		request.Out.Header.Del("Referer")
		if request.Out.Header.Get("Origin") != "" {
			request.Out.Header.Set("Origin", target.Scheme+"://"+target.Host)
		}
	}, ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "Nuxt development server unavailable", http.StatusBadGateway)
	}}
	wsScheme := "ws"
	if target.Scheme == "https" {
		wsScheme = "wss"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self' "+target.Scheme+"://"+target.Host+" "+wsScheme+"://"+target.Host+"; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		proxy.ServeHTTP(w, r)
	}), nil
}

// Nuxt's color-mode and runtime configuration scripts need an inline-script
// nonce. Use the HTML tokenizer so script contents are preserved verbatim;
// never weaken script-src with unsafe-inline or unsafe-eval.
func nonceHTML(input []byte, nonce string) []byte {
	tokenizer := html.NewTokenizer(bytes.NewReader(input))
	var out bytes.Buffer
	for {
		kind := tokenizer.Next()
		if kind == html.ErrorToken {
			if tokenizer.Err() == io.EOF {
				return out.Bytes()
			}
			return input
		}
		if kind == html.StartTagToken || kind == html.SelfClosingTagToken {
			name, _ := tokenizer.TagName()
			if string(name) == "head" {
				out.Write(tokenizer.Raw())
				out.WriteString(`<meta name="csp-nonce" property="csp-nonce" content="` + nonce + `">`)
				continue
			}
			if string(name) == "script" || string(name) == "style" {
				// TagName advances attribute iteration; reconstruct from Raw with
				// a separate tokenizer to retain every existing attribute.
				tag := html.NewTokenizer(bytes.NewReader(tokenizer.Raw()))
				tag.Next()
				token := tag.Token()
				token.Attr = append(token.Attr, html.Attribute{Key: "nonce", Val: nonce})
				out.WriteString(token.String())
				continue
			}
		}
		out.Write(tokenizer.Raw())
	}
}
