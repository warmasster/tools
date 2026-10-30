/*
 * TurboCAM: cámara nativa de Windows 11 (Media Foundation).
 *
 * Es una «fuente de vídeo» que carga el servicio de cámaras de Windows (Frame
 * Server), no las apps: las apps abren la cámara con piezas de Windows y no cargan
 * nada nuestro. Así funciona en las que no dejan cargar DLL de terceros (Discord
 * desde su versión de octubre de 2026) y en las que solo usan Media Foundation (la
 * app Cámara de Windows).
 *
 * La imagen sale de la misma memoria compartida que la de DirectShow (protocol.h).
 * El servicio corre en la sesión 0, así que la busca en la sesión del usuario con
 * el prefijo Session\<n>\.
 *
 * La da de alta TurboCAM.exe (con permiso de administrador una vez):
 * HKLM\Software\Classes\CLSID\{EA00E74F-...} y MFCreateVirtualCamera.
 */
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <sddl.h>
#include <mfapi.h>
#include <mfidl.h>
#include <mferror.h>
#include <ks.h>
#include <ksproxy.h>
#include <ksmedia.h>
#include <initguid.h>
#include <new>
#include "protocol.h"
#include "shared.h"

/* {EA00E74F-EAEE-4F98-B997-53B20BE0F23A}: también en nativecam.go. */
DEFINE_GUID(CLSID_TurboCamMF, 0xea00e74f, 0xeaee, 0x4f98, 0xb9, 0x97, 0x53, 0xb2, 0x0b, 0xe0, 0xf2, 0x3a);

#define MIN_SIDE 32
#define FPS 30
#define FRAME_100NS (10000000LL / FPS)

static const struct { int w, h; } kSizes[] = {{1920, 1080}, {1280, 720}, {960, 540}, {854, 480}, {640, 360}};
#define SIZE_COUNT ((int)(sizeof(kSizes) / sizeof(kSizes[0])))

static volatile LONG g_objects = 0;
static volatile LONG g_locks = 0;

template <class T> static void SafeRelease(T *&p) {
    if (p) {
        p->Release();
        p = nullptr;
    }
}

struct Lock {
    CRITICAL_SECTION *cs;
    explicit Lock(CRITICAL_SECTION *c) : cs(c) { EnterCriticalSection(cs); }
    ~Lock() { LeaveCriticalSection(cs); }
};

/* ------------------------------------------------------------------------- */
/* Memoria compartida (en la sesión del usuario)                             */
/* ------------------------------------------------------------------------- */

struct Shm {
    HANDLE map = nullptr;
    BYTE *base = nullptr;
    BOOL writable = FALSE;
    DWORD lastTry = 0;
    int consumer = -1;

    BOOL open() {
        if (base) return TRUE;
        DWORD now = GetTickCount();
        if (lastTry && now - lastTry < 1000) return FALSE;
        lastTry = now ? now : 1;
        /* El servicio está en la sesión 0: la memoria está en la de la consola (o en
           la nuestra, si esto corre dentro de la sesión del usuario, en pruebas). */
        WCHAR name[96];
        DWORD sessions[2] = {WTSGetActiveConsoleSessionId(), (DWORD)-1};
        DWORD mine = 0;
        ProcessIdToSessionId(GetCurrentProcessId(), &mine);
        for (int i = 0; i < 2; i++) {
            HANDLE m = nullptr;
            if (i == 0) {
                if (sessions[0] == 0xFFFFFFFF || sessions[0] == mine) continue;
                wsprintfW(name, L"Session\\%lu\\TurboCAM.Video.1", sessions[0]);
            } else {
                lstrcpyW(name, TC_MAP_NAME);
            }
            BOOL rw = TRUE;
            m = OpenFileMappingW(FILE_MAP_READ | FILE_MAP_WRITE, FALSE, name);
            if (!m) {
                m = OpenFileMappingW(FILE_MAP_READ, FALSE, name);
                rw = FALSE;
            }
            if (!m) continue;
            BYTE *v = (BYTE *)MapViewOfFile(m, rw ? FILE_MAP_READ | FILE_MAP_WRITE : FILE_MAP_READ, 0, 0, TC_MAP_SIZE);
            if (!v) {
                CloseHandle(m);
                continue;
            }
            map = m;
            base = v;
            writable = rw;
            consumer = -1;
            return TRUE;
        }
        return FALSE;
    }

    void releaseConsumer() {
        if (base && writable && consumer >= 0)
            InterlockedExchange((volatile LONG *)(base + TC_CONS_BASE + consumer * TC_CONS_SIZE + TC_CONS_PID), 0);
        consumer = -1;
    }

    void close() {
        releaseConsumer();
        if (base) UnmapViewOfFile(base);
        if (map) CloseHandle(map);
        base = nullptr;
        map = nullptr;
    }

    volatile LONG *u32(int off) { return (volatile LONG *)(base + off); }

    /* Apunta que la cámara está en uso y a qué tamaño (para la resolución automática
       y el ahorro de TurboCAM). No se sabe qué app es: el servicio no lo dice. */
    void beat(int w, int h) {
        if (!base || !writable) return;
        DWORD now = GetTickCount();
        LONG pid = (LONG)GetCurrentProcessId();
        if (consumer < 0) {
            for (int i = 0; i < TC_MAX_CONSUMERS && consumer < 0; i++) {
                BYTE *c = base + TC_CONS_BASE + i * TC_CONS_SIZE;
                LONG cur = *(volatile LONG *)(c + TC_CONS_PID);
                DWORD seen = (DWORD) * (volatile LONG *)(c + TC_CONS_SEEN);
                if (cur != 0 && now - seen < TC_CONSUMER_TIMEOUT_MS) continue;
                if (InterlockedCompareExchange((volatile LONG *)(c + TC_CONS_PID), pid, cur) != cur) continue;
                consumer = i;
                const WCHAR *name = L"Windows";
                WCHAR *exe = (WCHAR *)(c + TC_CONS_EXE);
                int k = 0;
                for (; k < TC_CONS_EXE_CHARS - 1 && name[k]; k++) exe[k] = name[k];
                for (; k < TC_CONS_EXE_CHARS; k++) exe[k] = 0;
            }
            if (consumer < 0) return;
        }
        BYTE *c = base + TC_CONS_BASE + consumer * TC_CONS_SIZE;
        *(volatile LONG *)(c + TC_CONS_WIDTH) = w;
        *(volatile LONG *)(c + TC_CONS_HEIGHT) = h;
        *(volatile LONG *)(c + TC_CONS_FPS) = FPS;
        InterlockedExchange((volatile LONG *)(c + TC_CONS_SEEN), (LONG)now);
    }
};

/* Lo que se enseña: el último fotograma de TurboCAM o la tarjeta del estado. */
struct Producer {
    Shm shm;
    BYTE *src = nullptr; /* copia del fotograma de la memoria compartida */
    BYTE *frame = nullptr;
    int w = 0, h = 0;
    int shown = -1;
    DWORD lastSeq = 0;
    BOOL valid = FALSE;

    ~Producer() {
        shm.close();
        if (src) HeapFree(GetProcessHeap(), 0, src);
        if (frame) HeapFree(GetProcessHeap(), 0, frame);
    }

    BOOL size(int nw, int nh) {
        if (nw == w && nh == h && frame) return TRUE;
        if (frame) HeapFree(GetProcessHeap(), 0, frame);
        frame = (BYTE *)HeapAlloc(GetProcessHeap(), 0, (size_t)nw * nh * 3 / 2);
        if (!src) src = (BYTE *)HeapAlloc(GetProcessHeap(), 0, TC_FRAME_BYTES);
        w = nw;
        h = nh;
        shown = -1;
        valid = FALSE;
        return frame && src;
    }

    /* Pone al día frame (NV12 compacto, w x h). */
    void update() {
        int state = TC_STATE_OFF;
        if (shm.open()) {
            BYTE *b = shm.base;
            DWORD beat = (DWORD)*shm.u32(TC_OFF_HEARTBEAT);
            if ((DWORD)*shm.u32(TC_OFF_MAGIC) == TC_MAGIC && (DWORD)*shm.u32(TC_OFF_VERSION) == TC_VERSION &&
                GetTickCount() - beat < TC_HEARTBEAT_TIMEOUT_MS)
                state = (int)*shm.u32(TC_OFF_STATE);
            shm.beat(w, h);
            if (state == TC_STATE_LIVE) {
                DWORD seq = (DWORD)*shm.u32(TC_OFF_LATEST);
                if (seq != 0 && (seq != lastSeq || shown != TC_STATE_LIVE)) {
                    BYTE *slot = b + TC_SLOT_BASE + (size_t)(seq % TC_SLOTS) * TC_SLOT_STRIDE;
                    DWORD sw = (DWORD) * (volatile LONG *)(slot + TC_SLOT_WIDTH);
                    DWORD sh = (DWORD) * (volatile LONG *)(slot + TC_SLOT_HEIGHT);
                    MemoryBarrier();
                    if ((DWORD) * (volatile LONG *)(slot + TC_SLOT_SEQ) == seq && sw >= MIN_SIDE && sh >= MIN_SIDE &&
                        sw <= TC_MAX_W && sh <= TC_MAX_H && !(sw & 1) && !(sh & 1)) {
                        CopyMemory(src, slot + TC_SLOT_DATA, (size_t)sw * sh * 3 / 2);
                        MemoryBarrier();
                        if ((DWORD) * (volatile LONG *)(slot + TC_SLOT_SEQ) == seq) {
                            scale_nv12(src, (int)sw, (int)sh, frame, w, h);
                            lastSeq = seq;
                            shown = TC_STATE_LIVE;
                            valid = TRUE;
                        }
                    }
                }
                if (shown == TC_STATE_LIVE) return;
                state = TC_STATE_WAITING;
            }
        } else {
            state = TC_STATE_OFF;
        }
        if (shown != state || !valid) {
            if (render_card(state, w, h, frame)) valid = TRUE;
            shown = state;
        }
    }
};

/* ------------------------------------------------------------------------- */
/* Tipos de medio                                                            */
/* ------------------------------------------------------------------------- */

static HRESULT make_type(int w, int h, IMFMediaType **out) {
    IMFMediaType *t = nullptr;
    HRESULT hr = MFCreateMediaType(&t);
    if (FAILED(hr)) return hr;
    t->SetGUID(MF_MT_MAJOR_TYPE, MFMediaType_Video);
    t->SetGUID(MF_MT_SUBTYPE, MFVideoFormat_NV12);
    MFSetAttributeSize(t, MF_MT_FRAME_SIZE, w, h);
    MFSetAttributeRatio(t, MF_MT_FRAME_RATE, FPS, 1);
    MFSetAttributeRatio(t, MF_MT_FRAME_RATE_RANGE_MAX, FPS, 1);
    MFSetAttributeRatio(t, MF_MT_FRAME_RATE_RANGE_MIN, FPS, 1);
    MFSetAttributeRatio(t, MF_MT_PIXEL_ASPECT_RATIO, 1, 1);
    t->SetUINT32(MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive);
    t->SetUINT32(MF_MT_ALL_SAMPLES_INDEPENDENT, TRUE);
    t->SetUINT32(MF_MT_FIXED_SIZE_SAMPLES, TRUE);
    t->SetUINT32(MF_MT_DEFAULT_STRIDE, (UINT32)w);
    t->SetUINT32(MF_MT_SAMPLE_SIZE, (UINT32)(w * h * 3 / 2));
    t->SetUINT32(MF_MT_AVG_BITRATE, (UINT32)(w * h * 12 * FPS));
    *out = t;
    return S_OK;
}

/* ------------------------------------------------------------------------- */
/* Flujo                                                                     */
/* ------------------------------------------------------------------------- */

class Source;

class Stream : public IMFMediaStream2, public IKsControl {
  public:
    Stream() { InitializeCriticalSection(&cs_); InterlockedIncrement(&g_objects); }

    HRESULT Init(Source *src, IMFMediaSource *srcItf) {
        source_ = srcItf; /* sin AddRef: la fuente vive más que el flujo */
        (void)src;
        HRESULT hr = MFCreateEventQueue(&queue_);
        if (FAILED(hr)) return hr;
        hr = MFCreateAttributes(&attrs_, 8);
        if (FAILED(hr)) return hr;
        attrs_->SetGUID(MF_DEVICESTREAM_STREAM_CATEGORY, PINNAME_VIDEO_CAPTURE);
        attrs_->SetUINT32(MF_DEVICESTREAM_STREAM_ID, 0);
        attrs_->SetUINT32(MF_DEVICESTREAM_FRAMESERVER_SHARED, 1);
        attrs_->SetUINT32(MF_DEVICESTREAM_ATTRIBUTE_FRAMESOURCE_TYPES, MFFrameSourceTypes_Color);

        IMFMediaType *types[SIZE_COUNT] = {};
        for (int i = 0; i < SIZE_COUNT; i++) {
            hr = make_type(kSizes[i].w, kSizes[i].h, &types[i]);
            if (FAILED(hr)) break;
        }
        if (SUCCEEDED(hr)) hr = MFCreateStreamDescriptor(0, SIZE_COUNT, types, &desc_);
        if (SUCCEEDED(hr)) {
            IMFMediaTypeHandler *th = nullptr;
            hr = desc_->GetMediaTypeHandler(&th);
            if (SUCCEEDED(hr)) hr = th->SetCurrentMediaType(types[1]); /* 720p */
            SafeRelease(th);
            attrs_->CopyAllItems(desc_);
        }
        for (auto &t : types) SafeRelease(t);
        if (FAILED(hr)) return hr;
        stop_ = CreateEventW(nullptr, TRUE, FALSE, nullptr);
        wake_ = CreateEventW(nullptr, FALSE, FALSE, nullptr);
        return stop_ && wake_ ? S_OK : E_OUTOFMEMORY;
    }

    /* IUnknown */
    STDMETHODIMP QueryInterface(REFIID riid, void **ppv) override {
        if (!ppv) return E_POINTER;
        if (riid == IID_IUnknown || riid == IID_IMFMediaEventGenerator || riid == IID_IMFMediaStream ||
            riid == IID_IMFMediaStream2)
            *ppv = static_cast<IMFMediaStream2 *>(this);
        else if (riid == __uuidof(IKsControl))
            *ppv = static_cast<IKsControl *>(this);
        else {
            *ppv = nullptr;
            return E_NOINTERFACE;
        }
        AddRef();
        return S_OK;
    }
    STDMETHODIMP_(ULONG) AddRef() override { return (ULONG)InterlockedIncrement(&ref_); }
    STDMETHODIMP_(ULONG) Release() override {
        LONG r = InterlockedDecrement(&ref_);
        if (r == 0) delete this;
        return (ULONG)r;
    }

    /* IMFMediaEventGenerator */
    STDMETHODIMP GetEvent(DWORD flags, IMFMediaEvent **e) override {
        IMFMediaEventQueue *q;
        {
            Lock l(&cs_);
            if (!queue_) return MF_E_SHUTDOWN;
            q = queue_;
            q->AddRef();
        }
        HRESULT hr = q->GetEvent(flags, e);
        q->Release();
        return hr;
    }
    STDMETHODIMP BeginGetEvent(IMFAsyncCallback *cb, IUnknown *st) override {
        Lock l(&cs_);
        return queue_ ? queue_->BeginGetEvent(cb, st) : MF_E_SHUTDOWN;
    }
    STDMETHODIMP EndGetEvent(IMFAsyncResult *r, IMFMediaEvent **e) override {
        Lock l(&cs_);
        return queue_ ? queue_->EndGetEvent(r, e) : MF_E_SHUTDOWN;
    }
    STDMETHODIMP QueueEvent(MediaEventType t, REFGUID ext, HRESULT st, const PROPVARIANT *v) override {
        Lock l(&cs_);
        return queue_ ? queue_->QueueEventParamVar(t, ext, st, v) : MF_E_SHUTDOWN;
    }

    /* IMFMediaStream */
    STDMETHODIMP GetMediaSource(IMFMediaSource **s) override {
        if (!s) return E_POINTER;
        Lock l(&cs_);
        if (!queue_) return MF_E_SHUTDOWN;
        *s = source_;
        source_->AddRef();
        return S_OK;
    }
    STDMETHODIMP GetStreamDescriptor(IMFStreamDescriptor **d) override {
        if (!d) return E_POINTER;
        Lock l(&cs_);
        if (!queue_) return MF_E_SHUTDOWN;
        *d = desc_;
        desc_->AddRef();
        return S_OK;
    }
    STDMETHODIMP RequestSample(IUnknown *token) override {
        Lock l(&cs_);
        if (!queue_) return MF_E_SHUTDOWN;
        if (state_ != MF_STREAM_STATE_RUNNING) return MF_E_INVALIDREQUEST;
        if (pending_ >= 8) return MF_E_SAMPLEALLOCATOR_EMPTY;
        tokens_[(head_ + pending_) % 8] = token;
        if (token) token->AddRef();
        pending_++;
        SetEvent(wake_);
        return S_OK;
    }

    /* IMFMediaStream2 */
    STDMETHODIMP SetStreamState(MF_STREAM_STATE s) override {
        if (s == MF_STREAM_STATE_RUNNING) return Start();
        if (s == MF_STREAM_STATE_STOPPED) return Stop();
        return MF_E_INVALID_STATE_TRANSITION;
    }
    STDMETHODIMP GetStreamState(MF_STREAM_STATE *s) override {
        if (!s) return E_POINTER;
        Lock l(&cs_);
        *s = state_;
        return S_OK;
    }

    /* IKsControl: no hay propiedades de cámara (brillo, zoom…): se cambian en el móvil. */
    STDMETHODIMP KsProperty(PKSPROPERTY, ULONG, LPVOID, ULONG, ULONG *) override {
        return HRESULT_FROM_WIN32(ERROR_SET_NOT_FOUND);
    }
    STDMETHODIMP KsMethod(PKSMETHOD, ULONG, LPVOID, ULONG, ULONG *) override {
        return HRESULT_FROM_WIN32(ERROR_SET_NOT_FOUND);
    }
    STDMETHODIMP KsEvent(PKSEVENT, ULONG, LPVOID, ULONG, ULONG *) override {
        return HRESULT_FROM_WIN32(ERROR_SET_NOT_FOUND);
    }

    HRESULT Start() {
        Lock l(&cs_);
        if (!queue_) return MF_E_SHUTDOWN;
        /* El tipo que haya elegido la app. */
        IMFMediaTypeHandler *th = nullptr;
        IMFMediaType *mt = nullptr;
        UINT32 w = 1280, h = 720;
        if (SUCCEEDED(desc_->GetMediaTypeHandler(&th)) && SUCCEEDED(th->GetCurrentMediaType(&mt)))
            MFGetAttributeSize(mt, MF_MT_FRAME_SIZE, &w, &h);
        SafeRelease(mt);
        SafeRelease(th);
        w &= ~1u;
        h &= ~1u;
        if (w < MIN_SIDE || h < MIN_SIDE || w > TC_MAX_W || h > TC_MAX_H) return MF_E_INVALIDMEDIATYPE;
        width_ = (int)w;
        height_ = (int)h;
        if (state_ != MF_STREAM_STATE_RUNNING) {
            state_ = MF_STREAM_STATE_RUNNING;
            ResetEvent(stop_);
            AddRef();
            thread_ = CreateThread(nullptr, 0, ThreadProc, this, 0, nullptr);
            if (!thread_) {
                Release();
                state_ = MF_STREAM_STATE_STOPPED;
                return E_OUTOFMEMORY;
            }
        }
        PROPVARIANT v;
        PropVariantInit(&v);
        v.vt = VT_I8;
        v.hVal.QuadPart = MFGetSystemTime();
        return queue_->QueueEventParamVar(MEStreamStarted, GUID_NULL, S_OK, &v);
    }

    HRESULT Stop() {
        HANDLE t = nullptr;
        {
            Lock l(&cs_);
            if (!queue_) return MF_E_SHUTDOWN;
            if (state_ == MF_STREAM_STATE_RUNNING) {
                state_ = MF_STREAM_STATE_STOPPED;
                SetEvent(stop_);
                t = thread_;
                thread_ = nullptr;
            }
        }
        if (t) {
            WaitForSingleObject(t, 5000);
            CloseHandle(t);
        }
        Lock l(&cs_);
        DropTokens();
        return queue_ ? queue_->QueueEventParamVar(MEStreamStopped, GUID_NULL, S_OK, nullptr) : MF_E_SHUTDOWN;
    }

    void Shutdown() {
        Stop();
        Lock l(&cs_);
        if (queue_) queue_->Shutdown();
        SafeRelease(queue_);
        SafeRelease(desc_);
        SafeRelease(attrs_);
    }

    HRESULT Attributes(IMFAttributes **a) {
        Lock l(&cs_);
        if (!attrs_) return MF_E_SHUTDOWN;
        *a = attrs_;
        attrs_->AddRef();
        return S_OK;
    }

  private:
    ~Stream() {
        for (int i = 0; i < pending_; i++) SafeRelease(tokens_[(head_ + i) % 8]);
        SafeRelease(queue_);
        SafeRelease(desc_);
        SafeRelease(attrs_);
        if (stop_) CloseHandle(stop_);
        if (wake_) CloseHandle(wake_);
        DeleteCriticalSection(&cs_);
        InterlockedDecrement(&g_objects);
    }

    void DropTokens() {
        while (pending_ > 0) {
            SafeRelease(tokens_[head_]);
            head_ = (head_ + 1) % 8;
            pending_--;
        }
    }

    static DWORD WINAPI ThreadProc(void *p) {
        Stream *s = (Stream *)p;
        s->Run();
        s->Release();
        return 0;
    }

    /* Entrega un fotograma por cada petición, a 30 por segundo como mucho. */
    void Run() {
        HRESULT co = CoInitializeEx(nullptr, COINIT_MULTITHREADED);
        SetThreadPriority(GetCurrentThread(), THREAD_PRIORITY_ABOVE_NORMAL);
        HANDLE timer = CreateWaitableTimerExW(nullptr, nullptr, CREATE_WAITABLE_TIMER_HIGH_RESOLUTION, TIMER_ALL_ACCESS);
        if (!timer) timer = CreateWaitableTimerExW(nullptr, nullptr, 0, TIMER_ALL_ACCESS);
        Producer prod;
        LONGLONG next = MFGetSystemTime();
        for (;;) {
            /* Esperar a que haya petición. */
            HANDLE w1[2] = {stop_, wake_};
            for (;;) {
                {
                    Lock l(&cs_);
                    if (state_ != MF_STREAM_STATE_RUNNING) goto out;
                    if (pending_ > 0) break;
                }
                if (WaitForMultipleObjects(2, w1, FALSE, INFINITE) == WAIT_OBJECT_0) goto out;
            }
            /* Esperar a su hora. */
            LONGLONG wait = next - MFGetSystemTime();
            if (wait > 0 && timer) {
                LARGE_INTEGER due;
                due.QuadPart = -wait;
                SetWaitableTimer(timer, &due, 0, nullptr, nullptr, FALSE);
                HANDLE w2[2] = {stop_, timer};
                if (WaitForMultipleObjects(2, w2, FALSE, INFINITE) == WAIT_OBJECT_0) goto out;
            }
            LONGLONG now = MFGetSystemTime();
            next += FRAME_100NS;
            if (next < now - FRAME_100NS) next = now; /* se ha quedado atrás: sin ráfagas */

            int w, h;
            IUnknown *token;
            {
                Lock l(&cs_);
                if (state_ != MF_STREAM_STATE_RUNNING || pending_ == 0) continue;
                w = width_;
                h = height_;
                token = tokens_[head_];
                tokens_[head_] = nullptr;
                head_ = (head_ + 1) % 8;
                pending_--;
            }
            IMFSample *sample = nullptr;
            if (prod.size(w, h)) {
                prod.update();
                sample = MakeSample(prod.frame, w, h, now);
            }
            if (sample) {
                if (token) sample->SetUnknown(MFSampleExtension_Token, token);
                Lock l(&cs_);
                if (queue_) queue_->QueueEventParamUnk(MEMediaSample, GUID_NULL, S_OK, sample);
            }
            SafeRelease(sample);
            SafeRelease(token);
        }
    out:
        if (timer) CloseHandle(timer);
        if (SUCCEEDED(co)) CoUninitialize();
    }

    static IMFSample *MakeSample(const BYTE *nv12, int w, int h, LONGLONG time) {
        DWORD n = (DWORD)w * h * 3 / 2;
        IMFMediaBuffer *buf = nullptr;
        IMFSample *s = nullptr;
        if (FAILED(MFCreateMemoryBuffer(n, &buf))) return nullptr;
        BYTE *p = nullptr;
        DWORD max = 0;
        if (SUCCEEDED(buf->Lock(&p, &max, nullptr))) {
            CopyMemory(p, nv12, n);
            buf->Unlock();
            buf->SetCurrentLength(n);
            if (SUCCEEDED(MFCreateSample(&s))) {
                s->AddBuffer(buf);
                s->SetSampleTime(time);
                s->SetSampleDuration(FRAME_100NS);
            }
        }
        buf->Release();
        return s;
    }

    LONG ref_ = 1;
    CRITICAL_SECTION cs_;
    IMFMediaSource *source_ = nullptr;
    IMFMediaEventQueue *queue_ = nullptr;
    IMFStreamDescriptor *desc_ = nullptr;
    IMFAttributes *attrs_ = nullptr;
    MF_STREAM_STATE state_ = MF_STREAM_STATE_STOPPED;
    HANDLE thread_ = nullptr, stop_ = nullptr, wake_ = nullptr;
    IUnknown *tokens_[8] = {};
    int head_ = 0, pending_ = 0;
    int width_ = 1280, height_ = 720;
};

/* ------------------------------------------------------------------------- */
/* Fuente                                                                    */
/* ------------------------------------------------------------------------- */

class Source : public IMFMediaSourceEx, public IMFGetService, public IKsControl {
  public:
    Source() { InitializeCriticalSection(&cs_); InterlockedIncrement(&g_objects); }

    HRESULT Init() {
        HRESULT hr = MFCreateEventQueue(&queue_);
        if (SUCCEEDED(hr)) hr = MFCreateAttributes(&attrs_, 4);
        if (FAILED(hr)) return hr;
        stream_ = new (std::nothrow) Stream();
        if (!stream_) return E_OUTOFMEMORY;
        hr = stream_->Init(this, static_cast<IMFMediaSourceEx *>(this));
        if (FAILED(hr)) return hr;
        IMFStreamDescriptor *sd = nullptr;
        hr = stream_->GetStreamDescriptor(&sd);
        if (SUCCEEDED(hr)) hr = MFCreatePresentationDescriptor(1, &sd, &pd_);
        if (SUCCEEDED(hr)) hr = pd_->SelectStream(0);
        SafeRelease(sd);
        return hr;
    }

    /* IUnknown */
    STDMETHODIMP QueryInterface(REFIID riid, void **ppv) override {
        if (!ppv) return E_POINTER;
        if (riid == IID_IUnknown || riid == IID_IMFMediaEventGenerator || riid == IID_IMFMediaSource ||
            riid == IID_IMFMediaSourceEx)
            *ppv = static_cast<IMFMediaSourceEx *>(this);
        else if (riid == IID_IMFGetService)
            *ppv = static_cast<IMFGetService *>(this);
        else if (riid == __uuidof(IKsControl))
            *ppv = static_cast<IKsControl *>(this);
        else {
            *ppv = nullptr;
            return E_NOINTERFACE;
        }
        AddRef();
        return S_OK;
    }
    STDMETHODIMP_(ULONG) AddRef() override { return (ULONG)InterlockedIncrement(&ref_); }
    STDMETHODIMP_(ULONG) Release() override {
        LONG r = InterlockedDecrement(&ref_);
        if (r == 0) delete this;
        return (ULONG)r;
    }

    /* IMFMediaEventGenerator */
    STDMETHODIMP GetEvent(DWORD flags, IMFMediaEvent **e) override {
        IMFMediaEventQueue *q;
        {
            Lock l(&cs_);
            if (!queue_) return MF_E_SHUTDOWN;
            q = queue_;
            q->AddRef();
        }
        HRESULT hr = q->GetEvent(flags, e);
        q->Release();
        return hr;
    }
    STDMETHODIMP BeginGetEvent(IMFAsyncCallback *cb, IUnknown *st) override {
        Lock l(&cs_);
        return queue_ ? queue_->BeginGetEvent(cb, st) : MF_E_SHUTDOWN;
    }
    STDMETHODIMP EndGetEvent(IMFAsyncResult *r, IMFMediaEvent **e) override {
        Lock l(&cs_);
        return queue_ ? queue_->EndGetEvent(r, e) : MF_E_SHUTDOWN;
    }
    STDMETHODIMP QueueEvent(MediaEventType t, REFGUID ext, HRESULT st, const PROPVARIANT *v) override {
        Lock l(&cs_);
        return queue_ ? queue_->QueueEventParamVar(t, ext, st, v) : MF_E_SHUTDOWN;
    }

    /* IMFMediaSource */
    STDMETHODIMP GetCharacteristics(DWORD *c) override {
        if (!c) return E_POINTER;
        Lock l(&cs_);
        if (!queue_) return MF_E_SHUTDOWN;
        *c = MFMEDIASOURCE_IS_LIVE;
        return S_OK;
    }
    STDMETHODIMP CreatePresentationDescriptor(IMFPresentationDescriptor **pd) override {
        if (!pd) return E_POINTER;
        Lock l(&cs_);
        if (!queue_) return MF_E_SHUTDOWN;
        return pd_->Clone(pd);
    }
    STDMETHODIMP Start(IMFPresentationDescriptor *pd, const GUID *fmt, const PROPVARIANT *) override {
        if (!pd) return E_INVALIDARG;
        if (fmt && *fmt != GUID_NULL) return MF_E_UNSUPPORTED_TIME_FORMAT;
        Lock l(&cs_);
        if (!queue_) return MF_E_SHUTDOWN;
        /* El tipo que haya elegido la app va en el descriptor que nos dan. */
        BOOL selected = FALSE;
        IMFStreamDescriptor *sd = nullptr;
        HRESULT hr = pd->GetStreamDescriptorByIndex(0, &selected, &sd);
        if (SUCCEEDED(hr) && selected) {
            IMFMediaTypeHandler *in = nullptr, *mine = nullptr;
            IMFMediaType *mt = nullptr;
            IMFStreamDescriptor *own = nullptr;
            if (SUCCEEDED(sd->GetMediaTypeHandler(&in)) && SUCCEEDED(in->GetCurrentMediaType(&mt)) &&
                SUCCEEDED(stream_->GetStreamDescriptor(&own)) && SUCCEEDED(own->GetMediaTypeHandler(&mine)))
                mine->SetCurrentMediaType(mt);
            SafeRelease(mine);
            SafeRelease(own);
            SafeRelease(mt);
            SafeRelease(in);
        }
        SafeRelease(sd);
        if (FAILED(hr)) return hr;
        if (!selected) return S_OK;
        IUnknown *su = nullptr;
        stream_->QueryInterface(IID_IUnknown, (void **)&su);
        hr = queue_->QueueEventParamUnk(started_ ? MEUpdatedStream : MENewStream, GUID_NULL, S_OK, su);
        SafeRelease(su);
        started_ = TRUE;
        if (SUCCEEDED(hr)) hr = stream_->Start();
        if (FAILED(hr)) return hr;
        PROPVARIANT v;
        PropVariantInit(&v);
        v.vt = VT_I8;
        v.hVal.QuadPart = MFGetSystemTime();
        return queue_->QueueEventParamVar(MESourceStarted, GUID_NULL, S_OK, &v);
    }
    STDMETHODIMP Stop() override {
        Lock l(&cs_);
        if (!queue_) return MF_E_SHUTDOWN;
        stream_->Stop();
        return queue_->QueueEventParamVar(MESourceStopped, GUID_NULL, S_OK, nullptr);
    }
    STDMETHODIMP Pause() override { return MF_E_INVALID_STATE_TRANSITION; }
    STDMETHODIMP Shutdown() override {
        Lock l(&cs_);
        if (!queue_) return MF_E_SHUTDOWN;
        if (stream_) {
            stream_->Shutdown();
            SafeRelease(stream_);
        }
        queue_->Shutdown();
        SafeRelease(queue_);
        SafeRelease(pd_);
        SafeRelease(attrs_);
        return S_OK;
    }

    /* IMFMediaSourceEx */
    STDMETHODIMP GetSourceAttributes(IMFAttributes **a) override {
        if (!a) return E_POINTER;
        Lock l(&cs_);
        if (!attrs_) return MF_E_SHUTDOWN;
        *a = attrs_;
        attrs_->AddRef();
        return S_OK;
    }
    STDMETHODIMP GetStreamAttributes(DWORD id, IMFAttributes **a) override {
        if (!a) return E_POINTER;
        Lock l(&cs_);
        if (!stream_) return MF_E_SHUTDOWN;
        if (id != 0) return MF_E_INVALIDSTREAMNUMBER;
        return stream_->Attributes(a);
    }
    STDMETHODIMP SetD3DManager(IUnknown *) override { return S_OK; }

    /* IMFGetService */
    STDMETHODIMP GetService(REFGUID, REFIID, LPVOID *) override { return MF_E_UNSUPPORTED_SERVICE; }

    /* IKsControl */
    STDMETHODIMP KsProperty(PKSPROPERTY, ULONG, LPVOID, ULONG, ULONG *) override {
        return HRESULT_FROM_WIN32(ERROR_SET_NOT_FOUND);
    }
    STDMETHODIMP KsMethod(PKSMETHOD, ULONG, LPVOID, ULONG, ULONG *) override {
        return HRESULT_FROM_WIN32(ERROR_SET_NOT_FOUND);
    }
    STDMETHODIMP KsEvent(PKSEVENT, ULONG, LPVOID, ULONG, ULONG *) override {
        return HRESULT_FROM_WIN32(ERROR_SET_NOT_FOUND);
    }

  private:
    ~Source() {
        if (stream_) {
            stream_->Shutdown();
            SafeRelease(stream_);
        }
        SafeRelease(queue_);
        SafeRelease(pd_);
        SafeRelease(attrs_);
        DeleteCriticalSection(&cs_);
        InterlockedDecrement(&g_objects);
    }

    LONG ref_ = 1;
    CRITICAL_SECTION cs_;
    IMFMediaEventQueue *queue_ = nullptr;
    IMFAttributes *attrs_ = nullptr;
    IMFPresentationDescriptor *pd_ = nullptr;
    Stream *stream_ = nullptr;
    BOOL started_ = FALSE;
};

/* ------------------------------------------------------------------------- */
/* Activador: lo que crea el servicio de cámaras con el CLSID                */
/* ------------------------------------------------------------------------- */

class Activator : public IMFActivate {
  public:
    Activator() { InitializeCriticalSection(&cs_); InterlockedIncrement(&g_objects); }
    HRESULT Init() { return MFCreateAttributes(&attrs_, 4); }

    STDMETHODIMP QueryInterface(REFIID riid, void **ppv) override {
        if (!ppv) return E_POINTER;
        if (riid == IID_IUnknown || riid == IID_IMFAttributes || riid == IID_IMFActivate) {
            *ppv = static_cast<IMFActivate *>(this);
            AddRef();
            return S_OK;
        }
        *ppv = nullptr;
        return E_NOINTERFACE;
    }
    STDMETHODIMP_(ULONG) AddRef() override { return (ULONG)InterlockedIncrement(&ref_); }
    STDMETHODIMP_(ULONG) Release() override {
        LONG r = InterlockedDecrement(&ref_);
        if (r == 0) delete this;
        return (ULONG)r;
    }

    /* IMFActivate */
    STDMETHODIMP ActivateObject(REFIID riid, void **ppv) override {
        Lock l(&cs_);
        if (!source_) {
            Source *s = new (std::nothrow) Source();
            if (!s) return E_OUTOFMEMORY;
            HRESULT hr = s->Init();
            if (FAILED(hr)) {
                s->Release();
                return hr;
            }
            source_ = s;
        }
        return source_->QueryInterface(riid, ppv);
    }
    STDMETHODIMP ShutdownObject() override {
        Lock l(&cs_);
        if (source_) {
            source_->Shutdown();
            SafeRelease(source_);
        }
        return S_OK;
    }
    STDMETHODIMP DetachObject() override {
        Lock l(&cs_);
        SafeRelease(source_);
        return S_OK;
    }

    /* IMFAttributes: al almacén de atributos. */
    STDMETHODIMP GetItem(REFGUID k, PROPVARIANT *v) override { return attrs_->GetItem(k, v); }
    STDMETHODIMP GetItemType(REFGUID k, MF_ATTRIBUTE_TYPE *t) override { return attrs_->GetItemType(k, t); }
    STDMETHODIMP CompareItem(REFGUID k, REFPROPVARIANT v, BOOL *r) override { return attrs_->CompareItem(k, v, r); }
    STDMETHODIMP Compare(IMFAttributes *o, MF_ATTRIBUTES_MATCH_TYPE m, BOOL *r) override {
        return attrs_->Compare(o, m, r);
    }
    STDMETHODIMP GetUINT32(REFGUID k, UINT32 *v) override { return attrs_->GetUINT32(k, v); }
    STDMETHODIMP GetUINT64(REFGUID k, UINT64 *v) override { return attrs_->GetUINT64(k, v); }
    STDMETHODIMP GetDouble(REFGUID k, double *v) override { return attrs_->GetDouble(k, v); }
    STDMETHODIMP GetGUID(REFGUID k, GUID *v) override { return attrs_->GetGUID(k, v); }
    STDMETHODIMP GetStringLength(REFGUID k, UINT32 *n) override { return attrs_->GetStringLength(k, n); }
    STDMETHODIMP GetString(REFGUID k, LPWSTR v, UINT32 n, UINT32 *l) override { return attrs_->GetString(k, v, n, l); }
    STDMETHODIMP GetAllocatedString(REFGUID k, LPWSTR *v, UINT32 *l) override {
        return attrs_->GetAllocatedString(k, v, l);
    }
    STDMETHODIMP GetBlobSize(REFGUID k, UINT32 *n) override { return attrs_->GetBlobSize(k, n); }
    STDMETHODIMP GetBlob(REFGUID k, UINT8 *b, UINT32 n, UINT32 *l) override { return attrs_->GetBlob(k, b, n, l); }
    STDMETHODIMP GetAllocatedBlob(REFGUID k, UINT8 **b, UINT32 *n) override {
        return attrs_->GetAllocatedBlob(k, b, n);
    }
    STDMETHODIMP GetUnknown(REFGUID k, REFIID i, LPVOID *p) override { return attrs_->GetUnknown(k, i, p); }
    STDMETHODIMP SetItem(REFGUID k, REFPROPVARIANT v) override { return attrs_->SetItem(k, v); }
    STDMETHODIMP DeleteItem(REFGUID k) override { return attrs_->DeleteItem(k); }
    STDMETHODIMP DeleteAllItems() override { return attrs_->DeleteAllItems(); }
    STDMETHODIMP SetUINT32(REFGUID k, UINT32 v) override { return attrs_->SetUINT32(k, v); }
    STDMETHODIMP SetUINT64(REFGUID k, UINT64 v) override { return attrs_->SetUINT64(k, v); }
    STDMETHODIMP SetDouble(REFGUID k, double v) override { return attrs_->SetDouble(k, v); }
    STDMETHODIMP SetGUID(REFGUID k, REFGUID v) override { return attrs_->SetGUID(k, v); }
    STDMETHODIMP SetString(REFGUID k, LPCWSTR v) override { return attrs_->SetString(k, v); }
    STDMETHODIMP SetBlob(REFGUID k, const UINT8 *b, UINT32 n) override { return attrs_->SetBlob(k, b, n); }
    STDMETHODIMP SetUnknown(REFGUID k, IUnknown *u) override { return attrs_->SetUnknown(k, u); }
    STDMETHODIMP LockStore() override { return attrs_->LockStore(); }
    STDMETHODIMP UnlockStore() override { return attrs_->UnlockStore(); }
    STDMETHODIMP GetCount(UINT32 *n) override { return attrs_->GetCount(n); }
    STDMETHODIMP GetItemByIndex(UINT32 i, GUID *k, PROPVARIANT *v) override { return attrs_->GetItemByIndex(i, k, v); }
    STDMETHODIMP CopyAllItems(IMFAttributes *d) override { return attrs_->CopyAllItems(d); }

  private:
    ~Activator() {
        SafeRelease(source_);
        SafeRelease(attrs_);
        DeleteCriticalSection(&cs_);
        InterlockedDecrement(&g_objects);
    }
    LONG ref_ = 1;
    CRITICAL_SECTION cs_;
    IMFAttributes *attrs_ = nullptr;
    Source *source_ = nullptr;
};

/* ------------------------------------------------------------------------- */
/* Fábrica y exportaciones                                                   */
/* ------------------------------------------------------------------------- */

class Factory : public IClassFactory {
  public:
    STDMETHODIMP QueryInterface(REFIID riid, void **ppv) override {
        if (!ppv) return E_POINTER;
        if (riid == IID_IUnknown || riid == IID_IClassFactory) {
            *ppv = static_cast<IClassFactory *>(this);
            return S_OK;
        }
        *ppv = nullptr;
        return E_NOINTERFACE;
    }
    STDMETHODIMP_(ULONG) AddRef() override { return 2; }
    STDMETHODIMP_(ULONG) Release() override { return 1; }
    STDMETHODIMP CreateInstance(IUnknown *outer, REFIID riid, void **ppv) override {
        if (!ppv) return E_POINTER;
        *ppv = nullptr;
        if (outer) return CLASS_E_NOAGGREGATION;
        Activator *a = new (std::nothrow) Activator();
        if (!a) return E_OUTOFMEMORY;
        HRESULT hr = a->Init();
        if (SUCCEEDED(hr)) hr = a->QueryInterface(riid, ppv);
        a->Release();
        return hr;
    }
    STDMETHODIMP LockServer(BOOL lock) override {
        if (lock)
            InterlockedIncrement(&g_locks);
        else
            InterlockedDecrement(&g_locks);
        return S_OK;
    }
};

static Factory g_factory;

extern "C" HRESULT __stdcall DllGetClassObject(REFCLSID clsid, REFIID riid, void **ppv) {
    if (!ppv) return E_POINTER;
    *ppv = nullptr;
    if (clsid != CLSID_TurboCamMF) return CLASS_E_CLASSNOTAVAILABLE;
    return g_factory.QueryInterface(riid, ppv);
}

extern "C" HRESULT __stdcall DllCanUnloadNow() { return g_objects == 0 && g_locks == 0 ? S_OK : S_FALSE; }

BOOL WINAPI DllMain(HINSTANCE inst, DWORD reason, LPVOID) {
    if (reason == DLL_PROCESS_ATTACH) DisableThreadLibraryCalls(inst);
    return TRUE;
}
