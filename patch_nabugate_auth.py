import sys
import urllib.parse

with open("internal/server/nabuauth.go", "r") as f:
    content = f.read()

# I need to change how q is built and how it redirects in consoleNabuStart
# Original logic:
'''
	q := url.Values{}
	q.Set("client_id", "nabugate")
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", "openid profile email wallet wallet.write offline")
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	if p := r.URL.Query().Get("provider"); p != "" {
		q.Set("provider", p)
	}

	http.Redirect(w, r, cfg.URL+"/oauth2/authorize?"+q.Encode(), http.StatusTemporaryRedirect)
'''

# New logic:
'''
	q := url.Values{}
	q.Set("client_id", "nabugate")
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", "openid profile email wallet wallet.write offline")
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")

	targetURL := cfg.URL + "/oauth2/authorize?" + q.Encode()

	if p := r.URL.Query().Get("provider"); p != "" {
		// NabuAuth allows direct login via /login/{provider}?next=...
		// We pass the authorize URL as the 'next' parameter to skip the login form
		targetURL = cfg.URL + "/login/" + url.PathEscape(p) + "?next=" + url.QueryEscape("/oauth2/authorize?"+q.Encode())
	}

	http.Redirect(w, r, targetURL, http.StatusTemporaryRedirect)
'''

content = content.replace(
    '''	q.Set("code_challenge_method", "S256")
	if p := r.URL.Query().Get("provider"); p != "" {
		q.Set("provider", p)
	}

	http.Redirect(w, r, cfg.URL+"/oauth2/authorize?"+q.Encode(), http.StatusTemporaryRedirect)''',
    '''	q.Set("code_challenge_method", "S256")

	targetURL := cfg.URL + "/oauth2/authorize?" + q.Encode()

	if p := r.URL.Query().Get("provider"); p != "" {
		targetURL = cfg.URL + "/login/" + url.PathEscape(p) + "?next=" + url.QueryEscape("/oauth2/authorize?"+q.Encode())
	}

	http.Redirect(w, r, targetURL, http.StatusTemporaryRedirect)'''
)

with open("internal/server/nabuauth.go", "w") as f:
    f.write(content)
