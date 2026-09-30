package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestRejectMaliciousArgonParametersBeforeComputing(t *testing.T) {
	h := NewDefaultHasher()
	for _, params := range []string{
		"m=4294967295,t=3,p=2", "m=65536,t=999,p=2", "m=65536,t=3,p=255",
		"m=65536,t=3,p=2junk", "m=65536,t=3,p=2,extra=1",
	} {
		phc := "$argon2id$v=19$" + params + "$MTIzNDU2Nzg5MDEyMzQ1Ng$MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI"
		_, _, err := h.Verify(phc, "senha-de-teste")
		if !errors.Is(err, ErrHashInvalido) {
			t.Errorf("%s: erro = %v", params, err)
		}
	}
	_, _, err := h.Verify(strings.Repeat("x", 513), "senha-de-teste")
	if !errors.Is(err, ErrHashInvalido) {
		t.Errorf("hash excessivo: %v", err)
	}
}
