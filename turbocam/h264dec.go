package main

// Decodificador H.264 del PC: el de Media Foundation, que viene con Windows (lo usa
// también la app Películas y TV). Recibe el vídeo tal cual sale del codificador por
// hardware del móvil (Annex B: con los SPS/PPS delante de cada fotograma clave) y
// devuelve NV12, que es lo que lee la cámara virtual.
//
// Se habla con él por COM a mano, sin librerías. Cada decodificador vive en un
// hilo fijo (runtime.LockOSThread) con COM iniciado: newH264Decoder, decode y close
// se llaman siempre desde ese mismo hilo.

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	mfplat                   = windows.NewLazySystemDLL("mfplat.dll")
	ole32                    = windows.NewLazySystemDLL("ole32.dll")
	procMFStartup            = mfplat.NewProc("MFStartup")
	procMFCreateMediaType    = mfplat.NewProc("MFCreateMediaType")
	procMFCreateSample       = mfplat.NewProc("MFCreateSample")
	procMFCreateMemoryBuffer = mfplat.NewProc("MFCreateMemoryBuffer")
	procCoCreateInstance     = ole32.NewProc("CoCreateInstance")
	procCoInitializeEx       = ole32.NewProc("CoInitializeEx")

	clsidH264Decoder = windows.GUID{Data1: 0x62CE7E72, Data2: 0x4C71, Data3: 0x4D20, Data4: [8]byte{0xB1, 0x5D, 0x45, 0x28, 0x31, 0xA8, 0x7D, 0x9D}}
	iidIMFTransform  = windows.GUID{Data1: 0xBF94C121, Data2: 0x5B05, Data3: 0x4E6F, Data4: [8]byte{0x80, 0x00, 0xBA, 0x59, 0x89, 0x61, 0x41, 0x4D}}

	mfMTMajorType       = windows.GUID{Data1: 0x48eba18e, Data2: 0xf8c9, Data3: 0x4687, Data4: [8]byte{0xbf, 0x11, 0x0a, 0x74, 0xc9, 0xf9, 0x6a, 0x8f}}
	mfMTSubtype         = windows.GUID{Data1: 0xf7e34c9a, Data2: 0x42e8, Data3: 0x4714, Data4: [8]byte{0xb7, 0x4b, 0xcb, 0x29, 0xd7, 0x2c, 0x35, 0xe5}}
	mfMTFrameSize       = windows.GUID{Data1: 0x1652c33d, Data2: 0xd6b2, Data3: 0x4012, Data4: [8]byte{0xb8, 0x34, 0x72, 0x03, 0x08, 0x49, 0xa3, 0x7d}}
	mfMTInterlace       = windows.GUID{Data1: 0xe2724bb8, Data2: 0xe676, Data3: 0x4806, Data4: [8]byte{0xb4, 0xb2, 0xa8, 0xd6, 0xef, 0xb4, 0x4c, 0xcd}}
	mfMTDefaultStride   = windows.GUID{Data1: 0x644b4e48, Data2: 0x1e02, Data3: 0x4516, Data4: [8]byte{0xb0, 0xeb, 0xc0, 0x1c, 0xa9, 0xd4, 0x9a, 0xc6}}
	mfMTMinimumAperture = windows.GUID{Data1: 0xd7388766, Data2: 0x18fe, Data3: 0x48c6, Data4: [8]byte{0xa1, 0x77, 0xee, 0x89, 0x48, 0x67, 0xc8, 0xc4}}
	mfLowLatency        = windows.GUID{Data1: 0x9c27891a, Data2: 0xed7a, Data3: 0x40e1, Data4: [8]byte{0x88, 0xe8, 0xb2, 0x27, 0x27, 0xa0, 0x24, 0xee}}
	mfMediaTypeVideo    = windows.GUID{Data1: 0x73646976, Data2: 0x0000, Data3: 0x0010, Data4: [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}
	mfVideoFormatH264   = windows.GUID{Data1: 0x34363248, Data2: 0x0000, Data3: 0x0010, Data4: [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}
	mfVideoFormatNV12   = windows.GUID{Data1: 0x3231564E, Data2: 0x0000, Data3: 0x0010, Data4: [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}
)

const (
	mfVersion          = 0x00020070
	coinitMultithread  = 0x0
	clsctxInprocServer = 0x1

	mftMessageCommandFlush      = 0x00000000
	mftMessageBeginStreaming    = 0x10000000
	mftMessageStartOfStream     = 0x10000003
	mftOutputProvidesSamples    = 0x100
	mftOutputCanProvideSamples  = 0x200
	hrNeedMoreInput             = 0xC00D6D72
	hrStreamChange              = 0xC00D6D61
	hrNotAccepting              = 0xC00D36B5
	hrNoMoreTypes               = 0xC00D36B9
	interlaceProgressive        = 2
	maxDecodeOutputsPerInput    = 8
	vtblRelease                 = 2
	vtblAttrGetUINT32           = 7
	vtblAttrGetUINT64           = 8
	vtblAttrGetGUID             = 10
	vtblAttrGetBlob             = 15
	vtblAttrSetUINT32           = 21
	vtblAttrSetUINT64           = 22
	vtblAttrSetGUID             = 24
	vtblMFTGetOutputStreamInfo  = 7
	vtblMFTGetAttributes        = 8
	vtblMFTGetOutputAvailType   = 14
	vtblMFTSetInputType         = 15
	vtblMFTSetOutputType        = 16
	vtblMFTProcessMessage       = 23
	vtblMFTProcessInput         = 24
	vtblMFTProcessOutput        = 25
	vtblSampleSetSampleTime     = 36
	vtblSampleGetBufferByIndex  = 40
	vtblSampleConvertContiguous = 41
	vtblSampleAddBuffer         = 42
	vtblBufferLock              = 3
	vtblBufferUnlock            = 4
	vtblBufferSetCurrentLength  = 6
)

// comObj es un objeto COM: su primer campo apunta a la tabla de métodos.
type comObj struct{ vtbl *[64]uintptr }

func (o *comObj) call(i int, args ...uintptr) uint32 {
	r, _, _ := syscall.SyscallN(o.vtbl[i], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return uint32(r)
}

func (o *comObj) release() {
	if o != nil {
		o.call(vtblRelease)
	}
}

func hrErr(what string, hr uint32) error {
	if int32(hr) >= 0 {
		return nil
	}
	return fmt.Errorf("%s: 0x%08X", what, hr)
}

var mfOnce sync.Once
var mfErr error

// comInitThread inicia COM en el hilo actual (que debe estar fijo).
func comInitThread() {
	procCoInitializeEx.Call(0, coinitMultithread) // S_FALSE o RPC_E_CHANGED_MODE: ya estaba
}

// h264Decoder decodifica un flujo H.264. No es seguro entre hilos.
type h264Decoder struct {
	tr      *comObj
	outSize uint32
	own     bool // el decodificador pone sus propias muestras
	w, h    int  // tamaño de los búferes de salida (alineado a 16)
	stride  int
	cropW   int // tamaño de verdad (sin el relleno hasta múltiplo de 16)
	cropH   int
	ts      int64
}

func newH264Decoder() (*h264Decoder, error) {
	mfOnce.Do(func() {
		if err := procMFStartup.Find(); err != nil {
			mfErr = errors.New("este Windows no tiene Media Foundation (en las ediciones N hay que instalar el Media Feature Pack)")
			return
		}
		r, _, _ := procMFStartup.Call(mfVersion, 0)
		mfErr = hrErr("MFStartup", uint32(r))
	})
	if mfErr != nil {
		return nil, mfErr
	}
	d := &h264Decoder{}
	r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidH264Decoder)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIMFTransform)), uintptr(unsafe.Pointer(&d.tr)))
	if err := hrErr("decodificador H.264 de Windows", uint32(r)); err != nil {
		return nil, err
	}
	// Baja latencia: cada fotograma sale en cuanto entra, sin esperar a los siguientes.
	var attrs *comObj
	if d.tr.call(vtblMFTGetAttributes, uintptr(unsafe.Pointer(&attrs))) == 0 && attrs != nil {
		attrs.call(vtblAttrSetUINT32, uintptr(unsafe.Pointer(&mfLowLatency)), 1)
		attrs.release()
	}
	var in *comObj
	r, _, _ = procMFCreateMediaType.Call(uintptr(unsafe.Pointer(&in)))
	if err := hrErr("MFCreateMediaType", uint32(r)); err != nil {
		d.close()
		return nil, err
	}
	in.call(vtblAttrSetGUID, uintptr(unsafe.Pointer(&mfMTMajorType)), uintptr(unsafe.Pointer(&mfMediaTypeVideo)))
	in.call(vtblAttrSetGUID, uintptr(unsafe.Pointer(&mfMTSubtype)), uintptr(unsafe.Pointer(&mfVideoFormatH264)))
	in.call(vtblAttrSetUINT32, uintptr(unsafe.Pointer(&mfMTInterlace)), interlaceProgressive)
	hr := d.tr.call(vtblMFTSetInputType, 0, uintptr(unsafe.Pointer(in)), 0)
	in.release()
	if err := hrErr("tipo de entrada H.264", hr); err != nil {
		d.close()
		return nil, err
	}
	if err := d.pickOutput(); err != nil {
		d.close()
		return nil, err
	}
	d.tr.call(vtblMFTProcessMessage, mftMessageBeginStreaming, 0)
	d.tr.call(vtblMFTProcessMessage, mftMessageStartOfStream, 0)
	return d, nil
}

// pickOutput elige NV12 como salida y apunta el tamaño de los fotogramas.
func (d *h264Decoder) pickOutput() error {
	for i := uintptr(0); ; i++ {
		var t *comObj
		hr := d.tr.call(vtblMFTGetOutputAvailType, 0, i, uintptr(unsafe.Pointer(&t)))
		if hr == hrNoMoreTypes {
			return errors.New("el decodificador H.264 no da NV12")
		}
		if err := hrErr("tipos de salida", hr); err != nil {
			return err
		}
		var sub windows.GUID
		t.call(vtblAttrGetGUID, uintptr(unsafe.Pointer(&mfMTSubtype)), uintptr(unsafe.Pointer(&sub)))
		if sub != mfVideoFormatNV12 {
			t.release()
			continue
		}
		hr = d.tr.call(vtblMFTSetOutputType, 0, uintptr(unsafe.Pointer(t)), 0)
		if err := hrErr("tipo de salida NV12", hr); err != nil {
			t.release()
			return err
		}
		var size uint64
		t.call(vtblAttrGetUINT64, uintptr(unsafe.Pointer(&mfMTFrameSize)), uintptr(unsafe.Pointer(&size)))
		d.w, d.h = int(size>>32), int(size&0xffffffff)
		var stride uint32
		if t.call(vtblAttrGetUINT32, uintptr(unsafe.Pointer(&mfMTDefaultStride)), uintptr(unsafe.Pointer(&stride))) == 0 && int32(stride) > 0 {
			d.stride = int(int32(stride))
		} else {
			d.stride = d.w
		}
		// La zona visible (1920x1080 dentro de un búfer de 1920x1088, por ejemplo).
		d.cropW, d.cropH = d.w, d.h
		var area struct {
			offX, offY int32 // MFOffset: parte entera y fracción (en 16+16 bits)
			cx, cy     int32
		}
		var got uint32
		if t.call(vtblAttrGetBlob, uintptr(unsafe.Pointer(&mfMTMinimumAperture)), uintptr(unsafe.Pointer(&area)),
			unsafe.Sizeof(area), uintptr(unsafe.Pointer(&got))) == 0 && area.cx > 0 && area.cy > 0 {
			d.cropW, d.cropH = int(area.cx), int(area.cy)
		}
		t.release()
		break
	}
	var info struct{ flags, size, align uint32 }
	if err := hrErr("info de salida", d.tr.call(vtblMFTGetOutputStreamInfo, 0, uintptr(unsafe.Pointer(&info)))); err != nil {
		return err
	}
	d.own = info.flags&(mftOutputProvidesSamples|mftOutputCanProvideSamples) == mftOutputProvidesSamples
	d.outSize = info.size
	return nil
}

func newSample(size int) (*comObj, error) {
	var s, b *comObj
	r, _, _ := procMFCreateSample.Call(uintptr(unsafe.Pointer(&s)))
	if err := hrErr("MFCreateSample", uint32(r)); err != nil {
		return nil, err
	}
	r, _, _ = procMFCreateMemoryBuffer.Call(uintptr(size), uintptr(unsafe.Pointer(&b)))
	if err := hrErr("MFCreateMemoryBuffer", uint32(r)); err != nil {
		s.release()
		return nil, err
	}
	s.call(vtblSampleAddBuffer, uintptr(unsafe.Pointer(b)))
	b.release()
	return s, nil
}

// fillSample copia data en el búfer de una muestra.
func fillSample(s *comObj, data []byte) error {
	var b *comObj
	if err := hrErr("búfer", s.call(vtblSampleGetBufferByIndex, 0, uintptr(unsafe.Pointer(&b)))); err != nil {
		return err
	}
	defer b.release()
	var p *byte
	var max, cur uint32
	if err := hrErr("Lock", b.call(vtblBufferLock, uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(&max)), uintptr(unsafe.Pointer(&cur)))); err != nil {
		return err
	}
	n := copy(unsafe.Slice(p, max), data)
	b.call(vtblBufferUnlock)
	b.call(vtblBufferSetCurrentLength, uintptr(n))
	return nil
}

// decode mete un trozo del flujo (uno o varios NAL con sus códigos de inicio) y
// devuelve los fotogramas que salgan (normalmente uno).
func (d *h264Decoder) decode(data []byte) ([]*image.YCbCr, error) {
	if len(data) == 0 {
		return nil, nil
	}
	in, err := newSample(len(data))
	if err != nil {
		return nil, err
	}
	defer in.release()
	if err := fillSample(in, data); err != nil {
		return nil, err
	}
	d.ts += 333333 // la hora da igual (no hay reordenación): solo tiene que avanzar
	in.call(vtblSampleSetSampleTime, uintptr(d.ts))
	var out []*image.YCbCr
	for try := 0; ; try++ {
		hr := d.tr.call(vtblMFTProcessInput, 0, uintptr(unsafe.Pointer(in)), 0)
		if hr == hrNotAccepting && try < 2 {
			// Tiene fotogramas por sacar antes de aceptar más.
			imgs, err := d.drain()
			out = append(out, imgs...)
			if err != nil {
				return out, err
			}
			continue
		}
		if err := hrErr("ProcessInput", hr); err != nil {
			return out, err
		}
		break
	}
	imgs, err := d.drain()
	return append(out, imgs...), err
}

// outputBuffer es MFT_OUTPUT_DATA_BUFFER.
type outputBuffer struct {
	streamID uint32
	_        uint32
	sample   *comObj
	status   uint32
	_        uint32
	events   *comObj
}

func (d *h264Decoder) drain() ([]*image.YCbCr, error) {
	var out []*image.YCbCr
	for i := 0; i < maxDecodeOutputsPerInput; i++ {
		ob := outputBuffer{}
		// Una muestra nueva para cada fotograma: si se le vuelve a dar la misma, el
		// decodificador de Windows no escribe en ella (la imagen se queda congelada).
		var mine *comObj
		if !d.own {
			s, err := newSample(int(d.outSize))
			if err != nil {
				return out, err
			}
			mine = s
			ob.sample = s
		}
		var status uint32
		hr := d.tr.call(vtblMFTProcessOutput, 0, 1, uintptr(unsafe.Pointer(&ob)), uintptr(unsafe.Pointer(&status)))
		if ob.events != nil {
			ob.events.release()
		}
		var img *image.YCbCr
		if hr == 0 {
			img = d.toYCbCr(ob.sample)
		}
		if mine != nil {
			mine.release()
		} else if ob.sample != nil {
			ob.sample.release() // la puso el decodificador
		}
		switch {
		case hr == hrNeedMoreInput:
			return out, nil
		case hr == hrStreamChange:
			// Ha leído el primer SPS (o ha cambiado el tamaño): otra vez NV12, ya con el
			// tamaño de verdad.
			if err := d.pickOutput(); err != nil {
				return out, err
			}
			continue
		}
		if err := hrErr("ProcessOutput", hr); err != nil {
			return out, err
		}
		if img != nil {
			out = append(out, img)
		}
	}
	return out, nil
}

// toYCbCr pasa el NV12 de una muestra a un 4:2:0 por planos con lados pares.
func (d *h264Decoder) toYCbCr(s *comObj) *image.YCbCr {
	if s == nil || d.w <= 0 || d.h <= 0 {
		return nil
	}
	w, h := d.cropW&^1, d.cropH&^1
	if w < 16 || h < 16 || w > d.stride {
		return nil
	}
	// La muestra puede venir en varios búferes: se junta en uno.
	var b *comObj
	if s.call(vtblSampleConvertContiguous, uintptr(unsafe.Pointer(&b))) != 0 || b == nil {
		return nil
	}
	defer b.release()
	var p *byte
	var max, cur uint32
	if b.call(vtblBufferLock, uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(&max)), uintptr(unsafe.Pointer(&cur))) != 0 {
		return nil
	}
	defer b.call(vtblBufferUnlock)
	src := unsafe.Slice(p, cur)
	uvAt := d.stride * d.h
	if len(src) < uvAt+d.stride*(h/2) {
		return nil
	}
	img := newYUV(w, h)
	for y := 0; y < h; y++ {
		copy(img.Y[y*img.YStride:y*img.YStride+w], src[y*d.stride:])
	}
	for y := 0; y < h/2; y++ {
		row := src[uvAt+y*d.stride : uvAt+y*d.stride+w]
		cb := img.Cb[y*img.CStride : y*img.CStride+w/2]
		cr := img.Cr[y*img.CStride : y*img.CStride+w/2]
		for x := range cb {
			cb[x] = row[2*x]
			cr[x] = row[2*x+1]
		}
	}
	return img
}

// reset olvida lo que tuviera a medias (tras un corte, antes del siguiente clave).
func (d *h264Decoder) reset() {
	if d.tr != nil {
		d.tr.call(vtblMFTProcessMessage, mftMessageCommandFlush, 0)
	}
}

func (d *h264Decoder) close() {
	d.tr.release()
	d.tr = nil
}

var h264Probe struct {
	once sync.Once
	ok   bool
}

// h264OK dice si este PC puede decodificar H.264 (se mira una vez).
func h264OK() bool {
	h264Probe.once.Do(func() {
		done := make(chan bool)
		go func() {
			runtime.LockOSThread() // el hilo se tira al acabar, con su COM
			comInitThread()
			d, err := newH264Decoder()
			if err == nil {
				d.close()
			}
			done <- err == nil
		}()
		h264Probe.ok = <-done
	})
	return h264Probe.ok
}

// format resume el formato de salida (para el diagnóstico).
func (d *h264Decoder) format() string {
	return fmt.Sprintf("%dx%d paso %d visible %dx%d", d.w, d.h, d.stride, d.cropW, d.cropH)
}
