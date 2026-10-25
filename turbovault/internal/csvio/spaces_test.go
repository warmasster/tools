package csvio

import (
	"strings"
	"testing"
)

// Un espacio al principio o al final de una contraseña es parte de ella.
func TestPasswordKeepsSpaces(t *testing.T) {
	in := "name,url,username,password,note\n" +
		"web,https://web.example/,  fer  ,\"  abc123xyz  \",\n"
	r, err := Read(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Records) != 1 {
		t.Fatalf("%+v", r)
	}
	if got := r.Records[0].Password; got != "  abc123xyz  " {
		t.Errorf("contraseña = %q, se esperaba con sus espacios", got)
	}
	if got := r.Records[0].Username; got != "fer" {
		t.Errorf("el usuario sí se limpia: %q", got)
	}
}
