// The frontend container sets the base path in the <base> tag of index.html (see frontend/nginx).
const baseHref = document.querySelector("base")?.getAttribute("href") ?? "/";

/** URL path the app is served under, such as "/snorlx". Empty at the host root. */
export const BASE_PATH = baseHref.replace(/\/+$/, "");

/** Prefix for backend requests. VITE_API_URL points a dev build at another origin and wins. */
export const API_URL = import.meta.env.VITE_API_URL || BASE_PATH;
