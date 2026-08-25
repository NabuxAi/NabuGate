import sys

with open("internal/server/console.go", "r") as f:
    content = f.read()

content = content.replace(
    '''	err := s.admin.AddPayment(body.Email, body.Amount, "admin-recharge", "manual-"+fmt.Sprint(time.Now().UnixNano()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}''',
    '''	newBal := s.admin.AddPayment(body.Email, body.Amount, "admin-recharge", "manual-"+fmt.Sprint(time.Now().UnixNano()))'''
)

with open("internal/server/console.go", "w") as f:
    f.write(content)
