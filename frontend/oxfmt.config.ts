import { defineConfig } from "oxfmt"

export default defineConfig({
  ignorePatterns: ["app/openapi/schema.d.ts"],
  semi: false,
  sortImports: true,
  sortTailwindcss: {
    stylesheet: "./app/app.css",
  },
})
