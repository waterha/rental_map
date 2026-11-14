# 邮箱账号系统

## 使用方式

地图左上角的账号图标打开邮箱注册 / 登录窗口。注册必须依次填写邮箱、密码、收到的 6 位验证码。服务器在验证成功的同一个数据库事务中创建账号、消费验证码并建立登录会话，发送验证码本身不会创建账号。

验证码仅在邮件发送成功后生效，有效时间 120 秒，达到过期时间即失效；最多允许输入错误 5 次，成功使用一次后删除，重新发送会替换此前验证码。服务器时间决定是否有效，前端倒计时只作提示。

密码要求 10–72 字节，至少包含英文字母和数字，不允许控制字符。使用 bcrypt（cost 12）保存；原始密码不入库、不写日志、不保存在浏览器存储。登录有效期为 7 天，采用 HttpOnly、SameSite=Strict Cookie；数据库仅保存会话令牌的 HMAC 摘要，退出登录会撤销该会话。

## 邮箱规则

允许的精确域名：`qq.com`、`163.com`、`126.com`、`gmail.com`、`outlook.com`、`hotmail.com`、`icloud.com`。域名大小写会归一化，前后空白会清除。

- 拒绝任意自定义后缀、子域名、看似白名单的后缀拼接，如 `qq.com.evil.invalid`。
- 拒绝连续点、首尾分隔符、非 ASCII 用户名、空白、换行、显示名和多个 `@`。
- QQ 号码邮箱限制号码格式；QQ 自定义别名支持合理的邮箱用户名格式。
- 网易用户名限制 6–18 位、字母开头、字母数字下划线；Gmail 检查用户名组成和长度。
- 是否真实存在、是否属于用户，最终以验证码收取与校验为准。单靠域名、DNS 或格式不能证明邮箱存在。

可通过 `AUTH_ALLOWED_EMAIL_DOMAINS` 将白名单缩小到上述域名的子集。前端显示后端下发的白名单，后端始终独立校验。

## QQ 发件配置

后端自动读取根目录 `.env.server.local`，已有进程环境变量优先。此文件被 Git 和 Docker 构建上下文排除，不会提交到公开仓库。不要把邮件凭据放入 `.env.local` 或 `VITE_` 变量；`VITE_` 会被打包到浏览器。

参考 `.env.example`，只将后端字段放入 `.env.server.local`：

```dotenv
SMTP_FROM=your_numeric_qq_address@qq.com
SMTP_AUTH_CODE=your_qq_smtp_authorization_code
AUTH_ALLOWED_EMAIL_DOMAINS=qq.com,163.com,126.com,gmail.com,outlook.com,hotmail.com,icloud.com
AUTH_COOKIE_SECURE=false
```

在 QQ 邮箱“设置 → 账号”开启 SMTP，获取授权码；不是 QQ 登录密码。连接固定使用 `smtp.qq.com:465` 的 TLS，最低 TLS 1.2，校验服务器证书。发信设置不通过公开 API 返回，SMTP 错误、收件人、授权码和验证码均不写入日志。

配置后重启后端，不需重建前端。未配置、发送失败时返回明确错误，不会回传验证码“假装发送成功”。SMTP 服务器接受邮件也不代表目标收件箱最终投递成功，用户需实际查收邮件或垃圾邮件。

HTTPS 正式部署将 `AUTH_COOKIE_SECURE=true`；回环 HTTP 开发保持 false。反向代理部署当前直接使用连接对端地址进行限流，不盲信 `X-Forwarded-For`，可在可信网关额外增加来源限制。

## 防滥用与存储

| 操作 | 限制 |
|---|---|
| 同邮箱重发 | 间隔至少 60 秒 |
| 发送 IP | 每 10 分钟最多 5 次 |
| 发送邮箱 | 每小时最多 10 次 |
| 注册 IP / 邮箱 | 每 10 分钟分别最多 20 / 15 次 |
| 登录 IP / 邮箱 | 每 15 分钟分别最多 30 / 10 次 |
| 单验证码错误 | 最多 5 次 |

请求体限制 4 KB，拒绝跨站提交及多余字段。限流键以 HMAC 摘要保存，不保存原始 IP。OTP 也只存 HMAC 摘要，避免数据库泄漏后低熵验证码被直接枚举。HMAC 密钥在 `DATA_DIR/runtime/auth-secret.key` 自动生成，必须和数据库一起保留、备份，不能提交到代码仓库；多实例部署必须共享该密钥。

数据表：`accounts`、`email_verifications`、`account_sessions`、`auth_rate_limits`。SQLite 和 PostgreSQL 共用迁移；PostgreSQL 注册时对验证码行加锁，SQLite 使用单连接事务，避免并发消费同一验证码。公开发布 API 在启用账号服务时要求登录，新增房源归属于账号 ID。

## API

| 方法与地址 | 用途 |
|---|---|
| `GET /api/auth/config` | 白名单、有效期、重发间隔、邮件是否可用 |
| `POST /api/auth/code` | `{email}`，向该邮箱发送注册验证码 |
| `POST /api/auth/register` | `{email,password,code}`，验证并注册 |
| `POST /api/auth/login` | `{email,password}`，建立 / 轮换会话 |
| `GET /api/auth/me` | 当前账号或 `user:null` |
| `POST /api/auth/logout` | 撤销当前会话 |

尚未实现找回密码、修改邮箱、多因素认证或第三方登录。本阶段不自动给任何地址发送测试邮件；真实邮件由用户点击“发送验证码”触发。

## 验证

Go 测试覆盖真实账号入库、密码与会话摘要、验证码过期边界（119 秒通过 / 120 秒拒绝）、一次性、重发失效、错误次数、邮箱绑定、并发重用、会话过期、发信失败、无 SMTP 配置、白名单、跨站与限流、登录退出及发布鉴权。测试使用仅存在于测试文件中的邮件捕获器，生产没有绕过邮件的注册模式。

浏览器用例覆盖实际后端白名单与未配置提示；倒计时、成功界面与过期禁用采用明确的响应模拟，不能将其当作实际邮件投递证明。`npm run test:e2e:isolated` 强制 `SMTP_DISABLED=true`，确保测试不会发送邮件、不会调用本地授权码。
