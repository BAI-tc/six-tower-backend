# Steam OpenID 认证集成文档

## 概述

本项目使用 NestJS 后端实现 Steam OpenID 2.0 认证登录功能，无需依赖 `passport-steam` 包，手动实现了完整的 OpenID 验证流程。

## 核心文件

### 1. SteamAuthService (`src/auth/steam.strategy.ts`)

核心服务，负责：
- 生成 Steam 登录 URL
- 验证 Steam 返回的 OpenID 响应
- 获取用户 Steam 资料信息

#### 主要方法

```typescript
// 生成 Steam 登录 URL
generateAuthUrl(returnUrl: string, realm: string): string

// 验证 OpenID 响应
verifyOpenIdResponse(query: any): Promise<{ steamId: string; username?: string; avatar?: string } | null>
```

### 2. AuthController (`src/auth/auth.controller.ts`)

提供两个认证端点：

| 端点 | 描述 |
|------|------|
| `GET /api/auth/steam/url` | 获取 Steam 登录 URL |
| `GET /api/auth/steam/login` | 直接跳转到 Steam 登录 |
| `GET /api/auth/steam/return` | Steam 回调处理 |

## 遇到的问题及解决方案

### 问题 1: realm 格式错误

**错误信息**: `Invalid claimed_id or identity`

**原因**: Steam OpenID 要求 realm 必须以结尾斜杠 `/` 结尾

**解决方案**:
```typescript
// 错误 - 不带斜杠
const realm = 'http://localhost:3000';

// 正确 - 带结尾斜杠
const realm = 'http://localhost:3000/';
```

**相关代码位置**: [auth.controller.ts:16](src/auth/auth.controller.ts#L16)

---

### 问题 2: OpenID 验证参数不完整

**错误信息**: Steam 返回 `mode=error` 和 `Invalid claimed_id or identity`

**原因**: 缺少必需的 OpenID 验证参数（namespace、op_endpoint 等）

**解决方案**: 添加完整的验证检查：

```typescript
// 1. 检查 namespace
if (openid_ns !== 'http://specs.openid.net/auth/2.0') {
  return null;
}

// 2. 检查 op_endpoint
if (openid_op_endpoint !== 'https://steamcommunity.com/openid/login') {
  return null;
}

// 3. 检查 claimed_id 和 identity 格式
const claimedIdRegex = /^https?:\/\/steamcommunity\.com\/openid\/id\/(\d+)$/;
if (!claimedIdRegex.test(openid_claimed_id) || !claimedIdRegex.test(openid_identity)) {
  return null;
}
```

**相关代码位置**: [steam.strategy.ts:47-69](src/auth/steam.strategy.ts#L47-L69)

---

### 问题 3: 缺少 STEAM_API_KEY 环境变量

**问题**: 无法获取用户 Steam 头像和昵称

**解决方案**: 在 `.env` 文件中配置：

```env
STEAM_API_KEY=your_steam_api_key
```

获取方式：
1. 访问 https://steamcommunity.com/dev/apikey
2. 填写 Domain Name（任意）
3. 复制 API Key

## 环境变量配置

| 变量 | 默认值 | 描述 |
|------|--------|------|
| `FRONTEND_URL` | `http://localhost:3000` | 前端地址 |
| `BACKEND_URL` | `http://localhost:3001` | 后端地址 |
| `STEAM_API_KEY` | - | Steam API 密钥（可选） |

## 测试步骤

1. 确保后端服务运行在 `http://localhost:3001`
2. 访问 `http://localhost:3001/api/auth/steam/login`
3. 跳转到 Steam 登录页面
4. 登录成功后自动回调到 `/api/auth/steam/return`

## Steam OpenID 2.0 协议要点

1. **发现阶段**: 用户被重定向到 `https://steamcommunity.com/openid/login`
2. **认证阶段**: 用户在 Steam 网站上完成登录
3. **返回阶段**: Steam 将用户重定向回应用的 `return_to` URL
4. **验证阶段**: 应用验证返回的 OpenID 参数

关键参数：
- `openid.ns`: 必须为 `http://specs.openid.net/auth/2.0`
- `openid.op_endpoint`: 必须为 `https://steamcommunity.com/openid/login`
- `openid.claimed_id`: 格式为 `https://steamcommunity.com/openid/id/{steamid}`
- `openid.identity`: 与 claimed_id 相同

## 参考资料

- [passport-steam 源码](https://github.com/liamcurry/passport-steam)
- [Steam OpenID Documentation](https://steamcommunity.com/dev)
- [OpenID 2.0 规范](http://openid.net/specs/openid-authentication-2_0.html)
