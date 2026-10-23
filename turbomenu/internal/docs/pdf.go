package docs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/klauspost/compress/zlib"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func init() {
	// Que pdfcpu no cree su carpeta de configuración en %APPDATA%.
	model.ConfigPath = "disable"
}

func pdfConf(cmd model.CommandMode) *model.Configuration {
	c := model.NewStatelessConfiguration()
	c.Cmd = cmd
	c.ValidationMode = model.ValidationRelaxed
	return c
}

// PDFInfo resume un PDF.
type PDFInfo struct {
	Pages     int    `json:"pages"`
	Images    int    `json:"images"`    // imágenes incrustadas (0 = solo texto/vectores)
	Size      int64  `json:"size"`      // bytes del archivo
	Encrypted bool   `json:"encrypted"` // tiene restricciones o contraseña
	Version   string `json:"version"`
}

// pdfReadErr traduce los errores de lectura de pdfcpu.
func pdfReadErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		return ErrCanceled
	}
	if errors.Is(err, pdfcpu.ErrWrongPassword) || strings.Contains(err.Error(), "password") {
		return uerr(err, "El PDF está protegido con contraseña")
	}
	return cleanErr(ctx, err, "El PDF está dañado o no se puede leer")
}

func readPDF(ctx context.Context, path string, conf *model.Configuration, validate bool) (*model.Context, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, cleanErr(ctx, err, "No se puede abrir el PDF")
	}
	defer f.Close()
	var pc *model.Context
	if validate {
		pc, err = api.ReadValidateAndOptimize(ctx, f, conf, nil)
	} else {
		pc, err = api.ReadContext(ctx, f, conf)
	}
	if err != nil {
		return nil, pdfReadErr(ctx, err)
	}
	return pc, nil
}

// InspectPDF devuelve páginas, nº de imágenes y tamaño de un PDF.
func InspectPDF(path string) (PDFInfo, error) {
	ctx := context.Background()
	pc, err := readPDF(ctx, path, pdfConf(model.VALIDATE), false)
	if err != nil {
		return PDFInfo{}, err
	}
	if err := pc.EnsurePageCount(); err != nil {
		return PDFInfo{}, pdfReadErr(ctx, err)
	}
	info := PDFInfo{Pages: pc.PageCount, Size: fileSize(path), Encrypted: pc.Encrypt != nil}
	if v := pc.HeaderVersion; v != nil {
		info.Version = v.String()
	}
	for _, e := range pc.Table {
		if e == nil || e.Free || e.Object == nil {
			continue
		}
		if sd, ok := e.Object.(types.StreamDict); ok {
			if st := sd.Subtype(); st != nil && *st == "Image" {
				if im := sd.BooleanEntry("ImageMask"); im == nil || !*im {
					info.Images++
				}
			}
		}
	}
	return info, nil
}

// PDFLevel es el nivel de reducción de un PDF.
type PDFLevel string

const (
	// PDFScreen: para leer en pantalla (imágenes ~150 ppp, JPEG calidad 60).
	PDFScreen PDFLevel = "screen"
	// PDFPrint: para imprimir (imágenes ~300 ppp, JPEG calidad 85).
	PDFPrint PDFLevel = "print"
)

type pdfLevelCfg struct {
	maxSide int
	quality int
}

func levelCfg(l PDFLevel) pdfLevelCfg {
	if l == PDFPrint {
		return pdfLevelCfg{maxSide: 3500, quality: 85}
	}
	return pdfLevelCfg{maxSide: 1600, quality: 60}
}

// ReducePDF guarda una versión más ligera del PDF («doc (reducido).pdf»):
// recomprime y reescala las imágenes, quita duplicados y comprime mejor el
// resto. Si no se gana nada devuelve ErrNotSmaller (sin guardar).
func ReducePDF(ctx context.Context, path string, level PDFLevel, progress Progress) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := levelCfg(level)
	before := fileSize(path)
	progress.set(0.02)
	pc, err := readPDF(ctx, path, pdfConf(model.OPTIMIZE), true)
	if err != nil {
		return Result{}, err
	}
	progress.set(0.2)
	if err := recompressImages(ctx, pc, cfg, progress.sub(0.2, 0.75)); err != nil {
		return Result{}, cleanErr(ctx, err, "No se ha podido reducir el PDF")
	}
	if err := recompressStreams(ctx, pc); err != nil {
		return Result{}, cleanErr(ctx, err, "No se ha podido reducir el PDF")
	}
	progress.set(0.85)
	var buf bytes.Buffer
	buf.Grow(int(before))
	if err := api.WriteContext(ctx, pc, &buf); err != nil {
		return Result{}, cleanErr(ctx, err, "No se ha podido reducir el PDF")
	}
	progress.set(0.92)
	res := Result{Before: before, After: int64(buf.Len())}
	res.Larger = res.After >= res.Before
	if res.Larger {
		progress.set(1)
		return res, ErrNotSmaller
	}
	// Comprobación de que el resultado se abre bien.
	if _, err := api.ReadAndValidate(ctx, bytes.NewReader(buf.Bytes()), pdfConf(model.VALIDATE)); err != nil {
		return Result{}, cleanErr(ctx, err, "No se ha podido reducir este PDF")
	}
	out, err := writeOut(path, "reducido", ".pdf", buf.Bytes())
	if err != nil {
		return Result{}, err
	}
	res.Out = out
	progress.set(1)
	return res, nil
}

// imgCand es una imagen del PDF que se puede recomprimir.
type imgCand struct {
	objNr  int
	sd     types.StreamDict
	w, h   int
	comps  int // 1 gris, 3 color
	filter string
	isMask bool // es la SMask de otra imagen: se mantiene sin pérdida
}

func recompressImages(ctx context.Context, pc *model.Context, cfg pdfLevelCfg, progress Progress) error {
	masks := map[int]bool{}
	for _, e := range pc.Table {
		if e == nil || e.Free || e.Object == nil {
			continue
		}
		if sd, ok := e.Object.(types.StreamDict); ok {
			if ir := sd.IndirectRefEntry("SMask"); ir != nil {
				masks[ir.ObjectNumber.Value()] = true
			}
		}
	}
	var cands []imgCand
	for nr, e := range pc.Table {
		if e == nil || e.Free || e.Object == nil {
			continue
		}
		sd, ok := e.Object.(types.StreamDict)
		if !ok {
			continue
		}
		if c, ok := candidate(pc, nr, sd, masks[nr]); ok {
			cands = append(cands, c)
		}
	}
	if len(cands) == 0 {
		return nil
	}
	var mu sync.Mutex
	done := 0
	var firstErr error
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for _, c := range cands {
		wg.Add(1)
		sem <- struct{}{}
		go func(c imgCand) {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			nsd := shrinkImage(ctx, c, cfg)
			mu.Lock()
			defer mu.Unlock()
			if nsd != nil {
				pc.Table[c.objNr].Object = *nsd
			}
			done++
			progress.set(float64(done) / float64(len(cands)))
		}(c)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ErrCanceled
	}
	return firstErr
}

// candidate decide si una imagen se puede recomprimir con seguridad.
func candidate(pc *model.Context, nr int, sd types.StreamDict, isMask bool) (imgCand, bool) {
	c := imgCand{objNr: nr, sd: sd, isMask: isMask}
	if st := sd.Subtype(); st == nil || *st != "Image" {
		return c, false
	}
	if im := sd.BooleanEntry("ImageMask"); im != nil && *im {
		return c, false
	}
	if sd.HasEntry("Mask") || sd.HasEntry("Decode") || sd.HasEntry("SMaskInData") {
		return c, false
	}
	if bpc := sd.IntEntry("BitsPerComponent"); bpc == nil || *bpc != 8 {
		return c, false
	}
	w, h := sd.IntEntry("Width"), sd.IntEntry("Height")
	if w == nil || h == nil || *w < 16 || *h < 16 {
		return c, false
	}
	c.w, c.h = *w, *h
	if len(sd.FilterPipeline) != 1 {
		return c, false
	}
	c.filter = sd.FilterPipeline[0].Name
	if c.filter != "DCTDecode" && c.filter != "FlateDecode" {
		return c, false
	}
	if len(sd.Raw) < 8<<10 {
		return c, false
	}
	// Espacio de color: gris o RGB (directo, ICC, Cal).
	cs, ok := sd.Find("ColorSpace")
	if !ok {
		return c, false
	}
	o, err := pc.Dereference(cs)
	if err != nil {
		return c, false
	}
	switch v := o.(type) {
	case types.Name:
		switch v {
		case "DeviceRGB":
			c.comps = 3
		case "DeviceGray":
			c.comps = 1
		}
	case types.Array:
		if len(v) < 1 {
			break
		}
		n, _ := v[0].(types.Name)
		switch n {
		case "CalRGB":
			c.comps = 3
		case "CalGray":
			c.comps = 1
		case "ICCBased":
			if len(v) > 1 {
				if icc, err := pc.Dereference(v[1]); err == nil {
					if isd, ok := icc.(types.StreamDict); ok {
						if nn := isd.IntEntry("N"); nn != nil && (*nn == 1 || *nn == 3) {
							c.comps = *nn
						}
					}
				}
			}
		}
	}
	if c.comps == 0 {
		return c, false
	}
	if isMask && c.comps != 1 {
		return c, false
	}
	return c, true
}

// shrinkImage devuelve el nuevo stream de la imagen, o nil si no compensa.
func shrinkImage(ctx context.Context, c imgCand, cfg pdfLevelCfg) *types.StreamDict {
	var img image.Image
	switch c.filter {
	case "DCTDecode":
		m, err := jpeg.Decode(bytes.NewReader(c.sd.Raw))
		if err != nil {
			return nil
		}
		switch m.(type) {
		case *image.YCbCr:
			if c.comps != 3 {
				return nil
			}
		case *image.Gray:
			if c.comps != 1 {
				return nil
			}
		default:
			return nil
		}
		b := m.Bounds()
		if b.Dx() != c.w || b.Dy() != c.h {
			return nil
		}
		img = normalize(m)
	case "FlateDecode":
		sd := c.sd
		sd.Content = nil
		if err := sd.DecodeWithLimit(int64(c.w*c.h*c.comps) + 1<<20); err != nil {
			return nil
		}
		data := sd.Content
		if len(data) < c.w*c.h*c.comps {
			return nil
		}
		if c.comps == 1 {
			img = &image.Gray{Pix: data[:c.w*c.h], Stride: c.w, Rect: image.Rect(0, 0, c.w, c.h)}
		} else {
			m := image.NewNRGBA(image.Rect(0, 0, c.w, c.h))
			for i, j := 0, 0; i < c.w*c.h*3; i, j = i+3, j+4 {
				m.Pix[j], m.Pix[j+1], m.Pix[j+2], m.Pix[j+3] = data[i], data[i+1], data[i+2], 255
			}
			img = m
		}
	}
	nw, nh := fitSize(c.w, c.h, cfg.maxSide, 0)
	resized := nw != c.w || nh != c.h
	lossless := c.isMask || (c.filter == "FlateDecode" && fewColors(img))
	if lossless && !resized {
		return nil
	}
	if resized {
		img = resizeImage(img, nw, nh)
	}
	var data []byte
	var filter types.PDFFilter
	if lossless {
		var p *pngImage
		if g, ok := img.(*image.Gray); ok {
			p = pngFromImage(g)
		} else {
			m := toNRGBA(img)
			p = &pngImage{w: nw, h: nh, colorTyp: 2, depth: 8, rowBytes: nw * 3, rows: func(y int, dst []byte) {
				s := m.Pix[y*m.Stride:]
				for x := range nw {
					dst[x*3], dst[x*3+1], dst[x*3+2] = s[x*4], s[x*4+1], s[x*4+2]
				}
			}}
		}
		z, err := p.idat(ctx, 9)
		if err != nil {
			return nil
		}
		data = z
		parms := types.Dict{}
		parms.Insert("Predictor", types.Integer(15))
		parms.Insert("Colors", types.Integer(c.comps))
		parms.Insert("BitsPerComponent", types.Integer(8))
		parms.Insert("Columns", types.Integer(nw))
		filter = types.PDFFilter{Name: "FlateDecode", DecodeParms: parms}
	} else {
		r, err := encodeJPEG(ctx, img, cfg.quality, nil, 0)
		if err != nil {
			return nil
		}
		data = r.data
		filter = types.PDFFilter{Name: "DCTDecode"}
	}
	// Solo si se gana al menos un 10 %.
	if len(data) > len(c.sd.Raw)*9/10 {
		return nil
	}
	d := c.sd.Dict.Clone().(types.Dict)
	d.Update("Width", types.Integer(nw))
	d.Update("Height", types.Integer(nh))
	d.Update("BitsPerComponent", types.Integer(8))
	d.Update("Filter", types.Name(filter.Name))
	d.Delete("DecodeParms")
	d.Delete("DL")
	if filter.DecodeParms != nil {
		d.Insert("DecodeParms", filter.DecodeParms)
	}
	l := int64(len(data))
	d.Update("Length", types.Integer(l))
	nsd := types.StreamDict{Dict: d, Raw: data, StreamLength: &l, FilterPipeline: []types.PDFFilter{filter}}
	return &nsd
}

// fewColors dice si la imagen parece un gráfico (pocos colores) y no una foto.
func fewColors(img image.Image) bool {
	seen := map[uint32]struct{}{}
	b := img.Bounds()
	step := max(1, b.Dx()*b.Dy()/65536)
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i++
			if i%step != 0 {
				continue
			}
			r, g, bb, _ := img.At(x, y).RGBA()
			seen[r>>8<<16|g>>8<<8|bb>>8] = struct{}{}
			if len(seen) > 512 {
				return false
			}
		}
	}
	return true
}

// recompressStreams comprime los streams sin filtro y vuelve a comprimir los
// Flate con el nivel máximo si así ocupan menos.
func recompressStreams(ctx context.Context, pc *model.Context) error {
	type item struct {
		nr int
		sd types.StreamDict
	}
	var items []item
	for nr, e := range pc.Table {
		if e == nil || e.Free || e.Object == nil {
			continue
		}
		sd, ok := e.Object.(types.StreamDict)
		if !ok || len(sd.Raw) < 256 {
			continue
		}
		if t := sd.Type(); t != nil && (*t == "Metadata" || *t == "XRef" || *t == "ObjStm") {
			continue
		}
		if st := sd.Subtype(); st != nil && *st == "Image" && len(sd.FilterPipeline) > 0 {
			continue
		}
		switch {
		case len(sd.FilterPipeline) == 0:
		case len(sd.FilterPipeline) == 1 && sd.FilterPipeline[0].Name == "FlateDecode" && sd.FilterPipeline[0].DecodeParms == nil:
		default:
			continue
		}
		items = append(items, item{nr, sd})
	}
	var mu sync.Mutex
	parallel(len(items), func(lo, hi int) {
		for _, it := range items[lo:hi] {
			if ctx.Err() != nil {
				return
			}
			sd := it.sd
			plain := sd.Raw
			if len(sd.FilterPipeline) == 1 {
				sd.Content = nil
				if err := sd.DecodeWithLimit(256 << 20); err != nil {
					continue
				}
				plain = sd.Content
			}
			var buf bytes.Buffer
			zw, _ := zlib.NewWriterLevel(&buf, 9)
			zw.Write(plain)
			zw.Close()
			if buf.Len() >= len(it.sd.Raw) {
				continue
			}
			d := it.sd.Dict.Clone().(types.Dict)
			d.Update("Filter", types.Name("FlateDecode"))
			d.Delete("DecodeParms")
			l := int64(buf.Len())
			d.Update("Length", types.Integer(l))
			nsd := types.StreamDict{Dict: d, Raw: buf.Bytes(), StreamLength: &l,
				FilterPipeline: []types.PDFFilter{{Name: "FlateDecode"}}, IsPageContent: it.sd.IsPageContent}
			mu.Lock()
			pc.Table[it.nr].Object = nsd
			mu.Unlock()
		}
	})
	return ctxErr(ctx)
}

// MergePDFs une varios PDF en uno, en el orden dado. Se guarda junto al
// primero como «nombre (unido).pdf».
func MergePDFs(ctx context.Context, paths []string, progress Progress) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(paths) < 2 {
		return Result{}, uerr(nil, "Hacen falta al menos dos PDF")
	}
	var before int64
	files := make([]io.ReadSeeker, 0, len(paths))
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return Result{}, cleanErr(ctx, err, "No se puede abrir uno de los PDF")
		}
		defer f.Close()
		files = append(files, f)
		before += fileSize(p)
	}
	progress.set(0.05)
	var buf bytes.Buffer
	if err := api.MergeRaw(ctx, files, &buf, false, pdfConf(model.MERGECREATE)); err != nil {
		return Result{}, pdfReadErr(ctx, err)
	}
	progress.set(0.9)
	out, err := writeOut(paths[0], "unido", ".pdf", buf.Bytes())
	if err != nil {
		return Result{}, err
	}
	progress.set(1)
	return Result{Out: out, Before: before, After: int64(buf.Len())}, nil
}

// SplitPDF separa cada página en un PDF suelto, dentro de la carpeta nueva
// «doc (páginas)». Result.Out es la carpeta.
func SplitPDF(ctx context.Context, path string, progress Progress) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	pc, err := readPDF(ctx, path, pdfConf(model.SPLIT), true)
	if err != nil {
		return Result{}, err
	}
	if err := pc.EnsurePageCount(); err != nil {
		return Result{}, pdfReadErr(ctx, err)
	}
	n := pc.PageCount
	if n < 2 {
		return Result{}, uerr(nil, "El PDF solo tiene una página")
	}
	dir, err := makeOutDir(path, "páginas")
	if err != nil {
		return Result{}, err
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	digits := len(fmt.Sprint(n))
	var total int64
	for i := 1; i <= n; i++ {
		if ctx.Err() != nil {
			os.RemoveAll(dir)
			return Result{}, ErrCanceled
		}
		one, err := pdfcpu.ExtractPages(ctx, pc, []int{i}, false)
		if err != nil {
			os.RemoveAll(dir)
			return Result{}, pdfReadErr(ctx, err)
		}
		var buf bytes.Buffer
		if err := api.WriteContext(ctx, one, &buf); err != nil {
			os.RemoveAll(dir)
			return Result{}, pdfReadErr(ctx, err)
		}
		name := filepath.Join(dir, fmt.Sprintf("%s - página %0*d.pdf", base, digits, i))
		if err := os.WriteFile(name, buf.Bytes(), 0o644); err != nil {
			os.RemoveAll(dir)
			return Result{}, cleanErr(ctx, err, "No se han podido guardar las páginas")
		}
		total += int64(buf.Len())
		progress.set(float64(i) / float64(n))
	}
	return Result{Out: dir, Before: fileSize(path), After: total}, nil
}

// makeOutDir crea una carpeta nueva y libre junto a path.
func makeOutDir(path, label string) (string, error) {
	reserveMu.Lock()
	defer reserveMu.Unlock()
	for range 50 {
		dir := uniqueDir(path, label)
		err := os.Mkdir(dir, 0o755)
		if err == nil {
			return dir, nil
		}
		if !os.IsExist(err) {
			return "", cleanErr(nil, err, "No se puede crear la carpeta de salida")
		}
	}
	return "", uerr(nil, "No se puede crear la carpeta de salida")
}
