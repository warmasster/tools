package main

// Imágenes, PDF y documentos de Office para «Reducir» y «Convertir».

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"turbomenu/internal/docs"
	"turbomenu/internal/media"
)

func officeEngine() string {
	name, ok := docs.OfficeEngine()
	if !ok {
		return ""
	}
	return name
}

func docsProgress(pr progressFn) docs.Progress {
	return func(f float64) { pr(f, 0, -1, "") }
}

// ---------- imágenes ----------

func (a *App) planImageReduce(p *Plan, files []FileInfo) {
	p.Image = &ImagePlan{Quality: 80}
	if q := getSettings().LastImage; q >= 10 && q <= 100 {
		p.Image.Quality = q
	}
	w, h := 0, 0
	if len(files) == 1 {
		if in, err := docs.InspectImage(files[0].Path); err == nil {
			w, h = in.Width, in.Height
			p.Info = fmt.Sprintf("Imagen %s · <b>%s</b> · %d × %d", in.Name, humanSize(in.Size), w, h)
			if in.Frames > 1 {
				p.Info += fmt.Sprintf(" · %d fotogramas", in.Frames)
			}
		} else {
			p.Error = err.Error()
			return
		}
	}
	for _, s := range []int{100, 75, 50, 25} {
		sc := PlanScale{ID: strconv.Itoa(s), Title: "Original"}
		if s != 100 {
			sc.Title = fmt.Sprintf("%d %%", s)
		}
		if w > 0 {
			sc.Sub = fmt.Sprintf("%d × %d", w*s/100, h*s/100)
		}
		p.Image.Scales = append(p.Image.Scales, sc)
	}
}

// ImagePreview devuelve el antes/después y el peso exacto que tendrá la imagen.
func (a *App) ImagePreview(path string, quality int, scale string) (map[string]any, error) {
	sc, _ := strconv.Atoi(scale)
	r, err := docs.Preview(path, docs.ImageOptions{Quality: quality, Scale: sc}, 760)
	if err != nil {
		return nil, err
	}
	return map[string]any{"before": r.Original, "after": r.Result, "size": r.Size, "origSize": r.OrigSize, "larger": r.Larger}, nil
}

func (a *App) planImageConvert(p *Plan, files []FileInfo) {
	ff := ffmpegPath()
	if len(files) > 1 {
		p.Targets = append(p.Targets, PlanTarget{ID: "pdfjoin", Ext: "pdf", Name: "Un solo PDF", Desc: fmt.Sprintf("Las %d imágenes en un PDF, una por página", len(files)), Group: "Juntar"})
	}
	for _, t := range docs.Targets(files[0].Path) {
		p.Targets = append(p.Targets, PlanTarget{ID: t.ID, Ext: strings.TrimPrefix(t.Ext, "."), Name: t.Name, Desc: t.Desc, Group: "Imagen", engine: "docs"})
	}
	own := lowerExt(files[0].Path)
	for _, t := range []PlanTarget{
		{ID: "webp", Ext: "webp", Name: "WEBP", Desc: "Muy ligero, ideal para webs"},
		{ID: "avif", Ext: "avif", Name: "AVIF", Desc: "Lo más ligero, para webs modernas"},
	} {
		if own == t.ID {
			continue
		}
		t.Group, t.engine = "Imagen", "media"
		if ff == "" {
			t.Disabled, t.Needs = true, "Necesita el motor de vídeo (FFmpeg)"
		}
		p.Targets = append(p.Targets, t)
	}
}

func (a *App) runImage(ctx context.Context, req RunReq, f FileInfo, pr progressFn) (string, string, error) {
	if req.Mode == "reduce" {
		sc, _ := strconv.Atoi(req.Scale)
		updateSettings(func(s *Settings) { s.LastImage = req.Quality })
		r, err := docs.ReduceImage(ctx, f.Path, docs.ImageOptions{Quality: req.Quality, Scale: sc}, docsProgress(pr))
		if errors.Is(err, docs.ErrNotSmaller) {
			return "", "", errors.New("Con esta calidad no pesaría menos. Prueba más baja o haz la imagen más pequeña.")
		}
		return r.Out, "", err
	}
	if req.Target == "webp" || req.Target == "avif" {
		ff := ffmpegPath()
		if ff == "" {
			return "", "", media.ErrNoFFmpeg
		}
		out := media.OutputPath(f.Path, "", req.Target)
		pr(0.1, 0, -1, "")
		if err := media.EncodeImage(ctx, ff, f.Path, out, req.Target, 80, 0); err != nil {
			return "", "", err
		}
		return out, "", nil
	}
	r, err := docs.Convert(ctx, f.Path, req.Target, docs.ConvertOptions{}, docsProgress(pr))
	return r.Out, "", err
}

func (a *App) joinToPDF(ctx context.Context, files []FileInfo, pr progressFn) (string, error) {
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	if files[0].Kind == "pdf" {
		r, err := docs.MergePDFs(ctx, paths, docsProgress(pr))
		return r.Out, err
	}
	r, err := docs.ImagesToPDF(ctx, paths, docs.PDFOptions{}, docsProgress(pr))
	return r.Out, err
}

// ---------- PDF ----------

func (a *App) planPDFConvert(p *Plan, files []FileInfo) {
	if len(files) > 1 {
		p.Targets = append(p.Targets, PlanTarget{ID: "pdfjoin", Ext: "pdf", Name: "Unir en un PDF", Desc: fmt.Sprintf("Los %d PDF en uno, en este orden", len(files)), Group: "Juntar"})
	}
	for _, t := range docs.Targets(files[0].Path) {
		p.Targets = append(p.Targets, PlanTarget{ID: t.ID, Ext: strings.TrimPrefix(t.Ext, "."), Name: t.Name, Desc: t.Desc, Group: "Imágenes", engine: "docs"})
	}
	p.Targets = append(p.Targets, PlanTarget{ID: "split", Ext: "pdf", Name: "Separar páginas", Desc: "Un PDF por cada página", Group: "PDF"})
	if len(files) == 1 {
		if in, err := docs.InspectPDF(files[0].Path); err == nil {
			p.Info = fmt.Sprintf("PDF · <b>%s</b> · %d páginas", humanSize(in.Size), in.Pages)
		}
	}
}

func (a *App) runPDF(ctx context.Context, req RunReq, f FileInfo, pr progressFn) (string, string, error) {
	var r docs.Result
	var err error
	switch {
	case req.Mode == "reduce":
		lvl := docs.PDFScreen
		if req.Preset == "print" {
			lvl = docs.PDFPrint
		}
		r, err = docs.ReducePDF(ctx, f.Path, lvl, docsProgress(pr))
		if errors.Is(err, docs.ErrNotSmaller) {
			return "", "", errors.New("Este PDF ya está muy optimizado: no se puede reducir más sin perder calidad.")
		}
	case req.Target == "split":
		r, err = docs.SplitPDF(ctx, f.Path, docsProgress(pr))
	default:
		r, err = docs.Convert(ctx, f.Path, req.Target, docs.ConvertOptions{}, docsProgress(pr))
	}
	return r.Out, "", err
}

// ---------- documentos de Office ----------

func (a *App) planDocConvert(p *Plan, files []FileInfo) {
	_, ok := docs.OfficeEngineFor(files[0].Path)
	if !ok {
		p.Needs = "office"
		return
	}
	for _, t := range docs.Targets(files[0].Path) {
		p.Targets = append(p.Targets, PlanTarget{ID: t.ID, Ext: strings.TrimPrefix(t.Ext, "."), Name: t.Name, Desc: t.Desc, Group: "Documento", engine: "docs"})
	}
}

func (a *App) runDoc(ctx context.Context, req RunReq, f FileInfo, pr progressFn) (string, string, error) {
	if req.Mode == "reduce" {
		return "", "", errors.New("Los documentos no se reducen: comprímelos en ZIP")
	}
	pr(0.05, 0, -1, "Abriendo el documento…")
	r, err := docs.OfficeToPDF(ctx, f.Path, "")
	return r.Out, "", err
}
