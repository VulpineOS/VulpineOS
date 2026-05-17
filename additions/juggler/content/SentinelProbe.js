/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

"use strict";

export const SENTINEL_PROBE_BINDING_NAME = "__jugglerSentinelReport";
export const SENTINEL_PROBE_BINDING_SCRIPT = "void 0;";

export const SENTINEL_PROBE_INIT_SCRIPT = `(() => {
  const globalObject = globalThis;
  const bindingName = "__jugglerSentinelReport";
  const installFlag = "__jugglerSentinelInstalled";
  if (globalObject[installFlag])
    return;
  const report = typeof globalObject[bindingName] === "function" ? globalObject[bindingName] : null;
  if (!report)
    return;
  Object.defineProperty(globalObject, installFlag, {
    value: true,
    configurable: false,
    enumerable: false,
    writable: false,
  });
  try {
    delete globalObject[bindingName];
  } catch (e) {
  }

  const counts = new Map();

  function nextCount(key) {
    const count = (counts.get(key) || 0) + 1;
    counts.set(key, count);
    return count;
  }

  function shouldReport(count) {
    return count <= 3 || count === 10 || count % 25 === 0;
  }

  function extractScriptURL(stack) {
    if (!stack)
      return "";
    const lines = String(stack).split("\\n");
    for (let i = 2; i < lines.length; i++) {
      const line = lines[i].trim();
      if (!line)
        continue;
      let candidate = line;
      const atIndex = candidate.indexOf("@");
      if (atIndex !== -1)
        candidate = candidate.slice(atIndex + 1);
      candidate = candidate.replace(/:\\d+:\\d+$/, "");
      if (!candidate || candidate === "debugger eval code")
        continue;
      return candidate;
    }
    return "";
  }

  function emitProbe(probeType, api, detail) {
    const normalizedDetail = detail ? String(detail).slice(0, 160) : "";
    const key = probeType + ":" + api + ":" + normalizedDetail;
    const count = nextCount(key);
    if (!shouldReport(count))
      return;
    let stack = "";
    let scriptURL = "";
    try {
      throw new Error("sentinel-probe");
    } catch (e) {
      stack = String(e && e.stack || "");
      scriptURL = extractScriptURL(stack);
    }
    try {
      report(JSON.stringify({
        probeType,
        api,
        detail: normalizedDetail,
        count,
        url: globalObject.location ? String(globalObject.location.href || "") : "",
        scriptURL,
        timestamp: Date.now(),
      }));
    } catch (e) {
    }
  }

  function wrapMethod(target, name, probeType, detailBuilder) {
    if (!target)
      return;
    let original;
    try {
      original = target[name];
    } catch (e) {
      return;
    }
    if (typeof original !== "function" || original.__vulpineSentinelWrapped)
      return;
    function wrapped() {
      let detail = "";
      try {
        if (detailBuilder)
          detail = detailBuilder.apply(this, arguments) || "";
      } catch (e) {
        detail = "";
      }
      emitProbe(probeType, name, detail);
      return original.apply(this, arguments);
    }
    Object.defineProperty(wrapped, "__vulpineSentinelWrapped", {
      value: true,
      configurable: false,
      enumerable: false,
      writable: false,
    });
    try {
      target[name] = wrapped;
    } catch (e) {
    }
  }

  function wrapGetter(target, name, probeType) {
    if (!target)
      return;
    const descriptor = Object.getOwnPropertyDescriptor(target, name);
    if (!descriptor || typeof descriptor.get !== "function" || descriptor.get.__vulpineSentinelWrapped)
      return;
    const originalGet = descriptor.get;
    function wrappedGet() {
      emitProbe(probeType, name, "");
      return originalGet.call(this);
    }
    Object.defineProperty(wrappedGet, "__vulpineSentinelWrapped", {
      value: true,
      configurable: false,
      enumerable: false,
      writable: false,
    });
    try {
      Object.defineProperty(target, name, {
        get: wrappedGet,
        set: descriptor.set,
        enumerable: descriptor.enumerable,
        configurable: true,
      });
    } catch (e) {
    }
  }

  wrapMethod(globalObject.HTMLCanvasElement && globalObject.HTMLCanvasElement.prototype, "toDataURL", "canvas_probe", function() {
    return (this && typeof this.width === "number" && typeof this.height === "number") ? this.width + "x" + this.height : "";
  });
  wrapMethod(globalObject.HTMLCanvasElement && globalObject.HTMLCanvasElement.prototype, "toBlob", "canvas_probe", function() {
    return (this && typeof this.width === "number" && typeof this.height === "number") ? this.width + "x" + this.height : "";
  });
  wrapMethod(globalObject.HTMLCanvasElement && globalObject.HTMLCanvasElement.prototype, "getContext", "canvas_probe", function(type) {
    return type || "";
  });
  wrapMethod(globalObject.OffscreenCanvas && globalObject.OffscreenCanvas.prototype, "convertToBlob", "canvas_probe", function() {
    return (this && typeof this.width === "number" && typeof this.height === "number") ? this.width + "x" + this.height : "";
  });
  wrapMethod(globalObject.OffscreenCanvas && globalObject.OffscreenCanvas.prototype, "getContext", "canvas_probe", function(type) {
    return type || "";
  });
  wrapMethod(globalObject.CanvasRenderingContext2D && globalObject.CanvasRenderingContext2D.prototype, "getImageData", "canvas_probe", function(x, y, width, height) {
    return [x, y, width, height].join(",");
  });

  wrapMethod(globalObject.WebGLRenderingContext && globalObject.WebGLRenderingContext.prototype, "getParameter", "webgl_probe", function(parameter) {
    return parameter === undefined ? "" : String(parameter);
  });
  wrapMethod(globalObject.WebGLRenderingContext && globalObject.WebGLRenderingContext.prototype, "getExtension", "webgl_probe", function(name) {
    return name || "";
  });
  wrapMethod(globalObject.WebGLRenderingContext && globalObject.WebGLRenderingContext.prototype, "getSupportedExtensions", "webgl_probe", function() {
    return "all";
  });
  wrapMethod(globalObject.WebGLRenderingContext && globalObject.WebGLRenderingContext.prototype, "readPixels", "webgl_probe", function(x, y, width, height) {
    return [x, y, width, height].join(",");
  });
  wrapMethod(globalObject.WebGL2RenderingContext && globalObject.WebGL2RenderingContext.prototype, "getParameter", "webgl_probe", function(parameter) {
    return parameter === undefined ? "" : String(parameter);
  });
  wrapMethod(globalObject.WebGL2RenderingContext && globalObject.WebGL2RenderingContext.prototype, "getExtension", "webgl_probe", function(name) {
    return name || "";
  });
  wrapMethod(globalObject.WebGL2RenderingContext && globalObject.WebGL2RenderingContext.prototype, "getSupportedExtensions", "webgl_probe", function() {
    return "all";
  });
  wrapMethod(globalObject.WebGL2RenderingContext && globalObject.WebGL2RenderingContext.prototype, "readPixels", "webgl_probe", function(x, y, width, height) {
    return [x, y, width, height].join(",");
  });

  wrapMethod(globalObject.AudioContext && globalObject.AudioContext.prototype, "createAnalyser", "audio_probe", function() {
    return "createAnalyser";
  });
  wrapMethod(globalObject.AudioContext && globalObject.AudioContext.prototype, "createDynamicsCompressor", "audio_probe", function() {
    return "createDynamicsCompressor";
  });
  wrapMethod(globalObject.AudioContext && globalObject.AudioContext.prototype, "createOscillator", "audio_probe", function() {
    return "createOscillator";
  });
  wrapMethod(globalObject.OfflineAudioContext && globalObject.OfflineAudioContext.prototype, "startRendering", "audio_probe", function() {
    return "startRendering";
  });
  wrapMethod(globalObject.AnalyserNode && globalObject.AnalyserNode.prototype, "getFloatFrequencyData", "audio_probe", function() {
    return "getFloatFrequencyData";
  });
  wrapMethod(globalObject.AnalyserNode && globalObject.AnalyserNode.prototype, "getByteFrequencyData", "audio_probe", function() {
    return "getByteFrequencyData";
  });

  wrapGetter(globalObject.Navigator && globalObject.Navigator.prototype, "userAgent", "navigator_probe");
  wrapGetter(globalObject.Navigator && globalObject.Navigator.prototype, "platform", "navigator_probe");
  wrapGetter(globalObject.Navigator && globalObject.Navigator.prototype, "hardwareConcurrency", "navigator_probe");
  wrapGetter(globalObject.Navigator && globalObject.Navigator.prototype, "deviceMemory", "navigator_probe");
  wrapGetter(globalObject.Navigator && globalObject.Navigator.prototype, "languages", "navigator_probe");
  wrapGetter(globalObject.Navigator && globalObject.Navigator.prototype, "language", "navigator_probe");
  wrapGetter(globalObject.Navigator && globalObject.Navigator.prototype, "maxTouchPoints", "navigator_probe");
  wrapGetter(globalObject.Navigator && globalObject.Navigator.prototype, "plugins", "navigator_probe");
  wrapGetter(globalObject.Navigator && globalObject.Navigator.prototype, "mimeTypes", "navigator_probe");

  wrapGetter(globalObject.Screen && globalObject.Screen.prototype, "width", "screen_probe");
  wrapGetter(globalObject.Screen && globalObject.Screen.prototype, "height", "screen_probe");
  wrapGetter(globalObject.Screen && globalObject.Screen.prototype, "availWidth", "screen_probe");
  wrapGetter(globalObject.Screen && globalObject.Screen.prototype, "availHeight", "screen_probe");
  wrapGetter(globalObject.Screen && globalObject.Screen.prototype, "colorDepth", "screen_probe");
  wrapGetter(globalObject.Screen && globalObject.Screen.prototype, "pixelDepth", "screen_probe");

  wrapMethod(globalObject.MediaDevices && globalObject.MediaDevices.prototype, "enumerateDevices", "media_probe", function() {
    return "enumerateDevices";
  });
  wrapMethod(globalObject.Permissions && globalObject.Permissions.prototype, "query", "permissions_probe", function(descriptor) {
    return descriptor && descriptor.name ? descriptor.name : "";
  });
  wrapMethod(globalObject.Intl && globalObject.Intl.DateTimeFormat && globalObject.Intl.DateTimeFormat.prototype, "resolvedOptions", "intl_probe", function() {
    return "resolvedOptions";
  });
})();`;
