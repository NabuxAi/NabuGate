import sys

with open("internal/server/nabuauth.go", "r") as f:
    content = f.read()

content = content.replace(
    '	q := url.Values{\n		"response_type":         {"code"},\n		"client_id":             {cfg.ClientID},\n		"redirect_uri":          {cfg.RedirectURI},\n		"scope":                 {cfg.Scopes},',
    '''	redirectURI := cfg.RedirectURI
	if redirectURI == "" {
		scheme := "http"
		if requestIsHTTPS(r) {
			scheme = "https"
		}
		redirectURI = scheme + "://" + r.Host + "/api/nabu/callback"
	}
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {cfg.Scopes},'''
)

with open("internal/server/nabuauth.go", "w") as f:
    f.write(content)

