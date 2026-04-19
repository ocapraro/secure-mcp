import http from "node:http";
import { defineConfig } from "vite";

const apiTarget = process.env.VITE_API_PROXY_TARGET ?? "http://127.0.0.1:8080";
/** Many concurrent long-lived streams to the API target (default Agent can be conservative). */
const proxyAgent = new http.Agent({ keepAlive: true, maxSockets: 64 });

const apiProxy = {
  target: apiTarget,
  changeOrigin: true,
  agent: proxyAgent,
} as const;

export default defineConfig({
  server: {
    proxy: {
      "/api": apiProxy,
    },
  },
  preview: {
    proxy: {
      "/api": apiProxy,
    },
  },
});
