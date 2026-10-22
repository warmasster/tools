package main

import (
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// Un grupo de cosas que se pueden borrar, con lo que libera.
type Candidate struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Desc      string   `json:"desc"`
	Level     string   `json:"level"` // "seguro", "revisar" o "info"
	Icon      string   `json:"icon"`
	Size      int64    `json:"size"`
	Count     int      `json:"count"`
	Where     []Item   `json:"where"`
	Permanent bool     `json:"permanent"` // se borra sin pasar por la papelera
	Tip       string   `json:"tip,omitempty"`
	targets   []int32  // lo que se borra de verdad
	special   string   // "papelera": se vacía con la API de Windows
	roots     []string // para la papelera: unidades
}

type cleanRule struct {
	id, title, desc, level, icon string
	permanent                    bool
	patterns                     []string // rutas absolutas en minúsculas; "*" vale por un nivel
	anywhere                     []string // terminaciones de ruta que valen en cualquier sitio
	contents                     bool     // borrar lo de dentro, no la carpeta
	files                        func(ext string, n *Node) bool
	dirMatch                     func(t *Tree, id int32, name string) bool // carpetas reconocidas por su contenido
	minAgeDays                   int
	tip                          string
}

func env(k string) string { return strings.ToLower(strings.TrimRight(os.Getenv(k), `\`)) }

func cleanRules() []cleanRule {
	local, roaming, home := env("LOCALAPPDATA"), env("APPDATA"), env("USERPROFILE")
	win, pdata := env("SystemRoot"), env("ProgramData")
	if win == "" {
		win = `c:\windows`
	}
	downloads := home + `\downloads`
	if p, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, 0); err == nil {
		downloads = strings.ToLower(p)
	}
	browsers := []string{
		local + `\google\chrome\user data\*`,
		local + `\microsoft\edge\user data\*`,
		local + `\bravesoftware\brave-browser\user data\*`,
		local + `\vivaldi\user data\*`,
		local + `\chromium\user data\*`,
	}
	var browserCaches []string
	for _, b := range browsers {
		for _, c := range []string{`cache`, `code cache`, `gpucache`, `service worker\cachestorage`, `dawncache`, `dawngraphitecache`, `grshadercache`, `graphitedawncache`} {
			browserCaches = append(browserCaches, b+`\`+c)
		}
		browserCaches = append(browserCaches, strings.TrimSuffix(b, `\*`)+`\shadercache`, strings.TrimSuffix(b, `\*`)+`\grshadercache`)
	}
	browserCaches = append(browserCaches,
		local+`\mozilla\firefox\profiles\*\cache2`,
		roaming+`\opera software\opera stable\cache`,
		local+`\opera software\opera stable\cache`,
		local+`\opera software\opera gx stable\cache`,
	)
	installer := func(ext string, n *Node) bool {
		switch ext {
		case "exe", "msi", "msix", "msixbundle", "appx":
			return true
		}
		return false
	}
	archive := func(ext string, n *Node) bool {
		switch ext {
		case "iso", "zip", "rar", "7z", "img", "tar", "gz":
			return n.Alloc >= 20<<20
		}
		return false
	}
	model := func(ext string, n *Node) bool {
		return n.Alloc >= 200<<20 && categoryOf(ext) == 9
	}

	return []cleanRule{
		{id: "temp", title: "Archivos temporales", icon: "temp", level: "seguro", permanent: true, contents: true,
			desc:     "Lo que dejan los programas al instalarse o trabajar. Lo que esté en uso se salta solo.",
			patterns: []string{env("TEMP"), local + `\temp`, win + `\temp`}},
		{id: "navegador", title: "Cachés de navegadores", icon: "globe", level: "seguro", permanent: true, contents: true,
			desc:     "Copias de páginas e imágenes de Chrome, Edge, Brave, Firefox y Opera. Se vuelven a crear al navegar. No toca contraseñas ni historial.",
			patterns: browserCaches},
		{id: "shaders", title: "Cachés de shaders (gráficos y juegos)", icon: "gpu", level: "seguro", permanent: true, contents: true,
			desc: "Las tarjetas gráficas y Steam los regeneran solos. Los juegos pueden tardar un poco más la primera vez.",
			patterns: []string{local + `\d3dscache`, local + `\nvidia\dxcache`, local + `\nvidia\glcache`, local + `\nvidia corporation\nv_cache`,
				local + `\amd\dxcache`, local + `\amd\dxccache`, local + `\amd\glcache`, local + `\amd\vkcache`, local + `\intel\shadercache`, home + `\appdata\locallow\intel\shadercache`},
			anywhere: []string{`steamapps\shadercache`}},
		{id: "apps", title: "Cachés de aplicaciones", icon: "apps", level: "seguro", permanent: true, contents: true,
			desc: "Discord, Spotify, VS Code, Teams, Slack y similares. Se vuelven a crear al usarlas.",
			patterns: []string{roaming + `\discord\cache`, roaming + `\discord\code cache`, roaming + `\discord\gpucache`,
				local + `\spotify\data`, roaming + `\code\cache`, roaming + `\code\cacheddata`, roaming + `\code\cachedextensionvsixs`, roaming + `\code\code cache`,
				roaming + `\slack\cache`, roaming + `\slack\code cache`, roaming + `\microsoft\teams\cache`, local + `\packages\msteams_8wekyb3d8bbwe\localcache`,
				roaming + `\obsidian\cache`, roaming + `\notion\cache`, roaming + `\whatsapp\cache`, local + `\microsoft\windows\inetcache`,
				local + `\steam\htmlcache`, local + `\epicgameslauncher\saved\webcache`, local + `\epicgameslauncher\saved\webcache_4430`}},
		{id: "dev", title: "Cachés de programación", icon: "code", level: "seguro", permanent: true, contents: true,
			desc: "Paquetes descargados de npm, pip, Go, Yarn, NuGet y Cargo. Se descargan otra vez si hacen falta.",
			patterns: []string{local + `\npm-cache`, local + `\pip\cache`, local + `\go-build`, local + `\yarn\cache`, local + `\nuget\v3-cache`,
				local + `\pnpm-cache`, home + `\.cargo\registry\cache`, home + `\.gradle\caches`, home + `\.cache\pip`}},
		{id: "volcados", title: "Volcados de errores", icon: "bug", level: "seguro", permanent: true, contents: true,
			desc: "Informes que guarda Windows cuando un programa se cuelga. Solo sirven para depurar.",
			patterns: []string{local + `\crashdumps`, win + `\minidump`, pdata + `\microsoft\windows\wer\reportarchive`, pdata + `\microsoft\windows\wer\reportqueue`,
				local + `\microsoft\windows\wer\reportarchive`, local + `\microsoft\windows\wer\reportqueue`, win + `\livekernelreports`}},
		{id: "updates", title: "Actualizaciones de Windows ya instaladas", icon: "update", level: "seguro", permanent: true, contents: true,
			desc:     "Descargas de Windows Update y de Optimización de distribución. Necesita modo turbo (administrador).",
			patterns: []string{win + `\softwaredistribution\download`, win + `\serviceprofiles\networkservice\appdata\local\microsoft\windows\deliveryoptimization\cache`}},
		{id: "instaladores", title: "Instaladores en Descargas", icon: "box", level: "revisar", contents: false,
			desc:     "Programas .exe y .msi que bajaste hace más de dos semanas. Normalmente ya están instalados.",
			patterns: []string{downloads}, files: installer, minAgeDays: 14},
		{id: "comprimidos", title: "Comprimidos e ISOs en Descargas", icon: "archive", level: "revisar",
			desc:     "Archivos .zip, .rar, .7z e .iso de más de 20 MB y más de dos semanas. Revisa que ya los hayas descomprimido.",
			patterns: []string{downloads}, files: archive, minAgeDays: 14},
		{id: "node", title: "Carpetas node_modules sin tocar", icon: "code", level: "revisar",
			desc:     "Dependencias de proyectos de JavaScript que no usas desde hace más de un mes. Se recuperan con npm install.",
			anywhere: []string{`node_modules`}, minAgeDays: 30},
		{id: "rust", title: "Compilaciones de Rust (carpetas target)", icon: "code", level: "revisar", permanent: true,
			desc:     "Lo que genera cargo al compilar. Se vuelve a crear con cargo build, aunque la primera compilación tardará más.",
			dirMatch: isCargoTarget, minAgeDays: 7},
		{id: "modelos", title: "Modelos de IA", icon: "brain", level: "revisar",
			desc:  "Archivos .gguf, .safetensors, .ckpt y parecidos de más de 200 MB. Mira si sigues usándolos.",
			files: model},
		{id: "windowsold", title: "Windows.old", icon: "update", level: "revisar", permanent: true,
			desc:     "La instalación anterior de Windows. Solo sirve para volver atrás de versión.",
			patterns: []string{volumePrefix() + `windows.old`},
			tip:      "Si TurboCUT no puede borrarla entera, usa «Liberador de espacio en disco» → «Limpiar archivos del sistema»."},
	}
}

// isCargoTarget reconoce la carpeta de compilación de cargo: se llama "target"
// y cargo deja dentro un CACHEDIR.TAG o un .rustc_info.json.
func isCargoTarget(t *Tree, id int32, name string) bool {
	if name != "target" {
		return false
	}
	found := false
	t.children(id, func(c int32) {
		if n := t.name(c); n == "CACHEDIR.TAG" || n == ".rustc_info.json" {
			found = true
		}
	})
	return found
}

func volumePrefix() string { return strings.ToLower(volumeOf(os.Getenv("SystemRoot"))) }

// matchPath compara una ruta con un patrón donde "*" vale por un nivel (sin reservar memoria).
func matchPath(path, pattern string) bool {
	for {
		i := strings.IndexByte(pattern, '*')
		if i < 0 {
			return path == pattern
		}
		if !strings.HasPrefix(path, pattern[:i]) {
			return false
		}
		path, pattern = path[i:], pattern[i+1:]
		j := strings.IndexByte(path, '\\')
		if j == 0 {
			return false
		}
		if j < 0 {
			path = ""
		} else {
			path = path[j:]
		}
	}
}

// findCandidates recorre las carpetas del escaneo buscando lo que se puede limpiar.
func findCandidates(t *Tree, measure func(*Node) int64) []*Candidate {
	rules := cleanRules()
	now := uint32(time.Now().Unix())
	cands := make([]*Candidate, len(rules))
	for i, r := range rules {
		cands[i] = &Candidate{ID: r.id, Title: r.title, Desc: r.desc, Level: r.level, Icon: r.icon, Permanent: r.permanent, Tip: r.tip}
	}
	whereSize := make([]map[int32]int64, len(rules))
	for i := range whereSize {
		whereSize[i] = map[int32]int64{}
	}
	addTarget := func(ri int, where int32, id int32) {
		c := cands[ri]
		c.targets = append(c.targets, id)
		sz := measure(&t.Nodes[id])
		c.Size += sz
		if t.Nodes[id].dir() {
			c.Count += int(t.Nodes[id].Files)
		} else {
			c.Count++
		}
		whereSize[ri][where] += sz
	}
	type item struct {
		id   int32
		path string
	}
	rootPath := strings.ToLower(strings.TrimRight(t.RootPath, `\`))
	stack := []item{{t.Root, rootPath}}
	for len(stack) > 0 {
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		matched := false
		for ri, r := range rules {
			hit := false
			for _, p := range r.patterns {
				if matchPath(it.path, p) {
					hit = true
					break
				}
			}
			for _, a := range r.anywhere {
				if strings.HasSuffix(it.path, `\`+a) {
					hit = true
					break
				}
			}
			if r.dirMatch != nil && it.id != t.Root {
				hit = hit || r.dirMatch(t, it.id, it.path[strings.LastIndexByte(it.path, '\\')+1:])
			}
			if !hit {
				continue
			}
			if r.files != nil {
				// reglas de archivos sueltos dentro de la carpeta (y subcarpetas)
				t.preorder(it.id, func(n int32) bool {
					nd := &t.Nodes[n]
					if nd.dir() {
						return true
					}
					if r.files(t.exts[nd.Ext], nd) && (r.minAgeDays == 0 || nd.MTime+uint32(r.minAgeDays)*86400 < now) {
						addTarget(ri, nd.Parent, n)
					}
					return false
				})
				continue
			}
			if r.minAgeDays > 0 && t.Nodes[it.id].MTime+uint32(r.minAgeDays)*86400 > now {
				matched = true // carpeta reciente: no se propone, pero tampoco se mira dentro
				continue
			}
			if r.contents {
				t.children(it.id, func(c int32) { addTarget(ri, it.id, c) })
			} else {
				addTarget(ri, it.id, it.id)
			}
			matched = true
		}
		if matched {
			continue
		}
		t.children(it.id, func(c int32) {
			if !t.Nodes[c].dir() || t.Nodes[c].Flags&fLink != 0 {
				return
			}
			p := it.path + `\` + strings.ToLower(t.name(c))
			stack = append(stack, item{c, p})
		})
	}

	// Reglas de archivos en cualquier sitio (modelos de IA).
	for ri, r := range rules {
		if r.files == nil || len(r.patterns) > 0 {
			continue
		}
		t.preorder(t.Root, func(n int32) bool {
			nd := &t.Nodes[n]
			if nd.dir() {
				return nd.Flags&fLink == 0
			}
			if r.files(t.exts[nd.Ext], nd) {
				addTarget(ri, nd.Parent, n)
			}
			return false
		})
	}

	// Papelera de reciclaje (se mide con la API de Windows, que sí ve la de este usuario).
	bin := &Candidate{ID: "papelera", Title: "Papelera de reciclaje", Icon: "trash", Level: "seguro", Permanent: true,
		Desc: "Lo que ya enviaste a la papelera. Al vaciarla se libera de verdad.", special: "papelera"}
	vol := volumeOf(t.RootPath)
	isDrive := strings.EqualFold(strings.TrimRight(t.RootPath, `\`)+`\`, vol)
	if sz, items := recycleBinSize(vol); sz > 0 && isDrive {
		bin.Size, bin.Count, bin.roots = sz, int(items), []string{vol}
		bin.Where = []Item{{ID: none, Name: "Papelera de " + strings.TrimRight(vol, `\`), Size: sz, Dir: true, Path: vol + `$Recycle.Bin`}}
	}

	out := []*Candidate{}
	if bin.Size > 0 {
		out = append(out, bin)
	}
	for ri, c := range cands {
		if c.Size <= 0 && len(c.targets) == 0 {
			continue
		}
		for where, sz := range whereSize[ri] {
			it := makeItem(t, where, measure)
			it.Size = sz
			it.Path = t.pathOf(where)
			c.Where = append(c.Where, it)
		}
		sort.Slice(c.Where, func(i, j int) bool { return c.Where[i].Size > c.Where[j].Size })
		if len(c.Where) > 30 {
			c.Where = c.Where[:30]
		}
		if c.Size >= 1<<20 {
			out = append(out, c)
		}
	}

	// Archivos del sistema que no se borran a mano pero conviene saber.
	if id := t.findPathIn(vol, "hiberfil.sys"); id != none {
		out = append(out, &Candidate{ID: "hiberfil", Title: "Archivo de hibernación", Icon: "moon", Level: "info", Size: measure(&t.Nodes[id]), Count: 1,
			Desc: "hiberfil.sys guarda la memoria al hibernar o con el inicio rápido.",
			Tip:  "Si no hibernas nunca, abre una terminal como administrador y escribe: powercfg /h off"})
	}
	if id := t.findPathIn(vol, "pagefile.sys"); id != none {
		out = append(out, &Candidate{ID: "pagefile", Title: "Archivo de paginación", Icon: "memory", Level: "info", Size: measure(&t.Nodes[id]), Count: 1,
			Desc: "pagefile.sys es memoria virtual. Windows lo gestiona solo; mejor no tocarlo.",
			Tip:  "Se ajusta en Configuración avanzada del sistema → Rendimiento → Memoria virtual."})
	}

	order := map[string]int{"seguro": 0, "revisar": 1, "info": 2}
	sort.SliceStable(out, func(i, j int) bool {
		if order[out[i].Level] != order[out[j].Level] {
			return order[out[i].Level] < order[out[j].Level]
		}
		return out[i].Size > out[j].Size
	})
	return out
}

// findPathIn busca un archivo en la raíz de la unidad, si el escaneo la incluye.
func (t *Tree) findPathIn(vol, rel string) int32 {
	root := strings.TrimRight(t.RootPath, `\`) + `\`
	if !strings.EqualFold(root, vol) {
		return none
	}
	return t.findPath(rel)
}
