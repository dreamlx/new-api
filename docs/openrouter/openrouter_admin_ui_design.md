# OpenRouter 配置模块页面设计

## 1. 目标
在后台管理页面新增一个“OpenRouter 配置”模块，帮助运营在可视化界面下维护 OpenRouter 接口所需的扩展参数和基础信息，支持热加载、权限控制和数据验证，避免频繁编辑 YAML 配置文件。

## 2. 页面结构与字段

### 2.1 入口
- 侧边栏：在「模型管理/配置管理」中新增“OpenRouter配置”菜单；
- 面包屑：配置管理 / OpenRouter配置；
- 权限：要求具备 `config:write` 和 `models:manage` 的角色。

### 2.2 顶部区域
- **启用开关**：切换 OpenRouter 模式（同 `config/openrouter_config.yaml` 的 `openrouter.enabled`）；
- **缓存 TTL**：数值输入（单位秒），默认 300；修改后提示重启/刷新缓存。
- **热重载按钮**：立即调用 `POST /api/openrouter/config/reload`；返回结果展示成功/失败原因。

### 2.3 模型列表
表格显示所有启用模型：
| 列名 | 说明 |
| --- | --- |
| 模型名称 | 关联 `models.model_name` |
| 已配置 | 是否被 `models_extension.yaml` 覆盖（Y/N） |
| canonical_slug | 当前值 |
| context_length | 当前值 |
| 是否归档 | 依据模型状态（已禁用则置灰） |
| 操作 | 编辑/预览/恢复默认 |

支持搜索（模型名/slug）、分页、按 `已配置` 筛选。

### 2.4 模型详情/编辑器
点击某条模型进入编辑页/弹窗，字段如下：
1. `canonical_slug`（文本）：必填，默认填 `model_name`；
2. `hugging_face_id`（文本，可选）；
3. `context_length`（数值）：默认 8192；需 >= 1024；
4. `architecture.modality`（下拉）：项包括 `text->text`、`text+image->text`、`image->text`；
5. `architecture.input_modalities`（多选）：`text`、`image`、`audio`；
6. `architecture.output_modalities`（多选）：`text`、`image`；
7. `architecture.tokenizer`（文本）；
8. `architecture.instruct_type`（文本，支持 `null`）；
9. `pricing.prompt`、`pricing.completion`、`pricing.request`、`pricing.image`、`pricing.web_search`、`pricing.internal_reasoning`（均为字符串数字，支持 `0`）；
10. `top_provider.context_length`、`top_provider.max_completion_tokens`（可为空）、`top_provider.is_moderated`（开关）；
11. `supported_parameters`（可搜索多选）：展示常见参数如 `max_tokens`, `temperature`, `top_p`, `frequency_penalty`, `presence_penalty`, `tools`, `structured_outputs`；允许自由输入新项；
12. `default_parameters`（可增删 key-value）：value 可以是 string/number/bool；
13. `per_request_limits`（JSON textarea，可选，若留空则为 `null`）：用于录入 `max_images`、`max_files` 等自定义限制；
14. 备注/说明（可选 Markdown ）。

底部有“保存配置”、“还原默认”、“取消”三按钮。保存会生成/更新 `models_extension.yaml` 对应条目。还原默认清除条目（即删除配置文件中的对象）。

### 2.5 预览 & 缓存
- 显示“当前OpenRouter输出预览”，实时调用缓存数据 `GET /api/openrouter/models/preview?model={model}`；用于验证调整后的参数。
- 显示缓存状态信息：缓存更新时间、命中率、是否过期；提供“刷新缓存”按钮调用 `POST /api/openrouter/cache/refresh`。

## 3. 接口设计

### 3.1 获取配置列表
- **接口**：`GET /api/openrouter/config/models`
- **返回**：配置列表（含模型名、slug、context_length、是否默认）
- **说明**：合并 `models_extension.yaml` + 所有启用模型（空缺字段填默认）

### 3.2 获取单个模型配置
- **接口**：`GET /api/openrouter/config/models/{model}`
- **返回**：当前对象整合了基础字段、配置内容、缓存值

### 3.3 保存/更新配置
- **接口**：`PUT /api/openrouter/config/models/{model}`
- **请求体**：上述编辑字段结构（JSON）
- **行为**：
  - 验证字段（context_length >= input_max，pricing 字符串数值格式）
  - 持久化到 `models_extension.yaml`（或配置中心，如 `kv_store`）
  - 触发缓存刷新（异步）

### 3.4 删除模型扩展配置
- **接口**：`DELETE /api/openrouter/config/models/{model}`
- **行为**：移除配置文件中的该条记录，缓存恢复默认值；

### 3.5 热重载 & 缓存
- **接口**：`POST /api/openrouter/config/reload`
  - 重新加载配置文件与外部数据源，返回加载状态和错误信息；
- **接口**：`POST /api/openrouter/cache/refresh`
  - 立即刷新 `OpenRouterCache`，可选 `model` 参数；

### 3.6 预览接口
- **接口**：`GET /api/openrouter/models/preview?model={model_name}`
- **返回**：`OpenRouterModel` 对象，包含当前缓存/配置融合后的最终值；供页面预览。

## 4. 角色与权限
- 仅开放给具有 `config:write` 或 `admin:models` 权限的用户；
- 每次操作记录日志（操作人、时间、变更字段）；
- 提供“配置历史”按钮，可拉取最近 5 次变更记录。

## 5. 数据验证与容错
- 所有数值字段需校验范围（context_length、max_completion_tokens、pricing 字段 >0）；
- 支持 JSON Schema 验证 `per_request_limits`；
- 配置保存后若后台校验失败，返回错误并提示“配置回滚”按钮；
- 外部 API 异常时可切换到“离线模式”以使用本地配置。

## 6. 备注
- 后端只需通过配置文件增删改，不直接写数据库；
- 可在 web 后台提供“导入/导出 YAML”功能，方便批量维护；
- 建议将此页面与现有“模型配置”共享一套权限体系。
