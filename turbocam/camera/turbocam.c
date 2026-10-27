/*
 * TurboCAM: cámara virtual DirectShow.
 *
 * Es un filtro de captura con una sola patilla de salida. Las apps (Discord, Meet,
 * OBS, Chrome…) la ven como una webcam más. La imagen la pone TurboCAM.exe en una
 * memoria compartida (ver protocol.h); aquí solo se copia, se adapta al tamaño y al
 * formato que haya pedido cada app y se entrega a su ritmo. Cuando no hay imagen se
 * dibuja una tarjeta que explica por qué.
 *
 * La DLL no se registra sola: lo hace TurboCAM.exe en HKCU, sin permisos de
 * administrador. Por eso solo exporta DllGetClassObject y DllCanUnloadNow.
 */
#define WIN32_LEAN_AND_MEAN
#define COBJMACROS
#define CINTERFACE
#include <windows.h>
#include <sddl.h>
#include <dshow.h>
#include <initguid.h>
#include <math.h>
#include "protocol.h"

#ifndef KSPROPERTY_SUPPORT_GET
#define KSPROPERTY_SUPPORT_GET 1
#endif

/* {F5716436-EE88-4461-A891-B532EE167F6C}: también en camreg.go. */
DEFINE_GUID(CLSID_TurboCam, 0xf5716436, 0xee88, 0x4461, 0xa8, 0x91, 0xb5, 0x32, 0xee, 0x16, 0x7f, 0x6c);
DEFINE_GUID(TC_SUB_NV12, 0x3231564e, 0x0000, 0x0010, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71);
DEFINE_GUID(TC_SUB_YUY2, 0x32595559, 0x0000, 0x0010, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71);
DEFINE_GUID(TC_SUB_I420, 0x30323449, 0x0000, 0x0010, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71);
DEFINE_GUID(TC_SUB_RGB24, 0xe436eb7d, 0x524f, 0x11ce, 0x9f, 0x53, 0x00, 0x20, 0xaf, 0x0b, 0xa7, 0x70);

#define PIN_NAME L"Capture"
#define FILTER_NAME L"TurboCAM"
#define DEFAULT_INTERVAL 333333LL /* 30 fps, en unidades de 100 ns */
#define MIN_INTERVAL 166666LL     /* 60 fps */
#define MAX_INTERVAL 10000000LL   /* 1 fps */
#define MIN_SIDE 32

/* ------------------------------------------------------------------------- */
/* Formatos                                                                  */
/* ------------------------------------------------------------------------- */

enum { FMT_NV12, FMT_YUY2, FMT_I420, FMT_RGB24, FMT_COUNT };

static const struct {
    const GUID *sub;
    DWORD compression;
    WORD bits;
} kFmts[FMT_COUNT] = {
    {&TC_SUB_NV12, MAKEFOURCC('N', 'V', '1', '2'), 12},
    {&TC_SUB_YUY2, MAKEFOURCC('Y', 'U', 'Y', '2'), 16},
    {&TC_SUB_I420, MAKEFOURCC('I', '4', '2', '0'), 12},
    {&TC_SUB_RGB24, BI_RGB, 24},
};

/* El primero es el que se ofrece por defecto. */
static const struct {
    int w, h;
} kSizes[] = {{1280, 720}, {1920, 1080}, {960, 540}, {640, 480}, {640, 360}};

#define SIZE_COUNT ((int)(sizeof(kSizes) / sizeof(kSizes[0])))
#define CAPS_COUNT (SIZE_COUNT * FMT_COUNT)

/* Formato de salida ya interpretado. */
typedef struct {
    int fmt;
    int w, h;          /* zona visible */
    int bufW, bufH;    /* tamaño del búfer (biWidth, |biHeight|) */
    int bottomUp;      /* RGB de abajo arriba */
    LONGLONG interval; /* duración de un fotograma */
    DWORD size;        /* bytes del búfer */
} OutFmt;

static DWORD frame_bytes(int fmt, int bufW, int bufH) {
    switch (fmt) {
    case FMT_YUY2: return (DWORD)bufW * 2 * bufH;
    case FMT_RGB24: return (DWORD)(((bufW * 3) + 3) & ~3) * bufH;
    default: return (DWORD)bufW * bufH * 3 / 2;
    }
}

static void free_mt(AM_MEDIA_TYPE *mt) {
    if (!mt) return;
    if (mt->cbFormat && mt->pbFormat) CoTaskMemFree(mt->pbFormat);
    if (mt->pUnk) IUnknown_Release(mt->pUnk);
    mt->cbFormat = 0;
    mt->pbFormat = NULL;
    mt->pUnk = NULL;
}

static void delete_mt(AM_MEDIA_TYPE *mt) {
    if (!mt) return;
    free_mt(mt);
    CoTaskMemFree(mt);
}

static HRESULT copy_mt(AM_MEDIA_TYPE *dst, const AM_MEDIA_TYPE *src) {
    *dst = *src;
    if (src->cbFormat && src->pbFormat) {
        dst->pbFormat = (BYTE *)CoTaskMemAlloc(src->cbFormat);
        if (!dst->pbFormat) {
            dst->cbFormat = 0;
            dst->pUnk = NULL;
            return E_OUTOFMEMORY;
        }
        CopyMemory(dst->pbFormat, src->pbFormat, src->cbFormat);
    } else {
        dst->pbFormat = NULL;
        dst->cbFormat = 0;
    }
    if (dst->pUnk) IUnknown_AddRef(dst->pUnk);
    return S_OK;
}

static AM_MEDIA_TYPE *dup_mt(const AM_MEDIA_TYPE *src) {
    AM_MEDIA_TYPE *mt = (AM_MEDIA_TYPE *)CoTaskMemAlloc(sizeof(*mt));
    if (!mt) return NULL;
    if (FAILED(copy_mt(mt, src))) {
        CoTaskMemFree(mt);
        return NULL;
    }
    return mt;
}

static HRESULT make_mt(int fmt, int w, int h, LONGLONG interval, AM_MEDIA_TYPE *mt) {
    VIDEOINFOHEADER *vih = (VIDEOINFOHEADER *)CoTaskMemAlloc(sizeof(VIDEOINFOHEADER));
    if (!vih) return E_OUTOFMEMORY;
    ZeroMemory(vih, sizeof(*vih));
    DWORD bytes = frame_bytes(fmt, w, h);
    vih->AvgTimePerFrame = interval;
    vih->dwBitRate = (DWORD)((LONGLONG)bytes * 8 * 10000000LL / interval);
    vih->bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
    vih->bmiHeader.biWidth = w;
    vih->bmiHeader.biHeight = h;
    vih->bmiHeader.biPlanes = 1;
    vih->bmiHeader.biBitCount = kFmts[fmt].bits;
    vih->bmiHeader.biCompression = kFmts[fmt].compression;
    vih->bmiHeader.biSizeImage = bytes;

    ZeroMemory(mt, sizeof(*mt));
    mt->majortype = MEDIATYPE_Video;
    mt->subtype = *kFmts[fmt].sub;
    mt->bFixedSizeSamples = TRUE;
    mt->bTemporalCompression = FALSE;
    mt->lSampleSize = bytes;
    mt->formattype = FORMAT_VideoInfo;
    mt->cbFormat = sizeof(VIDEOINFOHEADER);
    mt->pbFormat = (BYTE *)vih;
    return S_OK;
}

static HRESULT make_caps_mt(int idx, AM_MEDIA_TYPE *mt) {
    return make_mt(idx % FMT_COUNT, kSizes[idx / FMT_COUNT].w, kSizes[idx / FMT_COUNT].h, DEFAULT_INTERVAL, mt);
}

/* parse_mt dice si un tipo es de los nuestros y lo interpreta. */
static BOOL parse_mt(const AM_MEDIA_TYPE *mt, OutFmt *o) {
    if (!mt || !IsEqualGUID(&mt->majortype, &MEDIATYPE_Video)) return FALSE;
    if (!IsEqualGUID(&mt->formattype, &FORMAT_VideoInfo) || !mt->pbFormat ||
        mt->cbFormat < sizeof(VIDEOINFOHEADER))
        return FALSE;
    int fmt = -1;
    for (int i = 0; i < FMT_COUNT; i++)
        if (IsEqualGUID(&mt->subtype, kFmts[i].sub)) fmt = i;
    if (fmt < 0) return FALSE;

    const VIDEOINFOHEADER *vih = (const VIDEOINFOHEADER *)mt->pbFormat;
    const BITMAPINFOHEADER *bi = &vih->bmiHeader;
    if (bi->biBitCount != kFmts[fmt].bits) return FALSE;
    int bufW = bi->biWidth, bufH = bi->biHeight < 0 ? -bi->biHeight : bi->biHeight;
    int w = bufW, h = bufH;
    /* Algunos renderizadores piden un búfer más ancho que la imagen (rcTarget). */
    if (vih->rcTarget.right > vih->rcTarget.left && vih->rcTarget.bottom > vih->rcTarget.top) {
        w = vih->rcTarget.right - vih->rcTarget.left;
        h = vih->rcTarget.bottom - vih->rcTarget.top;
        if (vih->rcTarget.left || vih->rcTarget.top || w > bufW || h > bufH) return FALSE;
    }
    if (w < MIN_SIDE || h < MIN_SIDE || w > TC_MAX_W || h > TC_MAX_H || bufW > 4096 || bufH > TC_MAX_H)
        return FALSE;
    if (fmt != FMT_RGB24 && ((w | h | bufW | bufH) & 1)) return FALSE;
    /* Formatos YUV de abajo arriba no se usan en la práctica. */
    if (fmt != FMT_RGB24 && bi->biHeight < 0) return FALSE;

    o->fmt = fmt;
    o->w = w;
    o->h = h;
    o->bufW = bufW;
    o->bufH = bufH;
    o->bottomUp = fmt == FMT_RGB24 && bi->biHeight > 0;
    o->interval = vih->AvgTimePerFrame;
    if (o->interval <= 0) o->interval = DEFAULT_INTERVAL;
    if (o->interval < MIN_INTERVAL) o->interval = MIN_INTERVAL;
    if (o->interval > MAX_INTERVAL) o->interval = MAX_INTERVAL;
    o->size = frame_bytes(fmt, bufW, bufH);
    return TRUE;
}

/* Un tipo de medio parcial (p. ej. solo «vídeo») encaja con uno completo nuestro. */
static BOOL mt_matches_partial(const AM_MEDIA_TYPE *partial, const AM_MEDIA_TYPE *full) {
    if (!partial) return TRUE;
    if (!IsEqualGUID(&partial->majortype, &GUID_NULL) && !IsEqualGUID(&partial->majortype, &full->majortype))
        return FALSE;
    if (!IsEqualGUID(&partial->subtype, &GUID_NULL) && !IsEqualGUID(&partial->subtype, &full->subtype))
        return FALSE;
    if (!IsEqualGUID(&partial->formattype, &GUID_NULL) && !IsEqualGUID(&partial->formattype, &full->formattype))
        return FALSE;
    return TRUE;
}

static BOOL mt_is_complete(const AM_MEDIA_TYPE *mt) {
    return mt && !IsEqualGUID(&mt->majortype, &GUID_NULL) && !IsEqualGUID(&mt->subtype, &GUID_NULL) &&
           !IsEqualGUID(&mt->formattype, &GUID_NULL);
}

/* ------------------------------------------------------------------------- */
/* Imagen: escalado, conversión de color y empaquetado                       */
/* ------------------------------------------------------------------------- */

static __forceinline BYTE clamp255(int v) { return (BYTE)(v < 0 ? 0 : v > 255 ? 255 : v); }

/*
 * Escala un plano (canales = 1 para Y, 2 para UV entrelazado) de la zona
 * (cx, cy, cw, ch) del origen al destino entero, con interpolación bilineal en
 * coma fija 16.16.
 */
static void scale_plane(const BYTE *src, int sstride, int sw, int sh, int cx, int cy, int cw, int ch, BYTE *dst,
                        int dstride, int dw, int dh, int channels) {
    int xs[TC_MAX_W], xf[TC_MAX_W];
    for (int x = 0; x < dw; x++) {
        /* centro del píxel de destino llevado al origen */
        LONGLONG fx = ((LONGLONG)(2 * x + 1) * cw * 65536) / (2 * dw) - 32768 + (LONGLONG)cx * 65536;
        if (fx < 0) fx = 0;
        int ix = (int)(fx >> 16);
        if (ix >= sw - 1) {
            ix = sw - 1;
            fx = (LONGLONG)ix << 16;
        }
        xs[x] = ix;
        xf[x] = (int)(fx & 0xffff);
    }
    for (int y = 0; y < dh; y++) {
        LONGLONG fy = ((LONGLONG)(2 * y + 1) * ch * 65536) / (2 * dh) - 32768 + (LONGLONG)cy * 65536;
        if (fy < 0) fy = 0;
        int iy = (int)(fy >> 16);
        int wy = (int)(fy & 0xffff);
        if (iy >= sh - 1) {
            iy = sh - 1;
            wy = 0;
        }
        const BYTE *r0 = src + (size_t)iy * sstride;
        const BYTE *r1 = iy + 1 < sh ? r0 + sstride : r0;
        BYTE *out = dst + (size_t)y * dstride;
        for (int x = 0; x < dw; x++) {
            int ix = xs[x], wx = xf[x];
            int nx = ix + 1 < sw ? ix + 1 : ix;
            for (int c = 0; c < channels; c++) {
                int a = r0[ix * channels + c], b = r0[nx * channels + c];
                int d = r1[ix * channels + c], e = r1[nx * channels + c];
                int top = (a << 8) + (((b - a) * wx) >> 8);
                int bot = (d << 8) + (((e - d) * wx) >> 8);
                out[x * channels + c] = (BYTE)((((top << 8) + (bot - top) * (wy >> 8)) + 32768) >> 16);
            }
        }
    }
}

/*
 * Lleva un NV12 compacto (sw x sh) a otro NV12 compacto (dw x dh) recortando lo que
 * sobre para no deformar la imagen (rellenar, como haría una webcam).
 */
static void scale_nv12(const BYTE *src, int sw, int sh, BYTE *dst, int dw, int dh) {
    if (sw == dw && sh == dh) {
        CopyMemory(dst, src, (size_t)dw * dh * 3 / 2);
        return;
    }
    int cx = 0, cy = 0, cw = sw, ch = sh;
    if ((LONGLONG)sw * dh > (LONGLONG)dw * sh) {
        cw = (int)((LONGLONG)sh * dw / dh) & ~1;
        cx = ((sw - cw) / 2) & ~1;
    } else {
        ch = (int)((LONGLONG)sw * dh / dw) & ~1;
        cy = ((sh - ch) / 2) & ~1;
    }
    scale_plane(src, sw, sw, sh, cx, cy, cw, ch, dst, dw, dw, dh, 1);
    scale_plane(src + (size_t)sw * sh, sw, sw / 2, sh / 2, cx / 2, cy / 2, cw / 2, ch / 2, dst + (size_t)dw * dh, dw,
                dw / 2, dh / 2, 2);
}

/* Empaqueta un NV12 compacto (o->w x o->h) en el búfer de muestra con el formato pedido. */
static void pack_frame(const BYTE *nv12, const OutFmt *o, BYTE *out) {
    const int w = o->w, h = o->h;
    const BYTE *Y = nv12, *UV = nv12 + (size_t)w * h;
    if (o->bufW != w || o->bufH != h) ZeroMemory(out, o->size);
    switch (o->fmt) {
    case FMT_NV12: {
        int s = o->bufW;
        for (int y = 0; y < h; y++) CopyMemory(out + (size_t)y * s, Y + (size_t)y * w, w);
        BYTE *uv = out + (size_t)s * o->bufH;
        for (int y = 0; y < h / 2; y++) CopyMemory(uv + (size_t)y * s, UV + (size_t)y * w, w);
        break;
    }
    case FMT_I420: {
        int s = o->bufW;
        for (int y = 0; y < h; y++) CopyMemory(out + (size_t)y * s, Y + (size_t)y * w, w);
        BYTE *u = out + (size_t)s * o->bufH;
        BYTE *v = u + (size_t)(s / 2) * (o->bufH / 2);
        for (int y = 0; y < h / 2; y++) {
            const BYTE *in = UV + (size_t)y * w;
            BYTE *ur = u + (size_t)y * (s / 2), *vr = v + (size_t)y * (s / 2);
            for (int x = 0; x < w / 2; x++) {
                ur[x] = in[2 * x];
                vr[x] = in[2 * x + 1];
            }
        }
        break;
    }
    case FMT_YUY2: {
        int s = o->bufW * 2;
        for (int y = 0; y < h; y++) {
            const BYTE *yr = Y + (size_t)y * w, *uv = UV + (size_t)(y / 2) * w;
            BYTE *d = out + (size_t)y * s;
            for (int x = 0; x < w; x += 2) {
                d[0] = yr[x];
                d[1] = uv[x];
                d[2] = yr[x + 1];
                d[3] = uv[x + 1];
                d += 4;
            }
        }
        break;
    }
    case FMT_RGB24: {
        /* BT.601 de rango limitado, en coma fija. */
        int s = ((o->bufW * 3) + 3) & ~3;
        for (int y = 0; y < h; y++) {
            const BYTE *yr = Y + (size_t)y * w, *uv = UV + (size_t)(y / 2) * w;
            int row = o->bottomUp ? o->bufH - 1 - y : y;
            BYTE *d = out + (size_t)row * s;
            for (int x = 0; x < w; x++) {
                int c = 298 * (yr[x] - 16);
                int u = uv[x & ~1] - 128, v = uv[(x & ~1) + 1] - 128;
                d[0] = clamp255((c + 516 * u + 128) >> 8);
                d[1] = clamp255((c - 100 * u - 208 * v + 128) >> 8);
                d[2] = clamp255((c + 409 * v + 128) >> 8);
                d += 3;
            }
        }
        break;
    }
    }
}

/* BGRA de arriba abajo a NV12 compacto (BT.601, rango limitado). */
static void bgra_to_nv12(const BYTE *bgra, int w, int h, BYTE *nv12) {
    BYTE *Y = nv12, *UV = nv12 + (size_t)w * h;
    for (int y = 0; y < h; y++) {
        const BYTE *p = bgra + (size_t)y * w * 4;
        for (int x = 0; x < w; x++, p += 4)
            Y[(size_t)y * w + x] = (BYTE)(((66 * p[2] + 129 * p[1] + 25 * p[0] + 128) >> 8) + 16);
    }
    for (int y = 0; y < h; y += 2) {
        for (int x = 0; x < w; x += 2) {
            int r = 0, g = 0, b = 0;
            for (int k = 0; k < 4; k++) {
                const BYTE *p = bgra + ((size_t)(y + (k >> 1)) * w + x + (k & 1)) * 4;
                b += p[0];
                g += p[1];
                r += p[2];
            }
            r >>= 2, g >>= 2, b >>= 2;
            BYTE *uv = UV + (size_t)(y / 2) * w + x;
            uv[0] = (BYTE)(((-38 * r - 74 * g + 112 * b + 128) >> 8) + 128);
            uv[1] = (BYTE)(((112 * r - 94 * g - 18 * b + 128) >> 8) + 128);
        }
    }
}

/* ------------------------------------------------------------------------- */
/* Tarjetas sin imagen (TurboCAM cerrado, esperando al móvil…)               */
/* ------------------------------------------------------------------------- */

static float clampf(float v, float lo, float hi) { return v < lo ? lo : v > hi ? hi : v; }

/* Cobertura (0..1) de un rectángulo redondeado en (px, py), con borde suavizado. */
static float rrect_cover(float px, float py, float cx, float cy, float half, float radius) {
    float dx = fabsf(px - cx) - (half - radius), dy = fabsf(py - cy) - (half - radius);
    float ox = dx > 0 ? dx : 0, oy = dy > 0 ? dy : 0;
    float dist = sqrtf(ox * ox + oy * oy) + (dx > dy ? (dx < 0 ? dx : 0) : (dy < 0 ? dy : 0)) - radius;
    return clampf(0.5f - dist, 0, 1);
}

static float circle_cover(float px, float py, float cx, float cy, float r) {
    float d = sqrtf((px - cx) * (px - cx) + (py - cy) * (py - cy)) - r;
    return clampf(0.5f - d, 0, 1);
}

static void blend(BYTE *p, float r, float g, float b, float a) {
    p[0] = (BYTE)(p[0] + (b - p[0]) * a);
    p[1] = (BYTE)(p[1] + (g - p[1]) * a);
    p[2] = (BYTE)(p[2] + (r - p[2]) * a);
}

static void card_texts(int state, const WCHAR **title, const WCHAR **sub) {
    switch (state) {
    case TC_STATE_WAITING:
        *title = L"Esperando al móvil";
        *sub = L"Escanea el código QR de TurboCAM con la cámara del móvil";
        break;
    case TC_STATE_PAUSED:
        *title = L"El móvil está en pausa";
        *sub = L"Vuelve a TurboCAM en el móvil y la imagen seguirá sola";
        break;
    case TC_STATE_COVER:
        *title = L"Cámara tapada";
        *sub = L"Vuelve enseguida";
        break;
    default:
        *title = L"TurboCAM está cerrado";
        *sub = L"Ábrelo en el PC para usar el móvil como cámara";
        break;
    }
}

/* Dibuja la tarjeta del estado en NV12 compacto (w x h). Devuelve FALSE si falla GDI. */
static BOOL render_card(int state, int w, int h, BYTE *nv12) {
    BITMAPINFO bi;
    ZeroMemory(&bi, sizeof(bi));
    bi.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
    bi.bmiHeader.biWidth = w;
    bi.bmiHeader.biHeight = -h; /* de arriba abajo */
    bi.bmiHeader.biPlanes = 1;
    bi.bmiHeader.biBitCount = 32;
    bi.bmiHeader.biCompression = BI_RGB;
    BYTE *bits = NULL;
    HDC dc = CreateCompatibleDC(NULL);
    if (!dc) return FALSE;
    HBITMAP bmp = CreateDIBSection(dc, &bi, DIB_RGB_COLORS, (void **)&bits, NULL, 0);
    if (!bmp || !bits) {
        DeleteDC(dc);
        return FALSE;
    }
    HGDIOBJ oldBmp = SelectObject(dc, bmp);

    /* Fondo: el mismo de las ventanas de Turbo, con brillos violeta y cian. */
    const float W = (float)w, H = (float)h, S = H;
    for (int y = 0; y < h; y++) {
        BYTE *p = bits + (size_t)y * w * 4;
        for (int x = 0; x < w; x++, p += 4) {
            float fx = (float)x, fy = (float)y;
            float d1 = ((fx + .1f * W) * (fx + .1f * W) + (fy + .2f * H) * (fy + .2f * H)) / (S * S * .9f);
            float d2 = ((fx - 1.1f * W) * (fx - 1.1f * W) + (fy - .1f * H) * (fy - .1f * H)) / (S * S * .7f);
            float d3 = ((fx - .4f * W) * (fx - .4f * W) + (fy - 1.3f * H) * (fy - 1.3f * H)) / (S * S * .8f);
            float g1 = clampf(1 - d1, 0, 1) * .22f, g2 = clampf(1 - d2, 0, 1) * .12f, g3 = clampf(1 - d3, 0, 1) * .16f;
            float r = 10 + 124 * g1 + 6 * g2 + 67 * g3;
            float g = 11 + 58 * g1 + 182 * g2 + 56 * g3;
            float b = 18 + 237 * g1 + 212 * g2 + 202 * g3;
            p[0] = (BYTE)clampf(b, 0, 255);
            p[1] = (BYTE)clampf(g, 0, 255);
            p[2] = (BYTE)clampf(r, 0, 255);
            p[3] = 255;
        }
    }

    /* Logotipo: cuadrado con el degradado violeta-cian y un objetivo blanco. */
    float half = H * .085f, cx = W / 2, cy = H * .38f;
    int x0 = (int)(cx - half - 2), x1 = (int)(cx + half + 2), y0 = (int)(cy - half - 2), y1 = (int)(cy + half + 2);
    for (int y = y0 < 0 ? 0 : y0; y < h && y <= y1; y++) {
        for (int x = x0 < 0 ? 0 : x0; x < w && x <= x1; x++) {
            BYTE *p = bits + ((size_t)y * w + x) * 4;
            float px = x + .5f, py = y + .5f;
            float a = rrect_cover(px, py, cx, cy, half, half * .5f);
            if (a <= 0) continue;
            float t = clampf(((px - (cx - half)) + (py - (cy - half))) / (4 * half), 0, 1);
            float r, g, b;
            if (t < .45f) {
                float k = t / .45f;
                r = 139 + (99 - 139) * k, g = 92 + (102 - 92) * k, b = 246 + (241 - 246) * k;
            } else {
                float k = (t - .45f) / .55f;
                r = 99 + (6 - 99) * k, g = 102 + (182 - 102) * k, b = 241 + (212 - 241) * k;
            }
            blend(p, r, g, b, a);
            float ring = circle_cover(px, py, cx, cy, half * .48f) - circle_cover(px, py, cx, cy, half * .30f);
            if (ring > 0) blend(p, 255, 255, 255, ring);
            float dot = circle_cover(px, py, cx, cy, half * .13f);
            if (dot > 0) blend(p, 165, 243, 252, dot);
        }
    }

    /* Textos */
    const WCHAR *title, *sub;
    card_texts(state, &title, &sub);
    SetBkMode(dc, TRANSPARENT);
    HFONT ft = CreateFontW(-(int)(H * .056f), 0, 0, 0, FW_SEMIBOLD, 0, 0, 0, DEFAULT_CHARSET, OUT_TT_PRECIS,
                           CLIP_DEFAULT_PRECIS, ANTIALIASED_QUALITY, DEFAULT_PITCH, L"Segoe UI");
    HFONT fs = CreateFontW(-(int)(H * .03f), 0, 0, 0, FW_NORMAL, 0, 0, 0, DEFAULT_CHARSET, OUT_TT_PRECIS,
                           CLIP_DEFAULT_PRECIS, ANTIALIASED_QUALITY, DEFAULT_PITCH, L"Segoe UI");
    RECT rc = {0, (LONG)(cy + half + H * .07f), w, (LONG)(cy + half + H * .16f)};
    HGDIOBJ oldFont = SelectObject(dc, ft);
    SetTextColor(dc, RGB(236, 238, 248));
    DrawTextW(dc, title, -1, &rc, DT_CENTER | DT_SINGLELINE | DT_NOPREFIX | DT_VCENTER);
    SelectObject(dc, fs);
    SetTextColor(dc, RGB(145, 150, 174));
    RECT rs = {(LONG)(W * .08f), rc.bottom, (LONG)(W * .92f), rc.bottom + (LONG)(H * .07f)};
    DrawTextW(dc, sub, -1, &rs, DT_CENTER | DT_SINGLELINE | DT_NOPREFIX | DT_VCENTER | DT_END_ELLIPSIS);
    SelectObject(dc, oldFont);
    DeleteObject(ft);
    DeleteObject(fs);
    GdiFlush();

    bgra_to_nv12(bits, w, h, nv12);
    SelectObject(dc, oldBmp);
    DeleteObject(bmp);
    DeleteDC(dc);
    return TRUE;
}

/* ------------------------------------------------------------------------- */
/* Memoria compartida                                                        */
/* ------------------------------------------------------------------------- */

typedef struct {
    HANDLE map;
    BYTE *base;
    BOOL writable;
    DWORD lastTry;
    int consumer; /* ranura de consumidor que ocupamos, o -1 */
} Shm;

#define U32(base, off) ((volatile LONG *)((base) + (off)))

static BOOL shm_open(Shm *s) {
    if (s->base) return TRUE;
    DWORD now = GetTickCount();
    if (s->lastTry && now - s->lastTry < 1000) return FALSE;
    s->lastTry = now ? now : 1;

    /* Se crea si no existe: así TurboCAM, al abrirse, ve que ya hay quien mira. */
    SECURITY_ATTRIBUTES sa = {sizeof(sa), NULL, FALSE};
    PSECURITY_DESCRIPTOR sd = NULL;
    if (ConvertStringSecurityDescriptorToSecurityDescriptorW(TC_MAP_SDDL, SDDL_REVISION_1, &sd, NULL))
        sa.lpSecurityDescriptor = sd;
    HANDLE m = CreateFileMappingW(INVALID_HANDLE_VALUE, &sa, PAGE_READWRITE, 0, TC_MAP_SIZE, TC_MAP_NAME);
    if (sd) LocalFree(sd);
    BOOL rw = TRUE;
    if (!m) {
        /* Desde un proceso muy limitado quizá solo se pueda leer. */
        m = OpenFileMappingW(FILE_MAP_READ, FALSE, TC_MAP_NAME);
        rw = FALSE;
        if (!m) return FALSE;
    }
    BYTE *v = (BYTE *)MapViewOfFile(m, rw ? FILE_MAP_READ | FILE_MAP_WRITE : FILE_MAP_READ, 0, 0, TC_MAP_SIZE);
    if (!v) {
        CloseHandle(m);
        return FALSE;
    }
    s->map = m;
    s->base = v;
    s->writable = rw;
    s->consumer = -1;
    return TRUE;
}

static void shm_release_consumer(Shm *s) {
    if (s->base && s->writable && s->consumer >= 0) {
        BYTE *c = s->base + TC_CONS_BASE + s->consumer * TC_CONS_SIZE;
        InterlockedExchange(U32(c, TC_CONS_PID), 0);
    }
    s->consumer = -1;
}

static void shm_close(Shm *s) {
    shm_release_consumer(s);
    if (s->base) UnmapViewOfFile(s->base);
    if (s->map) CloseHandle(s->map);
    s->base = NULL;
    s->map = NULL;
}

/* Apunta (o renueva) que este proceso está usando la cámara y con qué formato. */
static void shm_beat_consumer(Shm *s, const OutFmt *o) {
    if (!s->base || !s->writable) return;
    DWORD now = GetTickCount();
    LONG pid = (LONG)GetCurrentProcessId();
    if (s->consumer < 0) {
        for (int i = 0; i < TC_MAX_CONSUMERS && s->consumer < 0; i++) {
            BYTE *c = s->base + TC_CONS_BASE + i * TC_CONS_SIZE;
            LONG cur = *U32(c, TC_CONS_PID);
            DWORD seen = (DWORD)*U32(c, TC_CONS_SEEN);
            if (cur != 0 && now - seen < TC_CONSUMER_TIMEOUT_MS) continue;
            if (InterlockedCompareExchange(U32(c, TC_CONS_PID), pid, cur) != cur) continue;
            s->consumer = i;
            WCHAR path[MAX_PATH], *name = path;
            DWORD n = GetModuleFileNameW(NULL, path, MAX_PATH);
            if (n == 0 || n >= MAX_PATH) path[0] = 0;
            for (WCHAR *q = path; *q; q++)
                if (*q == L'\\' || *q == L'/') name = q + 1;
            WCHAR *exe = (WCHAR *)(c + TC_CONS_EXE);
            int k = 0;
            for (; k < TC_CONS_EXE_CHARS - 1 && name[k]; k++) exe[k] = name[k];
            for (; k < TC_CONS_EXE_CHARS; k++) exe[k] = 0;
        }
        if (s->consumer < 0) return;
    }
    BYTE *c = s->base + TC_CONS_BASE + s->consumer * TC_CONS_SIZE;
    *U32(c, TC_CONS_WIDTH) = o->w;
    *U32(c, TC_CONS_HEIGHT) = o->h;
    *U32(c, TC_CONS_FPS) = (LONG)((10000000LL + o->interval / 2) / o->interval);
    InterlockedExchange(U32(c, TC_CONS_SEEN), (LONG)now);
}

/* ------------------------------------------------------------------------- */
/* Objetos COM                                                               */
/* ------------------------------------------------------------------------- */

static volatile LONG g_objects = 0; /* objetos vivos */
static volatile LONG g_locks = 0;   /* LockServer */

typedef struct Filter Filter;

typedef struct {
    const IPinVtbl *lpVtbl;
    const IAMStreamConfigVtbl *cfgVtbl;
    const IKsPropertySetVtbl *ksVtbl;
    Filter *filter;
    IPin *peer;
    IMemInputPin *input;
    IMemAllocator *alloc;
    AM_MEDIA_TYPE mt; /* tipo de la conexión */
    BOOL connected;
    AM_MEDIA_TYPE pref; /* el que pidió la app con SetFormat */
    BOOL hasPref;
    OutFmt of;
} Pin;

struct Filter {
    const IBaseFilterVtbl *lpVtbl;
    const IAMFilterMiscFlagsVtbl *miscVtbl;
    volatile LONG ref;
    CRITICAL_SECTION lock;      /* estado del filtro y de la conexión */
    CRITICAL_SECTION clockLock; /* reloj, que también lee el hilo de emisión */
    volatile LONG state;        /* FILTER_STATE */
    IReferenceClock *clock;
    IFilterGraph *graph; /* sin referencia, como manda DirectShow */
    WCHAR name[MAX_FILTER_NAME];
    REFERENCE_TIME tStart;
    Pin pin;

    HANDLE thread, stopEvent;
    Shm shm;
    /* Del hilo de emisión */
    BYTE *src;      /* copia del último fotograma (NV12 en su tamaño) */
    BYTE *frame;    /* NV12 al tamaño de salida */
    BYTE *out;      /* ya empaquetado en el formato de salida */
    DWORD lastSeq;  /* fotograma de la memoria compartida que hay en frame */
    int shown;      /* -1 nada; TC_STATE_LIVE imagen; otro, la tarjeta de ese estado */
    BOOL outValid;
};

#define FILTER_FROM_MISC(p) CONTAINING_RECORD(p, Filter, miscVtbl)
#define PIN_FROM_CFG(p) CONTAINING_RECORD(p, Pin, cfgVtbl)
#define PIN_FROM_KS(p) CONTAINING_RECORD(p, Pin, ksVtbl)

static ULONG filter_addref(Filter *f) { return (ULONG)InterlockedIncrement(&f->ref); }
static void filter_destroy(Filter *f);
static ULONG filter_release(Filter *f) {
    LONG r = InterlockedDecrement(&f->ref);
    if (r == 0) filter_destroy(f);
    return (ULONG)r;
}

/* ---------------- IEnumPins ---------------- */

typedef struct {
    const IEnumPinsVtbl *lpVtbl;
    volatile LONG ref;
    Filter *filter;
    ULONG pos;
} EnumPins;

static HRESULT enum_pins_new(Filter *f, ULONG pos, IEnumPins **out);

static HRESULT STDMETHODCALLTYPE ep_QueryInterface(IEnumPins *This, REFIID riid, void **ppv) {
    if (!ppv) return E_POINTER;
    if (IsEqualIID(riid, &IID_IUnknown) || IsEqualIID(riid, &IID_IEnumPins)) {
        *ppv = This;
        IEnumPins_AddRef(This);
        return S_OK;
    }
    *ppv = NULL;
    return E_NOINTERFACE;
}
static ULONG STDMETHODCALLTYPE ep_AddRef(IEnumPins *This) { return (ULONG)InterlockedIncrement(&((EnumPins *)This)->ref); }
static ULONG STDMETHODCALLTYPE ep_Release(IEnumPins *This) {
    EnumPins *e = (EnumPins *)This;
    LONG r = InterlockedDecrement(&e->ref);
    if (r == 0) {
        filter_release(e->filter);
        HeapFree(GetProcessHeap(), 0, e);
        InterlockedDecrement(&g_objects);
    }
    return (ULONG)r;
}
static HRESULT STDMETHODCALLTYPE ep_Next(IEnumPins *This, ULONG n, IPin **pins, ULONG *fetched) {
    EnumPins *e = (EnumPins *)This;
    if (!pins || (n > 1 && !fetched)) return E_POINTER;
    ULONG got = 0;
    while (got < n && e->pos < 1) {
        pins[got] = (IPin *)&e->filter->pin;
        IPin_AddRef(pins[got]);
        got++;
        e->pos++;
    }
    if (fetched) *fetched = got;
    return got == n ? S_OK : S_FALSE;
}
static HRESULT STDMETHODCALLTYPE ep_Skip(IEnumPins *This, ULONG n) {
    EnumPins *e = (EnumPins *)This;
    e->pos += n;
    return e->pos <= 1 ? S_OK : S_FALSE;
}
static HRESULT STDMETHODCALLTYPE ep_Reset(IEnumPins *This) {
    ((EnumPins *)This)->pos = 0;
    return S_OK;
}
static HRESULT STDMETHODCALLTYPE ep_Clone(IEnumPins *This, IEnumPins **out) {
    EnumPins *e = (EnumPins *)This;
    return enum_pins_new(e->filter, e->pos, out);
}

static const IEnumPinsVtbl kEnumPinsVtbl = {ep_QueryInterface, ep_AddRef, ep_Release, ep_Next,
                                            ep_Skip,           ep_Reset,  ep_Clone};

static HRESULT enum_pins_new(Filter *f, ULONG pos, IEnumPins **out) {
    if (!out) return E_POINTER;
    EnumPins *e = (EnumPins *)HeapAlloc(GetProcessHeap(), HEAP_ZERO_MEMORY, sizeof(EnumPins));
    if (!e) return E_OUTOFMEMORY;
    e->lpVtbl = &kEnumPinsVtbl;
    e->ref = 1;
    e->filter = f;
    e->pos = pos;
    filter_addref(f);
    InterlockedIncrement(&g_objects);
    *out = (IEnumPins *)e;
    return S_OK;
}

/* ---------------- IEnumMediaTypes ---------------- */

typedef struct {
    const IEnumMediaTypesVtbl *lpVtbl;
    volatile LONG ref;
    Filter *filter;
    ULONG pos;
} EnumTypes;

static HRESULT enum_types_new(Filter *f, ULONG pos, IEnumMediaTypes **out);

static HRESULT STDMETHODCALLTYPE et_QueryInterface(IEnumMediaTypes *This, REFIID riid, void **ppv) {
    if (!ppv) return E_POINTER;
    if (IsEqualIID(riid, &IID_IUnknown) || IsEqualIID(riid, &IID_IEnumMediaTypes)) {
        *ppv = This;
        IEnumMediaTypes_AddRef(This);
        return S_OK;
    }
    *ppv = NULL;
    return E_NOINTERFACE;
}
static ULONG STDMETHODCALLTYPE et_AddRef(IEnumMediaTypes *This) {
    return (ULONG)InterlockedIncrement(&((EnumTypes *)This)->ref);
}
static ULONG STDMETHODCALLTYPE et_Release(IEnumMediaTypes *This) {
    EnumTypes *e = (EnumTypes *)This;
    LONG r = InterlockedDecrement(&e->ref);
    if (r == 0) {
        filter_release(e->filter);
        HeapFree(GetProcessHeap(), 0, e);
        InterlockedDecrement(&g_objects);
    }
    return (ULONG)r;
}

/* Con un SetFormat pendiente, ese tipo va el primero de la lista. */
static HRESULT pin_type_at(Pin *p, ULONG i, AM_MEDIA_TYPE *mt) {
    BOOL pref;
    EnterCriticalSection(&p->filter->lock);
    pref = p->hasPref;
    HRESULT hr = S_FALSE;
    if (pref) {
        if (i == 0) hr = copy_mt(mt, &p->pref);
        else if (i <= CAPS_COUNT) hr = make_caps_mt((int)i - 1, mt);
    } else if (i < CAPS_COUNT) {
        hr = make_caps_mt((int)i, mt);
    }
    LeaveCriticalSection(&p->filter->lock);
    return hr;
}

static HRESULT STDMETHODCALLTYPE et_Next(IEnumMediaTypes *This, ULONG n, AM_MEDIA_TYPE **types, ULONG *fetched) {
    EnumTypes *e = (EnumTypes *)This;
    if (!types || (n > 1 && !fetched)) return E_POINTER;
    ULONG got = 0;
    while (got < n) {
        AM_MEDIA_TYPE tmp;
        if (pin_type_at(&e->filter->pin, e->pos, &tmp) != S_OK) break;
        AM_MEDIA_TYPE *mt = (AM_MEDIA_TYPE *)CoTaskMemAlloc(sizeof(*mt));
        if (!mt) {
            free_mt(&tmp);
            break;
        }
        *mt = tmp;
        types[got++] = mt;
        e->pos++;
    }
    if (fetched) *fetched = got;
    return got == n ? S_OK : S_FALSE;
}
static HRESULT STDMETHODCALLTYPE et_Skip(IEnumMediaTypes *This, ULONG n) {
    EnumTypes *e = (EnumTypes *)This;
    e->pos += n;
    return e->pos <= CAPS_COUNT ? S_OK : S_FALSE;
}
static HRESULT STDMETHODCALLTYPE et_Reset(IEnumMediaTypes *This) {
    ((EnumTypes *)This)->pos = 0;
    return S_OK;
}
static HRESULT STDMETHODCALLTYPE et_Clone(IEnumMediaTypes *This, IEnumMediaTypes **out) {
    EnumTypes *e = (EnumTypes *)This;
    return enum_types_new(e->filter, e->pos, out);
}

static const IEnumMediaTypesVtbl kEnumTypesVtbl = {et_QueryInterface, et_AddRef, et_Release, et_Next,
                                                   et_Skip,           et_Reset,  et_Clone};

static HRESULT enum_types_new(Filter *f, ULONG pos, IEnumMediaTypes **out) {
    if (!out) return E_POINTER;
    EnumTypes *e = (EnumTypes *)HeapAlloc(GetProcessHeap(), HEAP_ZERO_MEMORY, sizeof(EnumTypes));
    if (!e) return E_OUTOFMEMORY;
    e->lpVtbl = &kEnumTypesVtbl;
    e->ref = 1;
    e->filter = f;
    e->pos = pos;
    filter_addref(f);
    InterlockedIncrement(&g_objects);
    *out = (IEnumMediaTypes *)e;
    return S_OK;
}

/* ---------------- Patilla: IPin ---------------- */

static HRESULT pin_qi(Pin *p, REFIID riid, void **ppv) {
    if (!ppv) return E_POINTER;
    if (IsEqualIID(riid, &IID_IUnknown) || IsEqualIID(riid, &IID_IPin)) *ppv = &p->lpVtbl;
    else if (IsEqualIID(riid, &IID_IAMStreamConfig)) *ppv = &p->cfgVtbl;
    else if (IsEqualIID(riid, &IID_IKsPropertySet)) *ppv = &p->ksVtbl;
    else {
        *ppv = NULL;
        return E_NOINTERFACE;
    }
    filter_addref(p->filter);
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE pin_QueryInterface(IPin *This, REFIID riid, void **ppv) {
    return pin_qi((Pin *)This, riid, ppv);
}
static ULONG STDMETHODCALLTYPE pin_AddRef(IPin *This) { return filter_addref(((Pin *)This)->filter); }
static ULONG STDMETHODCALLTYPE pin_Release(IPin *This) { return filter_release(((Pin *)This)->filter); }

static void pin_drop_connection(Pin *p) {
    if (p->alloc) {
        IMemAllocator_Decommit(p->alloc);
        IMemAllocator_Release(p->alloc);
        p->alloc = NULL;
    }
    if (p->input) {
        IMemInputPin_Release(p->input);
        p->input = NULL;
    }
    if (p->peer) {
        IPin_Release(p->peer);
        p->peer = NULL;
    }
    if (p->connected) {
        free_mt(&p->mt);
        p->connected = FALSE;
    }
}

/* Pide búferes del tamaño de un fotograma al asignador. */
static HRESULT pin_size_allocator(Pin *p, IMemAllocator *a, const ALLOCATOR_PROPERTIES *req) {
    ALLOCATOR_PROPERTIES want = *req, got;
    if (want.cBuffers < 3) want.cBuffers = 3;
    if (want.cbBuffer < (long)p->of.size) want.cbBuffer = (long)p->of.size;
    if (want.cbAlign < 1) want.cbAlign = 1;
    HRESULT hr = IMemAllocator_SetProperties(a, &want, &got);
    if (FAILED(hr)) return hr;
    return got.cbBuffer >= (long)p->of.size ? S_OK : E_FAIL;
}

static HRESULT pin_decide_allocator(Pin *p) {
    ALLOCATOR_PROPERTIES req;
    ZeroMemory(&req, sizeof(req));
    IMemInputPin_GetAllocatorRequirements(p->input, &req);
    IMemAllocator *a = NULL;
    if (SUCCEEDED(IMemInputPin_GetAllocator(p->input, &a)) && a) {
        if (SUCCEEDED(pin_size_allocator(p, a, &req)) && SUCCEEDED(IMemInputPin_NotifyAllocator(p->input, a, FALSE))) {
            p->alloc = a;
            return S_OK;
        }
        IMemAllocator_Release(a);
        a = NULL;
    }
    HRESULT hr = CoCreateInstance(&CLSID_MemoryAllocator, NULL, CLSCTX_INPROC_SERVER, &IID_IMemAllocator, (void **)&a);
    if (FAILED(hr)) return hr;
    hr = pin_size_allocator(p, a, &req);
    if (SUCCEEDED(hr)) hr = IMemInputPin_NotifyAllocator(p->input, a, FALSE);
    if (FAILED(hr)) {
        IMemAllocator_Release(a);
        return hr;
    }
    p->alloc = a;
    return S_OK;
}

/* Intenta conectar con un tipo concreto. Con el cerrojo del filtro cogido. */
static HRESULT pin_try_connect(Pin *p, IPin *recv, const AM_MEDIA_TYPE *mt) {
    OutFmt of;
    if (!parse_mt(mt, &of)) return VFW_E_TYPE_NOT_ACCEPTED;
    p->of = of;
    HRESULT hr = IPin_ReceiveConnection(recv, (IPin *)p, mt);
    if (FAILED(hr)) return hr;
    p->peer = recv;
    IPin_AddRef(recv);
    hr = IPin_QueryInterface(recv, &IID_IMemInputPin, (void **)&p->input);
    if (SUCCEEDED(hr)) hr = pin_decide_allocator(p);
    if (SUCCEEDED(hr)) hr = copy_mt(&p->mt, mt);
    if (FAILED(hr)) {
        IPin_Disconnect(recv);
        pin_drop_connection(p);
        return hr;
    }
    p->connected = TRUE;
    p->filter->outValid = FALSE;
    p->filter->shown = -1;
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE pin_Connect(IPin *This, IPin *recv, const AM_MEDIA_TYPE *pmt) {
    Pin *p = (Pin *)This;
    Filter *f = p->filter;
    if (!recv) return E_POINTER;
    EnterCriticalSection(&f->lock);
    HRESULT hr = VFW_E_NO_ACCEPTABLE_TYPES;
    if (p->connected) hr = VFW_E_ALREADY_CONNECTED;
    else if (f->state != State_Stopped) hr = VFW_E_NOT_STOPPED;
    else if (mt_is_complete(pmt)) {
        hr = pin_try_connect(p, recv, pmt);
    } else {
        /* 1) lo que pidió la app con SetFormat; 2) nuestra lista; 3) la del otro lado. */
        if (p->hasPref && mt_matches_partial(pmt, &p->pref)) hr = pin_try_connect(p, recv, &p->pref);
        for (int i = 0; i < CAPS_COUNT && !p->connected; i++) {
            AM_MEDIA_TYPE mt;
            if (FAILED(make_caps_mt(i, &mt))) break;
            if (mt_matches_partial(pmt, &mt)) hr = pin_try_connect(p, recv, &mt);
            free_mt(&mt);
        }
        IEnumMediaTypes *en = NULL;
        if (!p->connected && SUCCEEDED(IPin_EnumMediaTypes(recv, &en)) && en) {
            AM_MEDIA_TYPE *mt = NULL;
            ULONG n = 0;
            while (!p->connected && IEnumMediaTypes_Next(en, 1, &mt, &n) == S_OK && n == 1) {
                OutFmt tmp;
                if (mt_matches_partial(pmt, mt) && parse_mt(mt, &tmp)) hr = pin_try_connect(p, recv, mt);
                delete_mt(mt);
                mt = NULL;
            }
            IEnumMediaTypes_Release(en);
        }
        if (!p->connected) hr = VFW_E_NO_ACCEPTABLE_TYPES;
    }
    LeaveCriticalSection(&f->lock);
    return hr;
}

static HRESULT STDMETHODCALLTYPE pin_ReceiveConnection(IPin *This, IPin *conn, const AM_MEDIA_TYPE *pmt) {
    (void)This, (void)conn, (void)pmt;
    return E_UNEXPECTED; /* somos una patilla de salida */
}

static HRESULT STDMETHODCALLTYPE pin_Disconnect(IPin *This) {
    Pin *p = (Pin *)This;
    EnterCriticalSection(&p->filter->lock);
    HRESULT hr = S_OK;
    if (p->filter->state != State_Stopped) hr = VFW_E_NOT_STOPPED;
    else if (!p->peer) hr = S_FALSE;
    else pin_drop_connection(p);
    LeaveCriticalSection(&p->filter->lock);
    return hr;
}

static HRESULT STDMETHODCALLTYPE pin_ConnectedTo(IPin *This, IPin **pin) {
    Pin *p = (Pin *)This;
    if (!pin) return E_POINTER;
    EnterCriticalSection(&p->filter->lock);
    *pin = p->peer;
    if (p->peer) IPin_AddRef(p->peer);
    LeaveCriticalSection(&p->filter->lock);
    return *pin ? S_OK : VFW_E_NOT_CONNECTED;
}

static HRESULT STDMETHODCALLTYPE pin_ConnectionMediaType(IPin *This, AM_MEDIA_TYPE *pmt) {
    Pin *p = (Pin *)This;
    if (!pmt) return E_POINTER;
    EnterCriticalSection(&p->filter->lock);
    HRESULT hr;
    if (p->connected) hr = copy_mt(pmt, &p->mt);
    else {
        ZeroMemory(pmt, sizeof(*pmt));
        hr = VFW_E_NOT_CONNECTED;
    }
    LeaveCriticalSection(&p->filter->lock);
    return hr;
}

static HRESULT STDMETHODCALLTYPE pin_QueryPinInfo(IPin *This, PIN_INFO *info) {
    Pin *p = (Pin *)This;
    if (!info) return E_POINTER;
    info->pFilter = (IBaseFilter *)p->filter;
    filter_addref(p->filter);
    info->dir = PINDIR_OUTPUT;
    lstrcpynW(info->achName, PIN_NAME, MAX_PIN_NAME);
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE pin_QueryDirection(IPin *This, PIN_DIRECTION *dir) {
    (void)This;
    if (!dir) return E_POINTER;
    *dir = PINDIR_OUTPUT;
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE pin_QueryId(IPin *This, LPWSTR *id) {
    (void)This;
    if (!id) return E_POINTER;
    *id = (LPWSTR)CoTaskMemAlloc(sizeof(PIN_NAME));
    if (!*id) return E_OUTOFMEMORY;
    CopyMemory(*id, PIN_NAME, sizeof(PIN_NAME));
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE pin_QueryAccept(IPin *This, const AM_MEDIA_TYPE *pmt) {
    (void)This;
    OutFmt o;
    return parse_mt(pmt, &o) ? S_OK : S_FALSE;
}

static HRESULT STDMETHODCALLTYPE pin_EnumMediaTypes(IPin *This, IEnumMediaTypes **en) {
    return enum_types_new(((Pin *)This)->filter, 0, en);
}

static HRESULT STDMETHODCALLTYPE pin_QueryInternalConnections(IPin *This, IPin **pins, ULONG *n) {
    (void)This, (void)pins, (void)n;
    return E_NOTIMPL;
}
static HRESULT STDMETHODCALLTYPE pin_EndOfStream(IPin *This) {
    (void)This;
    return E_UNEXPECTED;
}
static HRESULT STDMETHODCALLTYPE pin_BeginFlush(IPin *This) {
    (void)This;
    return E_UNEXPECTED;
}
static HRESULT STDMETHODCALLTYPE pin_EndFlush(IPin *This) {
    (void)This;
    return E_UNEXPECTED;
}
static HRESULT STDMETHODCALLTYPE pin_NewSegment(IPin *This, REFERENCE_TIME a, REFERENCE_TIME b, double rate) {
    (void)This, (void)a, (void)b, (void)rate;
    return S_OK;
}

static const IPinVtbl kPinVtbl = {
    pin_QueryInterface, pin_AddRef,          pin_Release,           pin_Connect,
    pin_ReceiveConnection, pin_Disconnect,   pin_ConnectedTo,       pin_ConnectionMediaType,
    pin_QueryPinInfo,   pin_QueryDirection,  pin_QueryId,           pin_QueryAccept,
    pin_EnumMediaTypes, pin_QueryInternalConnections, pin_EndOfStream, pin_BeginFlush,
    pin_EndFlush,       pin_NewSegment,
};

/* ---------------- Patilla: IAMStreamConfig ---------------- */

static HRESULT STDMETHODCALLTYPE cfg_QueryInterface(IAMStreamConfig *This, REFIID riid, void **ppv) {
    return pin_qi(PIN_FROM_CFG(This), riid, ppv);
}
static ULONG STDMETHODCALLTYPE cfg_AddRef(IAMStreamConfig *This) { return filter_addref(PIN_FROM_CFG(This)->filter); }
static ULONG STDMETHODCALLTYPE cfg_Release(IAMStreamConfig *This) { return filter_release(PIN_FROM_CFG(This)->filter); }

static HRESULT STDMETHODCALLTYPE cfg_SetFormat(IAMStreamConfig *This, AM_MEDIA_TYPE *pmt) {
    Pin *p = PIN_FROM_CFG(This);
    Filter *f = p->filter;
    OutFmt o;
    if (pmt && !parse_mt(pmt, &o)) return VFW_E_INVALIDMEDIATYPE;
    EnterCriticalSection(&f->lock);
    if (p->hasPref) {
        free_mt(&p->pref);
        p->hasPref = FALSE;
    }
    HRESULT hr = S_OK;
    if (pmt) {
        hr = copy_mt(&p->pref, pmt);
        p->hasPref = SUCCEEDED(hr);
    }
    BOOL reconnect = SUCCEEDED(hr) && pmt && p->connected && f->graph;
    if (reconnect && f->state != State_Stopped) hr = VFW_E_NOT_STOPPED;
    else if (reconnect && p->peer && IPin_QueryAccept(p->peer, pmt) != S_OK) hr = VFW_E_INVALIDMEDIATYPE;
    IFilterGraph *graph = f->graph;
    LeaveCriticalSection(&f->lock);
    /* Ya conectada: el grafo la reconecta y Connect probará primero este tipo. */
    if (SUCCEEDED(hr) && reconnect) hr = IFilterGraph_Reconnect(graph, (IPin *)p);
    return hr;
}

static HRESULT STDMETHODCALLTYPE cfg_GetFormat(IAMStreamConfig *This, AM_MEDIA_TYPE **ppmt) {
    Pin *p = PIN_FROM_CFG(This);
    if (!ppmt) return E_POINTER;
    EnterCriticalSection(&p->filter->lock);
    AM_MEDIA_TYPE tmp;
    HRESULT hr;
    if (p->connected) hr = copy_mt(&tmp, &p->mt);
    else if (p->hasPref) hr = copy_mt(&tmp, &p->pref);
    else hr = make_caps_mt(0, &tmp);
    LeaveCriticalSection(&p->filter->lock);
    if (FAILED(hr)) return hr;
    *ppmt = (AM_MEDIA_TYPE *)CoTaskMemAlloc(sizeof(AM_MEDIA_TYPE));
    if (!*ppmt) {
        free_mt(&tmp);
        return E_OUTOFMEMORY;
    }
    **ppmt = tmp;
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE cfg_GetNumberOfCapabilities(IAMStreamConfig *This, int *count, int *size) {
    (void)This;
    if (!count || !size) return E_POINTER;
    *count = CAPS_COUNT;
    *size = sizeof(VIDEO_STREAM_CONFIG_CAPS);
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE cfg_GetStreamCaps(IAMStreamConfig *This, int i, AM_MEDIA_TYPE **ppmt, BYTE *scc) {
    (void)This;
    if (!ppmt || !scc) return E_POINTER;
    if (i < 0) return E_INVALIDARG;
    if (i >= CAPS_COUNT) return S_FALSE;
    AM_MEDIA_TYPE *mt = (AM_MEDIA_TYPE *)CoTaskMemAlloc(sizeof(AM_MEDIA_TYPE));
    if (!mt) return E_OUTOFMEMORY;
    HRESULT hr = make_caps_mt(i, mt);
    if (FAILED(hr)) {
        CoTaskMemFree(mt);
        return hr;
    }
    *ppmt = mt;
    int w = kSizes[i / FMT_COUNT].w, h = kSizes[i / FMT_COUNT].h;
    DWORD bytes = frame_bytes(i % FMT_COUNT, w, h);
    VIDEO_STREAM_CONFIG_CAPS *c = (VIDEO_STREAM_CONFIG_CAPS *)scc;
    ZeroMemory(c, sizeof(*c));
    c->guid = FORMAT_VideoInfo;
    c->VideoStandard = 0;
    c->InputSize.cx = w;
    c->InputSize.cy = h;
    c->MinCroppingSize = c->MaxCroppingSize = c->InputSize;
    c->CropGranularityX = c->CropGranularityY = 1;
    c->CropAlignX = c->CropAlignY = 1;
    c->MinOutputSize = c->MaxOutputSize = c->InputSize;
    c->OutputGranularityX = c->OutputGranularityY = 1;
    c->MinFrameInterval = MIN_INTERVAL;
    c->MaxFrameInterval = MAX_INTERVAL;
    c->MinBitsPerSecond = (LONG)(bytes * 8);
    c->MaxBitsPerSecond = (LONG)((LONGLONG)bytes * 8 * 60 > 0x7fffffff ? 0x7fffffff : (LONGLONG)bytes * 8 * 60);
    return S_OK;
}

static const IAMStreamConfigVtbl kCfgVtbl = {cfg_QueryInterface, cfg_AddRef,
                                             cfg_Release,        cfg_SetFormat,
                                             cfg_GetFormat,      cfg_GetNumberOfCapabilities,
                                             cfg_GetStreamCaps};

/* ---------------- Patilla: IKsPropertySet (categoría «captura») ---------------- */

static HRESULT STDMETHODCALLTYPE ks_QueryInterface(IKsPropertySet *This, REFIID riid, void **ppv) {
    return pin_qi(PIN_FROM_KS(This), riid, ppv);
}
static ULONG STDMETHODCALLTYPE ks_AddRef(IKsPropertySet *This) { return filter_addref(PIN_FROM_KS(This)->filter); }
static ULONG STDMETHODCALLTYPE ks_Release(IKsPropertySet *This) { return filter_release(PIN_FROM_KS(This)->filter); }

static HRESULT STDMETHODCALLTYPE ks_Set(IKsPropertySet *This, REFGUID set, DWORD id, void *inst, DWORD ninst,
                                        void *data, DWORD ndata) {
    (void)This, (void)set, (void)id, (void)inst, (void)ninst, (void)data, (void)ndata;
    return E_NOTIMPL;
}

static HRESULT STDMETHODCALLTYPE ks_Get(IKsPropertySet *This, REFGUID set, DWORD id, void *inst, DWORD ninst,
                                        void *data, DWORD ndata, DWORD *returned) {
    (void)This, (void)inst, (void)ninst;
    if (!IsEqualGUID(set, &AMPROPSETID_Pin)) return E_PROP_SET_UNSUPPORTED;
    if (id != AMPROPERTY_PIN_CATEGORY) return E_PROP_ID_UNSUPPORTED;
    if (returned) *returned = sizeof(GUID);
    if (!data) return returned ? S_OK : E_POINTER;
    if (ndata < sizeof(GUID)) return E_UNEXPECTED;
    *(GUID *)data = PIN_CATEGORY_CAPTURE;
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE ks_QuerySupported(IKsPropertySet *This, REFGUID set, DWORD id, DWORD *support) {
    (void)This;
    if (!IsEqualGUID(set, &AMPROPSETID_Pin)) return E_PROP_SET_UNSUPPORTED;
    if (id != AMPROPERTY_PIN_CATEGORY) return E_PROP_ID_UNSUPPORTED;
    if (support) *support = KSPROPERTY_SUPPORT_GET;
    return S_OK;
}

static const IKsPropertySetVtbl kKsVtbl = {ks_QueryInterface, ks_AddRef, ks_Release, ks_Set, ks_Get,
                                           ks_QuerySupported};

/* ---------------- Emisión ---------------- */

static LONGLONG now_100ns(void) {
    static LARGE_INTEGER freq;
    LARGE_INTEGER c;
    if (!freq.QuadPart) QueryPerformanceFrequency(&freq);
    QueryPerformanceCounter(&c);
    return (LONGLONG)((double)c.QuadPart * 10000000.0 / (double)freq.QuadPart);
}

/* Reserva los búferes de trabajo para el formato actual. */
static BOOL stream_alloc(Filter *f) {
    const OutFmt *o = &f->pin.of;
    HANDLE heap = GetProcessHeap();
    if (!f->src) f->src = (BYTE *)HeapAlloc(heap, 0, TC_FRAME_BYTES);
    if (f->frame) HeapFree(heap, 0, f->frame);
    if (f->out) HeapFree(heap, 0, f->out);
    f->frame = (BYTE *)HeapAlloc(heap, 0, (size_t)o->w * o->h * 3 / 2);
    f->out = (BYTE *)HeapAlloc(heap, 0, o->size);
    f->outValid = FALSE;
    f->shown = -1;
    f->lastSeq = 0;
    return f->src && f->frame && f->out;
}

/*
 * Pone al día f->out con lo que toque enseñar: el último fotograma de TurboCAM o la
 * tarjeta del estado. Solo trabaja si algo ha cambiado.
 */
static void stream_update(Filter *f) {
    const OutFmt *o = &f->pin.of;
    Shm *s = &f->shm;
    int state = TC_STATE_OFF;
    if (shm_open(s)) {
        BYTE *b = s->base;
        DWORD beat = (DWORD)*U32(b, TC_OFF_HEARTBEAT);
        if ((DWORD)*U32(b, TC_OFF_MAGIC) == TC_MAGIC && (DWORD)*U32(b, TC_OFF_VERSION) == TC_VERSION &&
            GetTickCount() - beat < TC_HEARTBEAT_TIMEOUT_MS)
            state = (int)*U32(b, TC_OFF_STATE);
        shm_beat_consumer(s, o);
    }

    if (state == TC_STATE_LIVE) {
        BYTE *b = s->base;
        DWORD seq = (DWORD)*U32(b, TC_OFF_LATEST);
        if (seq != 0 && (seq != f->lastSeq || f->shown != TC_STATE_LIVE)) {
            BYTE *slot = b + TC_SLOT_BASE + (size_t)(seq % TC_SLOTS) * TC_SLOT_STRIDE;
            DWORD w = (DWORD)*U32(slot, TC_SLOT_WIDTH), h = (DWORD)*U32(slot, TC_SLOT_HEIGHT);
            MemoryBarrier();
            if ((DWORD)*U32(slot, TC_SLOT_SEQ) == seq && w >= MIN_SIDE && h >= MIN_SIDE && w <= TC_MAX_W &&
                h <= TC_MAX_H && !(w & 1) && !(h & 1)) {
                CopyMemory(f->src, slot + TC_SLOT_DATA, (size_t)w * h * 3 / 2);
                MemoryBarrier();
                /* Si la ranura se ha reescrito mientras copiábamos, la copia no vale. */
                if ((DWORD)*U32(slot, TC_SLOT_SEQ) == seq) {
                    scale_nv12(f->src, (int)w, (int)h, f->frame, o->w, o->h);
                    pack_frame(f->frame, o, f->out);
                    f->lastSeq = seq;
                    f->shown = TC_STATE_LIVE;
                    f->outValid = TRUE;
                }
            }
        }
        if (f->shown == TC_STATE_LIVE) return;
        state = TC_STATE_WAITING; /* todavía no hay ningún fotograma entero */
    }
    if (f->shown != state || !f->outValid) {
        if (render_card(state, o->w, o->h, f->frame)) {
            pack_frame(f->frame, o, f->out);
            f->outValid = TRUE;
        }
        f->shown = state;
    }
}

static DWORD WINAPI stream_thread(void *arg) {
    Filter *f = (Filter *)arg;
    Pin *p = &f->pin;
    BOOL comOk = SUCCEEDED(CoInitializeEx(NULL, COINIT_MULTITHREADED));
    SetThreadPriority(GetCurrentThread(), THREAD_PRIORITY_ABOVE_NORMAL);
    LONGLONG interval = p->of.interval;
    LONGLONG next = now_100ns();
    LONGLONG frameNo = 0;
    BOOL refused = FALSE;
    /* Temporizador de alta resolución (Windows 10 1803+): ritmo estable aunque nadie
       haya subido la resolución del reloj del sistema. Si no existe, uno normal. */
    HANDLE timer = CreateWaitableTimerExW(NULL, NULL, CREATE_WAITABLE_TIMER_HIGH_RESOLUTION, TIMER_ALL_ACCESS);
    if (!timer) timer = CreateWaitableTimerExW(NULL, NULL, 0, TIMER_ALL_ACCESS);
    HANDLE waits[2] = {f->stopEvent, timer};

    for (;;) {
        LONGLONG wait = next - now_100ns();
        if (wait > 0) {
            DWORD r;
            LARGE_INTEGER due;
            due.QuadPart = -wait;
            if (timer && SetWaitableTimer(timer, &due, 0, NULL, NULL, FALSE))
                r = WaitForMultipleObjects(2, waits, FALSE, INFINITE);
            else
                r = WaitForSingleObject(f->stopEvent, (DWORD)((wait + 9999) / 10000));
            if (r == WAIT_OBJECT_0) break;
        } else if (WaitForSingleObject(f->stopEvent, 0) == WAIT_OBJECT_0) {
            break;
        }
        LONGLONG now = now_100ns();
        next += interval;
        if (now - next > interval * 4) next = now + interval; /* se quedó atrás: no recuperar a ráfagas */
        if (f->state != State_Running || refused) continue;

        IMediaSample *s = NULL;
        HRESULT hr = IMemAllocator_GetBuffer(p->alloc, &s, NULL, NULL, 0);
        if (FAILED(hr) || !s) break; /* asignador desactivado: nos paran */

        /* El otro lado puede cambiar el formato (p. ej. otro ancho de línea). */
        AM_MEDIA_TYPE *dyn = NULL;
        if (IMediaSample_GetMediaType(s, &dyn) == S_OK && dyn) {
            OutFmt o;
            if (parse_mt(dyn, &o)) {
                EnterCriticalSection(&f->lock);
                free_mt(&p->mt);
                copy_mt(&p->mt, dyn);
                p->of = o;
                LeaveCriticalSection(&f->lock);
                interval = o.interval;
                if (!stream_alloc(f)) {
                    delete_mt(dyn);
                    IMediaSample_Release(s);
                    break;
                }
            }
            delete_mt(dyn);
        }

        stream_update(f);
        BYTE *dst = NULL;
        long cap = IMediaSample_GetSize(s);
        if (f->outValid && SUCCEEDED(IMediaSample_GetPointer(s, &dst)) && dst && cap >= (long)p->of.size) {
            CopyMemory(dst, f->out, p->of.size);
            IMediaSample_SetActualDataLength(s, (long)p->of.size);

            REFERENCE_TIME st = frameNo * interval, en;
            EnterCriticalSection(&f->clockLock);
            if (f->clock) {
                REFERENCE_TIME c;
                if (SUCCEEDED(IReferenceClock_GetTime(f->clock, &c))) st = c - f->tStart;
            }
            LeaveCriticalSection(&f->clockLock);
            if (st < 0) st = 0;
            en = st + interval;
            IMediaSample_SetTime(s, &st, &en);
            LONGLONG m0 = frameNo, m1 = frameNo + 1;
            IMediaSample_SetMediaTime(s, &m0, &m1);
            IMediaSample_SetSyncPoint(s, TRUE);
            IMediaSample_SetPreroll(s, FALSE);
            IMediaSample_SetDiscontinuity(s, frameNo == 0);
            hr = IMemInputPin_Receive(p->input, s);
            frameNo++;
            /* S_FALSE o error: el otro lado no quiere más hasta que se pare el grafo. */
            if (hr != S_OK) refused = TRUE;
        }
        IMediaSample_Release(s);
    }
    if (timer) CloseHandle(timer);
    shm_release_consumer(&f->shm);
    if (comOk) CoUninitialize();
    return 0;
}

/* ---------------- Filtro: IBaseFilter ---------------- */

static HRESULT filter_qi(Filter *f, REFIID riid, void **ppv) {
    if (!ppv) return E_POINTER;
    if (IsEqualIID(riid, &IID_IUnknown) || IsEqualIID(riid, &IID_IPersist) || IsEqualIID(riid, &IID_IMediaFilter) ||
        IsEqualIID(riid, &IID_IBaseFilter))
        *ppv = &f->lpVtbl;
    else if (IsEqualIID(riid, &IID_IAMFilterMiscFlags))
        *ppv = &f->miscVtbl;
    else {
        *ppv = NULL;
        return E_NOINTERFACE;
    }
    filter_addref(f);
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE flt_QueryInterface(IBaseFilter *This, REFIID riid, void **ppv) {
    return filter_qi((Filter *)This, riid, ppv);
}
static ULONG STDMETHODCALLTYPE flt_AddRef(IBaseFilter *This) { return filter_addref((Filter *)This); }
static ULONG STDMETHODCALLTYPE flt_Release(IBaseFilter *This) { return filter_release((Filter *)This); }

static HRESULT STDMETHODCALLTYPE flt_GetClassID(IBaseFilter *This, CLSID *id) {
    (void)This;
    if (!id) return E_POINTER;
    *id = CLSID_TurboCam;
    return S_OK;
}

/* Para la emisión. Con el cerrojo del filtro cogido. */
static void filter_stop_stream(Filter *f) {
    if (f->pin.alloc) IMemAllocator_Decommit(f->pin.alloc); /* despierta un GetBuffer bloqueado */
    if (f->thread) {
        SetEvent(f->stopEvent);
        WaitForSingleObject(f->thread, INFINITE);
        CloseHandle(f->thread);
        f->thread = NULL;
        ResetEvent(f->stopEvent);
    }
}

static HRESULT STDMETHODCALLTYPE flt_Stop(IBaseFilter *This) {
    Filter *f = (Filter *)This;
    EnterCriticalSection(&f->lock);
    InterlockedExchange(&f->state, State_Stopped);
    filter_stop_stream(f);
    LeaveCriticalSection(&f->lock);
    return S_OK;
}

static HRESULT filter_pause(Filter *f) {
    if (f->state == State_Stopped && f->pin.connected) {
        HRESULT hr = IMemAllocator_Commit(f->pin.alloc);
        if (FAILED(hr)) return hr;
        if (!stream_alloc(f)) {
            IMemAllocator_Decommit(f->pin.alloc);
            return E_OUTOFMEMORY;
        }
        ResetEvent(f->stopEvent);
        f->thread = CreateThread(NULL, 0, stream_thread, f, 0, NULL);
        if (!f->thread) {
            IMemAllocator_Decommit(f->pin.alloc);
            return HRESULT_FROM_WIN32(GetLastError());
        }
    }
    InterlockedExchange(&f->state, State_Paused);
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE flt_Pause(IBaseFilter *This) {
    Filter *f = (Filter *)This;
    EnterCriticalSection(&f->lock);
    HRESULT hr = filter_pause(f);
    LeaveCriticalSection(&f->lock);
    return hr;
}

static HRESULT STDMETHODCALLTYPE flt_Run(IBaseFilter *This, REFERENCE_TIME start) {
    Filter *f = (Filter *)This;
    EnterCriticalSection(&f->lock);
    HRESULT hr = S_OK;
    if (f->state == State_Stopped) hr = filter_pause(f);
    if (SUCCEEDED(hr)) {
        EnterCriticalSection(&f->clockLock);
        f->tStart = start;
        LeaveCriticalSection(&f->clockLock);
        InterlockedExchange(&f->state, State_Running);
    }
    LeaveCriticalSection(&f->lock);
    return hr;
}

static HRESULT STDMETHODCALLTYPE flt_GetState(IBaseFilter *This, DWORD ms, FILTER_STATE *st) {
    Filter *f = (Filter *)This;
    (void)ms;
    if (!st) return E_POINTER;
    *st = (FILTER_STATE)f->state;
    /* Fuente en directo: en pausa no entrega nada. */
    return *st == State_Paused ? VFW_S_CANT_CUE : S_OK;
}

static HRESULT STDMETHODCALLTYPE flt_SetSyncSource(IBaseFilter *This, IReferenceClock *clock) {
    Filter *f = (Filter *)This;
    if (clock) IReferenceClock_AddRef(clock);
    EnterCriticalSection(&f->clockLock);
    IReferenceClock *old = f->clock;
    f->clock = clock;
    LeaveCriticalSection(&f->clockLock);
    if (old) IReferenceClock_Release(old);
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE flt_GetSyncSource(IBaseFilter *This, IReferenceClock **clock) {
    Filter *f = (Filter *)This;
    if (!clock) return E_POINTER;
    EnterCriticalSection(&f->clockLock);
    *clock = f->clock;
    if (f->clock) IReferenceClock_AddRef(f->clock);
    LeaveCriticalSection(&f->clockLock);
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE flt_EnumPins(IBaseFilter *This, IEnumPins **en) {
    return enum_pins_new((Filter *)This, 0, en);
}

static HRESULT STDMETHODCALLTYPE flt_FindPin(IBaseFilter *This, LPCWSTR id, IPin **pin) {
    Filter *f = (Filter *)This;
    if (!id || !pin) return E_POINTER;
    if (lstrcmpW(id, PIN_NAME) != 0) {
        *pin = NULL;
        return VFW_E_NOT_FOUND;
    }
    *pin = (IPin *)&f->pin;
    IPin_AddRef(*pin);
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE flt_QueryFilterInfo(IBaseFilter *This, FILTER_INFO *info) {
    Filter *f = (Filter *)This;
    if (!info) return E_POINTER;
    EnterCriticalSection(&f->lock);
    lstrcpynW(info->achName, f->name, MAX_FILTER_NAME);
    info->pGraph = f->graph;
    if (f->graph) IFilterGraph_AddRef(f->graph);
    LeaveCriticalSection(&f->lock);
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE flt_JoinFilterGraph(IBaseFilter *This, IFilterGraph *graph, LPCWSTR name) {
    Filter *f = (Filter *)This;
    EnterCriticalSection(&f->lock);
    f->graph = graph;
    lstrcpynW(f->name, name ? name : FILTER_NAME, MAX_FILTER_NAME);
    LeaveCriticalSection(&f->lock);
    return S_OK;
}

static HRESULT STDMETHODCALLTYPE flt_QueryVendorInfo(IBaseFilter *This, LPWSTR *info) {
    (void)This, (void)info;
    return E_NOTIMPL;
}

static const IBaseFilterVtbl kFilterVtbl = {
    flt_QueryInterface, flt_AddRef,   flt_Release,         flt_GetClassID,      flt_Stop,
    flt_Pause,          flt_Run,      flt_GetState,        flt_SetSyncSource,   flt_GetSyncSource,
    flt_EnumPins,       flt_FindPin,  flt_QueryFilterInfo, flt_JoinFilterGraph, flt_QueryVendorInfo,
};

/* ---------------- Filtro: IAMFilterMiscFlags ---------------- */

static HRESULT STDMETHODCALLTYPE misc_QueryInterface(IAMFilterMiscFlags *This, REFIID riid, void **ppv) {
    return filter_qi(FILTER_FROM_MISC(This), riid, ppv);
}
static ULONG STDMETHODCALLTYPE misc_AddRef(IAMFilterMiscFlags *This) { return filter_addref(FILTER_FROM_MISC(This)); }
static ULONG STDMETHODCALLTYPE misc_Release(IAMFilterMiscFlags *This) {
    return filter_release(FILTER_FROM_MISC(This));
}
static ULONG STDMETHODCALLTYPE misc_GetMiscFlags(IAMFilterMiscFlags *This) {
    (void)This;
    return AM_FILTER_MISC_FLAGS_IS_SOURCE;
}

static const IAMFilterMiscFlagsVtbl kMiscVtbl = {misc_QueryInterface, misc_AddRef, misc_Release, misc_GetMiscFlags};

/* ---------------- Vida del filtro ---------------- */

static Filter *filter_new(void) {
    Filter *f = (Filter *)HeapAlloc(GetProcessHeap(), HEAP_ZERO_MEMORY, sizeof(Filter));
    if (!f) return NULL;
    f->stopEvent = CreateEventW(NULL, TRUE, FALSE, NULL);
    if (!f->stopEvent) {
        HeapFree(GetProcessHeap(), 0, f);
        return NULL;
    }
    f->lpVtbl = &kFilterVtbl;
    f->miscVtbl = &kMiscVtbl;
    f->ref = 1;
    InitializeCriticalSection(&f->lock);
    InitializeCriticalSection(&f->clockLock);
    f->state = State_Stopped;
    lstrcpynW(f->name, FILTER_NAME, MAX_FILTER_NAME);
    f->pin.lpVtbl = &kPinVtbl;
    f->pin.cfgVtbl = &kCfgVtbl;
    f->pin.ksVtbl = &kKsVtbl;
    f->pin.filter = f;
    f->shm.consumer = -1;
    f->shown = -1;
    InterlockedIncrement(&g_objects);
    return f;
}

static void filter_destroy(Filter *f) {
    HANDLE heap = GetProcessHeap();
    filter_stop_stream(f);
    pin_drop_connection(&f->pin);
    if (f->pin.hasPref) free_mt(&f->pin.pref);
    if (f->clock) IReferenceClock_Release(f->clock);
    shm_close(&f->shm);
    if (f->src) HeapFree(heap, 0, f->src);
    if (f->frame) HeapFree(heap, 0, f->frame);
    if (f->out) HeapFree(heap, 0, f->out);
    CloseHandle(f->stopEvent);
    DeleteCriticalSection(&f->lock);
    DeleteCriticalSection(&f->clockLock);
    HeapFree(heap, 0, f);
    InterlockedDecrement(&g_objects);
}

/* ------------------------------------------------------------------------- */
/* Fábrica y exportaciones                                                   */
/* ------------------------------------------------------------------------- */

static HRESULT STDMETHODCALLTYPE cf_QueryInterface(IClassFactory *This, REFIID riid, void **ppv) {
    if (!ppv) return E_POINTER;
    if (IsEqualIID(riid, &IID_IUnknown) || IsEqualIID(riid, &IID_IClassFactory)) {
        *ppv = This;
        return S_OK;
    }
    *ppv = NULL;
    return E_NOINTERFACE;
}
static ULONG STDMETHODCALLTYPE cf_AddRef(IClassFactory *This) {
    (void)This;
    return 2;
}
static ULONG STDMETHODCALLTYPE cf_Release(IClassFactory *This) {
    (void)This;
    return 1;
}
static HRESULT STDMETHODCALLTYPE cf_CreateInstance(IClassFactory *This, IUnknown *outer, REFIID riid, void **ppv) {
    (void)This;
    if (!ppv) return E_POINTER;
    *ppv = NULL;
    if (outer) return CLASS_E_NOAGGREGATION;
    Filter *f = filter_new();
    if (!f) return E_OUTOFMEMORY;
    HRESULT hr = filter_qi(f, riid, ppv);
    filter_release(f);
    return hr;
}
static HRESULT STDMETHODCALLTYPE cf_LockServer(IClassFactory *This, BOOL lock) {
    (void)This;
    if (lock) InterlockedIncrement(&g_locks);
    else InterlockedDecrement(&g_locks);
    return S_OK;
}

static const IClassFactoryVtbl kFactoryVtbl = {cf_QueryInterface, cf_AddRef, cf_Release, cf_CreateInstance,
                                               cf_LockServer};
static IClassFactory g_factory = {(IClassFactoryVtbl *)&kFactoryVtbl};

STDAPI DllGetClassObject(REFCLSID clsid, REFIID riid, void **ppv) {
    if (!ppv) return E_POINTER;
    *ppv = NULL;
    if (!IsEqualCLSID(clsid, &CLSID_TurboCam)) return CLASS_E_CLASSNOTAVAILABLE;
    return cf_QueryInterface(&g_factory, riid, ppv);
}

STDAPI DllCanUnloadNow(void) { return g_objects == 0 && g_locks == 0 ? S_OK : S_FALSE; }

BOOL WINAPI DllMain(HINSTANCE inst, DWORD reason, void *reserved) {
    (void)reserved;
    (void)inst, (void)reason;
    return TRUE;
}
