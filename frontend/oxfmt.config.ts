import { defineConfig } from "oxfmt"

export default defineConfig({
  semi: false,
  sortImports: true,
  sortTailwindcss: {
    stylesheet: "./app/app.css",
  },
})
