import sys
import urllib.parse

with open("internal/server/nabuauth.go", "r") as f:
    content = f.read()

content = content.replace(
    '''	if p := r.URL.Query().Get("provider"); p != "" {
		q.Set("provider", p)
	}
	http.Redirect(w, r, cfg.URL+"/oauth/authorize?"+q.Encode(), http.StatusFound)''',
    '''	targetURL := cfg.URL + "/oauth/authorize?" + q.Encode()
	if p := r.URL.Query().Get("provider"); p != "" {
		targetURL = cfg.URL + "/login/" + url.PathEscape(p) + "?next=" + url.QueryEscape("/oauth/authorize?"+q.Encode())
	}
	http.Redirect(w, r, targetURL, http.StatusFound)'''
)

with open("internal/server/nabuauth.go", "w") as f:
    f.write(content)
