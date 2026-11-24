package adminstore

import "testing"

func TestNormalizeMobile(t *testing.T) {
	ok := map[string]string{
		"09121234567":       "+989121234567",
		"۰۹۱۲۱۲۳۴۵۶۷":       "+989121234567",
		"٠٩١٢١٢٣٤٥٦٧":       "+989121234567",
		"0912 123 4567":     "+989121234567",
		"9121234567":        "+989121234567",
		"+98 912 123 4567":  "+989121234567",
		"+980912 1234567":   "+989121234567",
		"00989121234567":    "+989121234567",
		"+968 9123 4567":    "+96891234567",
		"+1 (415) 555-0100": "+14155550100",
	}
	for in, want := range ok {
		if got, valid := NormalizeMobile(in); !valid || got != want {
			t.Errorf("%q -> %q %v, want %q", in, got, valid, want)
		}
	}
	for _, in := range []string{"", "12345", "0212345678", "+98212345678", "0912123456", "09121234567x", "+0123456789", "91212345678901234"} {
		if got, valid := NormalizeMobile(in); valid {
			t.Errorf("%q accepted as %q", in, got)
		}
	}
}
