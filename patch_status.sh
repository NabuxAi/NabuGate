sed -i '' 's/_, authed = s.admin.ValidSession(c.Value)/info, authed := s.admin.ValidSession(c.Value)/g' internal/server/console.go
sed -i '' 's/"authenticated": authed,/"authenticated": authed,\n\t\t"is_admin":      info.IsAdmin,/g' internal/server/console.go
