package main

// «Reducir tamaño» y «Convertir»: decide qué opciones enseñar según lo seleccionado y
// ejecuta el trabajo con el motor que toque (vídeo/audio, imagen, PDF, documentos o ZIP).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"turbomenu/internal/lock"
	"turbomenu/internal/media"
)

type PlanOpt struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Sub      string `json:"sub,omitempty"`
	Est      int64  `json:"est,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Brand    string `json:"brand,omitempty"`
	Color    string `json:"color,omitempty"`
	Rec      bool   `json:"rec,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
	Needs    string `json:"needs,omitempty"`
}

type PlanTarget struct {
	ID       string `json:"id"`
	Ext      string `json:"ext"`
	Name     string `json:"name"`
	Desc     string `json:"desc"`
	Group    string `json:"group"`
	Recent   bool   `json:"recent,omitempty"`
	GIF      bool   `json:"gif,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
	Needs    string `json:"needs,omitempty"`
	engine   string // media | docs | pdfjoin
	kind     string
}

type PlanScale struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Sub   string `json:"sub,omitempty"`
}

type PlanNote struct {
	Kind string `json:"kind,omitempty"`
	Text string `json:"text"`
}

type Plan struct {
	Mode      string       `json:"mode"`
	Kind      string       `json:"kind"` // video | audio | image | pdf | zip | doc | mixed
	Info      string       `json:"info,omitempty"`
	Needs     string       `json:"needs,omitempty"` // ffmpeg | office
	OrigSize  int64        `json:"origSize"`
	Presets   []PlanOpt    `json:"presets,omitempty"`
	Qualities []PlanOpt    `json:"qualities,omitempty"`
	Custom    bool         `json:"custom,omitempty"`
	Image     *ImagePlan   `json:"image,omitempty"`
	Targets   []PlanTarget `json:"targets,omitempty"`
	Notes     []PlanNote   `json:"notes,omitempty"`
	Error     string       `json:"error,omitempty"`
}

type ImagePlan struct {
	Quality int         `json:"quality"`
	Scales  []PlanScale `json:"scales"`
}

// planKind decide cómo tratar la selección.
func planKind(files []FileInfo) string {
	count := map[string]int{}
	for _, f := range files {
		count[f.Kind]++
	}
	n := len(files)
	switch {
	case n == 0:
		return "zip"
	case count["video"]+count["audio"] == n && count["video"] > 0:
		return "video"
	case count["audio"] == n:
		return "audio"
	case count["image"] == n:
		return "image"
	case count["pdf"] == n:
		return "pdf"
	case count["doc"] == n:
		return "doc"
	}
	return "zip"
}

func ffmpegPath() string {
	ff, _ := media.Find(binDir())
	return ff
}

func ffprobePath() string {
	_, fp := media.Find(binDir())
	return fp
}

func (a *App) jobFiles() ([]FileInfo, *Job) {
	job := a.currentJob()
	if job == nil {
		return nil, nil
	}
	var files []FileInfo
	for _, p := range job.Paths {
		files = append(files, statFile(p))
	}
	return files, job
}

var probeCache sync.Map // ruta|fecha → *media.Info

func probe(ctx context.Context, f FileInfo) (*media.Info, error) {
	key := fmt.Sprintf("%s|%d|%d", f.Path, f.Mod, f.Size)
	if v, ok := probeCache.Load(key); ok {
		return v.(*media.Info), nil
	}
	fp := ffprobePath()
	if fp == "" {
		return nil, media.ErrNoFFmpeg
	}
	in, err := media.Probe(ctx, fp, f.Path)
	if err == nil {
		probeCache.Store(key, in)
	}
	return in, err
}

func mediaSub(in *media.Info) string {
	var parts []string
	label := "Vídeo"
	if in.Kind == "audio" {
		label = "Audio"
	}
	parts = append(parts, fmt.Sprintf("%s %s · <b>%s</b>", label, strings.ToUpper(in.Container), humanSize(in.Size)))
	if in.Kind == "video" && in.Height > 0 {
		short := min(in.Width, in.Height)
		res := fmt.Sprintf("%dp", short)
		if short >= 2100 {
			res = "4K"
		}
		parts = append(parts, res)
	}
	if in.Duration > 0 {
		d := int(in.Duration + .5)
		if d >= 3600 {
			parts = append(parts, fmt.Sprintf("%d:%02d:%02d", d/3600, d%3600/60, d%60))
		} else {
			parts = append(parts, fmt.Sprintf("%d:%02d", d/60, d%60))
		}
	}
	return strings.Join(parts, " · ")
}

func humanSize(b int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	f := float64(b)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", b)
	}
	s := fmt.Sprintf("%.1f", f)
	if f >= 100 {
		s = fmt.Sprintf("%.0f", f)
	}
	return strings.Replace(strings.TrimSuffix(s, ".0"), ".", ",", 1) + " " + units[i]
}

// Plan prepara las opciones de «Reducir tamaño» (mode "reduce") o «Convertir».
func (a *App) Plan(mode string) Plan {
	files, job := a.jobFiles()
	if job == nil || len(files) == 0 {
		return Plan{Error: "No hay nada seleccionado"}
	}
	gen := a.gen
	p := Plan{Mode: mode, Kind: planKind(files)}
	for _, f := range files {
		p.OrigSize += f.Size
		if f.Err != "" {
			return Plan{Error: f.Name + ": no se encuentra"}
		}
	}
	if mode == "convert" {
		a.planConvert(&p, files)
	} else {
		a.planReduce(&p, files, gen)
	}
	return p
}

func (a *App) planReduce(p *Plan, files []FileInfo, gen int) {
	switch p.Kind {
	case "video", "audio":
		if ffmpegPath() == "" {
			p.Needs = "ffmpeg"
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		var first *media.Info
		if len(files) == 1 {
			in, err := probe(ctx, files[0])
			if err != nil {
				p.Error = err.Error()
				return
			}
			first = in
			p.Info = mediaSub(in)
		}
		p.Presets = []PlanOpt{
			{ID: "discord", Title: "Para Discord", Sub: "Menos de 10 MB", Brand: "D", Color: "#5865f2", Rec: true},
			{ID: "whatsapp", Title: "Para WhatsApp", Sub: "Menos de 16 MB", Brand: "W", Color: "#25d366"},
		}
		if p.Kind == "audio" {
			p.Presets = nil
		}
		p.Qualities = []PlanOpt{{ID: "alta", Title: "Alta"}, {ID: "media", Title: "Media"}, {ID: "baja", Title: "Baja"}}
		p.Custom = len(files) == 1
		if first != nil {
			for i := range p.Presets {
				lim := media.SizeDiscord
				if p.Presets[i].ID == "whatsapp" {
					lim = media.SizeWhatsApp
				}
				if first.Size <= lim {
					p.Presets[i].Sub = "Ya cabe: se deja tal cual"
				}
			}
			for i := range p.Qualities {
				p.Qualities[i].Est = media.Estimate(first, media.ReduceOptions{Quality: p.Qualities[i].ID})
			}
			if p.Kind == "audio" {
				p.Qualities[1].Rec = true
			}
			// Estimaciones buenas (codificando unos segundos de muestra) en segundo plano.
			go a.sampleEstimates(first, gen)
		}
		if len(files) > 1 {
			p.Notes = append(p.Notes, PlanNote{Text: fmt.Sprintf("Se aplicará lo mismo a los %d archivos, uno detrás de otro.", len(files))})
		}
	case "image":
		a.planImageReduce(p, files)
	case "pdf":
		p.Presets = []PlanOpt{
			{ID: "screen", Title: "Para leer en pantalla", Sub: "Lo más ligero: ideal para enviarlo", Icon: "phone", Color: "linear-gradient(135deg,#8b5cf6,#06b6d4)", Rec: true},
			{ID: "print", Title: "Para imprimir", Sub: "Mantiene la nitidez de las imágenes", Icon: "doc", Color: "linear-gradient(135deg,#475569,#334155)"},
		}
	default:
		p.Kind = "zip"
		name := zipName(files)
		p.Presets = []PlanOpt{{ID: "zip", Title: "Comprimir en «" + name + "»", Icon: "archive", Color: "linear-gradient(135deg,#a16207,#ca8a04)", Rec: true,
			Sub: "Lo empaqueta todo en un único archivo comprimido, que se abre en cualquier PC."}}
		if len(files) == 1 && !files[0].IsDir {
			p.Notes = append(p.Notes, PlanNote{Text: "Este tipo de archivo no se puede reducir por dentro; comprimirlo en ZIP es lo único que ayuda (a veces poco)."})
		}
	}
}

func (a *App) sampleEstimates(in *media.Info, gen int) {
	ff := ffmpegPath()
	ctx := a.newTask("estimate")
	defer a.endTask("estimate")
	var wg sync.WaitGroup
	for _, q := range []string{"alta", "media", "baja"} {
		wg.Add(1)
		go func(q string) {
			defer wg.Done()
			if est, err := media.EstimateSampled(ctx, ff, in, media.ReduceOptions{Quality: q}); err == nil && a.gen == gen {
				a.emit("plan-est", map[string]any{"id": q, "est": est})
			}
		}(q)
	}
	wg.Wait()
}

func groupName(t media.Target, inKind string) string {
	switch t.Kind {
	case "gif":
		return "Animación"
	case "audio":
		if inKind == "video" {
			return "Solo audio"
		}
		return "Audio"
	case "image":
		return "Imagen"
	}
	return "Vídeo"
}

func (a *App) planConvert(p *Plan, files []FileInfo) {
	switch p.Kind {
	case "video", "audio":
		if ffmpegPath() == "" {
			p.Needs = "ffmpeg"
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		in, err := probe(ctx, files[0])
		if err != nil {
			p.Error = err.Error()
			return
		}
		if len(files) == 1 {
			p.Info = mediaSub(in)
		}
		for _, t := range media.Targets(in) {
			p.Targets = append(p.Targets, PlanTarget{ID: t.ID, Ext: t.Ext, Name: t.Name, Desc: stripPrefix(t.Desc, t.Name), Group: groupName(t, in.Kind), GIF: t.Kind == "gif", engine: "media", kind: in.Kind})
		}
	case "image":
		a.planImageConvert(p, files)
	case "pdf":
		a.planPDFConvert(p, files)
	case "doc":
		a.planDocConvert(p, files)
	default:
		p.Kind = "mixed"
	}
	// Los formatos usados hace poco, arriba.
	recent := getSettings().Recent[p.Kind]
	rank := func(id string) int {
		for i, r := range recent {
			if r == id {
				return i
			}
		}
		return 99
	}
	for i := range p.Targets {
		p.Targets[i].Recent = rank(p.Targets[i].ID) < 99
	}
	if len(recent) > 0 {
		groupOrder := map[string]int{}
		for _, t := range p.Targets {
			if _, ok := groupOrder[t.Group]; !ok {
				groupOrder[t.Group] = len(groupOrder)
			}
		}
		sort.SliceStable(p.Targets, func(i, j int) bool {
			gi, gj := groupOrder[p.Targets[i].Group], groupOrder[p.Targets[j].Group]
			if gi != gj {
				return gi < gj
			}
			return rank(p.Targets[i].ID) < rank(p.Targets[j].ID)
		})
	}
}

// stripPrefix quita «MP4: » del principio de la descripción (el nombre ya se ve al lado).
func stripPrefix(desc, name string) string {
	if i := strings.Index(desc, ": "); i > 0 && i < 16 {
		d := desc[i+2:]
		if d != "" {
			return strings.ToUpper(d[:1]) + d[1:]
		}
	}
	return desc
}

// ---------- ejecución ----------

type RunReq struct {
	Mode     string  `json:"mode"`
	Preset   string  `json:"preset"`
	TargetMB float64 `json:"targetMB"`
	Quality  int     `json:"quality"`
	Scale    string  `json:"scale"`
	Target   string  `json:"target"`
	GIFFps   int     `json:"gifFps"`
	GIFWidth int     `json:"gifWidth"`
	Replace  bool    `json:"replace"`
}

type RunItem struct {
	In     string `json:"in"`
	Out    string `json:"out,omitempty"`
	Before int64  `json:"before"`
	After  int64  `json:"after"`
	Error  string `json:"error,omitempty"`
	Note   string `json:"note,omitempty"`
}

type RunResult struct {
	Results  []RunItem `json:"results"`
	Canceled bool      `json:"canceled,omitempty"`
	Replaced bool      `json:"replaced,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// progressFn recibe el avance de un archivo (0..1), la velocidad y los segundos que faltan.
type progressFn func(frac, speed, eta float64, stage string)

// Run ejecuta «Reducir» o «Convertir» sobre la selección, archivo por archivo.
func (a *App) Run(req RunReq) RunResult {
	files, job := a.jobFiles()
	if job == nil {
		return RunResult{Error: "No hay nada seleccionado"}
	}
	a.endTask("estimate")
	ctx := a.newTask("job")
	defer a.endTask("job")
	kind := planKind(files)
	res := RunResult{}
	var total int64
	for _, f := range files {
		total += max(f.Size, 1)
	}
	var doneBytes int64
	start := time.Now()
	report := func(i int, f FileInfo) progressFn {
		return func(frac, speed, eta float64, stage string) {
			overall := (float64(doneBytes) + frac*float64(max(f.Size, 1))) / float64(total)
			gEta := eta
			if len(files) > 1 && overall > 0.02 {
				el := time.Since(start).Seconds()
				gEta = el/overall - el
			}
			a.emit("job-progress", map[string]any{"overall": overall, "frac": frac, "index": i, "eta": gEta, "speed": speed, "phase": stage})
		}
	}

	// Comprimir en ZIP o unir en PDF: una sola salida para todo.
	if req.Mode == "reduce" && kind != "video" && kind != "audio" && kind != "image" && kind != "pdf" || req.Mode == "convert" && req.Target == "pdfjoin" {
		var out string
		var err error
		before := total
		if req.Target == "pdfjoin" {
			out, err = a.joinToPDF(ctx, files, report(0, FileInfo{Size: total}))
		} else {
			var n int64
			out, n, err = makeZip(ctx, files, func(f, s, e float64, st string) { a.emit("job-progress", map[string]any{"overall": f, "frac": f, "index": 0, "eta": e, "phase": st}) })
			if n > 0 {
				before = n
			}
		}
		item := RunItem{In: files[0].Path, Before: before}
		if err != nil {
			item.Error = err.Error()
		} else {
			item.Out = out
			if st, e := os.Stat(out); e == nil {
				item.After = st.Size()
			}
		}
		res.Results = append(res.Results, item)
		res.Canceled = ctx.Err() != nil
		a.finishRun(&res, req, kind)
		return res
	}

	for i, f := range files {
		if ctx.Err() != nil {
			break
		}
		pr := report(i, f)
		pr(0, 0, -1, "")
		item := RunItem{In: f.Path, Before: f.Size}
		out, note, err := a.runOne(ctx, req, f, pr)
		if err != nil {
			if media.IsCanceled(err) || ctx.Err() != nil {
				res.Canceled = true
				break
			}
			item.Error = err.Error()
		} else {
			item.Out, item.Note = out, note
			if st, e := os.Stat(out); e == nil {
				item.After = st.Size()
			}
		}
		res.Results = append(res.Results, item)
		doneBytes += max(f.Size, 1)
	}
	if !res.Canceled && req.Replace && req.Mode == "reduce" {
		res.Replaced = replaceOriginals(res.Results)
	}
	a.finishRun(&res, req, kind)
	return res
}

func (a *App) finishRun(res *RunResult, req RunReq, kind string) {
	if req.Mode == "convert" && req.Target != "" {
		ok := false
		for _, r := range res.Results {
			ok = ok || r.Error == ""
		}
		if ok {
			rememberFormat(kind, req.Target)
		}
	}
	for _, r := range res.Results {
		if r.Out != "" {
			notifyCreated(r.Out)
		}
	}
}

func (a *App) runOne(ctx context.Context, req RunReq, f FileInfo, pr progressFn) (string, string, error) {
	switch f.Kind {
	case "video", "audio":
		ff := ffmpegPath()
		if ff == "" {
			return "", "", media.ErrNoFFmpeg
		}
		in, err := probe(ctx, f)
		if err != nil {
			return "", "", err
		}
		mp := func(p media.Progress) { pr(p.Fraction, p.Speed, p.ETA, p.Stage) }
		var r *media.Result
		if req.Mode == "reduce" {
			opt := media.ReduceOptions{}
			switch req.Preset {
			case "discord":
				opt.TargetBytes = media.SizeDiscord
			case "whatsapp":
				opt.TargetBytes = media.SizeWhatsApp
			case "target":
				opt.TargetBytes = int64(req.TargetMB * 1048576)
			case "alta", "media", "baja":
				opt.Quality = req.Preset
			default:
				opt.Quality = media.QualityMedium
			}
			r, err = media.Reduce(ctx, ff, in, opt, mp)
		} else {
			r, err = media.Convert(ctx, ff, in, req.Target, media.ConvertOptions{GIFFPS: req.GIFFps, GIFSize: req.GIFWidth}, mp)
		}
		if err != nil {
			return "", "", err
		}
		return r.Path, r.Note, nil
	case "image":
		return a.runImage(ctx, req, f, pr)
	case "pdf":
		return a.runPDF(ctx, req, f, pr)
	case "doc":
		return a.runDoc(ctx, req, f, pr)
	}
	return "", "", errors.New("No se sabe tratar este tipo de archivo")
}

// replaceOriginals manda los originales a la papelera y pone el resultado en su lugar.
func replaceOriginals(items []RunItem) bool {
	any := false
	for i, r := range items {
		if r.Error != "" || r.Out == "" {
			continue
		}
		if err := lock.Recycle([]string{r.In}); err != nil {
			continue
		}
		dst := strings.TrimSuffix(r.In, filepath.Ext(r.In)) + filepath.Ext(r.Out)
		if _, err := os.Stat(dst); err == nil {
			dst = uniquePath(filepath.Dir(dst), filepath.Base(dst))
		}
		if os.Rename(r.Out, dst) == nil {
			items[i].Out = dst
			any = true
		}
	}
	return any
}

// DownloadFFmpeg descarga el motor de vídeo con progreso (evento "ffmpeg-dl").
func (a *App) DownloadFFmpeg() error {
	ctx := a.newTask("ffmpeg")
	defer a.endTask("ffmpeg")
	var last time.Time
	err := media.Download(ctx, binDir(), func(done, total int64) {
		if time.Since(last) > 120*time.Millisecond || done == total {
			last = time.Now()
			a.emit("ffmpeg-dl", map[string]int64{"done": done, "total": total})
		}
	})
	if err == nil {
		go media.HardwareEncoder(context.Background(), ffmpegPath())
	}
	return err
}
