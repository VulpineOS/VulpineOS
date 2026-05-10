/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

"use strict";

// Public builds keep the protocol surface but do not ship a concrete provider.
// Private builds can replace this module with an implementation.

export class TrustWarmService {
  constructor() {
    this._stateChangedCallback = null;
  }

  setStateChangedCallback(callback) {
    this._stateChangedCallback = callback;
  }

  async start() {
    throw new Error("Trust warming is unavailable in this build.");
  }

  async stop() {
    this._emitStateChanged();
  }

  getStatus() {
    return {
      state: "unavailable",
      sitesWarmed: 0,
    };
  }

  async notifyIdle() {}

  async notifyBusy() {}

  dispose() {
    this.stop();
  }

  _emitStateChanged() {
    if (this._stateChangedCallback) {
      this._stateChangedCallback({
        state: "unavailable",
      });
    }
  }
}
