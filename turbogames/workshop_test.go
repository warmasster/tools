package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"turbogames/internal/steam"
	"turbogames/internal/steamworks"
)

// testApp es una App sin Steam y sin internet: los datos de los mods salen de
// fake (y si no están ahí, es que «ya no existen»).
func testApp(t *testing.T, fake map[uint64]steamworks.Item, cols map[uint64][]uint64) *App {
	t.Helper()
	dir := t.TempDir()
	oldD, oldC := webDetailsFn, webCollectionsFn
	t.Cleanup(func() { webDetailsFn, webCollectionsFn = oldD, oldC })
	webDetailsFn = func(ids []uint64) ([]steamworks.Item, error) {
		var out []steamworks.Item
		for _, id := range ids {
			if it, ok := fake[id]; ok {
				it.ID = id
				out = append(out, it)
			} else {
				out = append(out, steamworks.Item{ID: id, Missing: true})
			}
		}
		return out, nil
	}
	webCollectionsFn = func(ids []uint64) (map[uint64][]uint64, error) {
		out := map[uint64][]uint64{}
		for _, id := range ids {
			if c, ok := cols[id]; ok {
				out[id] = c
			}
		}
		return out, nil
	}
	return &App{
		wcache:  loadWsCache(filepath.Join(dir, "workshop.json")),
		presets: loadPresetStore(filepath.Join(dir, "presets.json")),
		gmeta:   loadGameMetaStore(filepath.Join(dir, "juegos.json")),
	}
}

func TestPresetCodeRoundTrip(t *testing.T) {
	p := Preset{AppID: 107410, Game: "Arma 3", Name: "Antistasi", Author: "Fer", Tags: []string{"coop"}, Notes: "con amigos"}
	for i := uint64(0); i < 300; i++ {
		p.Mods = append(p.Mods, PresetMod{ID: 450814997 + i*7919, Title: "x"})
	}
	code := encodeCode(p)
	if !strings.HasPrefix(code, codePrefix) || len(code) > 3000 {
		t.Fatalf("código de %d caracteres", len(code))
	}
	// partido en varias líneas, como lo dejan algunos chats
	broken := "hola\n" + code[:40] + "\n" + code[40:] + "\nadiós"
	d, ok := decodeCode(broken)
	if !ok || d.Name != "Antistasi" || d.App != 107410 || len(d.IDs) != 300 || d.IDs[299] != p.Mods[299].ID || d.By != "Fer" {
		t.Fatalf("%v %+v", ok, d)
	}
	if _, ok := decodeCode("TGP1:esto-no-es-un-codigo"); ok {
		t.Fatal("código falso aceptado")
	}
}

func TestParsePresetText(t *testing.T) {
	fake := map[uint64]steamworks.Item{
		111111: {AppID: 294100, Title: "Harmony", Size: 100},
		222222: {AppID: 294100, Title: "HugsLib", Size: 200},
		333333: {AppID: 108600, Title: "De otro juego"},
	}
	a := testApp(t, fake, nil)
	text := "Preset de TurboGAMES: «Mi partida» · RimWorld · 3 mods\nDe: Ana\n\n" +
		"1. Harmony — https://steamcommunity.com/sharedfiles/filedetails/?id=111111\n" +
		"2. HugsLib — https://steamcommunity.com/workshop/filedetails/?searchtext=&id=222222\n" +
		"333333\n" +
		"444444\n" // ya no existe
	d, err := a.ParsePreset(text)
	if err != nil {
		t.Fatal(err)
	}
	p := d.Preset
	if p.AppID != 294100 || p.Name != "Mi partida" || p.Author != "Ana" {
		t.Fatalf("%+v", p)
	}
	if len(p.Mods) != 3 || p.Mods[0].Title != "Harmony" || d.Others != 1 || d.Missing != 1 || d.Size != 300 {
		t.Fatalf("mods=%+v otros=%d faltan=%d tam=%d", p.Mods, d.Others, d.Missing, d.Size)
	}
	if _, err := a.ParsePreset("no hay nada aquí 12345"); err == nil {
		t.Fatal("debería decir que no hay mods")
	}
}

func TestParsePresetCodeAndCollection(t *testing.T) {
	fake := map[uint64]steamworks.Item{
		1000010: {AppID: 4000, Title: "Colección"},
		1000011: {AppID: 4000, Title: "A"},
		1000012: {AppID: 4000, Title: "B"},
	}
	a := testApp(t, fake, map[uint64][]uint64{1000010: {1000011, 1000012}})
	d, err := a.ParsePreset("https://steamcommunity.com/sharedfiles/filedetails/?id=1000010")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Preset.Mods) != 2 || d.Preset.Name != "Colección" || d.Preset.Source != "coleccion" {
		t.Fatalf("%+v", d.Preset)
	}
	saved, err := a.SavePreset(d.Preset)
	if err != nil {
		t.Fatal(err)
	}
	text, err := a.PresetText(saved.ID, true)
	if err != nil || !strings.Contains(text, "«Colección»") || !strings.Contains(text, "?id=1000011") {
		t.Fatalf("%v\n%s", err, text)
	}
	d2, err := a.ParsePreset(text)
	if err != nil || d2.Preset.AppID != 4000 || len(d2.Preset.Mods) != 2 || d2.Preset.Name != "Colección" {
		t.Fatalf("%v %+v", err, d2.Preset)
	}
}

func TestParseArma3Launcher(t *testing.T) {
	fake := map[uint64]steamworks.Item{450814997: {AppID: 107410, Title: "CBA_A3"}, 463939057: {AppID: 107410, Title: "ace"}}
	a := testApp(t, fake, nil)
	html := `<?xml version="1.0" encoding="utf-8"?>
<html>
  <!--Created by Arma 3 Launcher: https://arma3.com-->
  <head>
    <meta name="arma:Type" content="preset" />
    <meta name="arma:PresetName" content="Antistasi &amp; amigos" />
  </head>
  <body>
    <div class="mod-list">
      <table>
        <tr data-type="ModContainer">
          <td data-type="DisplayName">CBA_A3</td>
          <td><span class="from-steam">Steam</span></td>
          <td><a href="https://steamcommunity.com/sharedfiles/filedetails/?id=450814997" data-type="Link">https://steamcommunity.com/sharedfiles/filedetails/?id=450814997</a></td>
        </tr>
        <tr data-type="ModContainer">
          <td data-type="DisplayName">ace</td>
          <td><span class="from-steam">Steam</span></td>
          <td><a href="http://steamcommunity.com/sharedfiles/filedetails/?id=463939057" data-type="Link">x</a></td>
        </tr>
        <tr data-type="ModContainer">
          <td data-type="DisplayName">Mi mod local</td>
          <td><span class="from-local">Local</span></td>
          <td><span data-type="Link">local:C:\mods\@mio\</span></td>
        </tr>
      </table>
    </div>
  </body>
</html>`
	d, err := a.ParsePreset(html)
	if err != nil {
		t.Fatal(err)
	}
	if d.Preset.AppID != 107410 || d.Preset.Name != "Antistasi & amigos" || len(d.Preset.Mods) != 2 || d.Local != 1 || d.Preset.Source != "arma3" {
		t.Fatalf("%+v local=%d", d.Preset, d.Local)
	}
}

func TestAutoBackupKeepsLast(t *testing.T) {
	a := testApp(t, nil, nil)
	for i := 0; i < maxAutoCopies+3; i++ {
		if _, err := a.autoBackup(55, []uint64{uint64(i + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.SavePreset(Preset{AppID: 55, Name: "mío", Mods: []PresetMod{{ID: 9}}}); err != nil {
		t.Fatal(err)
	}
	autos, mine := 0, 0
	for _, p := range a.Presets() {
		if p.Auto {
			autos++
		} else {
			mine++
		}
	}
	if autos != maxAutoCopies || mine != 1 {
		t.Fatalf("copias=%d propios=%d", autos, mine)
	}
	if _, err := a.SavePreset(Preset{AppID: 55, Name: " ", Mods: []PresetMod{{ID: 9}}}); err == nil {
		t.Fatal("sin nombre debería fallar")
	}
}

func TestFlexNum(t *testing.T) {
	var v struct {
		A flexNum `json:"a"`
		B flexNum `json:"b"`
		C flexNum `json:"c"`
	}
	if err := json.Unmarshal([]byte(`{"a":"12345","b":678,"c":null}`), &v); err != nil || v.A != 12345 || v.B != 678 || v.C != 0 {
		t.Fatalf("%v %+v", err, v)
	}
}

func TestWorkshopFiles(t *testing.T) {
	dir := t.TempDir()
	ws := filepath.Join(dir, "steamapps", "workshop")
	os.MkdirAll(filepath.Join(ws, "content", "294100", "111"), 0o755)
	os.WriteFile(filepath.Join(dir, "steam.exe"), []byte("x"), 0o644)
	acf := `"AppWorkshop"
{
	"appid"		"294100"
	"SizeOnDisk"		"300"
	"NeedsUpdate"		"0"
	"WorkshopItemsInstalled"
	{
		"111"
		{
			"size"		"100"
			"timeupdated"		"1000"
		}
		"222"
		{
			"size"		"200"
			"timeupdated"		"1000"
		}
	}
	"WorkshopItemDetails"
	{
		"111"
		{
			"timeupdated"		"1000"
			"subscribedby"		"42"
			"latest_timeupdated"		"2000"
		}
		"333"
		{
			"subscribedby"		"42"
		}
	}
}`
	os.WriteFile(filepath.Join(ws, "appworkshop_294100.acf"), []byte(acf), 0o644)
	in := &steam.Install{Dir: dir}
	w := in.Workshop()[294100]
	if w == nil || len(w.Items) != 3 || w.Size != 300 {
		t.Fatalf("%+v", w)
	}
	got := map[uint64]steam.WorkshopItem{}
	for _, it := range w.Items {
		got[it.ID] = it
	}
	if a := got[111]; !a.Installed || a.SubscribedBy != 42 || a.Latest != 2000 || a.Dir == "" {
		t.Fatalf("111: %+v", a)
	}
	if b := got[222]; b.Installed { // su carpeta no está
		t.Fatalf("222: %+v", b)
	}
	if c := got[333]; c.Installed || c.SubscribedBy != 42 {
		t.Fatalf("333: %+v", c)
	}

	cs := filepath.Join(dir, "userdata", "42", "config", "cloudstorage")
	os.MkdirAll(cs, 0o755)
	col := `[["user-collections.favorite",{"key":"user-collections.favorite","timestamp":1,"value":"{\"id\":\"favorite\",\"name\":\"\",\"added\":[730,570],\"removed\":[570]}"}],
["user-collections.uc-x",{"key":"user-collections.uc-x","value":"{\"id\":\"uc-x\",\"name\":\"Coop\",\"added\":[294100]}"}],
["user-collections.uc-y",{"key":"user-collections.uc-y","is_deleted":true}],
["otra-cosa",{"key":"otra-cosa","value":"{}"}]]`
	os.WriteFile(filepath.Join(cs, "cloud-storage-namespace-1.json"), []byte(col), 0o644)
	c := in.UserCollections(42)
	if len(c) != 2 || len(c["Favoritos"]) != 1 || c["Favoritos"][0] != 730 || c["Coop"][0] != 294100 {
		t.Fatalf("%+v", c)
	}
}

func TestModHistoryAndTags(t *testing.T) {
	a := testApp(t, nil, nil)
	a.mstore = loadModStore(filepath.Join(t.TempDir(), "mods.json"))
	title := func(id uint64) string { return "M" + strconv.FormatUint(id, 10) }
	a.mstore.record("acc", 5, []uint64{1, 2, 3}, title)
	a.mstore.record("acc", 5, []uint64{2, 3, 4}, title)
	removed, since := a.mstore.history("acc", 5)
	if len(removed) != 1 || removed[0].ID != 1 || removed[0].Title != "M1" || len(since) != 3 || since[4] == 0 {
		t.Fatalf("%+v %v", removed, since)
	}
	a.mstore.record("acc", 5, []uint64{1, 2, 3, 4}, title) // vuelve a suscribirse
	if removed, _ := a.mstore.history("acc", 5); len(removed) != 0 {
		t.Fatalf("%+v", removed)
	}
	if removed, _ := a.mstore.history("otra", 5); len(removed) != 0 {
		t.Fatal("el historial es por cuenta")
	}
	got, err := a.SetModTags([]uint64{7, 8}, []string{"gráficos", "#probar"}, nil)
	if err != nil || len(got["7"]) != 2 || got["8"][1] != "probar" {
		t.Fatalf("%v %v", err, got)
	}
	got, _ = a.SetModTags([]uint64{7}, nil, []string{"Gráficos"})
	if len(got["7"]) != 1 || got["7"][0] != "probar" || len(a.mstore.tags(8)) != 2 {
		t.Fatalf("%v %v", got, a.mstore.tags(8))
	}
}

func TestPageGallery(t *testing.T) {
	page := `<div id="highlight_player_area"><script>var x = { YOUTUBE_VIDEO_ID: "yHBGkPQ6tVk", };</script>
	<div id="highlight_screenshot_0"><img src="https://images.steamusercontent.com/ugc/115/ED4F8D/?imw=5000"></div>
	<a onclick="ShowEnlargedImagePreview( 'https://images.steamusercontent.com/ugc/115/ED4F8D/?imw=5000' )"></a>
	<img src="https://images.steamusercontent.com/ugc/222/AB12/"></div>
	<div class="workshopItemDescription"><img src="https://images.steamusercontent.com/ugc/999/FFFF/"></div>`
	g := pageGallery(page)
	if len(g) != 3 || g[0].Video != "yHBGkPQ6tVk" || g[1].URL != "https://images.steamusercontent.com/ugc/115/ED4F8D/" || g[2].URL != "https://images.steamusercontent.com/ugc/222/AB12/" {
		t.Fatalf("%+v", g)
	}
	if len(pageGallery("sin galería")) != 0 {
		t.Fatal("sin zona de galería no debería sacar nada")
	}
	if safeImageURL("https://evil.com/x.png") || !safeImageURL("https://images.steamusercontent.com/ugc/1/2/") || validVideoID("../x") {
		t.Fatal("comprobaciones de direcciones")
	}
}

func TestSummary(t *testing.T) {
	got := summary("[h1]Mi mod[/h1]\n[img]https://x/y.png[/img]Hace [b]cosas[/b]\n\n[list][*]una[*]dos[/list] [url=https://a.b]enlace[/url]")
	if got != "Mi mod Hace cosas una dos enlace" {
		t.Fatalf("%q", got)
	}
}
