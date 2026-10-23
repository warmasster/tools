/* TurboBLOCK: sustituto del SDK de anuncios de vídeo de Google (IMA3).
   Los reproductores lo cargan antes de cada vídeo. Esta versión responde que no hay
   anuncios (respuesta VAST vacía), así que el reproductor pasa directamente al vídeo. */
(function () {
  'use strict';
  if (window.google && window.google.ima && window.google.ima.__turboblock) return;
  const noop = function () {};
  class Emitter {
    constructor() { this._l = new Map(); }
    addEventListener(types, fn, _capture, scope) {
      for (const t of [].concat(types)) {
        if (!this._l.has(t)) this._l.set(t, []);
        this._l.get(t).push(scope ? fn.bind(scope) : fn);
      }
    }
    removeEventListener(types, fn) {
      for (const t of [].concat(types)) {
        const l = this._l.get(t);
        if (l) this._l.set(t, l.filter((x) => x !== fn));
      }
    }
    _emit(type, ev) {
      for (const fn of this._l.get(type) || []) {
        try { fn(ev); } catch (e) { setTimeout(() => { throw e; }); }
      }
    }
  }
  const AdError = function (msg, code) { this.message = msg; this.code = code; };
  AdError.prototype.getErrorCode = function () { return this.code; };
  AdError.prototype.getVastErrorCode = function () { return 303; };
  AdError.prototype.getMessage = function () { return this.message; };
  AdError.prototype.getType = function () { return 'adLoadError'; };
  AdError.prototype.getInnerError = function () { return null; };
  AdError.prototype.toString = function () { return 'AdError ' + this.code + ': ' + this.message; };
  AdError.ErrorCode = { VAST_EMPTY_RESPONSE: 1009, UNKNOWN_ERROR: 900, VAST_NO_ADS_AFTER_WRAPPER: 303 };
  AdError.Type = { AD_LOAD: 'adLoadError', AD_PLAY: 'adPlayError' };
  const AdErrorEvent = function (err, ctx) { this.type = 'adError'; this._e = err; this._c = ctx; };
  AdErrorEvent.prototype.getError = function () { return this._e; };
  AdErrorEvent.prototype.getUserRequestContext = function () { return this._c || {}; };
  AdErrorEvent.Type = { AD_ERROR: 'adError' };

  const AdEvent = function (type) { this.type = type; };
  AdEvent.prototype.getAd = function () { return null; };
  AdEvent.prototype.getAdData = function () { return {}; };
  AdEvent.Type = {
    AD_BREAK_READY: 'adBreakReady', AD_BUFFERING: 'adBuffering', AD_CAN_PLAY: 'adCanPlay', AD_METADATA: 'adMetadata',
    ALL_ADS_COMPLETED: 'allAdsCompleted', CLICK: 'click', COMPLETE: 'complete', CONTENT_PAUSE_REQUESTED: 'contentPauseRequested',
    CONTENT_RESUME_REQUESTED: 'contentResumeRequested', DURATION_CHANGE: 'durationChange', FIRST_QUARTILE: 'firstQuartile',
    IMPRESSION: 'impression', INTERACTION: 'interaction', LINEAR_CHANGED: 'linearChanged', LOADED: 'loaded', LOG: 'log',
    MIDPOINT: 'midpoint', PAUSED: 'pause', RESUMED: 'resume', SKIPPABLE_STATE_CHANGED: 'skippableStateChanged',
    SKIPPED: 'skip', STARTED: 'start', THIRD_QUARTILE: 'thirdQuartile', USER_CLOSE: 'userClose',
    VIDEO_CLICKED: 'videoClicked', VIDEO_ICON_CLICKED: 'videoIconClicked', VOLUME_CHANGED: 'volumeChange', VOLUME_MUTED: 'mute',
  };

  class AdsManager extends Emitter {
    constructor() { super(); this._vol = 1; }
    collapse() {} configureAdsRenderingSettings() {} destroy() {} expand() {} focus() {} pause() {} resume() {} skip() {} stop() {}
    discardAdBreak() {} clicked() {} updateAdsRenderingSettings() {} resize() {} init() {}
    getAdSkippableState() { return false; } getCuePoints() { return []; } getCurrentAd() { return null; }
    getRemainingTime() { return 0; } getVolume() { return this._vol; } setVolume(v) { this._vol = v; }
    isCustomClickTrackingUsed() { return false; } isCustomPlaybackUsed() { return false; }
    start() {
      setTimeout(() => {
        this._emit(AdEvent.Type.CONTENT_RESUME_REQUESTED, new AdEvent(AdEvent.Type.CONTENT_RESUME_REQUESTED));
        this._emit(AdEvent.Type.ALL_ADS_COMPLETED, new AdEvent(AdEvent.Type.ALL_ADS_COMPLETED));
      }, 0);
    }
  }
  const AdsManagerLoadedEvent = function (ctx) { this.type = 'adsManagerLoaded'; this._c = ctx; };
  AdsManagerLoadedEvent.prototype.getAdsManager = function () { return new AdsManager(); };
  AdsManagerLoadedEvent.prototype.getUserRequestContext = function () { return this._c || {}; };
  AdsManagerLoadedEvent.Type = { ADS_MANAGER_LOADED: 'adsManagerLoaded' };

  class ImaSdkSettings {
    getCompanionBackfill() { return 'always'; } getDisableCustomPlaybackForIOS10Plus() { return false; }
    getFeatureFlags() { return {}; } getLocale() { return 'es'; } getNumRedirects() { return 4; } getPlayerType() { return ''; }
    getPlayerVersion() { return ''; } getPpid() { return ''; } isCookiesEnabled() { return false; } isVpaidAllowed() { return false; }
    setAutoPlayAdBreaks() {} setCompanionBackfill() {} setCookiesEnabled() {} setDisableCustomPlaybackForIOS10Plus() {}
    setFeatureFlags() {} setLocale() {} setNumRedirects() {} setPlayerType() {} setPlayerVersion() {} setPpid() {}
    setSessionId() {} setVpaidAllowed() {} setVpaidMode() {}
  }
  ImaSdkSettings.CompanionBackfillMode = { ALWAYS: 'always', ON_MASTER_AD: 'on_master_ad' };
  ImaSdkSettings.VpaidMode = { DISABLED: 0, ENABLED: 1, INSECURE: 2 };

  class AdsLoader extends Emitter {
    constructor() { super(); this._settings = new ImaSdkSettings(); }
    contentComplete() {} destroy() {}
    getVersion() { return '3.0.0'; }
    getSettings() { return this._settings; }
    requestAds(_req, ctx) {
      setTimeout(() => {
        const err = new AdError('No hay anuncios', AdError.ErrorCode.VAST_EMPTY_RESPONSE);
        this._emit(AdErrorEvent.Type.AD_ERROR, new AdErrorEvent(err, ctx));
      }, 0);
    }
  }
  function AdDisplayContainer() {}
  AdDisplayContainer.prototype.initialize = noop;
  AdDisplayContainer.prototype.destroy = noop;
  function AdsRequest() {}
  AdsRequest.prototype.setAdWillAutoPlay = noop;
  AdsRequest.prototype.setAdWillPlayMuted = noop;
  AdsRequest.prototype.setContinuousPlayback = noop;
  function AdsRenderingSettings() {}
  function CompanionAdSelectionSettings() {}
  CompanionAdSelectionSettings.CreativeType = { ALL: 'All', FLASH: 'Flash', IMAGE: 'Image' };
  CompanionAdSelectionSettings.ResourceType = { ALL: 'All', HTML: 'Html', IFRAME: 'IFrame', STATIC: 'Static' };
  CompanionAdSelectionSettings.SizeCriteria = { IGNORE: 'IgnoreSize', SELECT_EXACT_MATCH: 'SelectExactMatch', SELECT_NEAR_MATCH: 'SelectNearMatch' };
  const streamManager = function () {
    return { addEventListener: noop, removeEventListener: noop, requestStream: noop, reset: noop, setClickElement: noop,
      streamTimeForContentTime: (t) => t, contentTimeForStreamTime: (t) => t, previousCuePointForStreamTime: () => null };
  };

  window.google = window.google || {};
  window.google.ima = {
    __turboblock: true,
    AdCuePoints: function () {}, AdDisplayContainer, AdError, AdErrorEvent, AdEvent, AdsLoader, AdsManager,
    AdsManagerLoadedEvent, AdsRenderingSettings, AdsRequest, CompanionAdSelectionSettings, ImaSdkSettings,
    OmidAccessMode: { DOMAIN: 'domain', FULL: 'full', LIMITED: 'limited' },
    OmidVerificationVendor: {}, UiElements: { AD_ATTRIBUTION: 'adAttribution', COUNTDOWN: 'countdown' },
    UniversalAdIdInfo: function () {}, ViewMode: { FULLSCREEN: 'fullscreen', NORMAL: 'normal' },
    VERSION: '3.0.0', settings: new ImaSdkSettings(),
    dai: { api: { StreamManager: streamManager, VODStreamRequest: function () {}, LiveStreamRequest: function () {}, StreamEvent: { Type: {} } } },
  };
})();
