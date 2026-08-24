cat << 'INNER' > internal/server/console_status.go
package server
import (
	"net/http"
	"nabugate/internal/adminstore"
)

func (s *Server) consoleStatus(w http.ResponseWriter, r *http.Request) {
	authed := false
	var info adminstore.SessionInfo
	if c, err := r.Cookie(consoleCookie); err == nil {
		info, authed = s.admin.ValidSession(c.Value)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"needs_setup":   s.admin.NeedsSetup(),
		"authenticated": authed,
		"is_admin":      info.IsAdmin,
	})
}
INNER
sed -i '' '/func (s \*Server) consoleStatus/,/^}/d' internal/server/console.go
cat internal/server/console_status.go >> internal/server/console.go
rm internal/server/console_status.go
