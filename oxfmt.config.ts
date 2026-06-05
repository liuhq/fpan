import { defineConfig } from "oxfmt"

export default defineConfig({
  semi: false,
  sortImports: true,
  sortTailwindcss: {
    stylesheet: "./web/src/index.css",
    functions: ["clsx"],
  },
})
