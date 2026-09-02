import { reactRouter } from "@react-router/dev/vite"
import tailwindcss from "@tailwindcss/vite"
import { defineConfig, loadEnv } from "vite"

const defaultApiProxyTarget = "http://127.0.0.1:6313"

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "FPAN_")

  return {
    plugins: [tailwindcss(), reactRouter()],
    resolve: {
      tsconfigPaths: true,
    },
    server: {
      proxy: {
        "/api": {
          target: env.FPAN_API_PROXY_TARGET || defaultApiProxyTarget,
          changeOrigin: true,
        },
      },
    },
  }
})
