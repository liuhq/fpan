小型 Linux 文件云盘 —— 完整开发 PLAN

## 项目定位与最终决策清单

| 维度     | 决策                                                                |
| -------- | ------------------------------------------------------------------- |
| 类型     | 单用户、自托管、小型文件云盘                                        |
| 运行环境 | Deno + TypeScript                                                   |
| 前端     | Vite + SolidJS                                                      |
| 后端     | Hono（REST + RPC 类型导出）                                         |
| 数据存储 | SQLite 存 metadata；本地磁盘存二进制                                |
| ID 生成  | nanoid（21 位）                                                     |
| 校验     | zod                                                                 |
| 实体模型 | entries 统一抽象（folder / file，邻接表树）                         |
| 根目录   | 哨兵 ID ROOT_ID = "1"×21（启动插入哨兵行）                          |
| 鉴权     | 单用户，启动时环境变量配置凭据；JWT 维持会话                        |
| 删除     | 单列 deleted_at 软删除（递归打标记，保留树结构）                    |
| 重名冲突 | 撞活跃项→拒绝；撞软删项→统一二次确认后 purge（文件/文件夹一视同仁） |

## 技术架构

```
┌──────────────────────────────────────────────┐
│           Browser (SolidJS + Vite)            │
│      Hono RPC Client（类型安全调用）           │
└────────────────────┬─────────────────────────┘
                   │ HTTP REST (JSON + 二进制流)
┌────────────────────▼─────────────────────────┐
│               Hono Backend (Deno)             │
│  Auth MW │ Entries/Folders/Files Routes       │
│  Zod Validation │ onError 统一错误处理         │
└──────┬──────────────────────────┬─────────────┘
     │                          │
┌──────▼────────┐        ┌─────────▼──────────┐
│ SQLite        │        │ 本地文件系统        │
│ entries 表    │        │ ./storage/<storage_id>
└───────────────┘        └────────────────────┘
```

## 目录结构

```
file-cloud/
├── deno.json                 # tasks / imports
├── deno.lock
├── .env                      # AUTH_USERNAME / AUTH_PASSWORD_HASH / JWT_SECRET / PORT
├── storage/                  # 物理文件（不进 git）
├── data/app.db               # SQLite
├── scripts/
│   └── hash.ts               # deno task hash <password> 生成密码哈希
├── server/
│   ├── main.ts               # 入口：启动 + 静态托管前端
│   ├── app.ts                # Hono 组装 + onError + 导出 AppType
│   ├── config.ts             # 环境变量加载
│   ├── constants.ts          # ROOT_ID 等常量
│   ├── db/
│   │   ├── client.ts         # SQLite 连接 + PRAGMA + 初始化
│   │   └── schema.sql        # 建表 + 哨兵初始化
│   ├── schemas/
│   │   └── entry.ts          # zod schemas
│   ├── middleware/
│   │   └── auth.ts           # JWT 鉴权中间件
│   ├── routes/
│   │   ├── auth.ts
│   │   ├── entries.ts        # 列表/详情/路径/更新/删除/还原
│   │   ├── folders.ts        # 新建文件夹
│   │   └── files.ts          # 上传/下载
│   └── services/
│       ├── auth.service.ts
│       └── entry.service.ts  # 核心业务（树/软删/冲突/purge）
└── web/                      # SolidJS + Vite
  ├── vite.config.ts
  └── src/
      ├── api.ts            # hono/client RPC
      ├── components/
      │   ├── FileList.tsx
      │   ├── Breadcrumb.tsx
      │   ├── Uploader.tsx
      │   └── TrashView.tsx
      └── App.tsx
```

## 数据库设计（最终版）

```sql
-- server/db/schema.sql

CREATE TABLE IF NOT EXISTS entries (
id          TEXT PRIMARY KEY,            -- nanoid(21)
parent_id   TEXT NOT NULL,               -- 根目录指向 ROOT_ID，无 NULL
type        TEXT NOT NULL CHECK(type IN ('file','folder')),
name        TEXT NOT NULL,
mime_type   TEXT,                         -- file 专用
size        INTEGER,                      -- file 专用，字节
storage_id  TEXT,                         -- 磁盘物理文件名（独立 nanoid）
sha256      TEXT,                         -- 可选：去重/完整性
created_at  INTEGER NOT NULL,
updated_at  INTEGER NOT NULL,
deleted_at  INTEGER,                      -- NULL=正常；非NULL=软删（同批次共用时间戳）
FOREIGN KEY (parent_id) REFERENCES entries(id)
);

-- 仅未删除项参与重名校验（核心约束）
CREATE UNIQUE INDEX IF NOT EXISTS idx_entries_name
ON entries(parent_id, name) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_entries_parent  ON entries(parent_id);
CREATE INDEX IF NOT EXISTS idx_entries_deleted ON entries(deleted_at);

-- 哨兵根节点（parent 指向自身，满足外键）
INSERT OR IGNORE INTO entries (id, parent_id, type, name, created_at, updated_at)
VALUES ('111111111111111111111', '111111111111111111111', 'folder', 'root', 0, 0);
```

```typescript
// server/db/client.ts
import { Database } from "jsr:@db/sqlite"
export const db = new Database("./data/app.db")
db.exec("PRAGMA foreign_keys = ON;") // 外键必须开启
const schema = await Deno.readTextFile("./server/db/schema.sql")
db.exec(schema)

// server/constants.ts
export const ROOT_ID = "1".repeat(21)
export const STORAGE_DIR = "./storage"
export const MAX_FILE_SIZE = 100 * 1024 * 1024 // 100MB
```

## REST API 总表

| Method | 路径                               | 说明                         | 确认参数     |
| ------ | ---------------------------------- | ---------------------------- | ------------ |
| POST   | /api/auth/login                    | 登录返回 JWT                 | —            |
| GET    | /api/auth/me                       | 校验 token                   | —            |
| GET    | /api/entries?parentId=             | 列出目录子项（默认 ROOT_ID） | —            |
| GET    | /api/entries/:id                   | 单项详情                     | —            |
| GET    | /api/entries/:id/path              | 面包屑路径                   | —            |
| POST   | /api/folders                       | 新建文件夹                   | confirmPurge |
| POST   | /api/files?parentId=&confirmPurge= | 上传文件（multipart）        | confirmPurge |
| GET    | /api/files/:id/download            | 下载二进制（流式）           | —            |
| PATCH  | /api/entries/:id                   | 重命名 / 移动                | —            |
| DELETE | /api/entries/:id                   | 软删除（递归）               | —            |
| GET    | /api/trash                         | 列出回收站顶层项             | —            |
| POST   | /api/entries/:id/restore           | 还原（递归）                 | confirmPurge |
| DELETE | /api/trash/:id                     | 彻底删除单项（purge）        | —            |
| DELETE | /api/trash                         | 清空回收站                   | —            |

响应约定

```jsonc
{ "data": {...} } // 成功
{ "data": {...}, "warning": { "code": "TRASH_PURGED", "purgedId": "..." } } // 带提示
{ "error": { "code": "...", "message": "...", "conflict": {...} } } // 错误
```

状态码：200 / 201 / 400 / 401 / 404 / 409 / 413 / 500。

## 身份验证（单用户）

```bash
# .env

AUTH_USERNAME=admin
AUTH_PASSWORD_HASH=$2a$10$... # deno task hash <password> 生成
JWT_SECRET=<long-random>
PORT=8000
```

- 启动时从环境变量加载，只存哈希。
- POST /auth/login 比对 username + bcrypt 哈希 → 签发 JWT（24h）。
- 中间件校验 Authorization: Bearer <token>，仅返回布尔（无 owner_id）。

```typescript
// 鉴权中间件核心
export const authMiddleware = createMiddleware(async (c, next) => {
  const token = c.req.header("Authorization")?.replace("Bearer ", "")
  if (!token || !(await verifyToken(token)))
    return c.json({ error: { code: "UNAUTHORIZED", message: "未认证" } }, 401)
  await next()
})
```

## 核心业务逻辑清单（entry.service.ts）

| 函数                                       | 职责                                                 |
| ------------------------------------------ | ---------------------------------------------------- |
| list(parentId)                             | 列未删除子项，folder 优先 + 名称排序                 |
| getEntry(id) / getPath(id)                 | 详情 / 面包屑                                        |
| createFolder(parentId, name, confirmPurge) | 新建文件夹（走冲突解析）                             |
| saveFile(parentId, file, confirmPurge)     | 流式上传 + 写 metadata（走冲突解析）                 |
| updateEntry(id, {name?, parentId?})        | 重命名/移动，防成环 isDescendantOrSelf               |
| softDelete(id)                             | 递归给子树打 deleted_at（事务）                      |
| restore(id, confirmPurge)                  | 递归清 deleted_at（走冲突解析）                      |
| listTrash()                                | 回收站顶层项（parent 未删的已删项）                  |
| purge(id)                                  | 彻底删 DB 行 + 磁盘文件                              |
| emptyTrash()                               | 清空回收站全部软删项                                 |
| resolveNameConflict(...)                   | 统一冲突解析（活跃拒绝 / 软删需确认 / 已确认 purge） |
| collectSubtreeIds(id)                      | 收集子树所有 id                                      |
| collectStorageIds(id)                      | 收集子树所有 file 的 storage_id                      |
| assertValidParent(parentId)                | 校验目标是合法文件夹                                 |

关键不变量

1. 活跃项绝不被自动删除 —— 撞活跃同名一律 409 NAME_CONFLICT。
2. 任何 purge 软删项都需 confirmPurge=true（文件/文件夹统一），未确认返回 409 TRASH_NAME_CONFLICT + conflict（含 childCount）。
3. purge + 写入同事务 —— 避免删了旧的没写成新的。
4. 软删保留树结构 —— 同批次共用 deleted_at，整棵子树一起删/还原。
5. 物理删除仅在 purge 时发生 —— 软删不碰磁盘。

## 安全要点

| 风险     | 对策                                                          |
| -------- | ------------------------------------------------------------- |
| 路径穿越 | 物理文件名用独立 nanoid（storage_id），绝不用用户文件名拼路径 |
| 非法名称 | zod refine 过滤 / \ \0、.、..                                 |
| 明文密码 | bcrypt 哈希，启动时只加载哈希                                 |
| 大文件   | OOM 上传/下载全程流式（file.stream().pipeTo / f.readable）    |
| 超大文件 | MAX_FILE_SIZE + 413                                           |
| 误删数据 | 软删除 + 回收站；purge 需二次确认 + childCount 提示           |
| 移动成环 | isDescendantOrSelf 检查                                       |
| JWT      | 泄露 短过期 + HTTPS 部署                                      |
| 外键失效 | 连接时 PRAGMA foreign_keys = ON + 哨兵行                      |

## 前端要点（SolidJS）

- api.ts：hc<AppType> RPC client，自动带 Authorization。
- 目录浏览：FileList（folder 优先）+ Breadcrumb（调 /entries/:id/path）。
- 上传：FormData + fetch；遇 409 TRASH_NAME_CONFLICT → 弹确认框（文件夹显示 childCount）→ 带 confirmPurge=true 重发。
- 下载：<a href> 或 fetch blob。
- 回收站视图：列 /trash，提供「还原 / 彻底删除 / 清空」。
- 上传/新建/还原成功若含 warning → toast 提示已覆盖回收站同名项。

## deno.json 配置

```jsonc
{
  "tasks": {
    "dev": "deno run -A --watch server/main.ts",
    "web": "cd web && vite dev",
    "build": "cd web && vite build",
    "hash": "deno run scripts/hash.ts",
  },
  "imports": {
    "hono": "npm:hono@^4",
    "@hono/zod-validator": "npm:@hono/zod-validator",
    "zod": "npm:zod@^3",
    "nanoid": "npm:nanoid",
    "@db/sqlite": "jsr:@db/sqlite@^0.12",
    "@felix/bcrypt": "jsr:@felix/bcrypt",
    "@zaubrik/djwt": "jsr:@zaubrik/djwt",
  },
}
```

## 开发里程碑（建议实施顺序）

| 阶段    | 任务                                                   | 产出           |
| ------- | ------------------------------------------------------ | -------------- |
| M1 基建 | deno.json、config、db/client、schema + 哨兵、constants | 可启动、表就绪 |
| M2 鉴权 | auth.service、auth 路由、auth 中间件、hash 脚本        | 登录拿 token   |
| M3 读   | list / getEntry / getPath 路由                         | 能浏览目录树   |
| M4 写   | createFolder / saveFile（流式）+ download              | 能增删改文件   |
| M5 软删 | softDelete / restore / listTrash / purge / emptyTrash  | 回收站完整     |
| M6 冲突 | resolveNameConflict 接入三入口 + onError 透传 conflict | 二次确认机制   |
| M7 前端 | SolidJS：列表/面包屑/上传/回收站/确认弹窗              | 可用 UI        |
| M8 收尾 | 错误处理、静态托管、README、部署脚本                   | 可部署         |

## 后续可选增强（不在 MVP 内）

- 文件夹打包 zip 流式下载
- 基于 sha256 的秒传 / 去重（storage_id 复用）
- 回收站自动过期清理（定时任务删 deleted_at 超 N 天的项）
- 分片 / 断点续传
- 分享链接（带过期时间的临时下载 token）
