import solid from "eslint-plugin-solid/configs/typescript"
import { defineConfig } from "oxlint"

export default defineConfig({
  ignorePatterns: ["dist/**", "node_modules/**"],
  plugins: ["import"],
  jsPlugins: ["eslint-plugin-solid"],
  categories: {
    correctness: "error",
    suspicious: "warn",
    perf: "warn",
  },
  rules: {
    "import/no-cycle": ["error", { maxDepth: 3 }],
    ...solid.rules,
  },
  env: {
    builtin: true,
  },
})
