import { type RouteConfig, index, layout, route } from "@react-router/dev/routes"

export default [
  route("login", "routes/login.tsx"),
  route("logout", "routes/logout.tsx"),
  layout("layouts/authenticated.tsx", [
    index("routes/files-index.tsx"),
    route("folders/:folderId", "routes/files-folders.tsx"),
    route("trash", "routes/trash.tsx"),
  ]),
] satisfies RouteConfig
