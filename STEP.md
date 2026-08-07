# Fpan 开发步骤 (Step by Step)

> 技术栈: Go 1.26 + Gin + GORM + PostgreSQL  
> OIDC: https://pi.auth.hrtius.com  
> API Spec: api/openapi.yaml  

---

## Step 1: 配置 & 数据库 & GORM 模型

### 1.1 添加依赖
```bash
go get gorm.io/gorm gorm.io/driver/postgres
go get github.com/coreos/go-oidc/v3
go get golang.org/x/oauth2
```

### 1.2 环境变量设计
| 变量 | 说明 | 默认值 |
|------|------|--------|
| `FPAN_DATABASE_URL` | PostgreSQL DSN | `postgres://localhost:5432/fpan` |
| `FPAN_STORAGE_PATH` | blob 物理文件存储路径 | `./storage` |
| `FPAN_OIDC_ISSUER` | OIDC provider URL | spec 中的默认值 |
| `FPAN_OIDC_CLIENT_ID` | OIDC client ID | 必填 |
| `FPAN_LISTEN_ADDR` | HTTP 监听地址 | `:6313` |

### 1.3 创建文件
- `backend/internal/config/config.go` — env 加载
- `backend/internal/database/database.go` — GORM 连接 + AutoMigrate
- `backend/internal/models/blob.go` — Blob 模型
- `backend/internal/models/file.go` — File 模型
- `backend/internal/models/folder.go` — Folder 模型
- `backend/internal/models/share.go` — Share 模型
- 更新 `backend/cmd/fpan/main.go` — 启动时连接 DB

### 1.4 GORM 模型字段 (与 OpenAPI spec 对应)

**Blob**
| Go 字段 | DB 列 | GORM Tag |
|---------|-------|----------|
| ID | id | primaryKey;autoIncrement |
| SHA256 | sha256 | uniqueIndex;not null |
| Size | size | not null |
| CreatedAt | created_at | autoCreateTime;not null |

**File**
| Go 字段 | DB 列 | GORM Tag |
|---------|-------|----------|
| ID | id | primaryKey;autoIncrement |
| Display | display | not null |
| ParentID | parent_id | index;default:null |
| MimeType | mime_type | not null |
| BlobID | blob_id | not null;index |
| Blob | blob | foreignKey:BlobID |
| CreatedAt | created_at | autoCreateTime |
| UpdatedAt | updated_at | autoUpdateTime |
| DeletedAt | deleted_at | index (GORM 软删除) |

**Folder**
| Go 字段 | DB 列 | GORM Tag |
|---------|-------|----------|
| ID | id | primaryKey;autoIncrement |
| Display | display | not null |
| ParentID | parent_id | index;default:null |
| CreatedAt | created_at | autoCreateTime |
| UpdatedAt | updated_at | autoUpdateTime |
| DeletedAt | deleted_at | index (GORM 软删除) |

**Share**
| Go 字段 | DB 列 | GORM Tag |
|---------|-------|----------|
| ID | id | primaryKey;autoIncrement |
| EntryID | entry_id | not null;index |
| EntryType | entry_type | not null |
| Token | token | uniqueIndex;not null |
| PasswordHash | password_hash | default:null |
| ExpiresAt | expires_at | index;default:null |
| Permission | permission | not null;default:read |
| MaxDownloads | max_downloads | default:null |
| DownloadCount | download_count | not null;default:0 |
| CreatedAt | created_at | autoCreateTime |
| UpdatedAt | updated_at | autoUpdateTime |

> 注意: Share 的 `has_password` 和 `download_count` 是 computed 字段，JSON 序列化时处理。

---

## Step 2: OIDC 认证中间件

### 2.1 创建文件
- `backend/internal/app/middleware/auth.go` — JWT 验证中间件

### 2.2 功能
1. 启动时从 OIDC issuer 拉取 `.well-known/openid-configuration`
2. 用 `go-oidc` 的 `oidc.IDTokenVerifier` 验证 Bearer token
3. 将验证后的 claims 注入 `gin.Context`
4. 根据 endpoint tag 决定是否跳过认证（Shared 端点 `security: []`）

### 2.3 更新
- `backend/cmd/fpan/main.go` — 加载 OIDC provider + 注册中间件

---

## Step 3: Service 层 — Entries

### 3.1 创建文件
- `backend/internal/app/service/entries.go`

### 3.2 功能
| 函数 | 对应 API | 说明 |
|------|----------|------|
| `ListRootEntries()` | `GET /entries` | 查询 parent_id IS NULL 且未软删除的 files + folders，合并排序分页 |
| `ListFolderEntries(id)` | `GET /folders/{id}/entries` | 同上，parent_id = id |
| `Entry` 响应结构体 | — | `type` 判别字段 (`file` / `folder`)，扁平化返回 |

### 3.3 关键实现细节
- File 和 Folder 查询后合并为一个 `[]EntryResponse` 切片
- 分页需先查总数，再查当前页，统一排序 `ORDER BY {sort_by} {sort}`
- `filter` 参数做 `WHERE display ILIKE '%keyword%'`

---

## Step 4: Service 层 — Files

### 4.1 创建文件
- `backend/internal/app/service/files.go`

### 4.2 功能
| 函数 | 对应 API | 说明 |
|------|----------|------|
| `UploadFile(parentID, fileName, mimeType, reader)` | `POST /files`, `POST /folders/{id}/files` | 写物理存储 → 查/创建 Blob → 创建 File 记录 |
| `UploadFileStream(parentID, display, mimeType, reader)` | `POST /files/stream`, `POST /folders/{id}/files/stream` | 同上，文件名从 header 获取 |
| `GetFile(id)` | `GET /files/{id}` | 查询 + Preload Blob |
| `UpdateFile(id, display, parentID)` | `PUT /files/{id}` | 重命名/移动 |
| `DeleteFile(id)` | `DELETE /files/{id}` | 软删除 |

### 4.3 上传流程
1. 读取上传的字节流，计算 SHA256
2. 将文件写入 `{STORAGE_PATH}/{sha256[0:2]}/{sha256}`
3. 查询 Blob 是否存在，不存在则创建
4. 创建 File 记录 (display, parent_id, mime_type, blob_id)
5. 返回 File JSON（含 blob 信息）

---

## Step 5: Service 层 — Folders

### 5.1 创建文件
- `backend/internal/app/service/folders.go`

### 5.2 功能
| 函数 | 对应 API | 说明 |
|------|----------|------|
| `CreateFolder(display, parentID)` | `POST /folders` | 创建文件夹 |
| `GetFolder(id)` | `GET /folders/{id}` | 查询 |
| `UpdateFolder(id, display, parentID)` | `PUT /folders/{id}` | 重命名/移动 |
| `DeleteFolder(id)` | `DELETE /folders/{id}` | 软删除 |

---

## Step 6: Service 层 — Blobs

### 6.1 创建文件
- `backend/internal/app/service/blobs.go`
- `backend/internal/storage/blob.go` — 物理文件读写

### 6.2 功能
| 函数 | 对应 API | 说明 |
|------|----------|------|
| `GetBlobContent(sha256)` | `GET /blobs/{sha256}` | 读取物理文件，流式返回 |

### 6.3 存储路径约定
```
{STORAGE_PATH}/
  00/
    ab/  ← sha256 的前 4 位分两层目录
      {remaining_sha256}
```

---

## Step 7: Service 层 — Shares

### 7.1 创建文件
- `backend/internal/app/service/shares.go`

### 7.2 功能
| 函数 | 对应 API | 说明 |
|------|----------|------|
| `CreateShare(req)` | `POST /shares` | 生成随机 token，密码 bcrypt 哈希 |
| `ListShares(page, size)` | `GET /shares` | 分页查询 shares |
| `GetShare(id)` | `GET /shares/{id}` | 详情 |
| `UpdateShare(id, req)` | `PUT /shares/{id}` | 更新密码/过期/权限/下载限制 |
| `DeleteShare(id)` | `DELETE /shares/{id}` | 删除 share |

---

## Step 8: Service 层 — Shared (公开访问)

### 8.1 创建文件
- `backend/internal/app/service/shared.go`

### 8.2 功能
| 函数 | 对应 API | 说明 |
|------|----------|------|
| `AccessShared(token, password)` | `GET /s/{token}` | 验证 token，检查密码/过期/下载次数，返回 SharedResource |
| `ListSharedEntries(token, password, page, size, sort, sortBy, filter)` | `GET /s/{token}/entries` | 如果是文件夹 share，列出子条目 |
| `DownloadSharedBlob(token, sha256, password)` | `GET /s/{token}/blobs/{sha256}` | 验证 share + 记录下载次数 + 返回文件流 |

---

## Step 9: Route Handler 层

### 9.1 创建文件
- `backend/internal/app/handler/entries.go`
- `backend/internal/app/handler/files.go`
- `backend/internal/app/handler/folders.go`
- `backend/internal/app/handler/blobs.go`
- `backend/internal/app/handler/shares.go`
- `backend/internal/app/handler/shared.go`
- `backend/internal/app/handler/error.go` — 统一错误响应

### 9.2 路由注册 (Gin)
```
/api/v1
  GET    /entries                 → handler.ListRootEntries
  GET    /folders/:id/entries     → handler.ListFolderEntries
  POST   /files                   → handler.UploadFileToRoot
  POST   /files/stream            → handler.UploadFileToRootStream
  POST   /folders/:id/files       → handler.UploadFileToFolder
  POST   /folders/:id/files/stream → handler.UploadFileToFolderStream
  GET    /files/:id               → handler.GetFile
  PUT    /files/:id               → handler.UpdateFile
  DELETE /files/:id               → handler.DeleteFile
  POST   /folders                 → handler.CreateFolder
  GET    /folders/:id             → handler.GetFolder
  PUT    /folders/:id             → handler.UpdateFolder
  DELETE /folders/:id             → handler.DeleteFolder
  GET    /blobs/:sha256           → handler.GetBlobContent
  POST   /shares                  → handler.CreateShare
  GET    /shares                  → handler.ListShares
  GET    /shares/:id              → handler.GetShare
  PUT    /shares/:id              → handler.UpdateShare
  DELETE /shares/:id              → handler.DeleteShare
  GET    /s/:token                → handler.AccessShared
  GET    /s/:token/entries        → handler.ListSharedEntries
  GET    /s/:token/blobs/:sha256  → handler.DownloadSharedBlob
```

### 9.3 统一错误响应格式
```json
{ "code": 1001, "message": "folder not found" }
```

| Code | 含义 |
|------|------|
| 1001 | 资源不存在 |
| 1002 | 参数校验失败 |
| 1003 | 认证失败 |
| 1004 | 权限不足 |
| 1005 | 分享过期/密码错误/达到下载上限 |
| 1006 | 存储空间不足 |

---

## Step 10: 前端 (后续)

框架任选，基于 `api/openapi.yaml` 生成类型安全的 API 客户端。

---

## 文件结构总览

```
backend/
├── cmd/fpan/
│   └── main.go                      # 入口: config → db → oidc → routes → listen
├── frontend/
│   └── embed.go                     # 后续: embed 前端构建产物
├── go.mod
├── go.sum
└── internal/
    ├── config/
    │   └── config.go                # env 加载
    ├── database/
    │   └── database.go              # GORM 连接 + AutoMigrate
    ├── app/
    │   ├── middleware/
    │   │   └── auth.go              # OIDC JWT 验证
    │   ├── handler/
    │   │   ├── error.go             # 统一错误响应
    │   │   ├── entries.go
    │   │   ├── files.go
    │   │   ├── folders.go
    │   │   ├── blobs.go
    │   │   ├── shares.go
    │   │   └── shared.go
    │   └── service/
    │       ├── entries.go
    │       ├── files.go
    │       ├── folders.go
    │       ├── blobs.go
    │       ├── shares.go
    │       └── shared.go
    ├── models/
    │   ├── blob.go
    │   ├── file.go
    │   ├── folder.go
    │   └── share.go
    └── storage/
        └── blob.go                  # 物理文件读写
```

---

## 开发顺序总结

```
Step 1  [配置 + DB + Models]        ← 基础设施
Step 2  [OIDC 中间件]               ← 认证
Step 3  [Entries Service]           ← 核心读取
Step 4  [Files Service]             ← 上传 + CRUD
Step 5  [Folders Service]           ← 文件夹 CRUD
Step 6  [Blobs Service + Storage]   ← 物理文件下载
Step 7  [Shares Service]            ← 分享管理
Step 8  [Shared Service]            ← 公开分享访问
Step 9  [Route Handlers]            ← 对接到 Gin
Step 10 [前端]                      ← 后续
```
