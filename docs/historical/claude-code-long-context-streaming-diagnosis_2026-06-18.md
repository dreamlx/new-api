	一、面板数字告诉我们什么
	
	  ┌──────────┬────────────────────────────────┬─────────────────────────────────────────┐
	  │   现象   │              读数              │                  含义                   │
	  ├──────────┼────────────────────────────────┼─────────────────────────────────────────┤
	  │ 输入     │ 稳定 90k–165k                  │ Claude Code 每轮几乎重发全量            │
	  │ token    │                                │ context,且在涨                          │
	  ├──────────┼────────────────────────────────┼─────────────────────────────────────────┤
	  │ 首字     │ 11–42s,出问题那行 -1.0s        │ prefill 极慢;-1.0 = 一个 token          │
	  │ (TTFT)   │                                │ 都没吐出来                              │
	  ├──────────┼────────────────────────────────┼─────────────────────────────────────────┤
	  │ 输出     │ 正常 135–855,故障行 = 0        │ 请求计了 input、prefill 阶段就死了,没进 │
	  │ token    │                                │  decode                                 │
	  ├──────────┼────────────────────────────────┼─────────────────────────────────────────┤
	  │ 总用时   │ 18–214s,"Churned 9m 2s"        │ 单请求挂在链路里几分钟                  │
	  ├──────────┼────────────────────────────────┼─────────────────────────────────────────┤
	  │ 报错     │ socket connection was closed   │ TCP 被中间层掐断,不是模型返回的干净     │
	  │          │ unexpectedly                   │ HTTP error                              │
	  └──────────┴────────────────────────────────┴─────────────────────────────────────────┘
	
	  关键一行:输入 157794 / 输出 0 / 首字 -1.0s / 用时 87s。87 秒一个 token
	  都没出来然后断。这不是"生成到一半断",是 prefill 阶段就卡住/被中止。
	
	  二、根因分级
	
	  ① 链路 timeout(socket 断的直接原因)
	  你这套有个计费网关(看 UI 是 new-api/one-api 这类),前面大概率还有 nginx。这类反代默认
	  proxy_read_timeout / 流式 idle timeout 是 60s。当模型在长 prefill 期间几十秒不吐字节,nginx
	  判定上游 idle → 掐连接 → 客户端看到 "socket closed"。TTFT 30s+ 的请求必然反复踩这个。
	
	  ② prefill 太慢(为什么会 idle 到 timeout)
	  27B 模型 prefill 150k tokens,如果 GPU 配置不够(TP 太低 / 没开 chunked prefill /
	  单卡),几十秒甚至上分钟。那行 87s 0 输出就是 prefill 没跑完或排队卡死。这是物理瓶颈,光调
	  timeout 治标。
	
	  ③ context 太大(根上的放大器)
	  Claude Code 把全文件 + 工具输出全塞进 context,每轮还在涨。90k→165k 说明已经接近你 SGLang
	  --context-length 上限。一旦超过,SGLang 直接拒 → 输出 0。小模型本身 agentic loop 弱、churn
	  多 → 轮次更多 → context 涨更快 → 更快撞墙,恶性循环。
	
	  ④ 可能的 KV cache OOM / preemption
	  输出 0 + 断连也可能是 SGLang KV cache 满,把正在跑的请求 abort 掉。必须看 SGLang
	  日志确认(关键词 out of memory / aborted / preempt / decode out of memory)。
	
	  三、continue 不管用、--resume 管用 —— 这个正常
	
	  流断在 turn 中间 → 那条 assistant message 是残缺的。continue 复用内存里这个已损坏的 session
	  状态,重发立刻又撞同样的 timeout;claude --resume
	  从磁盘重建干净请求再发,所以能恢复。这是客户端行为,不是你 infra 的额外 bug,但它是"turn 死在
	  stream 里"的症状。
	
	  四、先做隔离排查(别瞎调)
	
	  1. 绕过网关直连 SGLang 发一个长 streaming curl,看断不断 → 判定是网关/nginx 还是 SGLang
	  本身:
	  curl -N http://<sglang>:30000/v1/chat/completions -d
	  '{...,"stream":true,"max_tokens":2000}'
	  2. 看 SGLang 启动日志确认 --context-length 实际值、max_total_tokens、是否 chunked prefill。
	  3. 故障时刻的 SGLang runtime 日志:有没有 OOM/abort/preempt。
	  4. nginx error.log:upstream timed out 还是 client closed connection ——
	  区分是反代杀的还是客户端杀的。
	
	  五、修复清单(按性价比排)
	
	  A. 立刻:放开整条链路 timeout(止血)
	  - nginx:proxy_read_timeout 600s; proxy_send_timeout 600s; proxy_buffering off;(SSE 必须关
	  buffering)
	  - 计费网关:把它的 relay/stream timeout 调到 600s+
	  - 客户端:export API_TIMEOUT_MS=600000
	
	  B. SGLang 调优(治本,降 TTFT)
	  - --enable-chunked-prefill(让 prefill 期间持续吐字节,既降 TTFT 又避免 idle timeout)
	  - --context-length 显式拉到模型上限(Qwen3 系列配 YaRN 可到 128k/256k),别让大请求被拒
	  - --mem-fraction-static 调大 KV cache,但盯住 OOM
	  - 提高 TP / 换更强卡 —— 150k prefill 本质是算力问题
	  - 看 --max-running-requests / --max-prefill-tokens 是否卡住并发
	
	  C. 压制 context 增长(真正的天花板)
	  27B 模型扛 150k context 跑 agentic coding,已经在能力边缘。建议客户:任务拆小、主动
	  /compact、少往 context 塞文件。否则 timeout 调多大都会再撞墙。