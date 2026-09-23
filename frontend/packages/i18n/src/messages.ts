export const messages = {
  "zh-CN": {
    "common.language": "语言",
    "common.locale.zh-CN": "简体中文",
    "common.locale.en-US": "English",
    "foundation.status": "基础服务已就绪",
    "foundation.description": "业务功能将按实施路线逐步开放。",
    "user.appName": "VPS 服务中心",
    "admin.appName": "VPS 管理中心"
  },
  "en-US": {
    "common.language": "Language",
    "common.locale.zh-CN": "简体中文",
    "common.locale.en-US": "English",
    "foundation.status": "Foundation services are ready",
    "foundation.description": "Business features will be enabled according to the implementation roadmap.",
    "user.appName": "VPS Service Center",
    "admin.appName": "VPS Administration"
  }
} as const;

export type Locale = keyof typeof messages;
export type MessageKey = keyof (typeof messages)["en-US"];
