package cronet

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// CookieJar implements http.CookieJar using Chromium's CookieMonster.
// This allows using cronet's cookie store with Go's standard http.Client.
type CookieJar struct {
	engine *Engine
}

var _ http.CookieJar = (*CookieJar)(nil)

// NewCookieJar creates a CookieJar backed by the given Engine.
// The engine must have been created with EnableCookies=true.
func NewCookieJar(engine *Engine) *CookieJar {
	return &CookieJar{engine: engine}
}

// SetCookies stores cookies for the given URL.
func (j *CookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	for _, c := range cookies {
		line := fmt.Sprintf("%s=%s", c.Name, c.Value)
		if c.Path != "" {
			line += "; path=" + c.Path
		}
		if c.Domain != "" {
			line += "; domain=" + c.Domain
		}
		if c.Secure {
			line += "; secure"
		}
		if c.HttpOnly {
			line += "; httponly"
		}
		if !c.Expires.IsZero() {
			line += "; expires=" + c.Expires.UTC().Format(http.TimeFormat)
		}
		j.engine.SetCookie(u, line)
	}
}

// Cookies returns the cookies for the given URL.
func (j *CookieJar) Cookies(u *url.URL) []*http.Cookie {
	raw := j.engine.GetCookies(u)
	if raw == "" {
		return nil
	}

	var cookies []*http.Cookie
	pairs := strings.Split(raw, "; ")
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		cookies = append(cookies, &http.Cookie{
			Name:  parts[0],
			Value: parts[1],
		})
	}
	return cookies
}
