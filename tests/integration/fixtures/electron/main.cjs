"use strict";
const path = require("node:path");
const { app, BrowserWindow } = require("electron");
const { registerLicenseHubIpc } = require("./licensehub-main.cjs");
let disposeLicenseHub;
app.whenReady().then(() => {
  disposeLicenseHub = registerLicenseHubIpc();
  new BrowserWindow({ webPreferences: {
    contextIsolation: true,
    nodeIntegration: false,
    preload: path.join(__dirname, "licensehub-preload.cjs")
  }}).loadFile("index.html");
});
app.once("before-quit", () => disposeLicenseHub?.());
