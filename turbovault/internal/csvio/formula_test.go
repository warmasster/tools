package csvio

import (
	"bytes"
	"strings"
	"testing"
)

// Las celdas que Excel tomaría por fórmulas salen con un apóstrofo; al volver a
// importar el CSV se quita. La contraseña no se toca.
func TestFormulaGuard(t *testing.T) {
	recs := []Record{{Title: `=HYPERLINK("http://x","clic")`, URL: "https://web.es", Username: "+34 600 111 222", Password: "=secreta", Notes: "@SUM(1)"}, {Title: "-2+3", Username: "-5"}}
	var b bytes.Buffer
	if err := Write(&b, recs); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{`"'=HYPERLINK(""http://x"",""clic"")"`, "+34 600 111 222", ",=secreta,", "'@SUM(1)", "'-2+3", ",-5,"} {
		if !strings.Contains(out, want) {
			t.Errorf("falta %q en:\n%s", want, out)
		}
	}
	res, err := Read(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Records) != 2 || res.Records[0].Title != recs[0].Title || res.Records[0].Notes != "@SUM(1)" || res.Records[0].Password != "=secreta" || res.Records[1].Title != "-2+3" {
		t.Fatalf("al volver a importar: %+v", res.Records)
	}
}
