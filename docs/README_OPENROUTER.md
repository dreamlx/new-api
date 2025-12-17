# OpenRouter Models 接口文档

> **状态**: ✅ 已完成 (2025-12-16)

## 功能概述

本项目实现了 OpenRouter 兼容的 `/api/openrouter/v1/models` 接口，采用**无侵入式设计**（零数据库修改），通过配置文件管理模型扩展信息。

---

## 快速开始

### 测试接口

```bash
# 获取模型列表
curl http://localhost:3000/api/openrouter/v1/models | jq

# 获取单个模型
curl http://localhost:3000/api/openrouter/v1/models/deepseek-chat | jq

# 获取状态
curl http://localhost:3000/api/openrouter/status | jq

# 运行完整测试
./scripts/test-openrouter-api.sh
```

### 配置管理（需管理员Token）

```bash
# 获取配置
curl -H "Authorization: Bearer <ADMIN_TOKEN>" \
  http://localhost:3000/api/openrouter/config | jq

# 重新加载配置
curl -X POST -H "Authorization: Bearer <ADMIN_TOKEN>" \
  http://localhost:3000/api/openrouter/config/reload
```

---

## API 端点

| 方法 | 端点 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/openrouter/v1/models` | 无 | 获取所有模型列表 |
| GET | `/api/openrouter/v1/models/:model` | 无 | 获取单个模型信息 |
| GET | `/api/openrouter/status` | 无 | 获取OpenRouter状态 |
| GET | `/api/openrouter/config` | Admin | 获取配置 |
| PUT | `/api/openrouter/config` | Admin | 更新配置 |
| POST | `/api/openrouter/config/reload` | Admin | 重新加载配置 |

---

## 文档索引

| 文档 | 用途 |
|------|------|
| [**实施清单**](openrouter_implementation_checklist.md) | 实际实施记录、文件清单、技术决策 |
| [**接口分析**](openrouter_models_interface_analysis.md) | OpenRouter API 结构分析、字段映射 |
| [**架构设计**](openrouter_non_invasive_design.md) | 无侵入式架构、缓存层、数据融合 |
| [**UI设计**](openrouter_admin_ui_design.md) | 后台管理页面设计规范 |

---

## 核心文件

| 文件 | 用途 |
|------|------|
| `model/openrouter.go` | 数据结构 + 缓存 + 配置加载 |
| `controller/openrouter.go` | API 控制器 |
| `config/openrouter.yaml` | 模型配置文件 |
| `scripts/test-openrouter-api.sh` | 集成测试脚本 |

---

## 设计要点

- **零数据库修改**: 完全通过配置文件扩展
- **向后兼容**: 不影响现有 `/v1/models` 接口
- **缓存优化**: TTL 5分钟，异步刷新
- **热重载**: 支持运行时更新配置

---

## 自定义配置

```bash
# 使用自定义配置路径
export OPENROUTER_CONFIG_PATH=/path/to/custom/openrouter.yaml
./one-api
```

配置文件示例见 `config/openrouter.yaml`。

---

*最后更新: 2025-12-16*
