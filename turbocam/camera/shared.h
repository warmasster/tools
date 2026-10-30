/*
 * Piezas que comparten las dos cámaras de TurboCAM: la de DirectShow (turbocam.c,
 * la cargan las apps) y la nativa de Windows 11 (mfcam.cpp, la carga el servicio de
 * cámaras de Windows). Escalar NV12 y dibujar las tarjetas sin imagen.
 * Vale para C y para C++.
 */
#ifndef TURBOCAM_SHARED_H
#define TURBOCAM_SHARED_H

#include <windows.h>
#include <math.h>
#include "protocol.h"

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

#endif
