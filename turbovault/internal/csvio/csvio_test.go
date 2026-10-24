package csvio

import (
	"bytes"
	"strings"
	"testing"
)

func TestChrome(t *testing.T) {
	in := "\xef\xbb\xbfname,url,username,password,note\n" +
		"github.com,https://github.com/login,fer,p1,\n" +
		"x,https://x.com/,,,\n" +
		"\"coma, rara\",https://a.com,\"u\",\"p\"\"2\",nota\n"
	r, err := Read(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if r.Source != "Chrome" || len(r.Records) != 2 || r.Skipped != 1 {
		t.Fatalf("%+v", r)
	}
	if r.Records[1].Title != "coma, rara" || r.Records[1].Password != "p\"2" || r.Records[1].Notes != "nota" {
		t.Fatalf("%+v", r.Records[1])
	}
}

func TestBitwardenFirefoxKeePass(t *testing.T) {
	bw := "folder,favorite,type,name,notes,fields,reprompt,login_uri,login_username,login_password,login_totp\n" +
		",1,login,Steam,,,0,\"https://store.steampowered.com,https://steamcommunity.com\",yo,pw,otpauth://totp/x?secret=JBSWY3DPEHPK3PXP\n"
	r, err := Read(strings.NewReader(bw))
	if err != nil || r.Source != "Bitwarden" || r.Records[0].URL != "https://store.steampowered.com" || r.Records[0].TOTP == "" {
		t.Fatalf("%v %+v", err, r)
	}
	ff := "url,username,password,httpRealm,formActionOrigin,guid,timeCreated,timeLastUsed,timePasswordChanged\nhttps://a.com,u,p,,https://a.com,{1},1,1,1\n"
	r, err = Read(strings.NewReader(ff))
	if err != nil || r.Source != "Firefox" || r.Records[0].Username != "u" {
		t.Fatalf("%v %+v", err, r)
	}
	kp := "\"Group\";\"Title\";\"Username\";\"Password\";\"URL\";\"Notes\"\n\"Root\";\"Banco\";\"yo\";\"pp\";\"https://banco.es\";\"\"\n"
	r, err = Read(strings.NewReader(kp))
	if err != nil || r.Source != "KeePass" || r.Records[0].Title != "Banco" || r.Records[0].URL != "https://banco.es" {
		t.Fatalf("%v %+v", err, r)
	}
}

func TestNoHeader(t *testing.T) {
	if _, err := Read(strings.NewReader("a,b,c\n1,2,3\n")); err != ErrNoHeader {
		t.Fatalf("%v", err)
	}
}

func TestRoundTrip(t *testing.T) {
	var b bytes.Buffer
	Write(&b, []Record{{Title: "t", URL: "https://u.com", Username: "u", Password: "p,\"x", Notes: "n\nl", TOTP: "S"}})
	r, err := Read(&b)
	if err != nil || len(r.Records) != 1 || r.Records[0].Password != "p,\"x" || r.Records[0].Notes != "n\nl" || r.Records[0].TOTP != "S" {
		t.Fatalf("%v %+v", err, r)
	}
}
