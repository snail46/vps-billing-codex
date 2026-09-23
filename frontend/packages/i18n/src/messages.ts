export const messages = {
  "zh-CN": {
    "common.language": "语言",
    "common.locale.zh-CN": "简体中文",
    "common.locale.en-US": "English",
    "foundation.status": "基础服务已就绪",
    "foundation.description": "业务功能将按实施路线逐步开放。",
    "user.appName": "VPS 服务中心",
    "admin.appName": "VPS 管理中心"
    ,"auth.title": "账户登录"
    ,"auth.adminTitle": "管理员登录"
    ,"auth.email": "电子邮箱"
    ,"auth.password": "密码"
    ,"auth.totp": "双重验证码（如已启用）"
    ,"auth.login": "登录"
    ,"auth.register": "注册"
    ,"auth.switchToRegister": "创建新账户"
    ,"auth.switchToLogin": "已有账户，返回登录"
    ,"auth.pending": "正在提交…"
    ,"auth.success": "认证成功"
    ,"errors.invalidCredentials": "邮箱或密码不正确"
    ,"errors.unauthenticated": "请先登录"
    ,"errors.accountInactive": "账户已停用"
    ,"errors.emailInUse": "该邮箱已注册"
    ,"errors.emailInvalid": "请输入有效的电子邮箱"
    ,"errors.passwordInvalid": "密码需为 12 至 128 个字符"
    ,"errors.requestInvalid": "请求内容无效"
    ,"errors.contentTypeInvalid": "请求格式不受支持"
    ,"errors.authUnavailable": "认证服务暂时不可用"
    ,"errors.rateLimited": "请求过于频繁，请稍后重试"
    ,"errors.csrfInvalid": "安全令牌无效，请刷新后重试"
    ,"errors.permissionDenied": "没有执行此操作的权限"
    ,"errors.totpRequired": "请输入双重验证码"
    ,"errors.totpInvalid": "双重验证码无效"
    ,"errors.internal": "服务暂时无法完成请求"
    ,"errors.originDenied": "请求来源不受信任"
  },
  "en-US": {
    "common.language": "Language",
    "common.locale.zh-CN": "简体中文",
    "common.locale.en-US": "English",
    "foundation.status": "Foundation services are ready",
    "foundation.description": "Business features will be enabled according to the implementation roadmap.",
    "user.appName": "VPS Service Center",
    "admin.appName": "VPS Administration"
    ,"auth.title": "Account sign in"
    ,"auth.adminTitle": "Administrator sign in"
    ,"auth.email": "Email"
    ,"auth.password": "Password"
    ,"auth.totp": "Two-factor code (when enabled)"
    ,"auth.login": "Sign in"
    ,"auth.register": "Create account"
    ,"auth.switchToRegister": "Create a new account"
    ,"auth.switchToLogin": "Already registered? Sign in"
    ,"auth.pending": "Submitting…"
    ,"auth.success": "Authentication succeeded"
    ,"errors.invalidCredentials": "The email or password is invalid"
    ,"errors.unauthenticated": "Please sign in"
    ,"errors.accountInactive": "This account is inactive"
    ,"errors.emailInUse": "This email is already registered"
    ,"errors.emailInvalid": "Enter a valid email address"
    ,"errors.passwordInvalid": "Password must contain 12 to 128 characters"
    ,"errors.requestInvalid": "The request is invalid"
    ,"errors.contentTypeInvalid": "The request format is unsupported"
    ,"errors.authUnavailable": "Authentication is temporarily unavailable"
    ,"errors.rateLimited": "Too many requests; try again later"
    ,"errors.csrfInvalid": "The security token is invalid; refresh and retry"
    ,"errors.permissionDenied": "You do not have permission for this action"
    ,"errors.totpRequired": "Enter your two-factor authentication code"
    ,"errors.totpInvalid": "The two-factor authentication code is invalid"
    ,"errors.internal": "The service could not complete the request"
    ,"errors.originDenied": "The request origin is not trusted"
  }
} as const;

export type Locale = keyof typeof messages;
export type MessageKey = keyof (typeof messages)["en-US"];
