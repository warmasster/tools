/* TurboBLOCK: sustituto de Google Publisher Tag (gpt.js). Las webs que esperan
   a googletag no se quedan colgadas, pero no se pide ningún anuncio. */
(function () {
  'use strict';
  const noop = function () {};
  const self = function () { return this; };
  const slot = {
    addService: self, clearCategoryExclusions: self, clearTargeting: self, defineSizeMapping: self,
    get: () => null, getAdUnitPath: () => '', getAttributeKeys: () => [], getCategoryExclusions: () => [],
    getDomId: () => '', getSlotElementId: () => '', getSlotId: self, getTargeting: () => [], getTargetingKeys: () => [],
    set: self, setCategoryExclusion: self, setClickUrl: self, setCollapseEmptyDiv: self, setTargeting: self, updateTargetingFromMap: self,
    getResponseInformation: () => null, setConfig: self,
  };
  const pubads = {
    addEventListener: self, removeEventListener: self, clear: noop, clearCategoryExclusions: self, clearTagForChildDirectedTreatment: self,
    clearTargeting: self, collapseEmptyDivs: self, defineOutOfPagePassback: () => slot, definePassback: () => slot, disableInitialLoad: noop,
    display: noop, enableAsyncRendering: noop, enableLazyLoad: noop, enableSingleRequest: noop, enableSyncRendering: noop,
    enableVideoAds: noop, get: () => null, getAttributeKeys: () => [], getTargeting: () => [], getTargetingKeys: () => [], getSlots: () => [],
    isInitialLoadDisabled: () => true, refresh: noop, set: self, setCategoryExclusion: self, setCentering: noop, setCookieOptions: self,
    setForceSafeFrame: self, setLocation: self, setPublisherProvidedId: self, setPrivacySettings: self, setRequestNonPersonalizedAds: self,
    setSafeFrameConfig: self, setTagForChildDirectedTreatment: self, setTargeting: self, setVideoContent: self, updateCorrelator: noop,
  };
  const gt = window.googletag = window.googletag || {};
  const cmd = gt.cmd || [];
  gt.apiReady = true; gt.pubadsReady = true;
  gt.pubads = () => pubads; gt.companionAds = () => pubads; gt.content = () => pubads;
  gt.defineSlot = () => slot; gt.defineOutOfPageSlot = () => slot; gt.destroySlots = noop; gt.disablePublisherConsole = noop;
  gt.display = noop; gt.enableServices = noop; gt.getVersion = () => ''; gt.setAdIframeTitle = noop; gt.sizeMapping = () => ({ addSize: self, build: () => [] });
  gt.openConsole = noop; gt.setConfig = noop;
  gt.cmd = { push(fn) { try { fn(); } catch {} return 1; } };
  if (Array.isArray(cmd)) for (const fn of cmd) { try { fn(); } catch {} }
})();
