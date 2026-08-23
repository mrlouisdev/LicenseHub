import { license } from "./licensehub-renderer.js";
license.status().then(status => { document.body.textContent = status.state; });
